package tfnode

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/keystore"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/utils"
	"github.com/libp2p/go-libp2p/core/crypto"
)

func GetNodeKey() (crypto.PrivKey, crypto.PubKey, error) {
	// Declarations
	var priv crypto.PrivKey
	var pub crypto.PubKey

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "node")
		return nil, nil, err
	}
	defer db.Close()

	// Check do we have a node key already
	node, err := FindItself()
	if err != nil && strings.ToLower(err.Error()) != "sql: no rows in result set" {
		msg := err.Error()
		utils.Log("error", msg, "node")
		return nil, nil, err
	}

	// If we don't have a key let's create it
	if !node.Self.Valid || !node.Self.Bool {
		// Create a keypair
		//		p, err := ecdsa.GenerateKey(secp256k1.S256(), rand.Reader)
		//privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		priv, pub, err = crypto.GenerateKeyPair(
			crypto.ECDSA,
			-1,
		)
		if err != nil {
			msg := fmt.Sprintf("Can generate key pair. (%s)", err.Error())
			utils.Log("error", msg, "api")
			return nil, nil, err
		}
	} else {
		// Find node key
		key, err := keystore.FindKey(node.NodeId.String)
		if err != nil {
			msg := err.Error()
			utils.Log("error", msg, "node")
			return nil, nil, err
		}
		if !key.Identifier.Valid {
			msg := fmt.Sprintf("Could not find a key for provided node identifier %s.", node.NodeId.String)
			utils.Log("error", msg, "api")
			return nil, nil, err
		}

		// Get private key
		switch key.Algorithm.String {
		case "ECDSA: secp256r1":
			priv, err = crypto.UnmarshalPrivateKey(key.Key)
			if err != nil {
				msg := err.Error()
				utils.Log("error", msg, "node")
				return nil, nil, err
			}
			pub = priv.GetPublic()
		default:
			msg := fmt.Sprintf("Could not extract a key for provided algorithm %s.", key.Algorithm.String)
			utils.Log("error", msg, "api")
			return nil, nil, err
		}
	}

	return priv, pub, nil
}

// Add node
func AddNode(nodeId string, multiaddrs string, self bool) error {
	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "node")
		return err
	}
	defer db.Close()

	_, err = db.ExecContext(context.Background(), "insert into nodes (node_id, multiaddrs, self) values (?, ?, ?);",
		nodeId, multiaddrs, self)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "node")
		return err
	}
	return nil
}

// Update node
func UpdateNode(nodeId string, multiaddrs string, self bool) error {
	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "node")
		return err
	}
	defer db.Close()

	_, err = db.ExecContext(context.Background(), "update nodes set multiaddrs = ?, self = ? where node_id = ?;",
		multiaddrs, self, nodeId)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "node")
		return err
	}
	return nil
}

// Delete node
func DeleteNode(nodeId string) error {
	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "node")
		return err
	}
	defer db.Close()

	_, err = db.ExecContext(context.Background(), "delete from nodes where node_id = ?;", nodeId)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "node")
		return err
	}
	return nil
}

// Find itself
func FindItself() (node_types.Node, error) {
	// Declarations
	var node node_types.Node

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "node")
		return node, err
	}
	defer db.Close()

	row := db.QueryRowContext(context.Background(), "select * from nodes where self = true;")

	err = row.Scan(&node.Id, &node.NodeId, &node.Multiaddrs, &node.Self)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "node")
		return node, err
	}
	return node, nil
}

// Find node
func FindNode(nodeId string) (node_types.Node, error) {
	// Declarations
	var node node_types.Node

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "node")
		return node, err
	}
	defer db.Close()

	row := db.QueryRowContext(context.Background(), "select * from nodes where node_id = ?;", nodeId)

	err = row.Scan(&node.Id, &node.NodeId, &node.Multiaddrs, &node.Self)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "node")
		return node, err
	}
	return node, nil
}

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
	var id utils.NullInt32
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
