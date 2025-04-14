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
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000", config["database_file"]))
	if err != nil {
		message := fmt.Sprintf("Can not create database connection. (%s)", err.Error())
		logsManager.Log("error", message, "database")
		return nil, err
	}
	db.SetMaxOpenConns(1)

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
	"id" INTEGER PRIMARY KEY AUTOINCREMENT,
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
	"id" INTEGER PRIMARY KEY AUTOINCREMENT,
	"resource_group" VARCHAR(255) NOT NULL,
	"resource" VARCHAR(255) NOT NULL,
	"resource_unit" VARCHAR(255) NOT NULL,
	"description" TEXT DEFAULT NULL,
	"active" BOOLEAN DEFAULT TRUE
);
CREATE UNIQUE INDEX IF NOT EXISTS resources_id_idx ON resources ("id");

INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Compute Resources (CPU & GPU)', 'vCPU (Virtual CPU)', 'core-second');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Compute Resources (CPU & GPU)', 'vCPU (Virtual CPU)', 'core-minute');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Compute Resources (CPU & GPU)', 'vCPU (Virtual CPU)', 'core-hour');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Compute Resources (CPU & GPU)', 'GPU (Graphics Processing Unit)', 'GPU-second');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Compute Resources (CPU & GPU)', 'GPU (Graphics Processing Unit)', 'GPU-minute');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Compute Resources (CPU & GPU)', 'GPU (Graphics Processing Unit)', 'GPU-hour');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Memory (RAM)', 'RAM (Random Access Memory)', 'MB-hour');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Memory (RAM)', 'RAM (Random Access Memory)', 'GB-hour');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Storage', 'Block Storage (SSD, HDD)', 'GB-month');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Storage', 'Block Storage (SSD, HDD)', 'GB-year');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Storage', 'Object Storage (S3-like storage)', 'GB-month');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Storage', 'Object Storage (S3-like storage)', 'GB-year');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Network Traffic', 'Ingress', 'GB');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Network Traffic', 'Egress', 'GB');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Execution Time', 'Docker Container Execution', 'container-second');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Execution Time', 'Docker Container Execution', 'container-minute');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Execution Time', 'Docker Container Execution', 'container-hour');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Execution Time', 'WASM Execution / Standalone executable', 'millisecond');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Execution Time', 'WASM Execution / Standalone executable', 'second');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Data Transactions', 'Files, Data Retrieval (Documents, API calls, smart contract queries, etc.)', 'request');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Data Transactions', 'Data Hosting (Data made available for a certain time)', 'GB-hour');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Data Transactions', 'Data Hosting (Data made available for a certain time)', 'GB-day');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Data Transactions', 'Data Hosting (Data made available for a certain time)', 'GB-month');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Data Transactions', 'Data Hosting (Data made available for a certain time)', 'GB-year');
INSERT INTO resources ("resource_group", "resource", "resource_unit") VALUES ('Energy Consumption', 'Power/Energy Usage', 'kWh');
`
		_, err = db.ExecContext(context.Background(), createResourcesTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `resources` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createServicesTable := `
