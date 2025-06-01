package main

import (
	"context"
	"fmt"

	"github.com/adgsm/trustflow-node/internal/dependencies"
	"github.com/adgsm/trustflow-node/internal/node"
	"github.com/adgsm/trustflow-node/internal/ui"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx  context.Context
	p2pm node.P2PManager
	dm   dependencies.DependencyManager
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	p2pm := node.NewP2PManager(ctx)
	a.p2pm = *p2pm
	a.dm = *dependencies.NewDependencyManager(ui.GUI{
		PrintFunc: func(msg string) {
			runtime.EventsEmit(a.ctx, "syslog-event", msg)
		},
		ConfirmFunc: func(question string) bool {
			// TODO
			//			return gui.ShowConfirmationDialog(question)
			return true
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
	})
	a.CheckAndInstallDependencies()
}

// Check and install node dependencies
func (a *App) CheckAndInstallDependencies() {
	a.dm.CheckAndInstallDependencies()
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
