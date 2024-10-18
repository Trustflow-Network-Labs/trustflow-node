package cmd

import (
	"github.com/adgsm/trustflow-node/cmd/cmd_helpers"
	"github.com/spf13/cobra"
)

var name string
var addResourceCmd = &cobra.Command{
	Use:     "add-resource",
	Aliases: []string{"resource-add"},
	Short:   "Add a resource",
	Long:    "Adding new resource will allow setting data/services pricing for that resource",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		cmd_helpers.AddResource(name)
	},
}

var removeResourceCmd = &cobra.Command{
	Use:     "remove-resource",
	Aliases: []string{"resource-remove"},
	Short:   "Remove a resource",
	Long:    "Removing a resource will prevent setting data/services pricing for that resource. Resource can not be removed if there is an price set for that resource",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		cmd_helpers.RemoveResource(name)
	},
}

var setResourceInactiveCmd = &cobra.Command{
	Use:     "set-resource-inactive",
	Aliases: []string{"deactivate-resource"},
	Short:   "Set resource active",
	Long:    "Setting resource to active will allow setting data/services pricing for that resource",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		cmd_helpers.SetResourceInactive(name)
	},
}

var setResourceActiveCmd = &cobra.Command{
	Use:     "set-resource-active",
	Aliases: []string{"activate-resource"},
	Short:   "Set resource active",
	Long:    "Setting resource to active will allow setting data/services pricing for that resource",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		cmd_helpers.SetResourceActive(name)
	},
}

func init() {
	addResourceCmd.Flags().StringVarP(&name, "resource", "r", "", "Resource to be added")
	addResourceCmd.MarkFlagRequired("resource")
	rootCmd.AddCommand(addResourceCmd)

	removeResourceCmd.Flags().StringVarP(&name, "resource", "r", "", "Resource to be removed")
	removeResourceCmd.MarkFlagRequired("resource")
	rootCmd.AddCommand(removeResourceCmd)

	setResourceInactiveCmd.Flags().StringVarP(&name, "resource", "r", "", "Resource to be set inactive")
	setResourceInactiveCmd.MarkFlagRequired("resource")
	rootCmd.AddCommand(setResourceInactiveCmd)

	setResourceActiveCmd.Flags().StringVarP(&name, "resource", "r", "", "Resource to be set active")
	setResourceActiveCmd.MarkFlagRequired("resource")
	rootCmd.AddCommand(setResourceActiveCmd)
}
