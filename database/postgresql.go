package database

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/adgsm/trustflow-node/utils"
	"github.com/jackc/pgx/v4/log/logrusadapter"
	"github.com/jackc/pgx/v4/pgxpool"
	log "github.com/sirupsen/logrus"
)

// provide configs file path
var configsPath string = "database/configs"

func createPool(host string, port int, user string,
	password string, dbname string) (*pgxpool.Pool, error) {
	psqlconn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s", user, password, host, port, dbname)

	// init config
	confs := utils.Config{
		"file": configsPath,
	}
	// read configs
	confs, err := utils.ReadConfigs(configsPath)
	if err != nil {
		panic(err)
	}

	// open log file
	logFile, err := os.OpenFile(confs["logfile"], os.O_APPEND|os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to opent log file: %v\n", err)
		os.Exit(1)
	}

	pgxPoolConfig, err := pgxpool.ParseConfig(psqlconn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to parse pgxPoolConfig: %v\n", err)
		os.Exit(1)
	}

	logger := &log.Logger{
		Out:          logFile,
		Formatter:    new(log.JSONFormatter),
		Hooks:        make(log.LevelHooks),
		Level:        log.WarnLevel,
		ExitFunc:     os.Exit,
		ReportCaller: false,
	}

	pgxPoolConfig.ConnConfig.Logger = logrusadapter.NewLogger(logger)

	db, err := pgxpool.ConnectConfig(context.Background(), pgxPoolConfig)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}

	return db, nil
}

// Create connection
func CreateConnection() (*pgxpool.Pool, error) {
	// Read configs
	config, err := utils.ReadConfigs(configsPath)
	if err != nil {
		message := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
		utils.Log("error", message, "api")
		return nil, err
	}

	// Cast port to int
	port, err := strconv.Atoi(config["postgresql_port"])

	if err != nil {
		message := fmt.Sprintf("Can not cast port from configs file to integer. (%s)", err.Error())
		utils.Log("error", message, "api")
		return nil, err
	}

	// Init db connection
	db, err := createPool(config["postgresql_host"], port,
		config["postgresql_user"], utils.EnvVariable(config["postgresql_password"]), config["postgresql_database"])

	if err != nil {
		message := fmt.Sprintf("Can not create database connection. (%s)", err.Error())
		utils.Log("error", message, "api")
		return nil, err
	}

	return db, nil
}
