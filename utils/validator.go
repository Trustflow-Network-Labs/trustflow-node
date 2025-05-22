package utils

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/libp2p/go-libp2p/core/peer"
)

type ValidatorManager struct {
	cm *ConfigManager
}

func NewValidatorManager() *ValidatorManager {
	return &ValidatorManager{
		cm: NewConfigManager(""),
	}
}

func (vm *ValidatorManager) NotEmpty(s string) error {
	if s == "" {
		return fmt.Errorf("expected non empty input string")
	}
	return nil
}

func (vm *ValidatorManager) MinLen(s string) error {
	config, err := vm.cm.ReadConfigs()
	if err != nil {
		return err
	}
	sl := config["phrase_min_len"]
	il, err := strconv.ParseInt(sl, 10, 64)
	if err != nil {
		return err
	}
	if len(s) < int(il) {
		return fmt.Errorf("expected non empty input string of min length %d (got %d)", int(il), len(s))
	}
	return nil
}

func (vm *ValidatorManager) IsPeer(s string) error {
	_, err := peer.Decode(s)
	if err != nil {
		return err
	}
	return nil
}

func (vm *ValidatorManager) ArePeers(s string) error {
	peers := strings.SplitSeq(s, ",")
	for p := range peers {
		p = strings.TrimSpace(p)
		err := vm.IsPeer(p)
		if err != nil {
			return err
		}
	}
	return nil
}

func (vm *ValidatorManager) IsBool(s string) error {
	_, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	return nil
}

func (vm *ValidatorManager) IsInt64(s string) error {
	_, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	return nil
}

func (vm *ValidatorManager) IsFloat64(s string) error {
	_, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	return nil
}

func (vm *ValidatorManager) IsValidFileNameUnix(path string) error {
	erra := vm.IsValidAbsoluteFileNameUnix(path)
	errr := vm.IsValidRelativeFileNameUnix(path)

	if erra == nil || errr == nil {
		return nil
	}

	return fmt.Errorf("%s is neither valid absolute nor relative path", path)
}

func (vm *ValidatorManager) IsValidFileNameWindows(path string) error {
	erra := vm.IsValidAbsoluteFileNameWindows(path)
	errr := vm.IsValidRelativeFileNameWindows(path)

	if erra == nil || errr == nil {
		return nil
	}

	return fmt.Errorf("%s is neither valid absolute nor relative path", path)
}

func (vm *ValidatorManager) IsValidAbsoluteFileNameUnix(path string) error {
	return vm.IsValidPath(path, true, true, "unix")
}

func (vm *ValidatorManager) IsValidAbsoluteFileNameWindows(path string) error {
	return vm.IsValidPath(path, true, true, "windows")
}

func (vm *ValidatorManager) IsValidRelativeFileNameUnix(path string) error {
	return vm.IsValidPath(path, true, false, "unix")
}

func (vm *ValidatorManager) IsValidRelativeFileNameWindows(path string) error {
	return vm.IsValidPath(path, true, false, "windows")
}

func (vm *ValidatorManager) IsValidMountPointUnix(path string) error {
	erra := vm.IsValidAbsoluteMountPointUnix(path)
	errr := vm.IsValidRelativeMountPointUnix(path)

	if erra == nil || errr == nil {
		return nil
	}

	return fmt.Errorf("%s is neither valid absolute nor relative mount path", path)
}

func (vm *ValidatorManager) IsValidMountPointWindows(path string) error {
	erra := vm.IsValidAbsoluteMountPointWindows(path)
	errr := vm.IsValidRelativeMountPointWindows(path)

	if erra == nil || errr == nil {
		return nil
	}

	return fmt.Errorf("%s is neither valid absolute nor relative mount path", path)
}

func (vm *ValidatorManager) IsValidAbsoluteMountPointUnix(path string) error {
	return vm.IsValidPath(path, false, true, "unix")
}

func (vm *ValidatorManager) IsValidAbsoluteMountPointWindows(path string) error {
	return vm.IsValidPath(path, false, true, "windows")
}

func (vm *ValidatorManager) IsValidRelativeMountPointUnix(path string) error {
	return vm.IsValidPath(path, false, false, "unix")
}

func (vm *ValidatorManager) IsValidRelativeMountPointWindows(path string) error {
	return vm.IsValidPath(path, false, false, "windows")
}

func (vm *ValidatorManager) IsValidFileNameOrMountPointUnix(path string) error {
	erra := vm.IsValidFileNameUnix(path)
	errr := vm.IsValidMountPointUnix(path)

	if erra == nil || errr == nil {
		return nil
	}

	return fmt.Errorf("%s is neither valid file name nor mount point", path)
}

func (vm *ValidatorManager) IsValidFileNameOrMountPointWindows(path string) error {
	erra := vm.IsValidFileNameWindows(path)
	errr := vm.IsValidMountPointWindows(path)

	if erra == nil || errr == nil {
		return nil
	}

	return fmt.Errorf("%s is neither valid file name nor mount point", path)
}

func (vm *ValidatorManager) IsValidPath(path string, fileName bool, absolute bool, targetOS string) error {
	if path == "" {
		return errors.New("path is empty")
	}
	if strings.ContainsRune(path, 0) {
		return errors.New("path contains null byte")
	}
	if strings.HasPrefix(path, "~") {
		return errors.New("path uses home directory reference '~'")
	}

	// Detect OS
	osType := targetOS
	if osType == "" {
		osType = runtime.GOOS
	}

	// Normalize
	cleaned := path
	if osType == "windows" {
		cleaned = filepath.Clean(path)
	} else {
		// Avoid using filepath.Clean() for Linux-style paths on Windows
		cleaned = path
	}

	// Absolute path check
	switch osType {
	case "windows":
		isAbs := regexp.MustCompile(`^[a-zA-Z]:\\`).MatchString(path)
		if absolute && !isAbs {
			return errors.New("windows path must be absolute (e.g. C:\\path)")
		}
		if !absolute && isAbs {
			return errors.New("windows path must not be absolute")
		}
	case "unix":
		isAbs := strings.HasPrefix(path, "/")
		if absolute && !isAbs {
			return errors.New("linux path must be absolute (start with '/')")
		}
		if !absolute && isAbs {
			return errors.New("linux path must not be absolute")
		}
	default:
		return fmt.Errorf("unsupported OS: %s", osType)
	}

	// Directory escape protection
	if strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, "/..") {
		return errors.New("path escapes working directory")
	}

	// OS-specific character checks
	if osType == "windows" {
		invalidChars := `<>:"/\|?*`
		for _, ch := range invalidChars {
			if strings.ContainsRune(path, ch) {
				return fmt.Errorf("windows path contains invalid character: %q", ch)
			}
		}
	}

	// File name validation
	if fileName {
		sep := "/"
		if osType == "windows" {
			sep = `\`
		}
		if strings.HasSuffix(path, sep) {
			return errors.New("path is a directory")
		}

		base := filepath.Base(cleaned)
		if base == "." || base == "" {
			return errors.New("path does not contain a file name")
		}

		// Windows reserved file names
		if osType == "windows" {
			reserved := map[string]bool{
				"CON": true, "PRN": true, "AUX": true, "NUL": true,
				"COM1": true, "COM2": true, "COM3": true, "COM4": true,
				"COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
				"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true,
				"LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
			}
			if reserved[strings.ToUpper(base)] {
				return errors.New("file name is reserved on Windows")
			}
		}
	}

	return nil
}
