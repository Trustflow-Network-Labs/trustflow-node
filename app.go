package main

import (
	"context"
	"fmt"

	"github.com/adgsm/trustflow-node-gui-client/internal/node"
	"github.com/adgsm/trustflow-node-gui-client/internal/utils"
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
	p2pm := node.NewP2PManager()
	a.p2pm = *p2pm
}

// Start P2P node
func (a *App) StartNode(port uint16) {
	a.p2pm.Start(port, true)
}

// Stop P2P node
func (a *App) StopNode(pid int) {
	if pid == 0 {
		// Read configs
		configManager := utils.NewConfigManager("")
		config, err := configManager.ReadConfigs()
		if err != nil {
			message := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
			fmt.Println(message)
			return
		}
		// PID file path
		pidPath := config["pid_path"]

		// Create PID Manager instance
		pm, err := utils.NewPIDManager(pidPath)
		if err != nil {
			msg := fmt.Sprintf("Error creating PID manager: %v\n", err)
			fmt.Println(msg)
			return
		}
		if pid, err = pm.ReadPID(); err != nil {
			msg := fmt.Sprintf("Error reading PID: %v\n", err)
			fmt.Println(msg)
			return
		}
	}

	err := a.p2pm.Stop(pid)
	if err != nil {
		msg := fmt.Sprintf("Error stopping node: %v\n", err)
		fmt.Println(msg)
		return
	}
}
