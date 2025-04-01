package utils

import (
	"fmt"
	"strconv"

	"github.com/libp2p/go-libp2p/core/peer"
)

type ValidatorManager struct {
}

func NewValidatorManager() *ValidatorManager {
	return &ValidatorManager{}
}

func (vm *ValidatorManager) NotEmpty(s string) error {
	if s == "" {
		return fmt.Errorf("expected non empty input string")
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
