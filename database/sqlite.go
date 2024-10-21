package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"

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

	// Check if DB exists
	var newDB bool = false
	if _, err := os.Stat(config["database_file"]); errors.Is(err, os.ErrNotExist) {
		newDB = true
	}

	// Init db connection
	db, err := sql.Open("sqlite", config["database_file"])

	if err != nil {
		message := fmt.Sprintf("Can not create database connection. (%s)", err.Error())
		utils.Log("error", message, "database")
		return nil, err
	}

	if newDB {
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

		createBlacklistedNodesTableSql := `
CREATE TABLE IF NOT EXISTS blacklisted_nodes (
	"id" INTEGER PRIMARY KEY,
	"node_id" VARCHAR(255) NOT NULL,
	"reason" TEXT NOT NULL,
	"timestamp" TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS blacklisted_nodes_id_idx ON blacklisted_nodes ("id");
CREATE INDEX IF NOT EXISTS blacklisted_nodes_node_id_idx ON blacklisted_nodes ("node_id");
`
		_, err = db.ExecContext(context.Background(), createBlacklistedNodesTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `blacklisted_nodes` table. (%s)", err.Error())
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

INSERT INTO currencies ("currency", "symbol") VALUES ('BITCOIN', 'BTC');
INSERT INTO currencies ("currency", "symbol") VALUES ('ETHER', 'ETH');
INSERT INTO currencies ("currency", "symbol") VALUES ('US Dollar', 'USD');
INSERT INTO currencies ("currency", "symbol") VALUES ('Euro', 'EUR');
INSERT INTO currencies ("currency", "symbol") VALUES ('Dirham', 'AED');
`
		_, err = db.ExecContext(context.Background(), createCurrenciesTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `currencies` table. (%s)", err.Error())
			utils.Log("error", message, "database")
			return nil, err
		}

		createResourcesTableSql := `
CREATE TABLE IF NOT EXISTS resources (
	"id" INTEGER PRIMARY KEY,
	"name" VARCHAR(255) NOT NULL,
	"active" BOOLEAN DEFAULT TRUE
);
CREATE UNIQUE INDEX IF NOT EXISTS resources_id_idx ON resources ("id");
CREATE UNIQUE INDEX IF NOT EXISTS resources_name_idx ON resources ("name");

INSERT INTO resources ("name") VALUES ('Data');
INSERT INTO resources ("name") VALUES ('CPU');
INSERT INTO resources ("name") VALUES ('Memory');
INSERT INTO resources ("name") VALUES ('Disk space');
INSERT INTO resources ("name") VALUES ('Ingress');
INSERT INTO resources ("name") VALUES ('Egress');
`
		_, err = db.ExecContext(context.Background(), createResourcesTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `resources` table. (%s)", err.Error())
			utils.Log("error", message, "database")
			return nil, err
		}

		createServiceTypesTableSql := `
CREATE TABLE IF NOT EXISTS service_types (
	"id" INTEGER PRIMARY KEY,
	"name" VARCHAR(255) NOT NULL,
	"active" BOOLEAN DEFAULT TRUE
);
CREATE UNIQUE INDEX IF NOT EXISTS service_types_id_idx ON service_types ("id");
CREATE INDEX IF NOT EXISTS service_types_name_idx ON service_types ("name");

INSERT INTO service_types ("name") VALUES ('Data');
INSERT INTO service_types ("name") VALUES ('Docker execution environment');
`
		_, err = db.ExecContext(context.Background(), createServiceTypesTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `service_types` table. (%s)", err.Error())
			utils.Log("error", message, "database")
			return nil, err
		}

		createServicesTable := `
CREATE TABLE IF NOT EXISTS services (
	"id" INTEGER PRIMARY KEY,
	"name" VARCHAR(255) NOT NULL,
	"description" TEXT DEFAULT NULL,
	"node_id" INTEGER NOT NULL,
	"service_type_id" INTEGER NOT NULL,
	FOREIGN KEY("node_id") REFERENCES nodes("id"),
	FOREIGN KEY("service_type_id") REFERENCES service_types("id")
);
CREATE UNIQUE INDEX IF NOT EXISTS services_id_idx ON services ("id");
CREATE INDEX IF NOT EXISTS services_name_idx ON services ("name");
`
		_, err = db.ExecContext(context.Background(), createServicesTable)
		if err != nil {
			message := fmt.Sprintf("Can not create `services` table. (%s)", err.Error())
			utils.Log("error", message, "database")
			return nil, err
		}

		createPricesTableSql := `
CREATE TABLE IF NOT EXISTS prices (
	"id" INTEGER PRIMARY KEY,
	"service_id" INTEGER NOT NULL,
	"resource_id" INTEGER NOT NULL,
	"price" DOUBLE PRECISION DEFAULT 0.0,
	"price_unit_normalizator" DOUBLE PRECISION DEFAULT 1.0,
	"price_interval" DOUBLE PRECISION DEFAULT 0.0,
	"currency_id" INTEGER NOT NULL,
	FOREIGN KEY("resource_id") REFERENCES resources("id"),
	FOREIGN KEY("service_id") REFERENCES nodes("id"),
	FOREIGN KEY("currency_id") REFERENCES currencies("id")
);
CREATE UNIQUE INDEX IF NOT EXISTS prices_id_idx ON prices ("id");
`
		_, err = db.ExecContext(context.Background(), createPricesTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `prices` table. (%s)", err.Error())
			utils.Log("error", message, "database")
			return nil, err
		}

		createJobsTable := `
CREATE TABLE IF NOT EXISTS jobs (
	"id" INTEGER PRIMARY KEY,
	"service_id" INTEGER NOT NULL,
	"status"  VARCHAR(10) CHECK( "status" IN ('IDLE', 'RUNNING', 'CANCELLED', 'ERRORED', 'FINISHED') ) NOT NULL DEFAULT 'IDLE',
	"started" TEXT DEFAULT NULL,
	"ended" TEXT DEFAULT NULL,
	FOREIGN KEY("service_id") REFERENCES services("id")
);
CREATE UNIQUE INDEX IF NOT EXISTS jobs_id_idx ON jobs ("id");
`
		_, err = db.ExecContext(context.Background(), createJobsTable)
		if err != nil {
			message := fmt.Sprintf("Can not create `jobs` table. (%s)", err.Error())
			utils.Log("error", message, "database")
			return nil, err
		}

		createJobInputsTableSql := `
CREATE TABLE IF NOT EXISTS job_inputs (
	"id" INTEGER PRIMARY KEY,
	"job_id" INTEGER NOT NULL,
	"input_job_id" DEFAULT NULL,
	FOREIGN KEY("job_id") REFERENCES jobs("id")
);
CREATE UNIQUE INDEX IF NOT EXISTS job_inputs_id_idx ON job_inputs ("id");
`
		_, err = db.ExecContext(context.Background(), createJobInputsTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `job_inputs` table. (%s)", err.Error())
			utils.Log("error", message, "database")
			return nil, err
		}

		createResourcesUtilizationsTableSql := `
CREATE TABLE IF NOT EXISTS resources_utilizations (
	"id" INTEGER PRIMARY KEY,
	"job_id" INTEGER NOT NULL,
	"resource_id" INTEGER NOT NULL,
	"utilization" DOUBLE PRECISION DEFAULT 0.0,
	"timestamp" TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY("resource_id") REFERENCES resources("id"),
	FOREIGN KEY("job_id") REFERENCES jobs("id")
);
CREATE UNIQUE INDEX IF NOT EXISTS resources_utilizations_id_idx ON resources_utilizations ("id");
`
		_, err = db.ExecContext(context.Background(), createResourcesUtilizationsTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `resources_utilizations` table. (%s)", err.Error())
			utils.Log("error", message, "database")
			return nil, err
		}

		createOrchestrationsTableSql := `
CREATE TABLE IF NOT EXISTS orchestrations (
	"id" INTEGER PRIMARY KEY,
	"job_ids" TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS orchestrations_id_idx ON orchestrations ("id");
`
		_, err = db.ExecContext(context.Background(), createOrchestrationsTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `orchestrations` table. (%s)", err.Error())
			utils.Log("error", message, "database")
			return nil, err
		}

		createExecutionsTable := `
CREATE TABLE IF NOT EXISTS executions (
	"id" INTEGER PRIMARY KEY,
	"orchestration_id" INTEGER NOT NULL,
	"status"  VARCHAR(10) CHECK( "status" IN ('IDLE', 'RUNNING', 'CANCELLED', 'ERRORED', 'FINISHED') ) NOT NULL DEFAULT 'IDLE',
	"started" TEXT DEFAULT NULL,
	"ended" TEXT DEFAULT NULL,
	FOREIGN KEY("orchestration_id") REFERENCES orchestrations("id")
);
CREATE UNIQUE INDEX IF NOT EXISTS executions_id_idx ON executions ("id");
`
		_, err = db.ExecContext(context.Background(), createExecutionsTable)
		if err != nil {
			message := fmt.Sprintf("Can not create `executions` table. (%s)", err.Error())
			utils.Log("error", message, "database")
			return nil, err
		}

		createWhitelistedReposTableSql := `
CREATE TABLE IF NOT EXISTS whitelisted_repos (
	"id" INTEGER PRIMARY KEY,
	"repo"  VARCHAR(1000) NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS whitelisted_repos_id_idx ON whitelisted_repos ("id");
`
		_, err = db.ExecContext(context.Background(), createWhitelistedReposTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `whitelisted_repos` table. (%s)", err.Error())
			utils.Log("error", message, "database")
			return nil, err
		}
	}

	return db, nil
}
