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

type SQLiteManager struct {
}

func NewSQLiteManager() *SQLiteManager {
	return &SQLiteManager{}
}

// Create connection
func (sqlm *SQLiteManager) CreateConnection() (*sql.DB, error) {
	// Read configs
	configManager := utils.NewConfigManager("")
	config, err := configManager.ReadConfigs()
	logsManager := utils.NewLogsManager()
	if err != nil {
		message := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
		logsManager.Log("error", message, "database")
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
		logsManager.Log("error", message, "database")
		return nil, err
	}

	if newDB {
		// Create DB structure if it's not existing
		createBlacklistedNodesTableSql := `
CREATE TABLE IF NOT EXISTS blacklisted_nodes (
	"node_id" TEXT PRIMARY KEY,
	"reason" TEXT DEFAULT NULL,
	"timestamp" TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS blacklisted_nodes_node_id_idx ON blacklisted_nodes ("node_id");
`
		_, err = db.ExecContext(context.Background(), createBlacklistedNodesTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `blacklisted_nodes` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
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
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createCurrenciesTableSql := `
CREATE TABLE IF NOT EXISTS currencies (
	"symbol" VARCHAR(255) PRIMARY KEY,
	"currency" VARCHAR(255) NOT NULL
);
CREATE INDEX IF NOT EXISTS currencies_symbol_idx ON currencies ("symbol");
CREATE INDEX IF NOT EXISTS currencies_currency_idx ON currencies ("currency");

INSERT INTO currencies ("symbol", "currency") VALUES ('BTC', 'BITCOIN');
INSERT INTO currencies ("symbol", "currency") VALUES ('ETH', 'ETHER');
INSERT INTO currencies ("symbol", "currency") VALUES ('USD', 'US Dollar');
INSERT INTO currencies ("symbol", "currency") VALUES ('EUR', 'Euro');
INSERT INTO currencies ("symbol", "currency") VALUES ('AED', 'Dirham');
`
		_, err = db.ExecContext(context.Background(), createCurrenciesTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `currencies` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createResourcesTableSql := `
CREATE TABLE IF NOT EXISTS resources (
	"resource" VARCHAR(255) PRIMARY KEY,
	"description" TEXT DEFAULT NULL,
	"active" BOOLEAN DEFAULT TRUE
);
CREATE UNIQUE INDEX IF NOT EXISTS resources_resource_idx ON resources ("resource");

INSERT INTO resources ("resource") VALUES ('Data');
INSERT INTO resources ("resource") VALUES ('CPU');
INSERT INTO resources ("resource") VALUES ('Memory');
INSERT INTO resources ("resource") VALUES ('Disk space');
INSERT INTO resources ("resource") VALUES ('Ingress');
INSERT INTO resources ("resource") VALUES ('Egress');
`
		_, err = db.ExecContext(context.Background(), createResourcesTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `resources` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createServicesTable := `
CREATE TABLE IF NOT EXISTS services (
	"id" INTEGER PRIMARY KEY,
	"name" VARCHAR(255) NOT NULL,
	"description" TEXT DEFAULT '',
	"node_id" TEXT NOT NULL,
	"service_type" VARCHAR(10) CHECK( "service_type" IN ('DATA', 'DOCKER EXECUTION ENVIRONMENT', 'WASM EXECUTION ENVIRONMENT') ) NOT NULL DEFAULT 'DATA',
	"path" TEXT NOT NULL,
	"repo" TEXT DEFAULT '',
	"active" BOOLEAN DEFAULT TRUE
);
CREATE UNIQUE INDEX IF NOT EXISTS services_id_idx ON services ("id");
CREATE INDEX IF NOT EXISTS services_name_idx ON services ("name");
`
		_, err = db.ExecContext(context.Background(), createServicesTable)
		if err != nil {
			message := fmt.Sprintf("Can not create `services` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createPricesTableSql := `
CREATE TABLE IF NOT EXISTS prices (
	"id" INTEGER PRIMARY KEY,
	"service_id" INTEGER NOT NULL,
	"resource" VARCHAR(255) NOT NULL,
	"price" DOUBLE PRECISION DEFAULT 0.0,
	"price_unit_normalizator" DOUBLE PRECISION DEFAULT 1.0,
	"price_interval" DOUBLE PRECISION DEFAULT 0.0,
	"currency_symbol" VARCHAR(255) NOT NULL,
	FOREIGN KEY("resource") REFERENCES resources("resource"),
	FOREIGN KEY("service_id") REFERENCES nodes("id"),
	FOREIGN KEY("currency_symbol") REFERENCES currencies("symbol")
);
CREATE UNIQUE INDEX IF NOT EXISTS prices_id_idx ON prices ("id");
`
		_, err = db.ExecContext(context.Background(), createPricesTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `prices` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createJobsTable := `
CREATE TABLE IF NOT EXISTS jobs (
	"id" INTEGER PRIMARY KEY,
	"ordering_node_id" TEXT NOT NULL,
	"service_id" INTEGER NOT NULL,
	"status" VARCHAR(10) CHECK( "status" IN ('IDLE', 'RUNNING', 'CANCELLED', 'ERRORED', 'FINISHED') ) NOT NULL DEFAULT 'IDLE',
	"started" TEXT DEFAULT '',
	"ended" TEXT DEFAULT '',
	FOREIGN KEY("service_id") REFERENCES services("id")
);
CREATE UNIQUE INDEX IF NOT EXISTS jobs_id_idx ON jobs ("id");
`
		_, err = db.ExecContext(context.Background(), createJobsTable)
		if err != nil {
			message := fmt.Sprintf("Can not create `jobs` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createResourcesUtilizationsTableSql := `
CREATE TABLE IF NOT EXISTS resources_utilizations (
	"id" INTEGER PRIMARY KEY,
	"job_id" INTEGER NOT NULL,
	"resource" VARCHAR(255) NOT NULL,
	"utilization" DOUBLE PRECISION DEFAULT 0.0,
	"timestamp" TEXT NOT NULL,
	FOREIGN KEY("resource") REFERENCES resources("resource"),
	FOREIGN KEY("job_id") REFERENCES jobs("id")
);
CREATE UNIQUE INDEX IF NOT EXISTS resources_utilizations_id_idx ON resources_utilizations ("id");
`
		_, err = db.ExecContext(context.Background(), createResourcesUtilizationsTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `resources_utilizations` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createJobInputsTableSql := `
CREATE TABLE IF NOT EXISTS job_inputs (
	"id" INTEGER PRIMARY KEY,
	"job_execution_node_id" TEXT NOT NULL,
	"execution_job_id" INTEGER NOT NULL,
	"execution_constraint" VARCHAR(20) CHECK( "execution_constraint" IN ('INPUTS READY', 'DATETIME', 'JOBS EXECUTED', 'MANUAL START') ) NOT NULL DEFAULT 'INPUTS READY',
	"constraints" TEXT DEFAULT '',
	"job_input_node_id" TEXT DEFAULT NULL,
	"input_job_id" INTEGER DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS job_inputs_id_idx ON job_inputs ("id");
`
		_, err = db.ExecContext(context.Background(), createJobInputsTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `job_inputs` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createOrchestrationsTableSql := `
CREATE TABLE IF NOT EXISTS orchestrations (
	"id" INTEGER PRIMARY KEY,
	"execution_jobs" TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS orchestrations_id_idx ON orchestrations ("id");
`
		_, err = db.ExecContext(context.Background(), createOrchestrationsTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `orchestrations` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createExecutionsTable := `
CREATE TABLE IF NOT EXISTS executions (
	"id" INTEGER PRIMARY KEY,
	"orchestration_id" INTEGER NOT NULL,
	"status" VARCHAR(10) CHECK( "status" IN ('IDLE', 'RUNNING', 'CANCELLED', 'ERRORED', 'FINISHED') ) NOT NULL DEFAULT 'IDLE',
	"started" TEXT DEFAULT '',
	"ended" TEXT DEFAULT '',
	FOREIGN KEY("orchestration_id") REFERENCES orchestrations("id")
);
CREATE UNIQUE INDEX IF NOT EXISTS executions_id_idx ON executions ("id");
`
		_, err = db.ExecContext(context.Background(), createExecutionsTable)
		if err != nil {
			message := fmt.Sprintf("Can not create `executions` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createWhitelistedReposTableSql := `
CREATE TABLE IF NOT EXISTS whitelisted_repos (
	"id" INTEGER PRIMARY KEY,
	"repo" TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS whitelisted_repos_id_idx ON whitelisted_repos ("id");
`
		_, err = db.ExecContext(context.Background(), createWhitelistedReposTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `whitelisted_repos` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createSettingsTableSql := `
CREATE TABLE IF NOT EXISTS settings (
	"key" TEXT PRIMARY KEY,
	"description" TEXT DEFAULT NULL,
	"setting_type" VARCHAR(10) CHECK( "setting_type" IN ('STRING', 'INTEGER', 'REAL', 'BOOLEAN', 'JSON') ) NOT NULL DEFAULT 'STRING',
	"value_string" TEXT DEFAULT NULL,
	"value_integer" INTEGER DEFAULT NULL,
	"value_real" REAL DEFAULT NULL,
	"value_boolean" INTEGER CHECK( "value_boolean" IN (0, 1) ) NOT NULL DEFAULT 0,
	"value_json" TEXT DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS settings_key_idx ON settings ("key");

INSERT INTO settings ("key", "description", "setting_type", "value_string") VALUES ('node_identifier', 'Node Identifier', 'STRING', '') ON CONFLICT(key) DO UPDATE SET "description" = 'Node Identifier', "setting_type" = 'STRING', "value_string" = '';
INSERT INTO settings ("key", "description", "setting_type", "value_boolean") VALUES ('accept_service_catalogue', 'Accept Service Catalogues sent by other peers', 'BOOLEAN', 1) ON CONFLICT(key) DO UPDATE SET "description" = 'Accept Service Catalogues sent by other peers', "setting_type" = 'BOOLEAN', "value_boolean" = 1;
INSERT INTO settings ("key", "description", "setting_type", "value_boolean") VALUES ('accept_sending_data', 'Accept sending data to requesting peer', 'BOOLEAN', 1) ON CONFLICT(key) DO UPDATE SET "description" = 'Accept sending data to requesting peer', "setting_type" = 'BOOLEAN', "value_boolean" = 1;
INSERT INTO settings ("key", "description", "setting_type", "value_boolean") VALUES ('accept_binary_stream', 'Accept receiving binary stream sent by other peers', 'BOOLEAN', 1) ON CONFLICT(key) DO UPDATE SET "description" = 'Accept receiving binary stream sent by other peers', "setting_type" = 'BOOLEAN', "value_boolean" = 1;
INSERT INTO settings ("key", "description", "setting_type", "value_boolean") VALUES ('accept_file', 'Accept receiving file sent by other peers', 'BOOLEAN', 1) ON CONFLICT(key) DO UPDATE SET "description" = 'Accept receiving file sent by other peers', "setting_type" = 'BOOLEAN', "value_boolean" = 1;
`
		_, err = db.ExecContext(context.Background(), createSettingsTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `settings` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}
	}

	return db, nil
}
