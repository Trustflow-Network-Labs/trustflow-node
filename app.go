package main

import (
	"context"
	"fmt"

	"github.com/adgsm/trustflow-node/internal/node"
)

// App struct
type App struct {
	ctx  context.Context
	p2pm node.P2PManager
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
func (a *App) StopNode() {
	err := a.p2pm.Stop()
	if err != nil {
		msg := fmt.Sprintf("Error stopping node: %v\n", err)
		fmt.Println(msg)
		return
	}
}
