package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
)

type LogsManager struct {
	dir         string
	logFileName string
	logger      *log.Logger
	file        *os.File
	mutex       sync.RWMutex
}

func NewLogsManager() *LogsManager {
	// read configs
	configManager := NewConfigManager("")
	config, err := configManager.ReadConfigs()
	if err != nil {
		panic(err)
	}

	paths := GetAppPaths("")
	lm := &LogsManager{
		dir:         paths.LogDir,
		logFileName: config["logfile"],
		logger:      log.New(),
	}

	// Initialize the log file and logger
	if err := lm.initLogger(); err != nil {
		panic(err)
	}

	return lm
}

func (lm *LogsManager) initLogger() error {
	// Make sure we have os specific path separator
	switch runtime.GOOS {
	case "linux", "darwin":
		lm.logFileName = filepath.ToSlash(lm.logFileName)
	case "windows":
		lm.logFileName = filepath.FromSlash(lm.logFileName)
	default:
		return fmt.Errorf("unsupported OS type `%s`", runtime.GOOS)
	}

	// open log file once
	path := filepath.Join(lm.dir, lm.logFileName)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return err
	}

	lm.file = file

	// Configure logger once
	lm.logger.SetLevel(log.TraceLevel)
	lm.logger.SetOutput(file)
	lm.logger.SetFormatter(&log.JSONFormatter{})

	return nil
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
	// Use read lock for thread-safe access
	lm.mutex.RLock()
	defer lm.mutex.RUnlock()

	// Create log entry with fields
	entry := lm.logger.WithFields(log.Fields{
		"category": category,
		"file":     lm.fileInfo(2),
	})

	// Log message based on level
	switch level {
	case "trace":
		entry.Trace(message)
	case "debug":
		entry.Debug(message)
	case "info":
		entry.Info(message)
	case "warn":
		entry.Warn(message)
	case "error":
		entry.Error(message)
	case "fatal":
		entry.Fatal(message)
	case "panic":
		entry.Panic(message)
	default:
		entry.Info(message)
	}
}

// Close closes the log file - call this when shutting down
func (lm *LogsManager) Close() error {
	lm.mutex.Lock()
	defer lm.mutex.Unlock()

	if lm.file != nil {
		return lm.file.Close()
	}
	return nil
}

// Rotate allows for log rotation - useful for log management
func (lm *LogsManager) Rotate() error {
	lm.mutex.Lock()
	defer lm.mutex.Unlock()

	// Close current file
	if lm.file != nil {
		lm.file.Close()
	}

	// Reinitialize with new file
	return lm.initLogger()
}
