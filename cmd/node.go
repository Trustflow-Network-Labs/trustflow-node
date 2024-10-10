package cmd

import (
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
		p2p.Start(port)
	},
}

func init() {
	nodeCmd.Flags().Uint16VarP(&port, "port", "p", 30609, "Serve node on specified port [1024-65535]")
	rootCmd.AddCommand(nodeCmd)
}
