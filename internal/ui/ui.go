package ui

import (
	"fmt"

	"github.com/adgsm/trustflow-node/internal/node_types"
)

type UI interface {
	Print(msg string)
	PromptConfirm(question string) bool
	Exit(code int)
	ServiceOffer(node_types.ServiceOffer)
}

func DetectUIType(u UI) (string, error) {
	switch u.(type) {
	case CLI:
		return "CLI", nil
	case GUI:
		return "GUI", nil
	default:
		err := fmt.Errorf("unknown UI type")
		return "", err
	}
}
