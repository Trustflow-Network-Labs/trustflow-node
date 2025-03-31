package cmd

/*
var serviceName string
var serviceDescription string
var serviceNodeId string
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
		p2pm := node.NewP2PManager()
		serviceManager := node.NewServiceManager(p2pm)
		serviceManager.AddService(serviceName, serviceDescription, serviceNodeId, serviceType, servicePath, serviceRepo, serviceActive)
	},
}

var removeServiceCmd = &cobra.Command{
	Use:     "remove-service",
	Aliases: []string{"service-remove"},
	Short:   "Remove a service",
	Long:    "Removing a service will prevent setting data/services pricing and creating jobs for that service. Service can not be removed if there is an price set or jobs created for that service",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		p2pm := node.NewP2PManager()
		serviceManager := node.NewServiceManager(p2pm)
		serviceManager.RemoveService(serviceId)
	},
}

var setServiceInactiveCmd = &cobra.Command{
	Use:     "set-service-inactive",
	Aliases: []string{"deactivate-service"},
	Short:   "Set service active",
	Long:    "Setting service to inactive will prevent setting data/services pricing and creating jobs for that service",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		p2pm := node.NewP2PManager()
		serviceManager := node.NewServiceManager(p2pm)
		serviceManager.SetServiceInactive(serviceId)
	},
}

var setServiceActiveCmd = &cobra.Command{
	Use:     "set-service-active",
	Aliases: []string{"activate-service"},
	Short:   "Set service active",
	Long:    "Setting service to active will allow setting data/services pricing and creating jobs for that service",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		p2pm := node.NewP2PManager()
		serviceManager := node.NewServiceManager(p2pm)
		serviceManager.SetServiceActive(serviceId)
	},
}

var searchServicesCmd = &cobra.Command{
	Use:     "list-services",
	Aliases: []string{"search-local-services"},
	Short:   "Search / list local services",
	Long:    "Search and filter local services",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		// Read configs
		configManager := utils.NewConfigManager("")
		config, err := configManager.ReadConfigs()
		logsManager := utils.NewLogsManager()
		if err != nil {
			message := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
			logsManager.Log("error", message, "p2p")
			panic(err)
		}

		var searchService node_types.SearchService
		searchService.Name = serviceName
		searchService.Description = serviceDescription
		searchService.NodeId = serviceNodeId
		searchService.Type = serviceType
		searchService.Repo = serviceRepo
		searchService.Active = serviceActive
		var offset uint32 = 0
		var limit uint32 = 10
		l := config["search_results"]
		l64, err := strconv.ParseUint(l, 10, 32)
		if err != nil {
			limit = 10
		} else {
			limit = uint32(l64)
		}
		p2pm := node.NewP2PManager()
		serviceManager := node.NewServiceManager(p2pm)

		for {
			services, err := serviceManager.SearchServices(searchService, offset, limit)
			if err != nil {
				panic(err)
			}

			if len(services) == 0 {
				break
			}

			offset += uint32(len(services))

			// TODO, make CLI output more readable
			for _, service := range services {
				fmt.Printf("Name: %s\n", service.Name)
				fmt.Printf("Description: %s\n", service.Description)
				fmt.Printf("Node: %s\n", service.NodeId)
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
		p2pm := node.NewP2PManager()
		serviceManager := node.NewServiceManager(p2pm)
		serviceManager.LookupRemoteService(serviceName, serviceDescription, serviceNodeId, serviceType, serviceRepo)
	},
}

func init() {
	addServiceCmd.Flags().StringVarP(&serviceName, "name", "s", "", "Service name to be added")
	addServiceCmd.MarkFlagRequired("name")
	addServiceCmd.Flags().StringVarP(&serviceDescription, "description", "d", "", "Service description to be added")
	addServiceCmd.Flags().StringVarP(&serviceNodeId, "node", "n", "", "Service node ID")
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
	searchServicesCmd.Flags().StringVarP(&serviceDescription, "description", "d", "", "Service description to lookup for (any word/sentence match, comma delimited)")
	searchServicesCmd.Flags().StringVarP(&serviceNodeId, "node", "i", "", "Service node identity ID to lookup for (any node identity ID match, comma delimited)")
	searchServicesCmd.Flags().StringVarP(&serviceType, "type", "t", "", "Service type to be lookup for (any listed type match /DATA, DOCKER EXECUTION ENVIRONMENT, WASM EXECUTION ENVIRONMENT/, comma delimited)")
	searchServicesCmd.Flags().StringVarP(&serviceRepo, "repo", "r", "", "Service repo (git repo) to lookup for (any repo address match, comma delimited)")
	searchServicesCmd.Flags().BoolVarP(&serviceActive, "active", "a", true, "Search for active or inactive services")
	rootCmd.AddCommand(searchServicesCmd)

	serviceLookupCmd.Flags().StringVarP(&serviceName, "name", "s", "", "Service name to lookup for (any word/sentence match, comma delimited)")
	serviceLookupCmd.MarkFlagRequired("name")
	serviceLookupCmd.Flags().StringVarP(&serviceDescription, "description", "d", "", "Service description to lookup for (any word/sentence match, comma delimited)")
	serviceLookupCmd.Flags().StringVarP(&serviceNodeId, "node", "n", "", "Service node identity ID to lookup for (any node identity ID match, comma delimited)")
	serviceLookupCmd.Flags().StringVarP(&serviceType, "type", "t", "", "Service type to be lookup for (any listed type match /DATA, DOCKER EXECUTION ENVIRONMENT, WASM EXECUTION ENVIRONMENT/, comma delimited)")
	serviceLookupCmd.Flags().StringVarP(&serviceRepo, "repo", "r", "", "Service repo (git repo) to lookup for (any repo address match, comma delimited)")
	rootCmd.AddCommand(serviceLookupCmd)
}
*/
