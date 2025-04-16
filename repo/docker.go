package repo

import (
	"os/exec"
)

type DockerManager struct {
}

func NewDockerManager() *DockerManager {
	return &DockerManager{}
}

// ValidateImage checks if a Docker image exists in a registry.
// Assumes user is authenticated via `docker login` for private registries.
func (dm *DockerManager) ValidateImage(image string) ([]byte, error) {
	cmd := exec.Command("docker", "manifest", "inspect", image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, err
	}

	return output, nil
}
