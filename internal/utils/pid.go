package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

type PIDManager struct {
	dir string
	cm  *ConfigManager
}

func NewPIDManager(cm *ConfigManager) (*PIDManager, error) {
	// Get os paths
	paths := GetAppPaths("")

	return &PIDManager{
		dir: paths.DataDir,
		cm:  cm,
	}, nil
}

func (p *PIDManager) WritePID(pid int) error {
	// Make sure we have os specific path separator since we are adding this path to host's path
	pidFileName := p.cm.GetConfigWithDefault("pid_path", "./trustflow.pid")
	switch runtime.GOOS {
	case "linux", "darwin":
		pidFileName = filepath.ToSlash(pidFileName)
	case "windows":
		pidFileName = filepath.FromSlash(pidFileName)
	default:
		err := fmt.Errorf("unsupported OS type `%s`", runtime.GOOS)
		return err
	}

	// create pid file path
	path := filepath.Join(p.dir, pidFileName)

	pidStr := strconv.Itoa(pid)

	return os.WriteFile(path, []byte(pidStr), 0644)
}

func (p *PIDManager) ReadPID() (int, error) {
	// Make sure we have os specific path separator since we are adding this path to host's path
	pidFileName := p.cm.GetConfigWithDefault("pid_path", "./trustflow.pid")
	switch runtime.GOOS {
	case "linux", "darwin":
		pidFileName = filepath.ToSlash(pidFileName)
	case "windows":
		pidFileName = filepath.FromSlash(pidFileName)
	default:
		err := fmt.Errorf("unsupported OS type `%s`", runtime.GOOS)
		return 0, err
	}

	// create pid file path
	path := filepath.Join(p.dir, pidFileName)

	data, err := os.ReadFile(path)
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

func (p *PIDManager) StopProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		// On Windows, Kill() is the only option
		return process.Kill()
	} else {
		// On Unix-like systems, try graceful termination first
		err = process.Signal(syscall.SIGTERM)
		if err != nil {
			return err
		}

		// Wait for graceful termination (5 seconds grace period)
		gracePeriod := 5 * time.Second
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		timeout := time.After(gracePeriod)

		for {
			select {
			case <-timeout:
				// Grace period expired, force kill
				return process.Signal(syscall.SIGKILL)
			case <-ticker.C:
				// Check if process still exists
				err := process.Signal(syscall.Signal(0)) // Signal 0 checks existence
				if err != nil {
					// Process has terminated
					return nil
				}
			}
		}
	}
}
