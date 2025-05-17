package utils

import (
	"errors"
	"fmt"
	"path/filepath"
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

func (vm *ValidatorManager) IsValidFileName(path string) error {
	erra := vm.IsValidAbsoluteFileName(path)
	errr := vm.IsValidRelativeFileName(path)

	if erra == nil || errr == nil {
		return nil
	}

	return fmt.Errorf("%s is neither valid absolute nor relative path", path)
}

func (vm *ValidatorManager) IsValidAbsoluteFileName(path string) error {
	return vm.IsValidPath(path, true, true)
}

func (vm *ValidatorManager) IsValidRelativeFileName(path string) error {
	return vm.IsValidPath(path, true, false)
}

func (vm *ValidatorManager) IsValidMountPoint(path string) error {
	erra := vm.IsValidAbsoluteMountPoint(path)
	errr := vm.IsValidRelativeMountPoint(path)

	if erra == nil || errr == nil {
		return nil
	}

	return fmt.Errorf("%s is neither valid absolute nor relative mount path", path)
}

func (vm *ValidatorManager) IsValidAbsoluteMountPoint(path string) error {
	return vm.IsValidPath(path, false, true)
}

func (vm *ValidatorManager) IsValidRelativeMountPoint(path string) error {
	return vm.IsValidPath(path, false, false)
}

func (vm *ValidatorManager) IsValidFileNameOrMountPoint(path string) error {
	erra := vm.IsValidFileName(path)
	errr := vm.IsValidMountPoint(path)

	if erra == nil || errr == nil {
		return nil
	}

	return fmt.Errorf("%s is neither valid file name nor mount point", path)
}

func (vm *ValidatorManager) IsValidPath(path string, fileName bool, absolute bool) error {
	if path == "" {
		return errors.New("path is empty")
	}

	if strings.ContainsRune(path, 0) {
		return errors.New("path contains null byte")
	}

	if strings.HasPrefix(path, "~") {
		return errors.New("path uses home directory reference '~'")
	}

	if absolute && !filepath.IsAbs(path) {
		return errors.New("path must be absolute")
	} else if !absolute && filepath.IsAbs(path) {
		return errors.New("path must not be absolute")
	}

	cleaned := filepath.Clean(path)

	if strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, "/..") {
		return errors.New("path escapes working directory")
	}

	if runtime.GOOS == "windows" {
		invalidChars := `<>:"/\|?*`
		for _, ch := range invalidChars {
			if strings.ContainsRune(path, ch) {
				return errors.New("path contains invalid character: " + string(ch))
			}
		}
	}

	// 🔍 If fileName is true, ensure the path ends in a filename (not directory)
	if fileName {
		base := filepath.Base(cleaned)
		if base == "." || base == "" {
			return errors.New("path does not contain a file name")
		}

		if runtime.GOOS == "windows" {
			reserved := map[string]bool{
				"CON": true, "PRN": true, "AUX": true, "NUL": true,
				"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
				"COM6": true, "COM7": true, "COM8": true, "COM9": true,
				"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
				"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
			}
			if reserved[strings.ToUpper(base)] {
				return errors.New("file name is a reserved word on Windows")
			}
		}
	}

	return nil
}