CREATE TABLE IF NOT EXISTS services (
	"id" INTEGER PRIMARY KEY AUTOINCREMENT,
	"name" VARCHAR(255) NOT NULL,
	"description" TEXT DEFAULT '',
	"service_type" VARCHAR(10) CHECK( "service_type" IN ('DATA', 'DOCKER EXECUTION ENVIRONMENT', 'STANDALONE EXECUTABLE') ) NOT NULL DEFAULT 'DATA',
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

		createDataServiceDetailsTable := `
CREATE TABLE IF NOT EXISTS data_service_details (
	"id" INTEGER PRIMARY KEY AUTOINCREMENT,
	"service_id" INTEGER NOT NULL,
	"path" TEXT NOT NULL,
	FOREIGN KEY("service_id") REFERENCES services("id")
);
CREATE UNIQUE INDEX IF NOT EXISTS data_service_details_id_idx ON data_service_details ("id");
`
		_, err = db.ExecContext(context.Background(), createDataServiceDetailsTable)
		if err != nil {
			message := fmt.Sprintf("Can not create `data_service_details` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createDockerServiceDetailsTable := `
CREATE TABLE IF NOT EXISTS docker_service_details (
	"id" INTEGER PRIMARY KEY AUTOINCREMENT,
	"service_id" INTEGER NOT NULL,
	"repo" TEXT DEFAULT '',
	"image" TEXT NOT NULL,
	FOREIGN KEY("service_id") REFERENCES services("id")
);
CREATE UNIQUE INDEX IF NOT EXISTS docker_service_details_id_idx ON docker_service_details ("id");
`
		_, err = db.ExecContext(context.Background(), createDockerServiceDetailsTable)
		if err != nil {
			message := fmt.Sprintf("Can not create `docker_service_details` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createExecutableServiceDetailsTable := `
CREATE TABLE IF NOT EXISTS executable_service_details (
	"id" INTEGER PRIMARY KEY AUTOINCREMENT,
	"service_id" INTEGER NOT NULL,
	"path" TEXT NOT NULL,
	FOREIGN KEY("service_id") REFERENCES services("id")
);
CREATE UNIQUE INDEX IF NOT EXISTS executable_service_details_id_idx ON executable_service_details ("id");
`
		_, err = db.ExecContext(context.Background(), createExecutableServiceDetailsTable)
		if err != nil {
			message := fmt.Sprintf("Can not create `executable_service_details` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createPricesTableSql := `
CREATE TABLE IF NOT EXISTS prices (
	"id" INTEGER PRIMARY KEY AUTOINCREMENT,
	"service_id" INTEGER NOT NULL,
	"resource_id" INTEGER NOT NULL,
	"price" DOUBLE PRECISION DEFAULT 0.0,
	"currency_symbol" VARCHAR(255) NOT NULL,
	FOREIGN KEY("resource_id") REFERENCES resources("id"),
	FOREIGN KEY("service_id") REFERENCES services("id"),
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
	"id" INTEGER PRIMARY KEY AUTOINCREMENT,
	"workflow_id" INTEGER NOT NULL,
	"service_id" INTEGER NOT NULL,
	"ordering_node_id" TEXT NOT NULL,
	"input_node_ids" TEXT DEFAULT '',
	"output_node_ids" TEXT DEFAULT '',
	"execution_constraint" VARCHAR(20) CHECK( "execution_constraint" IN ('NONE', 'INPUTS READY', 'DATETIME', 'JOBS EXECUTED', 'MANUAL START') ) NOT NULL DEFAULT 'NONE',
	"execution_constraint_detail" TEXT DEFAULT '',
	"status" VARCHAR(10) CHECK( "status" IN ('IDLE', 'READY', 'RUNNING', 'CANCELLED', 'ERRORED', 'COMPLETED') ) NOT NULL DEFAULT 'IDLE',
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
	"id" INTEGER PRIMARY KEY AUTOINCREMENT,
	"job_id" INTEGER NOT NULL,
	"resource_id" INTEGER NOT NULL,
	"utilization" DOUBLE PRECISION DEFAULT 0.0,
	"timestamp" TEXT NOT NULL,
	FOREIGN KEY("resource_id") REFERENCES resources("id"),
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

		createWorkflowsTableSql := `
CREATE TABLE IF NOT EXISTS workflows (
	"id" INTEGER PRIMARY KEY AUTOINCREMENT,
	"name" VARCHAR(255) NOT NULL,
	"description" TEXT DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS workflows_id_idx ON workflows ("id");
`
		_, err = db.ExecContext(context.Background(), createWorkflowsTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `orchestrations` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createOrchestrationsTableSql := `
CREATE TABLE IF NOT EXISTS workflow_jobs (
	"id" INTEGER PRIMARY KEY AUTOINCREMENT,
	"workflow_id" INTEGER NOT NULL,
	"node_id" TEXT NOT NULL,
	"job_id" INTEGER NOT NULL,
	"status" VARCHAR(10) CHECK( "status" IN ('IDLE', 'RUNNING', 'CANCELLED', 'ERRORED', 'COMPLETED') ) NOT NULL DEFAULT 'IDLE',
	FOREIGN KEY("workflow_id") REFERENCES workflows("id")
);
CREATE UNIQUE INDEX IF NOT EXISTS workflow_jobs_id_idx ON workflow_jobs ("id");
`
		_, err = db.ExecContext(context.Background(), createOrchestrationsTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `orchestrations` table. (%s)", err.Error())
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
INSERT INTO settings ("key", "description", "setting_type", "value_boolean") VALUES ('accept_job_run_request', 'Accept Job Run Requests  sent by other peers', 'BOOLEAN', 1) ON CONFLICT(key) DO UPDATE SET "description" = 'Accept Job Run Requests  sent by other peers', "setting_type" = 'BOOLEAN', "value_boolean" = 1;
INSERT INTO settings ("key", "description", "setting_type", "value_boolean") VALUES ('accept_job_run_response', 'Accept Job Run Responses sent by other peers', 'BOOLEAN', 1) ON CONFLICT(key) DO UPDATE SET "description" = 'Accept Job Run Responses sent by other peers', "setting_type" = 'BOOLEAN', "value_boolean" = 1;
INSERT INTO settings ("key", "description", "setting_type", "value_boolean") VALUES ('accept_job_run_status', 'Accept Job Run Status updates sent by other peers', 'BOOLEAN', 1) ON CONFLICT(key) DO UPDATE SET "description" = 'Accept Job Run Status updates sent by other peers', "setting_type" = 'BOOLEAN', "value_boolean" = 1;
INSERT INTO settings ("key", "description", "setting_type", "value_boolean") VALUES ('accept_binary_stream', 'Accept receiving binary stream sent by other peers', 'BOOLEAN', 1) ON CONFLICT(key) DO UPDATE SET "description" = 'Accept receiving binary stream sent by other peers', "setting_type" = 'BOOLEAN', "value_boolean" = 1;
INSERT INTO settings ("key", "description", "setting_type", "value_boolean") VALUES ('accept_file', 'Accept receiving file sent by other peers', 'BOOLEAN', 1) ON CONFLICT(key) DO UPDATE SET "description" = 'Accept receiving file sent by other peers', "setting_type" = 'BOOLEAN', "value_boolean" = 1;
INSERT INTO settings ("key", "description", "setting_type", "value_boolean") VALUES ('accept_service_request', 'Accept Service Requests sent by other peers', 'BOOLEAN', 1) ON CONFLICT(key) DO UPDATE SET "description" = 'Accept Service Requests sent by other peers', "setting_type" = 'BOOLEAN', "value_boolean" = 1;
INSERT INTO settings ("key", "description", "setting_type", "value_boolean") VALUES ('accept_service_response', 'Accept Service Responses sent by other peers', 'BOOLEAN', 1) ON CONFLICT(key) DO UPDATE SET "description" = 'Accept Service Responses sent by other peers', "setting_type" = 'BOOLEAN', "value_boolean" = 1;
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
