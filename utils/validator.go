package utils

import (
	"fmt"

	"github.com/libp2p/go-libp2p/core/peer"
)

type ValidatorManager struct {
}

func NewValidatorManager() *ValidatorManager {
	return &ValidatorManager{}
}

func (tm *ValidatorManager) NotEmpty(s string) error {
	if s == "" {
		return fmt.Errorf("expected non empty input string")
	}
	return nil
}

func (tm *ValidatorManager) IsPeer(s string) error {
	_, err := peer.Decode(s)
	if err != nil {
		return err
	}
	return nil
}
