package blacklist_node

import (
	"context"
	"errors"
	"fmt"

	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/utils"
)

// Is node blacklisted
func NodeBlacklisted(nodeId string) (error, bool) {
	if nodeId == "" {
		msg := "invalid Node ID"
		utils.Log("error", msg, "blacklist-node")
		return errors.New(msg), false
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "blacklist-node")
		return err, false
	}
	defer db.Close()

	// Check if node is already blacklisted
	var id node_types.NullInt32
	row := db.QueryRowContext(context.Background(), "select id from blacklisted_nodes where node_id = ?;", nodeId)

	err = row.Scan(&id)
	if err != nil {
		msg := err.Error()
		utils.Log("debug", msg, "blacklist-node")
		return nil, false
	}

	return nil, true
}

// Blacklist a node
func BlacklistNode(nodeId string, reason string) {
	err, blacklisted := NodeBlacklisted(nodeId)
	if err != nil {
		msg := err.Error()
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
	if blacklisted {
		msg := fmt.Sprintf("Node %s is already blacklisted", nodeId)
		utils.Log("warn", msg, "blacklist-node")
		return
	}

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

// Remove node from blacklist
func RemoveNodeFromBlacklist(nodeId string) {
	err, blacklisted := NodeBlacklisted(nodeId)
	if err != nil {
		msg := err.Error()
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
	if !blacklisted {
		msg := fmt.Sprintf("Node %s is not blacklisted", nodeId)
		utils.Log("warn", msg, "blacklist-node")
		return
	}

	// Remove node from blacklist
	utils.Log("debug", fmt.Sprintf("removing node %s from blacklist", nodeId), "blacklist-node")

	_, err = db.ExecContext(context.Background(), "delete from blacklisted_nodes where node_id = ?;", nodeId)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "blacklist-node")
		return
	}
}
