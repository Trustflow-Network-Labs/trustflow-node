package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/adgsm/trustflow-node/utils"
	_ "modernc.org/sqlite"
)

// provide configs file path
var configsPath string = "database/configs"

// Create connection
func CreateConnection() (*sql.DB, error) {
	// Read configs
	config, err := utils.ReadConfigs(configsPath)
	if err != nil {
		message := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
		utils.Log("error", message, "database")
		return nil, err
	}

	// Init db connection
	db, err := sql.Open("sqlite", config["database_file"])

	if err != nil {
		message := fmt.Sprintf("Can not create database connection. (%s)", err.Error())
		utils.Log("error", message, "database")
		return nil, err
	}

	// Create DB structure if it's not existing
	createNodesTableSql := `
CREATE TABLE IF NOT EXISTS nodes (
	"id" INTEGER PRIMARY KEY,
	"node_id" VARCHAR(255) NOT NULL,
	"multiaddrs" TEXT NOT NULL,
	"self" BOOLEAN DEFAULT FALSE
);
CREATE UNIQUE INDEX IF NOT EXISTS nodes_id_idx ON nodes ("id");
CREATE INDEX IF NOT EXISTS nodes_node_id_idx ON nodes ("node_id");
`
	_, err = db.ExecContext(context.Background(), createNodesTableSql)
	if err != nil {
		message := fmt.Sprintf("Can not create `nodes` table. (%s)", err.Error())
		utils.Log("error", message, "database")
		return nil, err
	}

	createKeystoreTableSql := `
CREATE TABLE IF NOT EXISTS keystore (
	"id" INTEGER PRIMARY KEY,
	"identifier" VARCHAR(255) NOT NULL,
	"algorithm" VARCHAR(255) NOT NULL,
	"key" BLOB NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS keystore_id_idx ON keystore ("id");
CREATE INDEX IF NOT EXISTS keystore_identifier_idx ON keystore ("identifier");
CREATE INDEX IF NOT EXISTS keystore_algorithm_idx ON keystore ("algorithm");
`
	_, err = db.ExecContext(context.Background(), createKeystoreTableSql)
	if err != nil {
		message := fmt.Sprintf("Can not create `keystore` table. (%s)", err.Error())
		utils.Log("error", message, "database")
		return nil, err
	}

	createCurrenciesTableSql := `
CREATE TABLE IF NOT EXISTS currencies (
	"id" INTEGER PRIMARY KEY,
	"currency" VARCHAR(255) NOT NULL,
	"symbol" VARCHAR(255) NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS currencies_id_idx ON currencies ("id");
CREATE INDEX IF NOT EXISTS currencies_currency_idx ON currencies ("currency");
CREATE INDEX IF NOT EXISTS currencies_symbol_idx ON currencies ("symbol");
`
	_, err = db.ExecContext(context.Background(), createCurrenciesTableSql)
	if err != nil {
		message := fmt.Sprintf("Can not create `currencies` table. (%s)", err.Error())
		utils.Log("error", message, "database")
		return nil, err
	}

	createServiceCatalogueTable := `
CREATE TABLE IF NOT EXISTS service_catalogue (
	"id" INTEGER PRIMARY KEY,
	"type" VARCHAR(255) NOT NULL,
	"name" VARCHAR(255) NOT NULL,
	"description" TEXT DEFAULT NULL,
	"price" DOUBLE PRECISION DEFAULT 0.0,
	"price_interval" INTEGER DEFAULT 0,
	"currency_id" INTEGER NOT NULL,
	"local_repo" VARCHAR(1000) DEFAULT NULL,
	"remote_repo" VARCHAR(1000) DEFAULT NULL,
	"node_id" INTEGER NOT NULL,
	FOREIGN KEY("node_id") REFERENCES nodes("id"),
	FOREIGN KEY("currency_id") REFERENCES currencies("id")
);
CREATE UNIQUE INDEX IF NOT EXISTS service_catalogue_id_idx ON service_catalogue ("id");
CREATE INDEX IF NOT EXISTS service_catalogue_name_idx ON service_catalogue ("name");
`
	_, err = db.ExecContext(context.Background(), createServiceCatalogueTable)
	if err != nil {
		message := fmt.Sprintf("Can not create `service_catalogue` table. (%s)", err.Error())
		utils.Log("error", message, "database")
		return nil, err
	}

	return db, nil
}
