package cmd

import (
	"github.com/adgsm/trustflow-node/cmd/shared"
	"github.com/spf13/cobra"
)

var serviceName string
var serviceDescription string
var serviceNodeId int32
var serviceNodeIdentityId string
var serviceType string
var serviceRepo string
var serviceActive bool
var serviceId int32

var addServiceCmd = &cobra.Command{
	Use:     "add-service",
	Aliases: []string{"service-add"},
	Short:   "Add a service",
	Long:    "Adding new service will allow setting data/services pricing and creating jobs for that service",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		shared.AddService(serviceName, serviceDescription, serviceNodeId, serviceType, serviceActive)
	},
}

var removeServiceCmd = &cobra.Command{
	Use:     "remove-service",
	Aliases: []string{"service-remove"},
	Short:   "Remove a service",
	Long:    "Removing a service will prevent setting data/services pricing and creating jobs for that service. Service can not be removed if there is an price set or jobs created for that service",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		shared.RemoveService(serviceId)
	},
}

var setServiceInactiveCmd = &cobra.Command{
	Use:     "set-service-inactive",
	Aliases: []string{"deactivate-service"},
	Short:   "Set service active",
	Long:    "Setting service to inactive will prevent setting data/services pricing and creating jobs for that service",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		shared.SetServiceInactive(serviceId)
	},
}

var setServiceActiveCmd = &cobra.Command{
	Use:     "set-service-active",
	Aliases: []string{"activate-service"},
	Short:   "Set service active",
	Long:    "Setting service to active will allow setting data/services pricing and creating jobs for that service",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		shared.SetServiceActive(serviceId)
	},
}

var serviceLookupCmd = &cobra.Command{
	Use:     "service-lookup",
	Aliases: []string{"search-remote-service"},
	Short:   "Send a search query looking for a remote service",
	Long:    "Service looupp will broadcast a search query for a remote service",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		shared.LookupRemoteService(serviceName, serviceDescription, serviceNodeIdentityId, serviceType, serviceRepo)
	},
}

func init() {
	addServiceCmd.Flags().StringVarP(&serviceName, "name", "n", "", "Service name to be added")
	addServiceCmd.MarkFlagRequired("name")
	addServiceCmd.Flags().StringVarP(&serviceDescription, "description", "d", "", "Service description to be added")
	addServiceCmd.Flags().Int32VarP(&serviceNodeId, "node", "i", 0, "Service node ID")
	addServiceCmd.MarkFlagRequired("node")
	addServiceCmd.Flags().StringVarP(&serviceType, "type", "t", "", "Service type")
	addServiceCmd.MarkFlagRequired("type")
	addServiceCmd.Flags().BoolVarP(&serviceActive, "active", "a", true, "Is service active?")
	rootCmd.AddCommand(addServiceCmd)

	removeServiceCmd.Flags().Int32VarP(&serviceId, "id", "i", 0, "Service id to be removed")
	removeServiceCmd.MarkFlagRequired("id")
	rootCmd.AddCommand(removeServiceCmd)

	setServiceInactiveCmd.Flags().Int32VarP(&serviceId, "id", "i", 0, "Service id to be set inactive")
	setServiceInactiveCmd.MarkFlagRequired("id")
	rootCmd.AddCommand(setServiceInactiveCmd)

	setServiceActiveCmd.Flags().Int32VarP(&serviceId, "id", "i", 0, "Service id to be set active")
	setServiceActiveCmd.MarkFlagRequired("id")
	rootCmd.AddCommand(setServiceActiveCmd)

	serviceLookupCmd.Flags().StringVarP(&serviceName, "name", "n", "", "Service name to lookup for (any word/sentence match, comma delimited)")
	serviceLookupCmd.MarkFlagRequired("name")
	serviceLookupCmd.Flags().StringVarP(&serviceDescription, "description", "d", "", "Service description to lookup for (any word/sentence match, comma delimited)")
	serviceLookupCmd.Flags().StringVarP(&serviceNodeIdentityId, "node", "i", "", "Service node identity ID to lookup for (any node identity ID match, comma delimited)")
	serviceLookupCmd.Flags().StringVarP(&serviceType, "type", "t", "", "Service type to be lookup for (any listed type match /DATA, DOCKER EXECUTION ENVIRONMENT, WASM EXECUTION ENVIRONMENT/, comma delimited)")
	serviceLookupCmd.Flags().StringVarP(&serviceRepo, "repo", "r", "", "Service repo (git repo) to lookup for (any repo address match, comma delimited)")
	rootCmd.AddCommand(serviceLookupCmd)
}
