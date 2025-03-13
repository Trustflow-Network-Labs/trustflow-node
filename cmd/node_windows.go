//go:build windows

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/windows"

	"github.com/adgsm/trustflow-node/cmd/shared"
	"github.com/adgsm/trustflow-node/utils"
	"github.com/spf13/cobra"
)

var port uint16
var daemon bool = false
var pid int
var nodeCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"interactive"},
	Short:   "Start a p2p node",
	Long:    "Start running a p2p node in trustflow network",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		p2pManager := shared.NewP2PManager()
		p2pManager.Start(port, daemon)
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

		// Ensure the path is absolute
		exePath, err := filepath.Abs(os.Args[0])
		if err != nil {
			fmt.Println("Error getting absolute path:", err)
			return
		}

		// Start the process in background
		command := exec.Command(exePath, "start", "-d=true")
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		command.Stdin = nil

		command.SysProcAttr = &windows.SysProcAttr{
			CreationFlags: windows.DETACHED_PROCESS,
		}
		command.Stdout = nil
		command.Stderr = nil

		err = command.Start()
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

		// Read configs
		configManager := utils.NewConfigManager("")
		config, err := configManager.ReadConfigs()
		if err != nil {
			message := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
			logsManager.Log("error", message, "node")
			return
		}
		// PID file path
		pidPath := config["pid_path"]

		// Create PID Manager instance
		pm, err := shared.NewPIDManager(pidPath)
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

		if pid == 0 {
			// Read configs
			configManager := utils.NewConfigManager("")
			config, err := configManager.ReadConfigs()
			if err != nil {
				message := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
				logsManager.Log("error", message, "node")
				return
			}
			// PID file path
			pidPath := config["pid_path"]

			// Create PID Manager instance
			pm, err := shared.NewPIDManager(pidPath)
			if err != nil {
				msg := fmt.Sprintf("Error creating PID manager: %v\n", err)
				fmt.Println(msg)
				logsManager.Log("error", msg, "node")
				return
			}
			if pid, err = pm.ReadPID(); err != nil {
				msg := fmt.Sprintf("Error reading PID: %v\n", err)
				fmt.Println(msg)
				logsManager.Log("error", msg, "node")
				return
			}
		}

		p2pManager := shared.NewP2PManager()
		err := p2pManager.Stop(pid)
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
	rootCmd.AddCommand(nodeCmd)

	rootCmd.AddCommand(nodeDaemonCmd)

	stopNodeCmd.Flags().IntVarP(&pid, "pid", "i", 0, "Stop node running as provided process Id")
	rootCmd.AddCommand(stopNodeCmd)
}
