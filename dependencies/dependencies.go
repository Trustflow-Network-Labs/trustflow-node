package dependencies

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/manifoldco/promptui"
)

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}

func installMissing(question string) bool {
	prompt := promptui.Prompt{
		Label:     question,
		IsConfirm: true,
	}
	result, err := prompt.Run()
	if err != nil {
		return false
	}

	answer := strings.ToLower(strings.TrimSpace(result))
	return answer == "y" || answer == "yes"
}

func dockerSubcommandExists(subcmd string) bool {
	cmd := exec.Command("docker", subcmd, "--help")
	err := cmd.Run()
	return err == nil
}

func CheckAndInstallDependencies() {
	if CheckDependencies() {
		// All good proceed
		var err error
		switch runtime.GOOS {
		case "linux":
			err = initLinuxDependencies()
		case "darwin":
			err = initDarwinDependencies()
		case "windows":
			err = initWindowsDependencies()
		default:
			err = errors.New("unsupported OS, please install missing tools manually")
		}

		if err != nil {
			fmt.Printf("Error: %s\n", err.Error())
			os.Exit(1)
		}
	} else {
		// Exit app
		os.Exit(1)
	}
}

func CheckDependencies() bool {
	fmt.Println("This program depends on Git and Docker.")
	fmt.Println("We will now check if these are installed on your system.")

	missing := []string{}

	if _, err := exec.LookPath("git"); err != nil {
		missing = append(missing, "Git")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		missing = append(missing, "Docker")
	}
	if !dockerSubcommandExists("compose") {
		missing = append(missing, "Docker Compose")
	}
	if !dockerSubcommandExists("buildx") {
		missing = append(missing, "Docker Buildx")
	}
	if _, err := exec.LookPath("kubectl"); err != nil {
		missing = append(missing, "Kubernetes")
	}

	switch runtime.GOOS {
	case "linux":
	case "darwin":
		if _, err := exec.LookPath("colima"); err != nil {
			missing = append(missing, "Colima")
		}
	case "windows":
	default:
		fmt.Println("Unsupported OS. Please install missing tools manually.")
		return false
	}

	if len(missing) == 0 {
		fmt.Println("✅ All required tools are installed.")
		return true
	}

	fmt.Printf("⚠️ Missing tools detected: %s\n", strings.Join(missing, ", "))
	if !installMissing("Install or open download links now?") {
		fmt.Println("Aborting. Please install the required tools manually.")
		return false
	}

	return installDependencies(missing)
}

func installDependencies(missing []string) bool {
	fmt.Println("We will now install dependencies on your system.")

	var err error

	switch runtime.GOOS {
	case "linux":
		err = installLinuxDependencies(missing)
	case "darwin":
		err = installDarwinDependencies(missing)
		return err == nil
	case "windows":
		err = installWindowsDependencies(missing)
		return err == nil
	default:
		fmt.Println("Unsupported OS. Please install missing tools manually.")
		return false
	}

	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())
	}

	return err == nil
}

func installLinuxDependencies(missing []string) error {
	fmt.Println("Detected Linux...")

	if contains(missing, "Git") {
		err := exec.Command("sh", "-c", "sudo apt install -y git").Run()
		if err != nil {
			fmt.Printf("Error: %s\n", err.Error())
		}
		return err
	}
	if contains(missing, "Docker") {
		err := exec.Command("sh", "-c", "sudo apt install -y docker.io").Run()
		if err != nil {
			fmt.Printf("Error: %s\n", err.Error())
		}
		return err
	}
	if contains(missing, "Kubernetes") {
		fmt.Println("Trying to install kubectl via Snap...")
		err := exec.Command("sh", "-c", "sudo snap install kubectl --classic").Run()
		if err != nil {
			fmt.Printf("Snap install failed: %s\n", err.Error())
			fmt.Println("Trying manual installation instead...")
			// fallback to manual download
			err = exec.Command("sh", "-c", `
				curl -LO "https://dl.k8s.io/release/$(curl -Ls https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" &&
				chmod +x kubectl &&
				sudo mv kubectl /usr/local/bin/
			`).Run()
			if err != nil {
				fmt.Printf("Manual install failed: %s\n", err.Error())
			}
			return err
		}
	}

	return nil
}

