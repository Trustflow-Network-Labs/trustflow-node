package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	log "github.com/sirupsen/logrus"
)

type LogsManager struct {
	dir string
}

func NewLogsManager() *LogsManager {
	paths := GetAppPaths("")
	return &LogsManager{
		dir: paths.LogDir,
	}
}

func (lm *LogsManager) fileInfo(skip int) string {
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

func (lm *LogsManager) Log(level string, message string, category string) {
	// read configs
	configManager := NewConfigManager("")
	config, err := configManager.ReadConfigs()
	if err != nil {
		panic(err)
	}

	// Make sure we have os specific path separator since we are adding this path to host's path
	logFileName := config["logfile"]
	switch runtime.GOOS {
	case "linux", "darwin":
		logFileName = filepath.ToSlash(logFileName)
	case "windows":
		logFileName = filepath.FromSlash(logFileName)
	default:
		err := fmt.Errorf("unsupported OS type `%s`", runtime.GOOS)
		panic(err)
	}

	// open log file
	path := filepath.Join(lm.dir, logFileName)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
	defer file.Close()

	// Set level
	log.SetLevel(log.TraceLevel)

	// set log output to log file
	log.SetOutput(file)

	// set formatter
	log.SetFormatter(&log.JSONFormatter{})

	// log message into a log file
	switch level {
	case "trace":
		log.WithFields(log.Fields{
			"category": category,
			"file":     lm.fileInfo(2),
		}).Trace(message)
	case "debug":
		log.WithFields(log.Fields{
			"category": category,
			"file":     lm.fileInfo(2),
		}).Debug(message)
	case "info":
		log.WithFields(log.Fields{
			"category": category,
			"file":     lm.fileInfo(2),
		}).Info(message)
	case "warn":
		log.WithFields(log.Fields{
			"category": category,
			"file":     lm.fileInfo(2),
		}).Warn(message)
	case "error":
		log.WithFields(log.Fields{
			"category": category,
			"file":     lm.fileInfo(2),
		}).Error(message)
	case "fatal":
		log.WithFields(log.Fields{
			"category": category,
			"file":     lm.fileInfo(2),
		}).Fatal(message)
	case "panic":
		log.WithFields(log.Fields{
			"category": category,
			"file":     lm.fileInfo(2),
		}).Panic(message)
	default:
		log.WithFields(log.Fields{
			"category": category,
			"file":     lm.fileInfo(2),
		}).Info(message)
	}
}
