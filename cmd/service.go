package cmd

import (
	"fmt"

	"github.com/adgsm/trustflow-node/cmd/shared"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/spf13/cobra"
)

var serviceName string
var serviceDescription string
var serviceNodeId int32
var serviceNodeIdentityId string
var serviceType string
var servicePath string
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
		shared.AddService(serviceName, serviceDescription, serviceNodeId, serviceType, servicePath, serviceRepo, serviceActive)
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

var searchServicesCmd = &cobra.Command{
	Use:     "list-services",
	Aliases: []string{"search-local-services"},
	Short:   "Search / list local services",
	Long:    "Search and filter local services",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		var searchService node_types.SearchService
		searchService.Name = serviceName
		searchService.Description = serviceDescription
		searchService.NodeId = serviceNodeIdentityId
		searchService.Type = serviceType
		searchService.Repo = serviceRepo
		searchService.Active = serviceActive
		var offset uint32 = 0
		var limit uint32 = 1
		for {
			services, err := shared.SearchServices(searchService, offset, limit)
			if err != nil {
				panic(err)
			}
			fmt.Printf("Offset: %d, Limit: %d\n", offset, limit)
			if len(services) == 0 {
				break
			}

			offset += uint32(len(services))

			// TODO, make CLI output more readable
			for _, service := range services {
				fmt.Printf("Name: %s\n", service.Name.String)
				fmt.Printf("Description: %s\n", service.Description.String)
				fmt.Printf("Node: %s\n", service.NodeId.String)
			}
		}
	},
}

var serviceLookupCmd = &cobra.Command{
	Use:     "service-lookup",
	Aliases: []string{"search-remote-services"},
	Short:   "Send a search query looking for a remote service",
	Long:    "Service lookup will broadcast a search query for a remote service",
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
	addServiceCmd.Flags().StringVarP(&servicePath, "path", "p", "", "Service path")
	addServiceCmd.MarkFlagRequired("path")
	addServiceCmd.Flags().StringVarP(&serviceRepo, "repo", "r", "", "Service repo")
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

	searchServicesCmd.Flags().StringVarP(&serviceName, "name", "n", "", "Service name to lookup for (any word/sentence match, comma delimited)")
	searchServicesCmd.MarkFlagRequired("name")
	searchServicesCmd.Flags().StringVarP(&serviceDescription, "description", "d", "", "Service description to lookup for (any word/sentence match, comma delimited)")
	searchServicesCmd.Flags().StringVarP(&serviceNodeIdentityId, "node", "i", "", "Service node identity ID to lookup for (any node identity ID match, comma delimited)")
	searchServicesCmd.Flags().StringVarP(&serviceType, "type", "t", "", "Service type to be lookup for (any listed type match /DATA, DOCKER EXECUTION ENVIRONMENT, WASM EXECUTION ENVIRONMENT/, comma delimited)")
	searchServicesCmd.Flags().StringVarP(&serviceRepo, "repo", "r", "", "Service repo (git repo) to lookup for (any repo address match, comma delimited)")
	searchServicesCmd.Flags().BoolVarP(&serviceActive, "active", "a", true, "Search for active or inactive services")
	rootCmd.AddCommand(searchServicesCmd)

	serviceLookupCmd.Flags().StringVarP(&serviceName, "name", "n", "", "Service name to lookup for (any word/sentence match, comma delimited)")
	serviceLookupCmd.MarkFlagRequired("name")
	serviceLookupCmd.Flags().StringVarP(&serviceDescription, "description", "d", "", "Service description to lookup for (any word/sentence match, comma delimited)")
	serviceLookupCmd.Flags().StringVarP(&serviceNodeIdentityId, "node", "i", "", "Service node identity ID to lookup for (any node identity ID match, comma delimited)")
	serviceLookupCmd.Flags().StringVarP(&serviceType, "type", "t", "", "Service type to be lookup for (any listed type match /DATA, DOCKER EXECUTION ENVIRONMENT, WASM EXECUTION ENVIRONMENT/, comma delimited)")
	serviceLookupCmd.Flags().StringVarP(&serviceRepo, "repo", "r", "", "Service repo (git repo) to lookup for (any repo address match, comma delimited)")
	rootCmd.AddCommand(serviceLookupCmd)
}