func initLinuxDependencies() error {
	// Start docker
	err := exec.Command("sh", "-c", "sudo systemctl start docker").Run()
	if err != nil {
		return err
	}

	// Docker API client version
	maxApiVersion := patchDockerAPIVersion()

	// Ask user to set it permanently in shell
	if maxApiVersion != "" {
		fmt.Println("⚠️ Please add the following to your shell profile:")
		fmt.Printf("export DOCKER_API_VERSION=%s\n", maxApiVersion)
	}

	return nil
}

func installDarwinDependencies(missing []string) error {
	fmt.Println("Detected macOS...")
	if contains(missing, "Git") {
		err := exec.Command("sh", "-c", "brew install git").Run()
		if err != nil {
			fmt.Printf("Error: %s\n", err.Error())
		}
		return err
	}
	if contains(missing, "Docker") || contains(missing, "Colima") ||
		contains(missing, "Docker Compose") || contains(missing, "Docker Buildx") || contains(missing, "Kubernetes") {
		fmt.Println("Installing Colima, Docker CLI tools, Kubernetes, and plugins...")
		commands := []string{
			"brew install colima",
			"brew install docker docker-compose docker-buildx",
			"mkdir -p ~/.docker/cli-plugins",
			"ln -sfn $(brew --prefix)/opt/docker-compose/bin/docker-compose ~/.docker/cli-plugins/docker-compose",
			"ln -sfn $(brew --prefix)/opt/docker-buildx/bin/docker-buildx ~/.docker/cli-plugins/docker-buildx",
			"brew install kubectl",
		}
		for _, cmd := range commands {
			fmt.Printf("Executing: %s\n", cmd)
			err := exec.Command("sh", "-c", cmd).Run()
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				return err
			}
		}
	}

	return nil
}

func initDarwinDependencies() error {
	// Start docker
	fmt.Println("Starting Colima")
	commands := []string{
		"colima start --with-kubernetes",
	}
	for _, cmd := range commands {
		fmt.Printf("Executing: %s\n", cmd)
		err := exec.Command("sh", "-c", cmd).Run()
		if err != nil {
			fmt.Printf("Error: %s\n", err.Error())
			return err
		}
	}

	// Docker socket
	os.Setenv("DOCKER_HOST", fmt.Sprintf("unix://%s/.colima/docker.sock", os.Getenv("HOME")))

	// Docker API client version
	maxApiVersion := patchDockerAPIVersion()

	// Ask user to set it permanently in shell
	fmt.Println("⚠️ Please add the following to your shell profile:")
	fmt.Println(`export DOCKER_HOST="unix://$HOME/.colima/docker.sock"`)
	if maxApiVersion != "" {
		fmt.Printf("export DOCKER_API_VERSION=%s\n", maxApiVersion)
	}

	return nil
}

func patchDockerAPIVersion() string {
	var maxApiVersion string
	os.Setenv("DOCKER_API_VERSION", "99.999")
	cmd := exec.Command("docker", "ps")
	output, err := cmd.CombinedOutput()
	if err != nil && strings.Contains(strings.ToLower(string(output)), strings.ToLower("Maximum supported API version is")) {
		fmt.Println("⚠️ Docker client version is too new. Patching with max supported API version.")
		// Detect from output
		re := regexp.MustCompile(strings.ToLower(`Maximum supported API version is ([\d.]+)`))
		match := re.FindStringSubmatch(strings.ToLower(string(output)))
		if len(match) > 1 {
			maxApiVersion = match[1]
			fmt.Printf("set DOCKER_API_VERSION to %s\n", maxApiVersion)
			os.Setenv("DOCKER_API_VERSION", maxApiVersion)
		}
	}

	return maxApiVersion
}

func installWindowsDependencies(missing []string) error {
	fmt.Println("Detected Windows...")
	if contains(missing, "Git") {
		err := exec.Command("rundll32", "url.dll,FileProtocolHandler", "https://git-scm.com/download/win").Run()
		if err != nil {
			fmt.Printf("Error: %s\n", err.Error())
		}
		return err
	}
	if contains(missing, "Docker") {
		err := exec.Command("rundll32", "url.dll,FileProtocolHandler", "https://www.docker.com/products/docker-desktop/").Run()
		if err != nil {
			fmt.Printf("Error: %s\n", err.Error())
		}
		return err
	}

	return nil
}

func initWindowsDependencies() error {
	// Try to start Docker Desktop
	return exec.Command("powershell", "-Command", "Start-Process 'Docker Desktop' -Verb runAs").Run()
}
