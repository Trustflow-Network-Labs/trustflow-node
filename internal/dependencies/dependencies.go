package dependencies

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"

	"github.com/adgsm/trustflow-node/internal/ui"
	"github.com/adgsm/trustflow-node/internal/utils"
)

type DependencyManager struct {
	UI ui.UI
	cm *utils.ConfigManager
}

func NewDependencyManager(ui ui.UI, cm *utils.ConfigManager) *DependencyManager {
	return &DependencyManager{
		UI: ui,
		cm: cm,
	}
}

func (dm *DependencyManager) contains(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}

func (dm *DependencyManager) isDockerResponsive() bool {
	cmd := exec.Command("docker", "info")
	err := cmd.Run()
	return err == nil
}

func (dm *DependencyManager) dockerSubcommandExists(subcmd string) bool {
	cmd := exec.Command("docker", subcmd, "--help")
	err := cmd.Run()
	return err == nil
}

func (dm *DependencyManager) CheckAndInstallDependencies() {
	if dm.CheckDependencies() {
		// All good proceed
		var err error
		switch runtime.GOOS {
		case "linux":
			err = dm.initLinuxDependencies()
		case "darwin":
			err = dm.initDarwinDependencies()
		case "windows":
			err = dm.initWindowsDependencies()
		default:
			err = errors.New("⚠️ unsupported OS, please install missing tools manually")
		}

		if err != nil {
			dm.UI.Print("Error: " + err.Error())
			dm.UI.Exit(1)
		}

		// Chek if Docker service is responsive
		if !dm.isDockerResponsive() {
			dm.UI.Print("⚠️ Docker is not responsive")
			dm.UI.Exit(1)
		}

	} else {
		// Exit app
		dm.UI.Exit(1)
	}
}

func (dm *DependencyManager) CheckDependencies() bool {
	dm.UI.Print("This program depends on Docker.")
	dm.UI.Print("We will now check if Docker is installed on your system.")

	missing := []string{}
	/*
		if _, err := exec.LookPath("git"); err != nil {
			missing = append(missing, "Git")
		}
	*/
	if _, err := exec.LookPath("docker"); err != nil {
		missing = append(missing, "Docker")
	}
	if !dm.dockerSubcommandExists("compose") {
		missing = append(missing, "Docker Compose")
	}
	if !dm.dockerSubcommandExists("buildx") {
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
		dm.UI.Print("⚠️ Unsupported OS. Please install missing tools manually.")
		return false
	}

	if len(missing) == 0 {
		dm.UI.Print("✅ All required tools are installed.")
		return true
	}

	dm.UI.Print(fmt.Sprintf("⚠️ Missing tools detected: %s\n", strings.Join(missing, ", ")))
	if !dm.UI.PromptConfirm("Install or open download links now?") {
		dm.UI.Print("⚠️ Aborting. Please install the required tools manually.")
		return false
	}

	return dm.installDependencies(missing)
}

func (dm *DependencyManager) installDependencies(missing []string) bool {
	dm.UI.Print("We will now install dependencies on your system.")

	var err error

	switch runtime.GOOS {
	case "linux":
		err = dm.installLinuxDependencies(missing)
	case "darwin":
		err = dm.installDarwinDependencies(missing)
		return err == nil
	case "windows":
		err = dm.installWindowsDependencies(missing)
		return err == nil
	default:
		dm.UI.Print("⚠️ Unsupported OS. Please install missing tools manually.")
		return false
	}

	if err != nil {
		dm.UI.Print(fmt.Sprintf("⚠️ Error: %s", err.Error()))
	}

	return err == nil
}

func (dm *DependencyManager) installLinuxDependencies(missing []string) error {
	dm.UI.Print("Detected Linux...")
	/*
		if dm.contains(missing, "Git") {
			err := exec.Command("sh", "-c", "sudo apt install -y git").Run()
			if err != nil {
				dm.UI.Print(fmt.Sprintf("⚠️ Error: %s", err.Error()))
			}
			return err
		}
	*/
	if dm.contains(missing, "Docker") {
		err := exec.Command("sh", "-c", "sudo apt install -y docker.io").Run()
		if err != nil {
			dm.UI.Print(fmt.Sprintf("⚠️ Error: %s", err.Error()))
		}
		return err
	}
	/*
		if dm.contains(missing, "Kubernetes") {
			dm.UI.Print("Trying to install kubectl via Snap...")
			err := exec.Command("sh", "-c", "sudo snap install kubectl --classic").Run()
			if err != nil {
				dm.UI.Print(fmt.Sprintf("⚠️ Snap install failed: %s\n", err.Error()))
				dm.UI.Print("Trying manual installation instead...")
				// fallback to manual download
				err = exec.Command("sh", "-c", `
					curl -LO "https://dl.k8s.io/release/$(curl -Ls https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" &&
					chmod +x kubectl &&
					sudo mv kubectl /usr/local/bin/
				`).Run()
				if err != nil {
					dm.UI.Print(fmt.Sprintf("⚠️ Manual install failed: %s\n", err.Error()))
				}
				return err
			}
		}
	*/
	return nil
}

