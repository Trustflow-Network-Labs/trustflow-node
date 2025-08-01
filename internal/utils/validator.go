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

func NewValidatorManager(cm *ConfigManager) *ValidatorManager {
	return &ValidatorManager{
		cm: cm,
	}
}

func (vm *ValidatorManager) NotEmpty(s string) error {
	if s == "" {
		return fmt.Errorf("expected non empty input string")
	}
	return nil
}

func (vm *ValidatorManager) MinLen(s string) error {
	sl := vm.cm.GetConfigWithDefault("phrase_min_len", "3")
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

	return fmt.Errorf("%s is neither valid absolute nor relative path `%s` `%s`", path, erra, errr)
}

func (vm *ValidatorManager) IsValidFileNameWindows(path string) error {
	erra := vm.IsValidAbsoluteFileNameWindows(path)
	errr := vm.IsValidRelativeFileNameWindows(path)

	if erra == nil || errr == nil {
		return nil
	}

	return fmt.Errorf("%s is neither valid absolute nor relative path `%s` `%s`", path, erra, errr)
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

	return fmt.Errorf("%s is neither valid absolute nor relative mount path `%s` `%s`", path, erra, errr)
}

func (vm *ValidatorManager) IsValidMountPointWindows(path string) error {
	erra := vm.IsValidAbsoluteMountPointWindows(path)
	errr := vm.IsValidRelativeMountPointWindows(path)

	if erra == nil || errr == nil {
		return nil
	}

	return fmt.Errorf("%s is neither valid absolute nor relative mount path `%s` `%s`", path, erra, errr)
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

	return fmt.Errorf("%s is neither valid file name nor mount point `%s` `%s`", path, erra, errr)
}

func (vm *ValidatorManager) IsValidFileNameOrMountPointWindows(path string) error {
	erra := vm.IsValidFileNameWindows(path)
	errr := vm.IsValidMountPointWindows(path)

	if erra == nil || errr == nil {
		return nil
	}

	return fmt.Errorf("%s is neither valid file name nor mount point `%s` `%s`", path, erra, errr)
}

func (vm *ValidatorManager) IsValidPath(path string, fileName bool, absolute bool, targetOS string) error {
	if path == "" {
		return errors.New("path is empty")
	}
	if strings.ContainsRune(path, '\x00') {
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

	// Normalize path
	cleaned := path
	if osType == "windows" {
		cleaned = filepath.Clean(path)
	}

	// Absolute/relative path check
	switch osType {
	case "windows":
		isAbs := strings.HasPrefix(path, `\\`) || regexp.MustCompile(`^[a-zA-Z]:\\`).MatchString(path)
		if absolute && !isAbs {
			return errors.New("windows path must be absolute (e.g. C:\\path or \\\\server\\share)")
		}
		if !absolute && isAbs {
			return errors.New("windows path must not be absolute")
		}
	case "unix":
		isAbs := strings.HasPrefix(path, "/")
		if absolute && !isAbs {
			return errors.New("unix path must be absolute (start with '/')")
		}
		if !absolute && isAbs {
			return errors.New("unix path must not be absolute")
		}
	default:
		return fmt.Errorf("unsupported OS: %s", osType)
	}

	// Path traversal prevention
	parts := strings.Split(cleaned, string(filepath.Separator))
	for _, part := range parts {
		if part == ".." {
			return errors.New("path escapes working directory via '..'")
		}
	}

	// File name specific validation
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

		if osType == "windows" {
			// Reserved file names
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

			// Invalid characters in file name
			invalidChars := []rune{'<', '>', ':', '"', '/', '\\', '|', '?', '*'}
			for _, ch := range invalidChars {
				if strings.ContainsRune(base, ch) {
					return fmt.Errorf("windows file name contains invalid character: %q", ch)
				}
			}
		}
	}

	return nil
}
