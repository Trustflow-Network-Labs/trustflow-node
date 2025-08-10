//go:build darwin || linux

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/adgsm/trustflow-node/internal/dependencies"
	"github.com/adgsm/trustflow-node/internal/node"
	"github.com/adgsm/trustflow-node/internal/ui"
	"github.com/adgsm/trustflow-node/internal/utils"
	"github.com/spf13/cobra"
)

var port uint16
var daemon bool = false
var relay bool = false
var pid int
var nodeCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"interactive"},
	Short:   "Start a p2p node",
	Long:    "Start running a p2p node in trustflow network",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		var public bool = false
		// Configs manager
		cm := utils.NewConfigManager("")

		// Dependencies manager
		dm := dependencies.NewDependencyManager(ui.CLI{}, cm)
		dm.CheckAndInstallDependencies()
		fmt.Println("\n🚀 Dependencies checked. Continuing to start the app...")

		// Determine node type (if node has public IP or not)
		ntm := utils.NewNodeTypeManager()
		nodeType, err := ntm.GetNodeTypeConfig([]uint16{port})
		if err != nil {
			fmt.Printf("⚠️ Can not determine node type:\n%v\n", err)
		} else {
			fmt.Printf("Node type: %s\n", nodeType.Type)
			fmt.Printf("Local IP: %s\n", nodeType.LocalIP)
			fmt.Printf("External IP: %s\n", nodeType.ExternalIP)
			for port, open := range nodeType.Connectivity {
				if !open {
					fmt.Printf("❌ Port %d is not open\n", port)
				} else {
					fmt.Printf("✅ Port %d is open\n", port)
				}
			}

			public = nodeType.Type == "public"
		}

		if !public && relay {
			fmt.Println("⚠️ Private node behind NAT should not be used as a relay.")
			relay = false
		}

		// Logs manager
		logsManager := utils.NewLogsManager(cm)
		defer logsManager.Close()

		// P2P Manager
		ctx := cmd.Context()
		p2pManager := node.NewP2PManager(ctx, ui.CLI{}, cm)
		defer p2pManager.Close()
		err = p2pManager.Start(port, daemon, public, relay)
		if err != nil {
			fmt.Printf("⚠️ Can not start p2p node:\n%v\n", err)
		}
		if !daemon {
			// Print interactive menu
			menuManager := node.NewMenuManager(p2pManager)
			menuManager.Run()
		} else {
			// Add signal handling
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

			select {
			case <-ctx.Done():
			case <-sigChan:
				logsManager.Log("info", "Received shutdown signal", "p2p")
			}
		}

		err = p2pManager.Stop()
		if err != nil {
			fmt.Printf("⚠️ Can not stop the node:\n%v\n", err)
		}
	},
}

var nodeDaemonCmd = &cobra.Command{
	Use:     "start-daemon",
	Aliases: []string{"daemon"},
	Short:   "Start a p2p node as a daemon",
	Long:    "Start running a p2p node as a daemon in trustflow network",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		// Configs manager
		cm := utils.NewConfigManager("")

		// Logs manager
		logsManager := utils.NewLogsManager(cm)
		defer logsManager.Close()

		// Start the process in background
		rel := fmt.Sprintf("-r=%t", relay)
		command := exec.Command(os.Args[0], "start", "-d=true", rel)
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		command.Stdin = nil

		command.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true,
			Pgid:    0,
		}

		err := command.Start()
		if err != nil {
			msg := fmt.Sprintf("Error starting daemon: %s", err.Error())
			fmt.Println(msg)
			logsManager.Log("error", msg, "node")
			return
		}
		pid = command.Process.Pid
		msg := fmt.Sprintf("Daemon started with PID: %d, command: %v", pid, command.Args)
		fmt.Println(msg)
		logsManager.Log("info", msg, "node")

		err = command.Process.Release()
		if err != nil {
			msg := fmt.Sprintf("Error occured whilst releasing a daemon process: %s", err.Error())
			fmt.Println(msg)
			logsManager.Log("error", msg, "node")
			return
		}

		// Create PID Manager instance
		pm, err := utils.NewPIDManager(cm)
		if err != nil {
			msg := fmt.Sprintf("Error creating PID manager: %v\n", err)
			fmt.Println(msg)
			logsManager.Log("error", msg, "node")
			return
		}

		// Write our PID
		if err := pm.WritePID(pid); err != nil {
			msg := fmt.Sprintf("Error writting PID: %v\n", err)
			fmt.Println(msg)
			logsManager.Log("error", msg, "node")
			return
		}

		os.Exit(0) // Exit the parent process
	},
}

var stopNodeCmd = &cobra.Command{
	Use:     "stop-node",
	Aliases: []string{"stop"},
	Short:   "Stops running p2p node",
	Long:    "Stops running p2p node in trustflow network",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		// Configs manager
		cm := utils.NewConfigManager("")

		// Logs manager
		logsManager := utils.NewLogsManager(cm)
		defer logsManager.Close()

		// Create PID Manager instance
		pm, err := utils.NewPIDManager(cm)
		if err != nil {
			msg := fmt.Sprintf("Error creating PID manager: %v\n", err)
			fmt.Println(msg)
			logsManager.Log("error", msg, "node")
			return
		}

		if pid == 0 {
			if pid, err = pm.ReadPID(); err != nil {
				msg := fmt.Sprintf("Error reading PID: %v\n", err)
				fmt.Println(msg)
				logsManager.Log("error", msg, "node")
				return
			}
		}

		err = pm.StopProcess(pid)
		if err != nil {
			msg := fmt.Sprintf("Error %s occured whilst trying to stop running node\n", err.Error())
			fmt.Println(msg)
			logsManager.Log("error", msg, "node")
		}
		msg := "Node stopped"
		fmt.Println(msg)
		logsManager.Log("info", msg, "node")
	},
}

func init() {
	nodeCmd.Flags().Uint16VarP(&port, "port", "p", 30609, "Serve node on specified port [1024-65535]")
	nodeCmd.Flags().BoolVarP(&daemon, "daemon", "d", false, "Serve node as daemon")
	nodeCmd.Flags().BoolVarP(&relay, "relay", "r", false, "Serve as relay node")
	rootCmd.AddCommand(nodeCmd)

	nodeDaemonCmd.Flags().Uint16VarP(&port, "port", "p", 30609, "Serve node on specified port [1024-65535]")
	nodeDaemonCmd.Flags().BoolVarP(&relay, "relay", "r", false, "Serve as relay node")
	rootCmd.AddCommand(nodeDaemonCmd)

	stopNodeCmd.Flags().IntVarP(&pid, "pid", "i", 0, "Stop node running as provided process Id")
	rootCmd.AddCommand(stopNodeCmd)
}
