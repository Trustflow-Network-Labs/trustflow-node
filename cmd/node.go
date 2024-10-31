package cmd

import (
	"fmt"

	"github.com/adgsm/trustflow-node/p2p"
	"github.com/spf13/cobra"
)

var port uint16
var nodeCmd = &cobra.Command{
	Use:     "node",
	Aliases: []string{"serve"},
	Short:   "Start a p2p node",
	Long:    "Start running a p2p node in trustflow network",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		if running, _ := p2p.IsHostRunning(); !running {
			p2p.Start(port)
			fmt.Printf("Started running node on port %d\n", port)
		}
	},
}

var stopNodeCmd = &cobra.Command{
	Use:     "stop-node",
	Aliases: []string{"stop-serving"},
	Short:   "Stops running p2p node",
	Long:    "Stops running p2p node in trustflow network",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		if running, _ := p2p.IsHostRunning(); running {
			err := p2p.Stop()
			if err != nil {
				fmt.Printf("Error %s occured whilst trying to stop running node\n", err.Error())
			}
			fmt.Println("Stopped running node")
		} else {
			fmt.Println("Node is not running")
		}
	},
}

func init() {
	nodeCmd.Flags().Uint16VarP(&port, "port", "p", 30609, "Serve node on specified port [1024-65535]")
	rootCmd.AddCommand(nodeCmd)

	rootCmd.AddCommand(stopNodeCmd)
}
