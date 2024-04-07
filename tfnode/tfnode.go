package tfnode

import (
	"context"
	"fmt"

	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/keystore"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/utils"
	"github.com/lib/pq"
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
	if err != nil {
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
func AddNode(nodeId string, multiaddrs []string, self bool) error {
	// Declarations
	var node node_types.Node

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "node")
		return err
	}
	defer db.Close()

	addNodeResult := db.QueryRow(context.Background(), "select * from trustflow_node.add_node($1, $2, $3::boolean);",
		nodeId, multiaddrs, self)
	err = addNodeResult.Scan(&node.Id, &node.NodeId, pq.Array(&node.Multiaddrs), &node.Self)
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

	findItselfResult := db.QueryRow(context.Background(), "select * from trustflow_node.find_itself();")
	err = findItselfResult.Scan(&node.Id, &node.NodeId, pq.Array(&node.Multiaddrs), &node.Self)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "node")
		return node, err
	}
	return node, nil
}
