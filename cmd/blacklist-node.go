package cmd

import (
	blacklist_node "github.com/adgsm/trustflow-node/cmd/blacklist-node"
	"github.com/spf13/cobra"
)

var nodeId string
var reason string
var blackListNodeCmd = &cobra.Command{
	Use:     "add-node-to-blacklist",
	Aliases: []string{"blacklist-node"},
	Short:   "Blacklist a node",
	Long:    "Blacklisting a node prevent any communication with that node",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		blacklist_node.BlacklistNode(nodeId, reason)
	},
}

var removeNodeFromBlacklistCmd = &cobra.Command{
	Use:     "remove-node-from-blacklist",
	Aliases: []string{"unblacklist-node"},
	Short:   "Remove a node from blacklist",
	Long:    "Removing a node from blacklist makes communication with that node possible again",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		blacklist_node.RemoveNodeFromBlacklist(nodeId)
	},
}

func init() {
	blackListNodeCmd.Flags().StringVarP(&nodeId, "nodeid", "i", "", "Node ID to be put on a blacklist")
	blackListNodeCmd.MarkFlagRequired("nodeid")
	blackListNodeCmd.Flags().StringVarP(&reason, "reason", "r", "", "Reason for blacklisting node")
	rootCmd.AddCommand(blackListNodeCmd)
	removeNodeFromBlacklistCmd.Flags().StringVarP(&nodeId, "nodeid", "i", "", "Node ID to be removed from blacklist")
	removeNodeFromBlacklistCmd.MarkFlagRequired("nodeid")
	rootCmd.AddCommand(removeNodeFromBlacklistCmd)
}
