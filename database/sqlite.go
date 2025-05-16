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
	db, err := sql.Open("sqlite",
		fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=1",
			config["database_file"]))
	if err != nil {
		message := fmt.Sprintf("Can not create database connection. (%s)", err.Error())
		logsManager.Log("error", message, "database")
		return nil, err
	}
	db.SetMaxOpenConns(1)

	// Explicitly enable foreign key enforcement
	_, err = db.Exec("PRAGMA foreign_keys = ON;")
	if err != nil {
		message := fmt.Sprintf("Failed to enable foreign keys: %s", err.Error())
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
	FOREIGN KEY("service_id") REFERENCES services("id") ON DELETE CASCADE
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
	"remote" TEXT DEFAULT '',
	"branch" TEXT DEFAULT '',
	"username" TEXT DEFAULT '',
	"token" TEXT DEFAULT '',
	"repo_docker_files" TEXT DEFAULT '',
	"repo_docker_composes" TEXT DEFAULT '',
	FOREIGN KEY("service_id") REFERENCES services("id") ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS docker_service_details_id_idx ON docker_service_details ("id");
`
		_, err = db.ExecContext(context.Background(), createDockerServiceDetailsTable)
		if err != nil {
			message := fmt.Sprintf("Can not create `docker_service_details` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createDockerServiceImagesTable := `
CREATE TABLE IF NOT EXISTS docker_service_images (
	"id" INTEGER PRIMARY KEY AUTOINCREMENT,
	"service_details_id" INTEGER NOT NULL,
	"image_id" TEXT NOT NULL,
	"image_name" TEXT NOT NULL,
	"image_entry_points" TEXT DEFAULT '',
	"image_commands" TEXT DEFAULT '',
	"image_tags" TEXT DEFAULT '',
	"image_digests" TEXT DEFAULT '',
	"timestamp" TEXT NOT NULL,
	FOREIGN KEY("service_details_id") REFERENCES docker_service_details("id") ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS docker_service_images_id_idx ON docker_service_images ("id");
`
		_, err = db.ExecContext(context.Background(), createDockerServiceImagesTable)
		if err != nil {
			message := fmt.Sprintf("Can not create `docker_service_images` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createDockerServiceImageInterfacesTable := `
CREATE TABLE IF NOT EXISTS docker_service_image_interfaces (
	"id" INTEGER PRIMARY KEY AUTOINCREMENT,
	"service_image_id" INTEGER NOT NULL,
	"interface_type" VARCHAR(10) CHECK( "interface_type" IN ('STDIN', 'STDOUT', 'MOUNT') ) NOT NULL DEFAULT 'MOUNT',
	"description" TEXT NOT NULL,
	"path" TEXT DEFAULT '',
	FOREIGN KEY("service_image_id") REFERENCES docker_service_images("id") ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS docker_service_image_interfaces_id_idx ON docker_service_image_interfaces ("id");
`
		_, err = db.ExecContext(context.Background(), createDockerServiceImageInterfacesTable)
		if err != nil {
			message := fmt.Sprintf("Can not create `docker_service_image_interfaces` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createExecutableServiceDetailsTable := `
CREATE TABLE IF NOT EXISTS executable_service_details (
	"id" INTEGER PRIMARY KEY AUTOINCREMENT,
	"service_id" INTEGER NOT NULL,
	"path" TEXT NOT NULL,
	FOREIGN KEY("service_id") REFERENCES services("id") ON DELETE CASCADE
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
	FOREIGN KEY("resource_id") REFERENCES resources("id") ON DELETE CASCADE,
	FOREIGN KEY("service_id") REFERENCES services("id") ON DELETE CASCADE,
	FOREIGN KEY("currency_symbol") REFERENCES currencies("symbol") ON DELETE CASCADE
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
	"execution_constraint" VARCHAR(20) CHECK( "execution_constraint" IN ('NONE', 'INPUTS READY', 'DATETIME', 'JOBS EXECUTED', 'MANUAL START') ) NOT NULL DEFAULT 'NONE',
	"execution_constraint_detail" TEXT DEFAULT '',
	"status" VARCHAR(10) CHECK( "status" IN ('IDLE', 'READY', 'RUNNING', 'CANCELLED', 'ERRORED', 'COMPLETED') ) NOT NULL DEFAULT 'IDLE',
	"entrypoint" TEXT DEFAULT '',
	"commands" TEXT DEFAULT '',
	"started" TEXT DEFAULT '',
	"ended" TEXT DEFAULT '',
	FOREIGN KEY("service_id") REFERENCES services("id") ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS jobs_id_idx ON jobs ("id");
`
		_, err = db.ExecContext(context.Background(), createJobsTable)
		if err != nil {
			message := fmt.Sprintf("Can not create `jobs` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createJobInterfacesTable := `
CREATE TABLE IF NOT EXISTS job_interfaces (
	"id" INTEGER PRIMARY KEY AUTOINCREMENT,
	"job_id" INTEGER NOT NULL,
	"interface_type" VARCHAR(10) CHECK( "interface_type" IN ('STDIN', 'STDOUT', 'MOUNT') ) NOT NULL DEFAULT 'MOUNT',
	"path" TEXT DEFAULT '',
	FOREIGN KEY("job_id") REFERENCES jobs("id") ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS job_interfaces_id_idx ON job_interfaces ("id");
`
		_, err = db.ExecContext(context.Background(), createJobInterfacesTable)
		if err != nil {
			message := fmt.Sprintf("Can not create `job_interfaces` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createJobInterfacePeersTable := `
CREATE TABLE IF NOT EXISTS job_interface_peers (
	"id" INTEGER PRIMARY KEY AUTOINCREMENT,
	"job_interface_id" INTEGER NOT NULL,
	"peer_node_id" TEXT NOT NULL,
	"peer_job_id" INTEGER NOT NULL,
	"path" TEXT DEFAULT '',
	FOREIGN KEY("job_interface_id") REFERENCES job_interfaces("id") ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS job_interface_peers_id_idx ON job_interface_peers ("id");
`
		_, err = db.ExecContext(context.Background(), createJobInterfacePeersTable)
		if err != nil {
			message := fmt.Sprintf("Can not create `job_interface_peers` table. (%s)", err.Error())
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
	FOREIGN KEY("resource_id") REFERENCES resources("id") ON DELETE CASCADE,
	FOREIGN KEY("job_id") REFERENCES jobs("id") ON DELETE CASCADE
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
	"expected_job_outputs" TEXT NOT NULL,
	"status" VARCHAR(10) CHECK( "status" IN ('IDLE', 'RUNNING', 'CANCELLED', 'ERRORED', 'COMPLETED') ) NOT NULL DEFAULT 'IDLE',
	FOREIGN KEY("workflow_id") REFERENCES workflows("id") ON DELETE CASCADE
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
INSERT INTO settings ("key", "description", "setting_type", "value_boolean") VALUES ('accept_job_run_status_request', 'Accept Job Run Status requests for update sent by other peers', 'BOOLEAN', 1) ON CONFLICT(key) DO UPDATE SET "description" = 'Accept Job Run Status requests for update sent by other peers', "setting_type" = 'BOOLEAN', "value_boolean" = 1;
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

		createAuditLogTableSql := `
CREATE TABLE IF NOT EXISTS audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    table_name TEXT NOT NULL,
    operation TEXT NOT NULL, -- INSERT, UPDATE, DELETE
    record_id INTEGER,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    data TEXT -- JSON payload with changed values
);
`
		_, err = db.ExecContext(context.Background(), createAuditLogTableSql)
		if err != nil {
			message := fmt.Sprintf("Can not create `audit_logs` table. (%s)", err.Error())
			logsManager.Log("error", message, "database")
			return nil, err
		}

		createAuditTriggersSql := `
-- === JOBS Triggers ===
CREATE TRIGGER IF NOT EXISTS trg_jobs_insert
AFTER INSERT ON jobs
BEGIN
  INSERT INTO audit_logs (table_name, operation, record_id, data)
  VALUES ('jobs', 'INSERT', NEW.id, json_object('workflow_id', NEW.workflow_id, 'service_id', NEW.service_id, 'status', NEW.status));
END;

CREATE TRIGGER IF NOT EXISTS trg_jobs_update
AFTER UPDATE ON jobs
BEGIN
  INSERT INTO audit_logs (table_name, operation, record_id, data)
  VALUES ('jobs', 'UPDATE', NEW.id, json_object('workflow_id', NEW.workflow_id, 'service_id', NEW.service_id, 'status', NEW.status));
END;

CREATE TRIGGER IF NOT EXISTS trg_jobs_delete
AFTER DELETE ON jobs
BEGIN
  INSERT INTO audit_logs (table_name, operation, record_id, data)
  VALUES ('jobs', 'DELETE', OLD.id, json_object('workflow_id', OLD.workflow_id, 'service_id', OLD.service_id, 'status', OLD.status));
END;

-- === JOB_INTERFACES Triggers ===
CREATE TRIGGER IF NOT EXISTS trg_job_interfaces_insert
AFTER INSERT ON job_interfaces
BEGIN
  INSERT INTO audit_logs (table_name, operation, record_id, data)
  VALUES ('job_interfaces', 'INSERT', NEW.id, json_object('job_id', NEW.job_id, 'interface_type', NEW.interface_type, 'path', NEW.path));
END;

CREATE TRIGGER IF NOT EXISTS trg_job_interfaces_update
AFTER UPDATE ON job_interfaces
BEGIN
  INSERT INTO audit_logs (table_name, operation, record_id, data)
  VALUES ('job_interfaces', 'UPDATE', NEW.id, json_object('job_id', NEW.job_id, 'interface_type', NEW.interface_type, 'path', NEW.path));
END;

CREATE TRIGGER IF NOT EXISTS trg_job_interfaces_delete
AFTER DELETE ON job_interfaces
BEGIN
  INSERT INTO audit_logs (table_name, operation, record_id, data)
  VALUES ('job_interfaces', 'DELETE', OLD.id, json_object('job_id', OLD.job_id, 'interface_type', OLD.interface_type, 'path', OLD.path));
END;

-- === JOB_INTERFACE_PEERS Triggers ===
CREATE TRIGGER IF NOT EXISTS trg_job_interface_peers_insert
AFTER INSERT ON job_interface_peers
BEGIN
  INSERT INTO audit_logs (table_name, operation, record_id, data)
  VALUES ('job_interface_peers', 'INSERT', NEW.id, json_object('job_interface_id', NEW.job_interface_id, 'peer_node_id', NEW.peer_node_id, 'peer_job_id', NEW.peer_job_id, 'path', NEW.path));
END;

CREATE TRIGGER IF NOT EXISTS trg_job_interface_peers_update
AFTER UPDATE ON job_interface_peers
BEGIN
  INSERT INTO audit_logs (table_name, operation, record_id, data)
  VALUES ('job_interface_peers', 'UPDATE', NEW.id, json_object('job_interface_id', NEW.job_interface_id, 'peer_node_id', NEW.peer_node_id, 'peer_job_id', NEW.peer_job_id, 'path', NEW.path));
END;

CREATE TRIGGER IF NOT EXISTS trg_job_interface_peers_delete
AFTER DELETE ON job_interface_peers
BEGIN
  INSERT INTO audit_logs (table_name, operation, record_id, data)
  VALUES ('job_interface_peers', 'DELETE', OLD.id, json_object('job_interface_id', OLD.job_interface_id, 'peer_node_id', OLD.peer_node_id, 'peer_job_id', OLD.peer_job_id, 'path', OLD.path));
END;

-- === RESOURCES_UTILIZATIONS Triggers ===
CREATE TRIGGER IF NOT EXISTS trg_resources_utilizations_insert
AFTER INSERT ON resources_utilizations
BEGIN
  INSERT INTO audit_logs (table_name, operation, record_id, data)
  VALUES ('resources_utilizations', 'INSERT', NEW.id, json_object('job_id', NEW.job_id, 'resource_id', NEW.resource_id, 'utilization', NEW.utilization));
END;

CREATE TRIGGER IF NOT EXISTS trg_resources_utilizations_update
AFTER UPDATE ON resources_utilizations
BEGIN
  INSERT INTO audit_logs (table_name, operation, record_id, data)
  VALUES ('resources_utilizations', 'UPDATE', NEW.id, json_object('job_id', NEW.job_id, 'resource_id', NEW.resource_id, 'utilization', NEW.utilization));
END;

CREATE TRIGGER IF NOT EXISTS trg_resources_utilizations_delete
AFTER DELETE ON resources_utilizations
BEGIN
  INSERT INTO audit_logs (table_name, operation, record_id, data)
  VALUES ('resources_utilizations', 'DELETE', OLD.id, json_object('job_id', OLD.job_id, 'resource_id', OLD.resource_id, 'utilization', OLD.utilization));
END;

-- === WORKFLOWS Triggers ===
CREATE TRIGGER IF NOT EXISTS trg_workflows_insert
AFTER INSERT ON workflows
BEGIN
  INSERT INTO audit_logs (table_name, operation, record_id, data)
  VALUES ('workflows', 'INSERT', NEW.id, json_object('name', NEW.name, 'description', NEW.description));
END;

CREATE TRIGGER IF NOT EXISTS trg_workflows_update
AFTER UPDATE ON workflows
BEGIN
  INSERT INTO audit_logs (table_name, operation, record_id, data)
  VALUES ('workflows', 'UPDATE', NEW.id, json_object('name', NEW.name, 'description', NEW.description));
END;

CREATE TRIGGER IF NOT EXISTS trg_workflows_delete
AFTER DELETE ON workflows
BEGIN
  INSERT INTO audit_logs (table_name, operation, record_id, data)
  VALUES ('workflows', 'DELETE', OLD.id, json_object('name', OLD.name, 'description', OLD.description));
END;

-- === WORKFLOW_JOBS Triggers ===
CREATE TRIGGER IF NOT EXISTS trg_workflow_jobs_insert
AFTER INSERT ON workflow_jobs
BEGIN
  INSERT INTO audit_logs (table_name, operation, record_id, data)
  VALUES ('workflow_jobs', 'INSERT', NEW.id, json_object('workflow_id', NEW.workflow_id, 'job_id', NEW.job_id, 'status', NEW.status));
END;

CREATE TRIGGER IF NOT EXISTS trg_workflow_jobs_update
AFTER UPDATE ON workflow_jobs
BEGIN
  INSERT INTO audit_logs (table_name, operation, record_id, data)
  VALUES ('workflow_jobs', 'UPDATE', NEW.id, json_object('workflow_id', NEW.workflow_id, 'job_id', NEW.job_id, 'status', NEW.status));
END;

CREATE TRIGGER IF NOT EXISTS trg_workflow_jobs_delete
AFTER DELETE ON workflow_jobs
BEGIN
  INSERT INTO audit_logs (table_name, operation, record_id, data)
  VALUES ('workflow_jobs', 'DELETE', OLD.id, json_object('workflow_id', OLD.workflow_id, 'job_id', OLD.job_id, 'status', OLD.status));
END;
`
		_, err = db.ExecContext(context.Background(), createAuditTriggersSql)
		if err != nil {
			logsManager.Log("error", fmt.Sprintf("Failed to create audit triggers: %s", err.Error()), "database")
			return nil, err
		}
	}

	return db, nil
}
