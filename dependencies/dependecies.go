package dependencies

import (
	"fmt"
	"os"
	"os/exec"
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

func installDependecies(question string) bool {
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

func CheckAndInstallDependencies() {
	fmt.Println("This program depends on Git and Docker.")
	fmt.Println("We will now check if these are installed on your system.")

	missing := []string{}

	if _, err := exec.LookPath("git"); err != nil {
		missing = append(missing, "Git")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		missing = append(missing, "Docker")
	}

	if len(missing) == 0 {
		fmt.Println("✅ All required tools are installed.")
		return
	}

	fmt.Printf("⚠️ Missing tools detected: %s\n", strings.Join(missing, ", "))
	if !installDependecies("Install or open download links now?") {
		fmt.Println("Aborting. Please install the required tools manually.")
		os.Exit(1)
	}

	switch runtime.GOOS {
	case "linux":
		fmt.Println("Detected Linux...")
		if contains(missing, "Git") {
			exec.Command("sh", "-c", "sudo apt install -y git").Run()
		}
		if contains(missing, "Docker") {
			exec.Command("sh", "-c", "sudo apt install -y docker.io").Run()
		}
	case "darwin":
		fmt.Println("Detected macOS...")
		if contains(missing, "Git") {
			exec.Command("sh", "-c", "brew install git").Run()
		}
		if contains(missing, "Docker") {
			exec.Command("sh", "-c", "brew install --cask docker").Run()
		}
	case "windows":
		fmt.Println("Detected Windows...")
		if contains(missing, "Git") {
			exec.Command("rundll32", "url.dll,FileProtocolHandler", "https://git-scm.com/download/win").Run()
		}
		if contains(missing, "Docker") {
			exec.Command("rundll32", "url.dll,FileProtocolHandler", "https://www.docker.com/products/docker-desktop/").Run()
		}
	default:
		fmt.Println("Unsupported OS. Please install missing tools manually.")
	}
	os.Exit(1)
}
