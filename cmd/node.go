package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/adgsm/trustflow-node/cmd/shared"
	"github.com/adgsm/trustflow-node/utils"
	"github.com/spf13/cobra"
)

var port uint16
var daemon bool = false
var pid int
var nodeCmd = &cobra.Command{
	Use:     "node",
	Aliases: []string{"interactive"},
	Short:   "Start a p2p node",
	Long:    "Start running a p2p node in trustflow network",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		shared.Start(port, daemon)
	},
}

var nodeDaemonCmd = &cobra.Command{
	Use:     "node-daemon",
	Aliases: []string{"daemon"},
	Short:   "Start a p2p node as a daemon",
	Long:    "Start running a p2p node as a daemon in trustflow network",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		// Start the process in background
		command := exec.Command(os.Args[0], "node", "-d=true")
		command.Stdout = nil
		command.Stderr = nil
		command.Stdin = nil
		command.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true,
			Pgid:    0,
		}

		err := command.Start()
		if err != nil {
			msg := fmt.Sprintf("Error starting daemon: %s", err.Error())
			fmt.Println(msg)
			utils.Log("error", msg, "node")
			return
		}
		msg := fmt.Sprintf("Daemon started with PID: %d, command: %v", command.Process.Pid, command.Args)
		fmt.Println(msg)
		utils.Log("info", msg, "node")

		err = command.Process.Release()
		if err != nil {
			msg := fmt.Sprintf("Error occured whilst releasing a daemon process: %s", err.Error())
			fmt.Println(msg)
			utils.Log("error", msg, "node")
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
		err := shared.Stop(pid)
		if err != nil {
			msg := fmt.Sprintf("Error %s occured whilst trying to stop running node\n", err.Error())
			fmt.Println(msg)
			utils.Log("error", msg, "node")
		}
		msg := "Node stopped"
		fmt.Println(msg)
		utils.Log("info", msg, "node")
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
