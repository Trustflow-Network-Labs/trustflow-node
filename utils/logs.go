package utils

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	log "github.com/sirupsen/logrus"
)

func fileInfo(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		file = "<???>"
		line = 1
	} else {
		slash := strings.LastIndex(file, "/")
		if slash >= 0 {
			file = file[slash+1:]
		}
	}
	return fmt.Sprintf("%s:%d", file, line)
}

func Log(level string, message string, category string) {
	// provide configs file path
	configsPath := "utils/configs"

	// init config
	config := Config{
		"file": configsPath,
	}

	// read configs
	config, err := ReadConfigs(configsPath)
	if err != nil {
		panic(err)
	}

	// open log file
	file, err := os.OpenFile(config["logfile"], os.O_APPEND|os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
	defer file.Close()

	// set log output to log file
	log.SetOutput(file)

	// set formatter
	log.SetFormatter(&log.JSONFormatter{})

	// log message into a log file
	switch level {
	case "trace":
		log.WithFields(log.Fields{
			"category": category,
			"file":     fileInfo(2),
		}).Trace(message)
	case "debug":
		log.WithFields(log.Fields{
			"category": category,
			"file":     fileInfo(2),
		}).Debug(message)
	case "info":
		log.WithFields(log.Fields{
			"category": category,
			"file":     fileInfo(2),
		}).Info(message)
	case "warn":
		log.WithFields(log.Fields{
			"category": category,
			"file":     fileInfo(2),
		}).Warn(message)
	case "error":
		log.WithFields(log.Fields{
			"category": category,
			"file":     fileInfo(2),
		}).Error(message)
	case "fatal":
		log.WithFields(log.Fields{
			"category": category,
			"file":     fileInfo(2),
		}).Fatal(message)
	case "panic":
		log.WithFields(log.Fields{
			"category": category,
			"file":     fileInfo(2),
		}).Panic(message)
	default:
		log.WithFields(log.Fields{
			"category": category,
			"file":     fileInfo(2),
		}).Info(message)
	}
}
