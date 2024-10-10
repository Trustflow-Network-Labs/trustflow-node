package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "trustflow",
	Short: "trustflow is a cli tool for p2p doker orchestration",
	Long:  "trustflow is a cli tool for p2p doker orchestration - decentralized data & resource service provider.",
	Run: func(cmd *cobra.Command, args []string) {

	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Oops. An error while executing trustflow '%s'\n", err)
		os.Exit(1)
	}
}
