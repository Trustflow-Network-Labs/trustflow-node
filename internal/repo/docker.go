package repo

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/adgsm/trustflow-node/internal/node_types"
	"github.com/adgsm/trustflow-node/internal/ui"
	"github.com/adgsm/trustflow-node/internal/utils"

	"github.com/compose-spec/compose-go/loader"
	composetypes "github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/cli/config"
	dockerTypes "github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/joho/godotenv"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"gopkg.in/yaml.v3"
)

type DockerManager struct {
	lm *utils.LogsManager
	UI ui.UI
}

func NewDockerManager(ui ui.UI, lm *utils.LogsManager) *DockerManager {
	return &DockerManager{
		lm: lm,
		UI: ui,
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
		relPath = filepath.ToSlash(relPath) // ensures POSIX-style path

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

func (dm *DockerManager) imageMetadata(cli *client.Client, imageName string) (*image.InspectResponse, error) {
	ctx := context.Background()
	img, err := cli.ImageInspect(ctx, imageName)
	if err != nil {
		return nil, err
	}
	return &img, nil
}

func (dm *DockerManager) imageExistsLocally(cli *client.Client, imageName string) (bool, error) {
	_, err := cli.ImageInspect(context.Background(), imageName)
	if err == nil {
		return true, nil
	}
	if client.IsErrNotFound(err) {
		return false, nil
	}
	return false, err
}

func (dm *DockerManager) getAuthConfig(imageName string) (dockerTypes.AuthConfig, error) {
	// Parse registry from image name (default to Docker Hub if no registry specified)
	registry := "https://index.docker.io/v1/"
	if strings.Contains(imageName, "/") {
		parts := strings.Split(imageName, "/")
		if strings.Contains(parts[0], ".") {
			registry = parts[0]
		}
	}

	// Defaults from env vars
	username := os.Getenv("DOCKER_REGISTRY_USER")
	password := os.Getenv("DOCKER_REGISTRY_PASS")

	// CLI overrides
	if val := os.Getenv("DOCKER_AUTH_OVERRIDE"); val == "1" {
		if user := os.Getenv("DOCKER_AUTH_USER"); user != "" {
			username = user
		}
		if pass := os.Getenv("DOCKER_AUTH_PASS"); pass != "" {
			password = pass
		}

		return dockerTypes.AuthConfig{
			Username: username,
			Password: password,
		}, nil
	}

	// Load auth config from default docker config file
	configFile, err := config.Load(config.Dir())
	if err != nil {
		return dockerTypes.AuthConfig{}, err
	}
	authConfig, err := configFile.GetAuthConfig(registry)
	if err != nil {
		return dockerTypes.AuthConfig{}, err
	}
	return authConfig, nil
}

func (dm *DockerManager) encodeAuthToBase64(authConfig dockerTypes.AuthConfig) (string, error) {
	encodedJSON, err := json.Marshal(authConfig)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(encodedJSON), nil
}

func (dm *DockerManager) pullImage(cli *client.Client, imageName string) error {
	ctx := context.Background()

	exists, err := dm.imageExistsLocally(cli, imageName)
	if err != nil {
		return err
	}
	if exists {
		dm.lm.Log("info", fmt.Sprintf("Image %s already exists locally, skipping pull", imageName), "docker")
		return nil
	}

	dm.lm.Log("info", fmt.Sprintf("Pulling image %s...", imageName), "docker")

	authConfig, err := dm.getAuthConfig(imageName)
	if err != nil {
		return fmt.Errorf("failed to get auth config: %w", err)
	}
	encodedAuth, err := dm.encodeAuthToBase64(authConfig)
	if err != nil {
		return fmt.Errorf("failed to encode auth: %w", err)
	}

	platform := dm.getPlatform(cli)
	var platformStr string
	if platform != nil {
		platformStr = fmt.Sprintf("%s/%s", platform.OS, platform.Architecture)
	}

	reader, err := cli.ImagePull(ctx, imageName, image.PullOptions{
		Platform:     platformStr,
		RegistryAuth: encodedAuth,
	})
	if err != nil {
		return err
	}
	defer reader.Close()

	// Save image logs
	configManager := utils.NewConfigManager("")
	configs, err := configManager.ReadConfigs()
	if err != nil {
		return err
	}

	err = dm.processDockerPullOutput(reader, imageName, configs)
	if err != nil {
		return err
	}

	return nil
}

func (dm *DockerManager) processDockerPullOutput(reader io.Reader, imageName string, configs map[string]string) error {
	sanitizedImageName := dm.sanitizeImageName(imageName)
	logPath := filepath.Join(configs["local_docker_root"], sanitizedImageName, "logs", "pull.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return err
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer logFile.Close()

	// MultiWriter to log everything
	mw := io.MultiWriter(logFile)

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Bytes()

		// Write raw line to log file
		_, _ = mw.Write(append(line, '\n'))

		// Parse JSON
		var msg map[string]any
		if err := json.Unmarshal(line, &msg); err != nil {
			continue // skip non-JSON lines
		}

		id := fmt.Sprintf("%v", msg["id"])
		status := fmt.Sprintf("%v", msg["status"])
		progress := fmt.Sprintf("%v", msg["progress"])

		// Extract human-readable byte progress from "progress"
		dm.UI.Print(fmt.Sprintf("id: %s", id))
		dm.UI.Print(fmt.Sprintf("status: %s", status))
		if progress != "<nil>" {
			dm.UI.Print(fmt.Sprintf("progress: %s", dm.extractProgressAmount(progress)))
			dm.UI.Print(dm.extractProgressBar(progress))
		}
		dm.UI.Print("")
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

func (dm *DockerManager) extractProgressAmount(progress string) string {
	// Example: "[==========>      ]   720B/3.156kB"
	// Split by "]" to isolate the amount
	parts := []rune(progress)
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == ']' {
			return string(parts[i+1:])
		}
	}
	return progress
}

func (dm *DockerManager) extractProgressBar(progress string) string {
	// Extract between '[' and ']'
	start := -1
	end := -1
	for i, ch := range progress {
		if ch == '[' {
			start = i
		}
		if ch == ']' && start != -1 {
			end = i
			break
		}
	}
	if start != -1 && end != -1 {
		return progress[start : end+1]
	}
	return ""
}

// Parses and formats Docker build output
func (dm *DockerManager) processDockerBuildOutput(reader io.Reader, imageName string, configs map[string]string) error {
	sanitizedImageName := dm.sanitizeImageName(imageName)
	logPath := filepath.Join(configs["local_docker_root"], sanitizedImageName, "logs", "build.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return err
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer logFile.Close()

	mw := io.MultiWriter(logFile)

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Bytes()

		// Write raw line to log
		_, _ = mw.Write(append(line, '\n'))

		// Parse JSON
		var msg map[string]interface{}
		if err := json.Unmarshal(line, &msg); err != nil {
			continue // skip non-JSON lines
		}

		if stream, ok := msg["stream"].(string); ok {
			dm.UI.Print(stream) // stream usually contains newlines
		} else if errMsg, ok := msg["error"].(string); ok {
			msg := fmt.Sprintf("Build error: %s\n", errMsg)
			dm.UI.Print(msg)
			fmt.Fprintf(os.Stderr, "Build error: %s\n", errMsg)
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

func (dm *DockerManager) buildImage(
	cli *client.Client,
	contextDir, imageName, dockerfile string,
	entrypoint, cmd []string,
) (node_types.DockerImage, error) {
	var dockerImage node_types.DockerImage
	ctx := context.Background()
	tarBuf, err := dm.tarDirectory(contextDir)
	if err != nil {
		return dockerImage, err
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
		return dockerImage, err
	}

	// Save image logs
	configManager := utils.NewConfigManager("")
	configs, err := configManager.ReadConfigs()
	if err != nil {
		return dockerImage, err
	}
	defer resp.Body.Close()

	err = dm.processDockerBuildOutput(resp.Body, imageName, configs)
	if err != nil {
		return dockerImage, err
	}

	img, err := dm.imageMetadata(cli, imageName)
	if err != nil {
		return dockerImage, err
	}

	dockerImage = node_types.DockerImage{
		Id:          img.ID,
		Name:        imageName,
		EntryPoints: entrypoint,
		Commands:    cmd,
		Tags:        img.RepoTags,
		Digests:     img.RepoDigests,
		Os:          img.Os,
		BuiltAt:     time.Now(),
	}

	return dockerImage, err
}

func (dm *DockerManager) RemoveImage(imageName string, force bool) error {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		dm.lm.Log("error", fmt.Sprintf("Docker client error: %v", err), "docker")
		return err
	}
	ctx := context.Background()

	dm.lm.Log("info", fmt.Sprintf("Removing image %s (force=%v)...", imageName, force), "docker")

	options := image.RemoveOptions{
		Force:         force,
		PruneChildren: true,
	}
	removedImages, err := cli.ImageRemove(ctx, imageName, options)
	if err != nil {
		return fmt.Errorf("failed to remove image %s: %w", imageName, err)
	}

	for _, img := range removedImages {
		if img.Deleted != "" {
			dm.lm.Log("info", fmt.Sprintf("Deleted image: %s", img.Deleted), "docker")
		}
		if img.Untagged != "" {
			dm.lm.Log("info", fmt.Sprintf("Untagged image: %s", img.Untagged), "docker")
		}
	}

	return nil
}

func (dm *DockerManager) detectDockerfiles(path string) []composetypes.ServiceConfig {
	var services []composetypes.ServiceConfig
	var skipDirs = make(map[string]bool)

	configManager := utils.NewConfigManager("")
	configs, err := configManager.ReadConfigs()
	if err != nil {
		dm.lm.Log("error", err.Error(), "docker")
		return services
	}

	dockerScanSkip := strings.SplitSeq(configs["docker_scan_skip"], ",")
	for skip := range dockerScanSkip {
		skip = strings.TrimSpace(skip)
		skipDirs[skip] = true
	}

	err = filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip unwanted directories
		if info.IsDir() && skipDirs[info.Name()] {
			return filepath.SkipDir
		}

		if info.Name() == "Dockerfile" {
			ctxDir := filepath.Dir(filePath)
			name := strings.ToLower(filepath.Base(ctxDir))
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
	job *node_types.Job,
	cli *client.Client,
	svc composetypes.ServiceConfig,
	inputs []string,
	outputs []string,
	mounts map[string]string,
	entrypoint, cmd []string,
) (string, node_types.DockerImage, error) {
	var image node_types.DockerImage
	var err error

	// Add context with timeout to container operations
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if svc.Build != nil {
		dockerfile := svc.Build.Dockerfile
		if dockerfile == "" {
			dockerfile = "Dockerfile"
		}
		if image, err = dm.buildImage(
			cli,
			svc.Build.Context,
			svc.Image,
			dockerfile,
			entrypoint,
			cmd,
		); err != nil {
			return "", image, err
		}
	}

	// Check do we have image ready
	exists, err := dm.imageExistsLocally(cli, svc.Image)
	if err != nil {
		return "", image, err
	}
	if !exists {
		if err := dm.pullImage(cli, svc.Image); err != nil {
			return "", image, fmt.Errorf("failed to pull image %s: %w", svc.Image, err)
		}
	}

	// Inspect and attach metadata
	meta, err := dm.imageMetadata(cli, svc.Image)
	if err != nil {
		return "", image, err
	}
	image = node_types.DockerImage{
		Id:          meta.ID,
		Name:        svc.Image,
		EntryPoints: meta.Config.Entrypoint,
		Commands:    meta.Config.Cmd,
		Tags:        meta.RepoTags,
		Digests:     meta.RepoDigests,
		Os:          meta.Os,
		BuiltAt:     time.Now(), // Not really built, but we use it for consistency
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

	// Use custom entrypoint/cmd if provided, otherwise fall back to image defaults
	containerCmd := cmd
	if len(containerCmd) == 0 && len(svc.Command) > 0 {
		containerCmd = svc.Command
	}
	containerCmd = strslice.StrSlice(containerCmd)
	entryPoint := entrypoint
	if len(entryPoint) == 0 && len(svc.Entrypoint) > 0 {
		entryPoint = svc.Entrypoint
	}
	entryPoint = strslice.StrSlice(entryPoint)

	platform := dm.getPlatform(cli)

	containerConfig := container.Config{
		Image:        svc.Image,
		Entrypoint:   entrypoint,
		Cmd:          containerCmd,
		Tty:          false,
		OpenStdin:    inputs != nil,
		StdinOnce:    inputs != nil,
		AttachStdin:  inputs != nil,
		AttachStdout: outputs != nil,
		AttachStderr: outputs != nil,
	}

	if runtime.GOOS != "windows" {
		containerConfig.User = fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	}

	resp, err := cli.ContainerCreate(ctx, &containerConfig, hostConfig, netConfig, platform, svc.Name)
	if err != nil {
		var rid string = ""
		if resp.ID != "" {
			rid = resp.ID
		}
		return rid, image, err
	}

	// Save container logs
	configManager := utils.NewConfigManager("")
	configs, err := configManager.ReadConfigs()
	if err != nil {
		dm.lm.Log("error", err.Error(), "docker")
		return resp.ID, image, err
	}

	// Create job dir
	jobDir := filepath.Join(configs["local_storage"], "workflows", job.OrderingNodeId, strconv.FormatInt(job.WorkflowId, 10), "job", strconv.FormatInt(job.Id, 10))
	err = os.MkdirAll(jobDir, 0755)
	if err != nil {
		dm.lm.Log("error", err.Error(), "docker")
		return resp.ID, image, err
	}

	// Create stdin/stdout/stderr dir
	logsDir := filepath.Join(jobDir, "logs")
	err = os.MkdirAll(logsDir, 0755)
	if err != nil {
		dm.lm.Log("error", err.Error(), "docker")
		return resp.ID, image, err
	}
	// Log stdin, stdout, stderr
	stdoutFile, _ := os.Create(filepath.Join(logsDir, "stdout.log"))
	stderrFile, _ := os.Create(filepath.Join(logsDir, "stderr.log"))
	stdinFile, _ := os.Create(filepath.Join(logsDir, "stdin.log"))
	defer stdoutFile.Close()
	defer stderrFile.Close()
	defer stdinFile.Close()

	// Create job inputs dir
	inputDir := filepath.Join(jobDir, "input")
	err = os.MkdirAll(inputDir, 0755)
	if err != nil {
		dm.lm.Log("error", err.Error(), "docker")
		return resp.ID, image, err
	}

	// Create job outputs dir
	outputDir := filepath.Join(jobDir, "output")
	err = os.MkdirAll(outputDir, 0755)
	if err != nil {
		dm.lm.Log("error", err.Error(), "docker")
		return resp.ID, image, err
	}

	attachResp, err := cli.ContainerAttach(ctx, resp.ID, container.AttachOptions{
		Stream: true,
		Stdin:  inputs != nil,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		dm.lm.Log("error", err.Error(), "docker")
		return resp.ID, image, err
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		attachResp.Close()
		dm.lm.Log("error", err.Error(), "docker")
		return resp.ID, image, err
	}

	dm.lm.Log("info", fmt.Sprintf("Started container %s (%s)", svc.Name, resp.ID[:12]), "docker")

	var wgIO sync.WaitGroup
	errChanIn := make(chan error, 1)
	if inputs != nil {
		wgIO.Add(1)
		go func() {
			defer wgIO.Done()
			var files []*os.File
			// Convert to buffered readers
			bufferedInputs := make([]io.Reader, len(inputs))
			for i, input := range inputs {
				file, err := os.Open(input)
				if err != nil {
					errChanIn <- err
					return
				}
				files = append(files, file)
				bufferedInputs[i] = bufio.NewReader(file)
			}
			defer func() {
				for _, f := range files {
					f.Close()
				}
			}()

			// Combine into a single reader (optional)
			combinedInput := io.MultiReader(bufferedInputs...)

			written, err := io.Copy(io.MultiWriter(attachResp.Conn, stdinFile), combinedInput)
			if err != nil {
				dm.lm.Log("error", fmt.Sprintf("Failed to copy to stdin: %v", err), "docker")
				errChanIn <- err
				return
			}
			dm.lm.Log("debug", fmt.Sprintf("Copied %d bytes to stdin", written), "docker")
			if err := attachResp.CloseWrite(); err != nil {
				dm.lm.Log("error", fmt.Sprintf("Failed to close stdin: %v", err), "docker")
				errChanIn <- err
				return
			}
			errChanIn <- nil
		}()
	} else {
		errChanIn <- nil
	}

	if errIn := <-errChanIn; errIn != nil {
		err := fmt.Errorf("input copy failed: %w", errIn)
		dm.lm.Log("error", err.Error(), "docker")
		return resp.ID, image, err
	}

	errChanOut := make(chan error, 1)
	wgIO.Add(1)
	go func() {
		defer wgIO.Done()
		var stdoutWriter io.Writer = stdoutFile
		if outputs != nil {
			var files []*os.File
			// Convert to buffered readers
			bufferedOutputs := make([]io.Writer, len(outputs))
			for i, output := range outputs {
				file, err := os.Create(output)
				if err != nil {
					errChanOut <- err
					return
				}
				files = append(files, file)
				bufferedOutputs[i] = bufio.NewWriter(file)
			}
			defer func() {
				for _, f := range files {
					f.Close()
				}
			}()

			// Ensure all buffers are flushed at the end
			defer func() {
				for _, w := range bufferedOutputs {
					if bw, ok := w.(*bufio.Writer); ok {
						bw.Flush() // Flush any remaining data
					}
				}
			}()

			// Combine into a single reader (optional)
			combinedOutput := io.MultiWriter(bufferedOutputs...)

			stdoutWriter = io.MultiWriter(combinedOutput, stdoutFile)
		}

		written, err := stdcopy.StdCopy(stdoutWriter, stderrFile, attachResp.Reader)
		if err != nil {
			dm.lm.Log("error", fmt.Sprintf("Failed to copy to stdout: %v", err), "docker")
			errChanOut <- err
			return
		}
		dm.lm.Log("debug", fmt.Sprintf("Copied %d bytes to stdout", written), "docker")
		errChanOut <- nil
	}()

	if errOut := <-errChanOut; errOut != nil {
		err := fmt.Errorf("output copy failed: %w", errOut)
		dm.lm.Log("error", err.Error(), "docker")
		return resp.ID, image, err
	}

	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case <-statusCh:
	case err := <-errCh:
		dm.lm.Log("error", err.Error(), "docker")
		return resp.ID, image, err
	}

	// Wait for the goroutine to complete and get the error
	wgIO.Wait()

	attachResp.Close()

	go dm.streamLogs(cli, resp.ID, svc.Name)

	if svc.HealthCheck != nil {
		waitCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		for {
			select {
			case <-waitCtx.Done():
				return resp.ID, image, fmt.Errorf("timeout waiting for container %s to become healthy", svc.Name)
			default:
				inspect, err := cli.ContainerInspect(context.Background(), resp.ID)
				if err != nil {
					return resp.ID, image, err
				}
				if inspect.State != nil && inspect.State.Health != nil &&
					inspect.State.Health.Status == "healthy" {
					return resp.ID, image, nil
				}
				time.Sleep(1 * time.Second)
			}
		}
	}

	return resp.ID, image, nil
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
	path string,
	job *node_types.Job,
	buildOnly bool,
	singleService string,
	cleanup bool,
	composeFile string,
	envFile string,
	inputs, outputs []string,
	mounts map[string]string,
	entrypoint, cmd []string,
) ([]string, []node_types.DockerImage, []error) {
	var (
		project     *composetypes.Project
		images      []node_types.DockerImage
		err         error
		errrs       []error
		ssImageName string
		jobId       int64 = int64(0)
	)

	if job != nil {
		jobId = job.Id
	}
	// Set container name
	ssImageName = dm.generateContainerName(jobId, strings.ToLower(singleService))

	// Try to load compose file if specified
	if composeFile != "" {
		if _, statErr := os.Stat(composeFile); statErr == nil {
			project, err = dm.parseCompose(composeFile, envFile)
			if err != nil {
				dm.lm.Log("error", fmt.Sprintf("Parse error: %v", err), "docker")
				return nil, images, []error{err}
			}
		}
	}

	// If no compose file and singleService is specified, create a minimal project
	if project == nil && singleService != "" {
		// Define project
		project = &composetypes.Project{
			Services: []composetypes.ServiceConfig{
				{
					Name:  ssImageName,
					Image: strings.ToLower(singleService),
				},
			},
		}
	} else if project == nil {
		// Fallback: Scan for Dockerfiles
		services := dm.detectDockerfiles(path)
		if len(services) == 0 {
			dm.lm.Log("error", "No docker-compose.yml, Dockerfiles, or image specified", "docker")
			return nil, images, []error{err}
		}
		project = &composetypes.Project{Services: services}
	}

	var (
		started []string
		wg      sync.WaitGroup
		mu      sync.Mutex
		errsMu  sync.Mutex
	)

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		dm.lm.Log("error", fmt.Sprintf("Docker client error: %v", err), "docker")
		return nil, images, []error{err}
	}
	ctx := context.Background()

	defer func() {
		if cleanup || len(errrs) > 0 {
			stopOptions := container.StopOptions{}
			for _, id := range started {
				_ = cli.ContainerStop(ctx, id, stopOptions)
				_ = cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
				dm.lm.Log("info", fmt.Sprintf("Cleaned up container %s", id[:12]), "docker")
			}
		}
	}()

	for name := range project.Volumes {
		_, _ = cli.VolumeCreate(ctx, volume.CreateOptions{Name: name})
	}
	for name := range project.Networks {
		_, _ = cli.NetworkCreate(ctx, name, network.CreateOptions{})
	}

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
		if singleService != "" && svc.Name != ssImageName {
			continue
		}

		wg.Add(1)
		go func(svc composetypes.ServiceConfig) {
			defer wg.Done()
			var id string
			var err error
			var image node_types.DockerImage

			switch {
			case buildOnly && svc.Build != nil:
				// Build image from Dockerfile / Compose
				dockerfile := svc.Build.Dockerfile
				if dockerfile == "" {
					dockerfile = "Dockerfile"
				}
				image, err = dm.buildImage(
					cli,
					svc.Build.Context,
					svc.Image,
					dockerfile,
					entrypoint,
					cmd,
				)
				if err == nil {
					images = append(images, image)
				}
			case buildOnly && svc.Build == nil:
				// Pull image only
				if err := dm.pullImage(cli, svc.Image); err == nil {
					img, ierr := dm.imageMetadata(cli, svc.Image)
					if ierr == nil {
						image := node_types.DockerImage{
							Id:          img.ID,
							Name:        svc.Name,
							EntryPoints: img.Config.Entrypoint,
							Commands:    img.Config.Cmd,
							Tags:        img.RepoTags,
							Digests:     img.RepoDigests,
							Os:          img.Os,
							BuiltAt:     time.Now(),
						}
						images = append(images, image)
					}
				} else {
					msg := fmt.Sprintf("Failed to pull image %s: %v", svc.Name, err)
					dm.lm.Log("error", msg, "docker")
					dm.UI.Print(msg)
				}
			default:
				// Pull image if not built or missing locally
				exists, err := dm.imageExistsLocally(cli, svc.Image)
				if err != nil {
					msg := fmt.Sprintf("Failed to locate image %s in local repo: %v", svc.Name, err)
					dm.lm.Log("error", msg, "docker")
					dm.UI.Print(msg)
					return
				}
				if svc.Build == nil && !exists {
					if err := dm.pullImage(cli, svc.Image); err != nil {
						errsMu.Lock()
						errrs = append(errrs, fmt.Errorf("failed to pull image %s: %w", svc.Image, err))
						errsMu.Unlock()
						return
					}
				}

				// Run container
				id, image, err = dm.runService(
					job,
					cli,
					svc,
					inputs,
					outputs,
					mounts,
					entrypoint,
					cmd,
				)
				mu.Lock()
				if id != "" {
					started = append(started, id)
				}
				images = append(images, image)
				mu.Unlock()
				if err != nil {
					errr := fmt.Errorf("failed to start service %s: %v", svc.Name, err)
					dm.lm.Log("error", errr.Error(), "docker")
					dm.UI.Print(errr.Error())
					errsMu.Lock()
					errrs = append(errrs, errr)
					errsMu.Unlock()
				}
			}
			if err != nil {
				errsMu.Lock()
				errrs = append(errrs, fmt.Errorf("service %s: %w", svc.Name, err))
				errsMu.Unlock()
			}
		}(svc)
	}
	wg.Wait()

	return started, images, errrs
}

func (dm *DockerManager) generateContainerName(jobId int64, image string) string {
	// Extract base name (strip tag)
	base := strings.Split(image, ":")[0]
	// Replace invalid chars
	base = strings.ReplaceAll(base, "/", "_")
	// Create random suffix
	rnd, _ := dm.secureRandomString(6)
	// Add job ID and random suffix
	return fmt.Sprintf("job_%d_%s_%s", jobId, base, rnd)
}

func (dm *DockerManager) secureRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes)[:length], nil
}

func (dm *DockerManager) sanitizeImageName(name string) string {
	// Replace invalid characters with underscore
	invalidChars := []string{`<`, `>`, `:`, `"`, `/`, `\`, `|`, `?`, `*`}
	for _, ch := range invalidChars {
		name = strings.ReplaceAll(name, ch, "_")
	}

	// Windows reserved filenames
	reserved := map[string]bool{
		"CON": true, "PRN": true, "AUX": true, "NUL": true,
		"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
		"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
	}
	upper := strings.ToUpper(name)
	if reserved[upper] {
		name = "_" + name
	}

	return name
}
