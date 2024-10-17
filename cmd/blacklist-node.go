package cmd

import (
	"context"
	"fmt"

	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/utils"
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
		blacklistNode()
	},
}

var removeNodeFromBlacklistCmd = &cobra.Command{
	Use:     "remove-node-from-blacklist",
	Aliases: []string{"unblacklist-node"},
	Short:   "Remove a node from blacklist",
	Long:    "Removing a node from blacklist makes communication with that node possible again",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		removeNodeFromBlacklist()
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

func blacklistNode() {
	if nodeId == "" {
		msg := "Invalid Node ID"
		utils.Log("error", msg, "blacklist-node")
		return
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "blacklist-node")
		return
	}
	defer db.Close()

	// Check if node is already blacklisted
	var id utils.NullInt32
	row := db.QueryRowContext(context.Background(), "select id from blacklisted_nodes where node_id = ?;", nodeId)

	err = row.Scan(&id)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "blacklist-node")

		// Add node to blacklist
		utils.Log("debug", fmt.Sprintf("add node %s to blacklist", nodeId), "blacklist-node")

		_, err = db.ExecContext(context.Background(), "insert into blacklisted_nodes (node_id, reason) values (?, ?);",
			nodeId, reason)
		if err != nil {
			msg := err.Error()
			utils.Log("error", msg, "blacklist-node")
			return
		}
	}
}

func removeNodeFromBlacklist() {
	if nodeId == "" {
		msg := "Invalid Node ID"
		utils.Log("error", msg, "blacklist-node")
		return
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "blacklist-node")
		return
	}
	defer db.Close()

	// Check if node is already blacklisted
	var id utils.NullInt32
	row := db.QueryRowContext(context.Background(), "select id from blacklisted_nodes where node_id = ?;", nodeId)

	err = row.Scan(&id)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "blacklist-node")
		return
	}

	// Remove node from blacklist
	utils.Log("debug", fmt.Sprintf("removing node %s from blacklist", nodeId), "blacklist-node")

	_, err = db.ExecContext(context.Background(), "delete from blacklisted_nodes where id = ?;", id.Int32)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "blacklist-node")
		return
	}
}
