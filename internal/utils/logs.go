package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	yamuxmux "github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-yamux/v5"
	log "github.com/sirupsen/logrus"
)

// RotationInterval defines rotation time intervals
type RotationInterval string

const (
	RotationHourly  RotationInterval = "hourly"
	RotationDaily   RotationInterval = "daily"
	RotationWeekly  RotationInterval = "weekly"
	RotationMonthly RotationInterval = "monthly"
)

// LogRotationConfig holds rotation configuration
type LogRotationConfig struct {
	MaxSizeMB        int64            // Maximum file size in MB before rotation
	MaxAge           int              // Maximum days to retain old logs (0 = keep all)
	MaxBackups       int              // Maximum number of backup files to keep (0 = keep all)
	TimeInterval     RotationInterval // Time-based rotation interval
	EnableRotation   bool             // Enable/disable rotation
}

type LogsManager struct {
	cm              *ConfigManager
	dir             string
	logFileName     string
	logger          *log.Logger
	File            *os.File // allow other packages to use same log output
	mutex           sync.RWMutex
	rotationConfig  LogRotationConfig
	lastRotateCheck time.Time
	fileSize        int64
}

func NewLogsManager(cm *ConfigManager) *LogsManager {
	paths := GetAppPaths("")
	logFileName := cm.GetConfigWithDefault("logfile", "log")

	// Load rotation configuration from config
	rotationConfig := LogRotationConfig{
		MaxSizeMB:      parseConfigInt64(cm.GetConfigWithDefault("log_max_size_mb", "100")),     // 100MB default
		MaxAge:         parseConfigInt(cm.GetConfigWithDefault("log_max_age_days", "30")),       // 30 days default
		MaxBackups:     parseConfigInt(cm.GetConfigWithDefault("log_max_backups", "10")),        // 10 backups default
		TimeInterval:   RotationInterval(cm.GetConfigWithDefault("log_rotation_interval", "daily")), // daily default
		EnableRotation: parseConfigBool(cm.GetConfigWithDefault("log_enable_rotation", "true")), // enabled by default
	}

	lm := &LogsManager{
		cm:              cm,
		dir:             paths.LogDir,
		logFileName:     logFileName,
		logger:          log.New(),
		rotationConfig:  rotationConfig,
		lastRotateCheck: time.Now(),
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

	// Get initial file size
	if stat, err := file.Stat(); err == nil {
		lm.fileSize = stat.Size()
	}

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
	// Check if rotation is needed before writing
	if lm.rotationConfig.EnableRotation {
		lm.checkAndRotate()
	}

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

	// Update file size after write (approximate)
	lm.fileSize += int64(len(message) + 100) // Rough estimate including JSON overhead
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

// checkAndRotate checks if rotation is needed and performs it
func (lm *LogsManager) checkAndRotate() {
	now := time.Now()
	
	// Check size-based rotation
	if lm.rotationConfig.MaxSizeMB > 0 && lm.fileSize > lm.rotationConfig.MaxSizeMB*1024*1024 {
		lm.rotateWithBackup("size")
		return
	}

	// Check time-based rotation (only check every minute to avoid excessive checks)
	if now.Sub(lm.lastRotateCheck) > time.Minute {
		lm.lastRotateCheck = now
		if lm.shouldRotateByTime(now) {
			lm.rotateWithBackup("time")
		}
	}
}

// shouldRotateByTime determines if rotation is needed based on time interval
func (lm *LogsManager) shouldRotateByTime(now time.Time) bool {
	if lm.File == nil {
		return false
	}

	stat, err := lm.File.Stat()
	if err != nil {
		return false
	}

	modTime := stat.ModTime()
	
	switch lm.rotationConfig.TimeInterval {
	case RotationHourly:
		return now.Hour() != modTime.Hour() || now.Day() != modTime.Day()
	case RotationDaily:
		return now.Day() != modTime.Day() || now.Month() != modTime.Month()
	case RotationWeekly:
		_, nowWeek := now.ISOWeek()
		_, modWeek := modTime.ISOWeek()
		return nowWeek != modWeek || now.Year() != modTime.Year()
	case RotationMonthly:
		return now.Month() != modTime.Month() || now.Year() != modTime.Year()
	}
	
	return false
}

// rotateWithBackup performs log rotation with backup management
func (lm *LogsManager) rotateWithBackup(reason string) {
	lm.mutex.Lock()
	defer lm.mutex.Unlock()

	// Generate backup filename with timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	backupFileName := fmt.Sprintf("%s.%s.bak", lm.logFileName, timestamp)
	backupPath := filepath.Join(lm.dir, backupFileName)
	currentPath := filepath.Join(lm.dir, lm.logFileName)

	// Close current file
	if lm.File != nil {
		lm.File.Close()
	}

	// Rename current log to backup
	if _, err := os.Stat(currentPath); err == nil {
		if err := os.Rename(currentPath, backupPath); err != nil {
			// If rename fails, log to stderr and continue
			fmt.Fprintf(os.Stderr, "Failed to create backup %s: %v\n", backupPath, err)
		}
	}

	// Reinitialize with new file
	if err := lm.initLogger(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to reinitialize logger after rotation: %v\n", err)
		return
	}

	// Clean up old backups
	lm.cleanupOldBackups()

	// Log the rotation event to the new file
	lm.logger.WithFields(log.Fields{
		"category": "logrotate",
		"reason":   reason,
		"backup":   backupFileName,
	}).Info("Log rotated")
}

// cleanupOldBackups removes old backup files based on MaxAge and MaxBackups settings
func (lm *LogsManager) cleanupOldBackups() {
	if lm.rotationConfig.MaxAge <= 0 && lm.rotationConfig.MaxBackups <= 0 {
		return // No cleanup configured
	}

	files, err := filepath.Glob(filepath.Join(lm.dir, lm.logFileName+"*.bak"))
	if err != nil {
		return
	}

	// Sort files by modification time (oldest first)
	type fileInfo struct {
		path    string
		modTime time.Time
	}

	var backups []fileInfo
	now := time.Now()

	for _, file := range files {
		if stat, err := os.Stat(file); err == nil {
			// Check age limit
			if lm.rotationConfig.MaxAge > 0 {
				age := now.Sub(stat.ModTime()).Hours() / 24 // days
				if age > float64(lm.rotationConfig.MaxAge) {
					os.Remove(file)
					continue
				}
			}
			
			backups = append(backups, fileInfo{
				path:    file,
				modTime: stat.ModTime(),
			})
		}
	}

	// Remove excess backups if MaxBackups is set
	if lm.rotationConfig.MaxBackups > 0 && len(backups) > lm.rotationConfig.MaxBackups {
		// Sort by modification time (oldest first)
		for i := 0; i < len(backups)-1; i++ {
			for j := i + 1; j < len(backups); j++ {
				if backups[i].modTime.After(backups[j].modTime) {
					backups[i], backups[j] = backups[j], backups[i]
				}
			}
		}

		// Remove oldest files
		excess := len(backups) - lm.rotationConfig.MaxBackups
		for i := 0; i < excess; i++ {
			os.Remove(backups[i].path)
		}
	}
}

// Rotate allows for manual log rotation
func (lm *LogsManager) Rotate() error {
	lm.rotateWithBackup("manual")
	return nil
}

// GetRotationConfig returns current rotation configuration
func (lm *LogsManager) GetRotationConfig() LogRotationConfig {
	return lm.rotationConfig
}

// UpdateRotationConfig updates rotation configuration
func (lm *LogsManager) UpdateRotationConfig(config LogRotationConfig) {
	lm.mutex.Lock()
	defer lm.mutex.Unlock()
	lm.rotationConfig = config
}

/*
* HELPER FUNCTIONS
 */

// parseConfigInt64 safely parses string to int64 with fallback
func parseConfigInt64(value string) int64 {
	if result, err := strconv.ParseInt(value, 10, 64); err == nil {
		return result
	}
	return 100 // fallback default
}

// parseConfigInt safely parses string to int with fallback
func parseConfigInt(value string) int {
	if result, err := strconv.Atoi(value); err == nil {
		return result
	}
	return 30 // fallback default
}

// parseConfigBool safely parses string to bool with fallback
func parseConfigBool(value string) bool {
	if result, err := strconv.ParseBool(value); err == nil {
		return result
	}
	return true // fallback default
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
