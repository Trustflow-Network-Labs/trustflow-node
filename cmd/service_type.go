package cmd

import (
	"github.com/adgsm/trustflow-node/cmd/cmd_helpers"
	"github.com/spf13/cobra"
)

var serviceTypeName string
var addServiceTypeCmd = &cobra.Command{
	Use:     "add-service-type",
	Aliases: []string{"service-type-add"},
	Short:   "Add a service type",
	Long:    "Adding new service type will allow setting data/services pricing for that service type",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		cmd_helpers.AddServiceType(serviceTypeName)
	},
}

var removeServiceTypeCmd = &cobra.Command{
	Use:     "remove-service-type",
	Aliases: []string{"service-type-remove"},
	Short:   "Remove a service type",
	Long:    "Removing a service type will prevent setting data/services pricing for that service type. Service type can not be removed if there is an price set for that service type",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		cmd_helpers.RemoveServiceType(serviceTypeName)
	},
}

var setServiceTypeInactiveCmd = &cobra.Command{
	Use:     "set-service-type-inactive",
	Aliases: []string{"deactivate-service-type"},
	Short:   "Set service type active",
	Long:    "Setting service type to active will allow setting data/services pricing for that service type",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		cmd_helpers.SetServiceTypeInactive(serviceTypeName)
	},
}

var setServiceTypeActiveCmd = &cobra.Command{
	Use:     "set-service-type-active",
	Aliases: []string{"activate-service-type"},
	Short:   "Set service type active",
	Long:    "Setting service type to active will allow setting data/services pricing for that service type",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		cmd_helpers.SetServiceTypeActive(serviceTypeName)
	},
}

func init() {
	addServiceTypeCmd.Flags().StringVarP(&serviceTypeName, "type", "t", "", "Service type to be added")
	addServiceTypeCmd.MarkFlagRequired("type")
	rootCmd.AddCommand(addServiceTypeCmd)

	removeServiceTypeCmd.Flags().StringVarP(&serviceTypeName, "type", "t", "", "Service type to be removed")
	removeServiceTypeCmd.MarkFlagRequired("type")
	rootCmd.AddCommand(removeServiceTypeCmd)

	setServiceTypeInactiveCmd.Flags().StringVarP(&serviceTypeName, "type", "t", "", "Service type to be set inactive")
	setServiceTypeInactiveCmd.MarkFlagRequired("type")
	rootCmd.AddCommand(setServiceTypeInactiveCmd)

	setServiceTypeActiveCmd.Flags().StringVarP(&serviceTypeName, "type", "t", "", "Service type to be set active")
	setServiceTypeActiveCmd.MarkFlagRequired("type")
	rootCmd.AddCommand(setServiceTypeActiveCmd)
}
