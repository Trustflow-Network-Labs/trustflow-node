package repo

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/adgsm/trustflow-node/utils"

	"github.com/compose-spec/compose-go/loader"
	composetypes "github.com/compose-spec/compose-go/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/joho/godotenv"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"gopkg.in/yaml.v3"
)

type DockerManager struct {
	lm *utils.LogsManager
}

func NewDockerManager() *DockerManager {
	return &DockerManager{
		lm: utils.NewLogsManager(),
	}
}

func (dm *DockerManager) ValidateImage(image string) ([]byte, error) {
	cmd := exec.Command("docker", "manifest", "inspect", image)
	output, err := cmd.CombinedOutput()
	return output, err
}

func (dm *DockerManager) parseCompose(path string, envFile string) (*composetypes.Project, error) {
	_ = godotenv.Load(envFile)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return nil, err
	}
	return loader.Load(composetypes.ConfigDetails{
		ConfigFiles: []composetypes.ConfigFile{{Filename: path, Config: raw}},
		WorkingDir:  filepath.Dir(path),
		Environment: dm.getEnvMap(),
	})
}

func (dm *DockerManager) getEnvMap() map[string]string {
	env := map[string]string{}
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}
	return env
}

func (dm *DockerManager) tarDirectory(dir string) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(dir, path)
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(tw, file)
		return err
	})

	if err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf, nil
}

func (dm *DockerManager) getPlatform(cli *client.Client) *specs.Platform {
	// Prefer env var if available
	if env := os.Getenv("DOCKER_DEFAULT_PLATFORM"); env != "" {
		parts := strings.Split(env, "/")
		if len(parts) == 2 {
			return &specs.Platform{
				OS:           parts[0],
				Architecture: parts[1],
			}
		}
	}

	// Fallback: use `cli.Info` to detect platform
	info, err := cli.Info(context.Background())
	if err != nil {
		dm.lm.Log("warn", fmt.Sprintf("Could not detect host platform, using default: %v", err), "docker")
		return nil // fallback to default behavior
	}

	return &specs.Platform{
		OS:           info.OSType,
		Architecture: info.Architecture,
	}
}

func (dm *DockerManager) buildImage(cli *client.Client, contextDir, imageName, dockerfile string) error {
	ctx := context.Background()
	tarBuf, err := dm.tarDirectory(contextDir)
	if err != nil {
		return err
	}
	platform := dm.getPlatform(cli)
	var platformStr string
	if platform != nil {
		platformStr = fmt.Sprintf("%s/%s", platform.OS, platform.Architecture)
	}
	resp, err := cli.ImageBuild(ctx, bytes.NewReader(tarBuf.Bytes()), types.ImageBuildOptions{
		Tags:       []string{imageName},
		Dockerfile: dockerfile,
		Remove:     true,
		Platform:   platformStr,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = io.Copy(os.Stdout, resp.Body)
	return err
}

func (dm *DockerManager) detectDockerfiles() []composetypes.ServiceConfig {
	var services []composetypes.ServiceConfig

	skipDirs := map[string]bool{
		".git":         true,
		"packages":     true,
		"node_modules": true,
		".idea":        true,
		".vscode":      true,
	}

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip unwanted directories
		if info.IsDir() && skipDirs[info.Name()] {
			return filepath.SkipDir
		}

		if info.Name() == "Dockerfile" {
			ctxDir := filepath.Dir(path)
			name := filepath.Base(ctxDir)
			services = append(services, composetypes.ServiceConfig{
				Name:  name,
				Image: name + ":latest",
				Build: &composetypes.BuildConfig{
					Context:    ctxDir,
					Dockerfile: "Dockerfile",
				},
			})
		}
		return nil
	})

	if err != nil {
		dm.lm.Log("error", fmt.Sprintf("Error walking for Dockerfiles: %v", err), "docker")
	}
	return services
}

func (dm *DockerManager) runService(
	cli *client.Client,
	svc composetypes.ServiceConfig,
	wg *sync.WaitGroup,
	detach bool,
	input io.Reader,
	output io.Writer,
	mounts map[string]string,
) (string, error) {
	//	defer wg.Done()
	ctx := context.Background()

	if svc.Build != nil {
		dockerfile := svc.Build.Dockerfile
		if dockerfile == "" {
			dockerfile = "Dockerfile"
		}
		if err := dm.buildImage(cli, svc.Build.Context, svc.Image, dockerfile); err != nil {
			return "", err
		}
	}

	hostConfig := &container.HostConfig{Binds: []string{}}
	netConfig := &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{}}

	// Mounts from compose file
	for _, vol := range svc.Volumes {
		if vol.Source != "" && vol.Target != "" {
			hostConfig.Binds = append(hostConfig.Binds, fmt.Sprintf("%s:%s", vol.Source, vol.Target))
		}
	}

	// Mounts from code input
	for hostPath, containerPath := range mounts {
		hostConfig.Binds = append(hostConfig.Binds, fmt.Sprintf("%s:%s", hostPath, containerPath))
	}

	for name := range svc.Networks {
		netConfig.EndpointsConfig[name] = &network.EndpointSettings{}
	}

	var cmd strslice.StrSlice
	if len(svc.Command) > 0 {
		cmd = strslice.StrSlice(svc.Command)
	}

	platform := dm.getPlatform(cli)

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image:        svc.Image,
		Cmd:          cmd,
		Tty:          false,
		OpenStdin:    input != nil,
		StdinOnce:    input != nil,
		AttachStdin:  input != nil,
		AttachStdout: output != nil,
		AttachStderr: output != nil,
	}, hostConfig, netConfig, platform, svc.Name)
	if err != nil {
		return "", err
	}

	if input != nil || output != nil {
		attachResp, err := cli.ContainerAttach(ctx, resp.ID, container.AttachOptions{
			Stream: true,
			Stdin:  input != nil,
			Stdout: output != nil,
			Stderr: output != nil,
		})
		if err != nil {
			return "", err
		}

		if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
			attachResp.Close()
			return "", err
		}

		dm.lm.Log("info", fmt.Sprintf("Started container %s (%s)", svc.Name, resp.ID[:12]), "docker")

		var wgIO sync.WaitGroup
		if input != nil {
			wgIO.Add(1)
			go func() {
				defer wgIO.Done()
				_, _ = io.Copy(attachResp.Conn, input)
				_ = attachResp.CloseWrite()
			}()
		}
		if output != nil {
			wgIO.Add(1)
			go func() {
				defer wgIO.Done()
				_, _ = io.Copy(output, attachResp.Reader)
			}()
		}

		statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
		select {
		case <-statusCh:
		case err := <-errCh:
			return resp.ID, err
		}

		wgIO.Wait()
		attachResp.Close()
	} else {
		if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
			return "", err
		}
		dm.lm.Log("info", fmt.Sprintf("Started container %s (%s)", svc.Name, resp.ID[:12]), "docker")

		if !detach {
			go dm.streamLogs(cli, resp.ID, svc.Name)
		}
	}

	if svc.HealthCheck != nil {
		waitCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		for {
			select {
			case <-waitCtx.Done():
				return resp.ID, fmt.Errorf("timeout waiting for container %s to become healthy", svc.Name)
			default:
				inspect, err := cli.ContainerInspect(context.Background(), resp.ID)
				if err != nil {
					return resp.ID, err
				}
				if inspect.State != nil && inspect.State.Health != nil &&
					inspect.State.Health.Status == "healthy" {
					return resp.ID, nil
				}
				time.Sleep(1 * time.Second)
			}
		}
	}

	return resp.ID, nil
}

