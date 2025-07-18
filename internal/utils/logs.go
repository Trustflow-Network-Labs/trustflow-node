package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	yamuxmux "github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-yamux/v5"
	log "github.com/sirupsen/logrus"
)

type LogsManager struct {
	dir         string
	logFileName string
	logger      *log.Logger
	File        *os.File // allow other packages to use same log output
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

	lm.File = file

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

	if lm.File != nil {
		return lm.File.Close()
	}
	return nil
}

// Rotate allows for log rotation - useful for log management
func (lm *LogsManager) Rotate() error {
	lm.mutex.Lock()
	defer lm.mutex.Unlock()

	// Close current file
	if lm.File != nil {
		lm.File.Close()
	}

	// Reinitialize with new file
	return lm.initLogger()
}

/*
* YAMUX
 */

// Write implements io.Writer interface for yamux
func (lm *LogsManager) Write(p []byte) (n int, err error) {
	message := strings.TrimSpace(string(p))

	// Skip empty messages
	if message == "" {
		return len(p), nil
	}

	// Determine log level based on message content
	level := lm.determineLogLevel(message)

	// Clean up the message (remove timestamp and log prefixes from yamux)
	cleanMessage := lm.cleanYamuxMessage(message)

	// Log using existing logger with "yamux" category
	lm.Log(level, cleanMessage, "yamux")

	return len(p), nil
}

// determineLogLevel analyzes the yamux message to determine appropriate log level
func (lm *LogsManager) determineLogLevel(message string) string {
	messageLower := strings.ToLower(message)

	// Error conditions
	if strings.Contains(messageLower, "error") ||
		strings.Contains(messageLower, "failed") ||
		strings.Contains(messageLower, "panic") ||
		strings.Contains(messageLower, "fatal") {
		return "error"
	}

	// Warning conditions
	if strings.Contains(messageLower, "warn") ||
		strings.Contains(messageLower, "timeout") ||
		strings.Contains(messageLower, "retry") ||
		strings.Contains(messageLower, "disconnect") ||
		strings.Contains(messageLower, "goaway") {
		return "warning"
	}

	// Info conditions
	if strings.Contains(messageLower, "connect") ||
		strings.Contains(messageLower, "accept") ||
		strings.Contains(messageLower, "stream") {
		return "info"
	}

	// Default to debug for yamux internal messages
	return "debug"
}

// cleanYamuxMessage removes yamux prefixes and cleans up the message
func (lm *LogsManager) cleanYamuxMessage(message string) string {
	// Remove common yamux prefixes
	prefixes := []string{
		"[yamux]",
		"[YAMUX]",
		"yamux:",
		"YAMUX:",
	}

	cleanMsg := message
	for _, prefix := range prefixes {
		cleanMsg = strings.TrimPrefix(cleanMsg, prefix)
	}

	// Remove timestamp if present (yamux sometimes adds its own)
	// Pattern: 2023/01/01 12:00:00
	if len(cleanMsg) > 19 && cleanMsg[4] == '/' && cleanMsg[7] == '/' && cleanMsg[10] == ' ' {
		if spaceIndex := strings.Index(cleanMsg[11:], " "); spaceIndex != -1 {
			cleanMsg = cleanMsg[11+spaceIndex+1:]
		}
	}

	return strings.TrimSpace(cleanMsg)
}

// Create a custom yamux configuration with logger
func (lm *LogsManager) CreateYamuxConfigWithLogger() *yamuxmux.Transport {
	// Create new config
	yamuxConfig := &yamux.Config{
		AcceptBacklog:          256,
		EnableKeepAlive:        true,
		KeepAliveInterval:      30 * time.Second,
		ConnectionWriteTimeout: 10 * time.Second,
		MaxStreamWindowSize:    16 * 1024 * 1024, // 16MB stream window
		LogOutput:              lm.File,
	}

	// Convert yamux.Config to yamuxmux.Transport
	return (*yamuxmux.Transport)(yamuxConfig)
}
