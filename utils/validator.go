package utils

import (
	"fmt"
	"strconv"

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
