package cmd

import (
	"fmt"

	settings "github.com/adgsm/trustflow-node/settings"
	"github.com/spf13/cobra"
)

var key string
var value string
var readSettingCmd = &cobra.Command{
	Use:     "read-setting",
	Aliases: []string{"read-option"},
	Short:   "Read a setting",
	Long:    "Read a setting using a unique setting key (e.g. accept_service_catalogue, accept_binary_stream, accept_file, etc)",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		settingsManager := settings.NewSettingsManager()
		s, err := settingsManager.Read(key)
		if err != nil {
			fmt.Printf("Error occured while reading setting for a key %s. (%s)\n", key, err.Error())
			return
		}
		// TODO, Make CLI output readable and nice looking
		fmt.Printf("Setting value for a key %s is %v\n", key, s)
	},
}

var modifySettingCmd = &cobra.Command{
	Use:     "modify-setting",
	Aliases: []string{"configure-option"},
	Short:   "Modify a setting",
	Long:    "Modify a setting using a unique setting key (e.g. accept_service_catalogue, accept_binary_stream, accept_file, etc)",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		settingsManager := settings.NewSettingsManager()
		settingsManager.Modify(key, value)
	},
}

func init() {
	readSettingCmd.Flags().StringVarP(&key, "key", "k", "", "Unique setting key")
	readSettingCmd.MarkFlagRequired("key")
	rootCmd.AddCommand(readSettingCmd)
	modifySettingCmd.Flags().StringVarP(&key, "key", "k", "", "Unique setting key")
	modifySettingCmd.MarkFlagRequired("key")
	modifySettingCmd.Flags().StringVarP(&value, "value", "v", "", "Key value to be set")
	rootCmd.AddCommand(modifySettingCmd)
}