func (dm *DependencyManager) initLinuxDependencies() error {
	if !dm.isDockerRunningLinux() {
		// Start docker
		dm.UI.Print("We will now try to start Docker on your system.")
		err := exec.Command("sh", "-c", "sudo systemctl start docker").Run()
		if err != nil {
			return err
		}
	}

	if dm.isDockerRunningLinux() {
		dm.UI.Print("✅ Docker started successfully.")
	} else {
		err := errors.New("could not start Docker")
		return err
	}

	// Add user to docker group
	err := dm.addUserToDockerGroupLinux()
	if err != nil {
		dm.UI.Print(fmt.Sprintf("⚠️ Failed to add user to docker group: %s", err.Error()))
		dm.UI.Print("Please run the following command manually to grant Docker access:")
		dm.UI.Print("sudo usermod -aG docker $USER")
		return err
	}

	// Docker API client version
	maxApiVersion := dm.patchDockerAPIVersion()

	// Ask user to set it permanently in shell
	if maxApiVersion != "" {
		dm.UI.Print("⚠️ Please add the following to your shell profile:")
		dm.UI.Print(fmt.Sprintf("export DOCKER_API_VERSION=%s", maxApiVersion))
	}

	return nil
}

func (dm *DependencyManager) isDockerRunningLinux() bool {
	cmd := exec.Command("sh", "-c", "systemctl is-active docker")
	output, err := cmd.Output()
	return err == nil && strings.TrimSpace(strings.ToLower(string(output))) == "active"
}

func (dm *DependencyManager) isUserInDockerGroupLinux() error {
	currentUser, err := user.Current()
	if err != nil {
		return err
	}

	// Run `id -Gn <username>` to get group names
	out, err := exec.Command("id", "-Gn", currentUser.Username).Output()
	if err != nil {
		return err
	}

	groups := strings.Fields(string(out))
	inDockerGroup := slices.Contains(groups, "docker")

	if inDockerGroup {
		return nil
	} else {
		return fmt.Errorf("⚠️ User `%s` is NOT in the 'docker' group", currentUser.Username)
	}
}

func (dm *DependencyManager) addUserToDockerGroupLinux() error {
	err := dm.isUserInDockerGroupLinux()
	if err == nil {
		return nil
	} else {
		dm.UI.Print(err.Error())
	}

	currentUser, err := user.Current()
	if err != nil {
		return err
	}

	cmd := exec.Command("sudo", "usermod", "-aG", "docker", currentUser.Username)
	cmd.Stdout = nil
	cmd.Stderr = nil

	err = cmd.Run()
	if err != nil {
		return err
	}

	dm.UI.Print("✅ User successfully added to docker group.")
	dm.UI.Print("⚠️  To apply the changes, you need to log out and log back in.")
	dm.UI.Print("Alternatively, you can run:")
	dm.UI.Print("  source ~/.bashrc")

	return nil
}

func (dm *DependencyManager) installDarwinDependencies(missing []string) error {
	dm.UI.Print("Detected macOS...")
	/*
		if dm.contains(missing, "Git") {
			err := exec.Command("sh", "-c", "brew install git").Run()
			if err != nil {
				dm.UI.Print(fmt.Sprintf("⚠️ Error: %s", err.Error()))
			}
			return err
		}
	*/
	if dm.contains(missing, "Docker") || dm.contains(missing, "Colima") ||
		dm.contains(missing, "Docker Compose") || dm.contains(missing, "Docker Buildx") || dm.contains(missing, "Kubernetes") {
		dm.UI.Print("Installing Colima, Docker CLI tools, and plugins...")
		commands := []string{
			"brew install colima",
			"brew install docker docker-compose docker-buildx",
			"mkdir -p ~/.docker/cli-plugins",
			"ln -sfn $(brew --prefix)/opt/docker-compose/bin/docker-compose ~/.docker/cli-plugins/docker-compose",
			"ln -sfn $(brew --prefix)/opt/docker-buildx/bin/docker-buildx ~/.docker/cli-plugins/docker-buildx",
			//			"brew install kubectl",
		}
		for _, cmd := range commands {
			dm.UI.Print(fmt.Sprintf("Executing: %s", cmd))
			err := exec.Command("sh", "-c", cmd).Run()
			if err != nil {
				dm.UI.Print(fmt.Sprintf("⚠️ Error: %s", err.Error()))
				return err
			}
		}
	}

	return nil
}

