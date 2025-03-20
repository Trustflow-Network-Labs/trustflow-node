package cmd

import (
	"github.com/adgsm/trustflow-node/cmd/shared"
	"github.com/spf13/cobra"
)

var jobOrderingNodeId string
var jobServiceId int32
var jobStatus string
var jobStarted string
var jobEnded string
var jobId int32
var createJobCmd = &cobra.Command{
	Use:     "create-job",
	Aliases: []string{"add-job"},
	Short:   "Create new job",
	Long:    "Create a new job will add the job to the queue to be executed on a hosting machine",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		p2pm := shared.NewP2PManager()
		jobManager := shared.NewJobManager(p2pm)
		jobManager.CreateJob(jobOrderingNodeId, jobServiceId)
	},
}

var runJobCmd = &cobra.Command{
	Use:     "run-job",
	Aliases: []string{"start-job"},
	Short:   "Run job",
	Long:    "Run a job from a queue will start executing the job on a hosting machine",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		p2pm := shared.NewP2PManager()
		jobManager := shared.NewJobManager(p2pm)
		jobManager.RunJob(jobId)
	},
}

func init() {
	createJobCmd.Flags().StringVarP(&jobOrderingNodeId, "ordering-node", "n", "", "Ordering node ID")
	createJobCmd.MarkFlagRequired("ordering-node")
	createJobCmd.Flags().Int32VarP(&jobServiceId, "service", "s", 0, "Service ID")
	createJobCmd.MarkFlagRequired("service")
	rootCmd.AddCommand(createJobCmd)

	runJobCmd.Flags().Int32VarP(&jobId, "id", "i", 0, "Job ID")
	runJobCmd.MarkFlagRequired("id")
	rootCmd.AddCommand(runJobCmd)
}
