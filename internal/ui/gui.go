package ui

import (
	"github.com/adgsm/trustflow-node/internal/node_types"
)

type GUI struct {
	ConfirmFunc      func(question string) bool
	PrintFunc        func(msg string)
	ExitFunc         func(code int)
	ServiceOfferFunc func(node_types.ServiceOffer)
}

func (g GUI) Print(msg string) {
	if g.PrintFunc != nil {
		g.PrintFunc(msg)
	}
}

func (g GUI) PromptConfirm(question string) bool {
	if g.ConfirmFunc != nil {
		return g.ConfirmFunc(question)
	}
	return false
}

func (g GUI) Exit(code int) {
	if g.ExitFunc != nil {
		g.ExitFunc(code)
	}
}

func (g GUI) ServiceOffer(serviceOffer node_types.ServiceOffer) {
	if g.ServiceOfferFunc != nil {
		g.ServiceOfferFunc(serviceOffer)
	}
}
