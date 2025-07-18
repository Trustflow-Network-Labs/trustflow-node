//go:build darwin || linux

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/adgsm/trustflow-node/internal/dependencies"
	"github.com/adgsm/trustflow-node/internal/node"
	"github.com/adgsm/trustflow-node/internal/ui"
	"github.com/adgsm/trustflow-node/internal/utils"
	"github.com/spf13/cobra"
)

var port uint16
var daemon bool = false
var public bool = false
var relay bool = false
var pid int
var nodeCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"interactive"},
	Short:   "Start a p2p node",
	Long:    "Start running a p2p node in trustflow network",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		dm := dependencies.NewDependencyManager(ui.CLI{})
		dm.CheckAndInstallDependencies()
		fmt.Println("\n🚀 Dependencies checked. Continuing to start the app...")
		/* TODO, this is just a test of node type functionality
		ntm := utils.NewNodeTypeManager()
		nodeType, err := ntm.GetNodeTypeConfig([]uint16{port})
		if err != nil {
			fmt.Printf("error:\n%v\n", err)
		}
		fmt.Printf("%v\n", nodeType)
		*/
		p2pManager := node.NewP2PManager(cmd.Context(), ui.CLI{})
		defer p2pManager.Close()
		p2pManager.Start(port, daemon, public, relay)
	},
}

var nodeDaemonCmd = &cobra.Command{
	Use:     "start-daemon",
	Aliases: []string{"daemon"},
	Short:   "Start a p2p node as a daemon",
	Long:    "Start running a p2p node as a daemon in trustflow network",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		logsManager := utils.NewLogsManager()
		defer logsManager.Close()

		// Start the process in background
		if !public {
			relay = false
		}
		if relay {
			public = true
		}
		pub := fmt.Sprintf("-b=%t", public)
		rel := fmt.Sprintf("-r=%t", relay)
		command := exec.Command(os.Args[0], "start", "-d=true", pub, rel)
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
		pm, err := utils.NewPIDManager()
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
		logsManager := utils.NewLogsManager()
		defer logsManager.Close()

		// Create PID Manager instance
		pm, err := utils.NewPIDManager()
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
	nodeCmd.Flags().BoolVarP(&public, "public", "b", false, "Public IP node")
	nodeCmd.Flags().BoolVarP(&relay, "relay", "r", false, "Serve as relay node")
	rootCmd.AddCommand(nodeCmd)

	nodeDaemonCmd.Flags().Uint16VarP(&port, "port", "p", 30609, "Serve node on specified port [1024-65535]")
	nodeDaemonCmd.Flags().BoolVarP(&public, "public", "b", false, "Public IP node")
	nodeDaemonCmd.Flags().BoolVarP(&relay, "relay", "r", false, "Serve as relay node")
	rootCmd.AddCommand(nodeDaemonCmd)

	stopNodeCmd.Flags().IntVarP(&pid, "pid", "i", 0, "Stop node running as provided process Id")
	rootCmd.AddCommand(stopNodeCmd)
}
