package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type PIDManager struct {
	path string
}

func NewPIDManager(pidFilePath string) (*PIDManager, error) {
	// Ensure directory exists
	dir := filepath.Dir(pidFilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create PID directory: %v", err)
	}

	return &PIDManager{path: pidFilePath}, nil
}

func (p *PIDManager) WritePID(pid int) error {
	pidStr := strconv.Itoa(pid)

	return os.WriteFile(p.path, []byte(pidStr), 0644)
}

func (p *PIDManager) ReadPID() (int, error) {
	data, err := os.ReadFile(p.path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, errors.New("PID file does not exist")
		}
		return 0, fmt.Errorf("failed to read PID file: %v", err)
	}

	pidStr := string(data)
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid PID format: %v", err)
	}

	return pid, nil
}
