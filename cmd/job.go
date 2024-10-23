package cmd

import (
	"github.com/adgsm/trustflow-node/cmd/cmd_helpers"
	"github.com/spf13/cobra"
)

var jobOrderingNodeId int32
var jobServiceId int32
var jobStatus string
var jobStarted string
var jobEnded string
var createJobCmd = &cobra.Command{
	Use:     "create-job",
	Aliases: []string{"add-job"},
	Short:   "Create new job",
	Long:    "Create a new job to be executed on a hosting machine",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		cmd_helpers.CreateJob(jobOrderingNodeId, jobServiceId)
	},
}

func init() {
	createJobCmd.Flags().Int32VarP(&jobOrderingNodeId, "ordering-node", "n", 0, "Ordering node ID")
	createJobCmd.MarkFlagRequired("ordering-node")
	createJobCmd.Flags().Int32VarP(&jobServiceId, "service", "s", 0, "Service ID")
	createJobCmd.MarkFlagRequired("service")
	rootCmd.AddCommand(createJobCmd)
}
