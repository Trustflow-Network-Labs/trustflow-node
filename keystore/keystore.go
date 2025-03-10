package keystore

import (
	"context"

	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/utils"
)

type KeyStoreManager struct {
}

func NewKeyStoreManager() *KeyStoreManager {
	return &KeyStoreManager{}
}

// Add node
func (ksm *KeyStoreManager) AddKey(identifier string, algorithm string, key []byte) error {
	// Create a database connection
	sqlManager := database.NewSQLiteManager()
	db, err := sqlManager.CreateConnection()
	logsManager := utils.NewLogsManager()
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "keystore")
		return err
	}
	defer db.Close()

	_, err = db.ExecContext(context.Background(), "insert into keystore (identifier, algorithm, key) values (?, ?, ?);",
		identifier, algorithm, key)
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "keystore")
		return err
	}

	return nil
}

// Find key
func (ksm *KeyStoreManager) FindKey(identifier string) (node_types.Key, error) {
	// Declarations
	var k node_types.Key

	// Create a database connection
	sqlManager := database.NewSQLiteManager()
	db, err := sqlManager.CreateConnection()
	logsManager := utils.NewLogsManager()
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "keystore")
		return k, err
	}
	defer db.Close()

	row := db.QueryRowContext(context.Background(), "select * from keystore where identifier = ?;", identifier)
	err = row.Scan(&k.Id, &k.Identifier, &k.Algorithm, &k.Key)
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "keystore")
		return k, err
	}
	return k, nil
}
