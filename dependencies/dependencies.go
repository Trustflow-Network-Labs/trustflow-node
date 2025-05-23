package dependencies

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/adgsm/trustflow-node/utils"
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

func isDockerResponsive() bool {
	cmd := exec.Command("docker", "info")
	err := cmd.Run()
	return err == nil
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

		// Chek if Docker service is responsive
		if !isDockerResponsive() {
			fmt.Printf("⚠️ Docker is not responsive\n")
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
	/*
		if _, err := exec.LookPath("git"); err != nil {
			missing = append(missing, "Git")
		}
	*/
	if _, err := exec.LookPath("docker"); err != nil {
		missing = append(missing, "Docker")
	}
	if !dockerSubcommandExists("compose") {
		missing = append(missing, "Docker Compose")
	}
	if !dockerSubcommandExists("buildx") {
		missing = append(missing, "Docker Buildx")
	}
	/*
		if _, err := exec.LookPath("kubectl"); err != nil {
			missing = append(missing, "Kubernetes")
		}
	*/
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
	/*
		if contains(missing, "Git") {
			err := exec.Command("sh", "-c", "sudo apt install -y git").Run()
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
			}
			return err
		}
	*/
	if contains(missing, "Docker") {
		err := exec.Command("sh", "-c", "sudo apt install -y docker.io").Run()
		if err != nil {
			fmt.Printf("Error: %s\n", err.Error())
		}
		return err
	}
	/*
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
	*/
	return nil
}

func initLinuxDependencies() error {
	if !isDockerRunningLinux() {
		// Start docker
		fmt.Println("We will now try to start Docker on your system.")
		err := exec.Command("sh", "-c", "sudo systemctl start docker").Run()
		if err != nil {
			return err
		}
	}

	if isDockerRunningLinux() {
		fmt.Println("✅ Docker started successfully.")
	} else {
		err := errors.New("could not start Docker")
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

func isDockerRunningLinux() bool {
	cmd := exec.Command("sh", "-c", "systemctl is-active docker")
	output, err := cmd.Output()
	return err == nil && strings.TrimSpace(strings.ToLower(string(output))) == "active"
}

func installDarwinDependencies(missing []string) error {
	fmt.Println("Detected macOS...")
	/*
		if contains(missing, "Git") {
			err := exec.Command("sh", "-c", "brew install git").Run()
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
			}
			return err
		}
	*/
	if contains(missing, "Docker") || contains(missing, "Colima") ||
		contains(missing, "Docker Compose") || contains(missing, "Docker Buildx") || contains(missing, "Kubernetes") {
		//		fmt.Println("Installing Colima, Docker CLI tools, Kubernetes, and plugins...")
		fmt.Println("Installing Colima, Docker CLI tools, and plugins...")
		commands := []string{
			"brew install colima",
			"brew install docker docker-compose docker-buildx",
			"mkdir -p ~/.docker/cli-plugins",
			"ln -sfn $(brew --prefix)/opt/docker-compose/bin/docker-compose ~/.docker/cli-plugins/docker-compose",
			"ln -sfn $(brew --prefix)/opt/docker-buildx/bin/docker-buildx ~/.docker/cli-plugins/docker-buildx",
			//			"brew install kubectl",
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
	if !isColimaRunning() {

		configManager := utils.NewConfigManager("")
		configs, err := configManager.ReadConfigs()
		if err != nil {
			return err
		}

		// Start docker
		fmt.Println("Starting Colima")
		storagePath, err := filepath.Abs(configs["local_storage"])
		if err != nil {
			fmt.Printf("Error: %s\n", err.Error())
			return err
		}

		commands := []string{
			//		"colima start --with-kubernetes",
			fmt.Sprintf("colima start --mount %s:w", storagePath),
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

	if !isColimaRunning() {
		return fmt.Errorf("colima did not start correctly")
	}

	fmt.Println("✅ Colima (and Docker) are running.")

	return nil
}

func isColimaRunning() bool {
	cmd := exec.Command("colima", "status")
	output, err := cmd.CombinedOutput()
	return err == nil && strings.Contains(strings.ToLower(string(output)), "running")
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
	/*
		if contains(missing, "Git") {
			err := exec.Command("rundll32", "url.dll,FileProtocolHandler", "https://git-scm.com/download/win").Run()
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
			}
			return err
		}
	*/
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
	if err := checkSymlinkSupport(); err != nil {
		fmt.Println("❌ Symlink creation is not supported for your current user.")
		fmt.Println("To fix this, you can do one of the following:")

		fmt.Println("\n➡ OPTION 1: Enable Developer Mode (recommended)")
		fmt.Println("  Run this in PowerShell (as Administrator):")
		fmt.Println(`  reg add "HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock" /t REG_DWORD /f /v "AllowDevelopmentWithoutDevLicense" /d "1"`)
		fmt.Println("  OR open Settings → Privacy & Security → For Developers → Enable Developer Mode")

		fmt.Println("\n➡ OPTION 2: Grant SeCreateSymbolicLinkPrivilege to your user (advanced)")
		fmt.Println("  Open Local Security Policy (secpol.msc) → Local Policies → User Rights Assignment")
		fmt.Println("  → Find 'Create symbolic links' → Add your user → Restart")

		fmt.Println("\nAfter applying one of the options above, please restart your computer and try again.")
		return fmt.Errorf("symlink creation is not permitted for the current user")
	}

	candidates := []string{
		`Start-Process "Docker" -Verb runAs`,
		`Start-Process "Docker Desktop" -Verb runAs`,
		`Start-Process "C:\Program Files\Docker\Docker\Docker Desktop.exe" -Verb runAs`,
	}

	for _, cmd := range candidates {
		err := exec.Command("powershell", "-Command", cmd).Run()
		if err == nil {
			return nil
		}
	}

	return fmt.Errorf("could not start Docker Desktop")
}

func checkSymlinkSupport() error {
	// Try to create a dummy symlink and remove it
	tmpDir := os.TempDir()
	target := filepath.Join(tmpDir, "symlink_target.txt")
	link := filepath.Join(tmpDir, "symlink_link.txt")

	_ = os.WriteFile(target, []byte("test"), 0644)
	defer os.Remove(target)

	err := os.Symlink(target, link)
	defer os.Remove(link)

	if err != nil {
		return fmt.Errorf("symlinks not supported: %w", err)
	}
	return nil
}
