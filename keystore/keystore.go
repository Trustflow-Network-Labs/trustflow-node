package keystore

import (
	"context"

	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/utils"
)

// Add node
func AddKey(identifier string, algorithm string, key []byte) error {
	// Declarations
	var k node_types.Key

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "node")
		return err
	}
	defer db.Close()

	addKeyResult := db.QueryRow(context.Background(), "select * from trustflow_node.add_key($1, $2, $3::bytea);",
		identifier, algorithm, key)
	err = addKeyResult.Scan(&k.Id, &k.Identifier, &k.Algorithm, &k.Key)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "node")
		return err
	}
	return nil
}

// Find key
func FindKey(identifier string) (node_types.Key, error) {
	// Declarations
	var k node_types.Key

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "node")
		return k, err
	}
	defer db.Close()

	addKeyResult := db.QueryRow(context.Background(), "select * from trustflow_node.find_key($1);", identifier)
	err = addKeyResult.Scan(&k.Id, &k.Identifier, &k.Algorithm, &k.Key)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "node")
		return k, err
	}
	return k, nil
}