func (dm *DependencyManager) initDarwinDependencies() error {
	if !dm.isColimaRunning() {
		// Start docker
		dm.UI.Print("Starting Colima")
		storagePath, err := filepath.Abs(dm.cm.GetConfigWithDefault("local_storage", "./local_storage/"))
		if err != nil {
			dm.UI.Print(fmt.Sprintf("⚠️ Error: %s", err.Error()))
			return err
		}

		commands := []string{
			//			fmt.Sprintf("colima start --with-kubernetes --mount %s:w", storagePath),
			fmt.Sprintf("colima start --mount %s:w", storagePath),
		}
		for _, cmd := range commands {
			dm.UI.Print(fmt.Sprintf("Executing: %s", cmd))
			err := exec.Command("sh", "-c", cmd).Run()
			if err != nil {
				dm.UI.Print(fmt.Sprintf("⚠️ Error: %s", err.Error()))
				return err
			}
		}
	}

	// Docker socket
	os.Setenv("DOCKER_HOST", fmt.Sprintf("unix://%s/.colima/docker.sock", os.Getenv("HOME")))

	// Docker API client version
	maxApiVersion := dm.patchDockerAPIVersion()

	// Ask user to set it permanently in shell
	dm.UI.Print("⚠️ Please add the following to your shell profile:")
	dm.UI.Print(`export DOCKER_HOST="unix://$HOME/.colima/docker.sock"`)
	if maxApiVersion != "" {
		dm.UI.Print(fmt.Sprintf("export DOCKER_API_VERSION=%s", maxApiVersion))
	}

	if !dm.isColimaRunning() {
		return fmt.Errorf("colima did not start correctly")
	}

	dm.UI.Print("✅ Colima (and Docker) are running.")

	return nil
}

func (dm *DependencyManager) isColimaRunning() bool {
	cmd := exec.Command("colima", "status")
	output, err := cmd.CombinedOutput()
	return err == nil && strings.Contains(strings.ToLower(string(output)), "running")
}

func (dm *DependencyManager) patchDockerAPIVersion() string {
	var maxApiVersion string
	os.Setenv("DOCKER_API_VERSION", "99.999")
	cmd := exec.Command("docker", "ps")
	output, err := cmd.CombinedOutput()
	if err != nil && strings.Contains(strings.ToLower(string(output)), strings.ToLower("Maximum supported API version is")) {
		dm.UI.Print("⚠️ Docker client version is too new. Patching with max supported API version.")
		// Detect from output
		re := regexp.MustCompile(strings.ToLower(`Maximum supported API version is ([\d.]+)`))
		match := re.FindStringSubmatch(strings.ToLower(string(output)))
		if len(match) > 1 {
			maxApiVersion = match[1]
			dm.UI.Print(fmt.Sprintf("set DOCKER_API_VERSION to %s\n", maxApiVersion))
			os.Setenv("DOCKER_API_VERSION", maxApiVersion)
		}
	}

	return maxApiVersion
}

func (dm *DependencyManager) installWindowsDependencies(missing []string) error {
	dm.UI.Print("Detected Windows...")
	/*
		if contains(missing, "Git") {
			err := exec.Command("rundll32", "url.dll,FileProtocolHandler", "https://git-scm.com/download/win").Run()
			if err != nil {
				dm.UI.Print(fmt.Sprintf("⚠️ Error: %s", err.Error()))
			}
			return err
		}
	*/
	if dm.contains(missing, "Docker") {
		err := exec.Command("rundll32", "url.dll,FileProtocolHandler", "https://www.docker.com/products/docker-desktop/").Run()
		if err != nil {
			dm.UI.Print(fmt.Sprintf("⚠️ Error: %s", err.Error()))
		}
		return err
	}

	return nil
}

func (dm *DependencyManager) initWindowsDependencies() error {
	if err := dm.checkSymlinkSupport(); err != nil {
		dm.UI.Print("❌ Symlink creation is not supported for your current user.")
		dm.UI.Print("To fix this, you can do one of the following:")

		dm.UI.Print("\n➡ OPTION 1: Enable Developer Mode (recommended)")
		dm.UI.Print("  Run this in PowerShell (as Administrator):")
		dm.UI.Print(`  reg add "HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock" /t REG_DWORD /f /v "AllowDevelopmentWithoutDevLicense" /d "1"`)
		dm.UI.Print("  OR open Settings → Privacy & Security → For Developers → Enable Developer Mode")

		dm.UI.Print("\n➡ OPTION 2: Grant SeCreateSymbolicLinkPrivilege to your user (advanced)")
		dm.UI.Print("  Open Local Security Policy (secpol.msc) → Local Policies → User Rights Assignment")
		dm.UI.Print("  → Find 'Create symbolic links' → Add your user → Restart")

		dm.UI.Print("\nAfter applying one of the options above, please restart your computer and try again.")

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

func (dm *DependencyManager) checkSymlinkSupport() error {
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