func (dm *DockerManager) streamLogs(cli *client.Client, containerID, name string) {
	ctx := context.Background()
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       "20",
	}
	r, err := cli.ContainerLogs(ctx, containerID, options)
	if err != nil {
		msg := fmt.Sprintf("Log stream error for %s: %v", name, err)
		dm.lm.Log("error", msg, "docker")
		return
	}
	defer r.Close()

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		msg := fmt.Sprintf("[%s] %s", name, scanner.Text())
		dm.lm.Log("info", msg, "docker")
	}
}

func (dm *DockerManager) Run(
	buildOnly bool,
	singleService string,
	cleanup bool, detach bool,
	composeFile string,
	envFile string,
	input io.Reader,
	output io.Writer,
	mounts map[string]string) ([]string, []error) {
	var (
		project *composetypes.Project
		err     error
		errors  []error
	)

	if _, statErr := os.Stat(composeFile); statErr == nil {
		project, err = dm.parseCompose(composeFile, envFile)
		if err != nil {
			dm.lm.Log("error", fmt.Sprintf("Parse error: %v", err), "docker")
			return nil, []error{err}
		}
	} else {
		services := dm.detectDockerfiles()
		if len(services) == 0 {
			dm.lm.Log("error", "No docker-compose.yml or Dockerfiles found", "docker")
			return nil, []error{err}
		}
		project = &composetypes.Project{Services: services}
	}

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		dm.lm.Log("error", fmt.Sprintf("Docker client error: %v", err), "docker")
		return nil, []error{err}
	}
	ctx := context.Background()

	for name := range project.Volumes {
		_, _ = cli.VolumeCreate(ctx, volume.CreateOptions{Name: name})
	}
	for name := range project.Networks {
		_, _ = cli.NetworkCreate(ctx, name, network.CreateOptions{})
	}

	var (
		started []string
		wg      sync.WaitGroup
		mu      sync.Mutex
		errsMu  sync.Mutex
	)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		dm.lm.Log("info", "Caught signal. Cleaning up...", "docker")
		stopOptions := container.StopOptions{}
		mu.Lock()
		for _, id := range started {
			_ = cli.ContainerStop(ctx, id, stopOptions)
			_ = cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
			dm.lm.Log("info", fmt.Sprintf("Cleaned up container %s", id[:12]), "docker")
		}
		mu.Unlock()
		signal.Stop(sigs)
		close(sigs)
		os.Exit(0)
	}()

	for _, svc := range project.Services {
		if singleService != "" && svc.Name != singleService {
			continue
		}

		wg.Add(1)
		go func(svc composetypes.ServiceConfig) {
			defer wg.Done()
			var id string
			var err error

			if buildOnly && svc.Build != nil {
				dockerfile := svc.Build.Dockerfile
				if dockerfile == "" {
					dockerfile = "Dockerfile"
				}
				err = dm.buildImage(cli, svc.Build.Context, svc.Image, dockerfile)
				//			continue
			} else {
				//		wg.Add(1)
				//		go func(svc composetypes.ServiceConfig) {
				id, err = dm.runService(cli, svc, &wg, detach, input, output, mounts)
				if err == nil {
					mu.Lock()
					started = append(started, id)
					mu.Unlock()
				} else {
					dm.lm.Log("warn", fmt.Sprintf("Failed to start service %s: %v", svc.Name, err), "docker")
				}
			}

			if err != nil {
				errsMu.Lock()
				errors = append(errors, fmt.Errorf("service %s: %w", svc.Name, err))
				errsMu.Unlock()
			}
		}(svc)
	}
	wg.Wait()

	if cleanup || len(errors) > 0 {
		stopOptions := container.StopOptions{}
		for _, id := range started {
			_ = cli.ContainerStop(ctx, id, stopOptions)
			_ = cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
			dm.lm.Log("info", fmt.Sprintf("Cleaned up container %s", id[:12]), "docker")
		}
	}

	return started, errors
}
