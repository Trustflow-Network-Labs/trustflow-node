package main

import (
	"context"
	"fmt"
	"time"

	"github.com/adgsm/trustflow-node/internal/dependencies"
	"github.com/adgsm/trustflow-node/internal/node"
	"github.com/adgsm/trustflow-node/internal/node_types"
	"github.com/adgsm/trustflow-node/internal/ui"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx               context.Context
	p2pm              node.P2PManager
	dm                dependencies.DependencyManager
	sm                node.ServiceManager
	confirmFuncChan   chan bool
	frontendReadyChan chan struct{}
	gui               ui.UI
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.frontendReadyChan = make(chan struct{})
	a.gui = ui.GUI{
		PrintFunc: func(msg string) {
			runtime.EventsEmit(a.ctx, "syslog-event", msg)
		},
		ConfirmFunc: func(question string) bool {
			a.confirmFuncChan = make(chan bool)

			// Send the prompt to frontend
			runtime.EventsEmit(a.ctx, "sysconfirm-event", question)

			// Wait for frontend response (blocks until received)
			response := <-a.confirmFuncChan

			return response
		},
		ExitFunc: func(code int) {
			var msg = ""
			switch code {
			case 1:
				msg = "Cannot continue until dependencies are installed."
			default:
				msg = fmt.Sprintf("Unknown application exit code `%d`", code)
			}
			runtime.EventsEmit(a.ctx, "exitlog-event", msg)
		},
		ServiceOfferFunc: func(serviceOffer node_types.ServiceOffer) {
			runtime.EventsEmit(a.ctx, "serviceofferlog-event", serviceOffer)
		},
	}

	p2pm := node.NewP2PManager(ctx, a.gui)
	a.p2pm = *p2pm
	a.dm = *dependencies.NewDependencyManager(a.gui)
	a.sm = *node.NewServiceManager(p2pm)
	select {
	case <-a.frontendReadyChan:
		a.CheckAndInstallDependencies()
		runtime.EventsEmit(a.ctx, "dependenciesready-event", true)
	case <-time.After(10 * time.Second): // Optional timeout
		fmt.Println("Timeout waiting for frontend readiness")
	}
}

// Signal that frontend is ready
func (a *App) NotifyFrontendReady() {
	if a.frontendReadyChan != nil {
		close(a.frontendReadyChan)
		a.frontendReadyChan = nil // Avoid multiple closes
	}
}

// Check and install node dependencies
func (a *App) CheckAndInstallDependencies() {
	a.dm.CheckAndInstallDependencies()
}

// User confirm with the response
func (a *App) SetUserConfirmation(response bool) {
	if a.confirmFuncChan != nil {
		a.confirmFuncChan <- response
	}
}

// Is P2P host running
func (a *App) IsHostRunning() bool {
	return a.p2pm.IsHostRunning()
}

// Start P2P node
func (a *App) StartNode(port uint16) {
	a.p2pm.Start(port, true)
}

// Stop P2P node
func (a *App) StopNode() error {
	err := a.p2pm.Stop()
	if err != nil {
		return err
	}
	return nil
}

// Find services
func (a *App) FindServices(searchPhrases string, serviceTypes string) error {
	return a.sm.LookupRemoteService(searchPhrases, serviceTypes)
}
