package tfnode

import (
	"context"
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
	} else if err != nil && strings.ToLower(err.Error()) == "sql: no rows in result set" {
		// We don't have a key let's create it
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
		// We already have a key created before
		key, err := keystore.FindKey(node.NodeId)
		if err != nil && strings.ToLower(err.Error()) != "sql: no rows in result set" {
			msg := err.Error()
			utils.Log("error", msg, "node")
			return nil, nil, err
		} else if err != nil && strings.ToLower(err.Error()) == "sql: no rows in result set" {
			// but we can't find it
			msg := fmt.Sprintf("Could not find a key for provided node identifier %s.", node.NodeId)
			utils.Log("error", msg, "api")
			return nil, nil, err
		} else {
			// Get private key
			switch key.Algorithm {
			case "ECDSA: secp256r1":
				priv, err = crypto.UnmarshalPrivateKey(key.Key)
				if err != nil {
					msg := err.Error()
					utils.Log("error", msg, "node")
					return nil, nil, err
				}
				pub = priv.GetPublic()
			default:
				msg := fmt.Sprintf("Could not extract a key for provided algorithm %s.", key.Algorithm)
				utils.Log("error", msg, "api")
				return nil, nil, err
			}
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

// Find node by DB ID
func FindNodeById(id int32) (node_types.Node, error) {
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

	row := db.QueryRowContext(context.Background(), "select * from nodes where id = ?;", id)

	err = row.Scan(&node.Id, &node.NodeId, &node.Multiaddrs, &node.Self)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "node")
		return node, err
	}
	return node, nil
}
