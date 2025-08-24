package utils

import (
	"bufio"
	"embed"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed configs
var defaultConfig embed.FS

type Config map[string]string

type ConfigManager struct {
	configsPath string
	configs     Config
	configMutex sync.RWMutex
}

func NewConfigManager(path string) *ConfigManager {
	err := ensureConfig()
	if err != nil {
		panic(err)
	}

	if path == "" {
		paths := GetAppPaths("")
		path = filepath.Join(paths.ConfigDir, "configs")
	}

	configs, err := readConfigs(path)
	if err != nil {
		panic(err)
	}

	return &ConfigManager{
		configsPath: path,
		configs:     configs,
	}
}

func ensureConfig() error {
	paths := GetAppPaths("")
	configPath := filepath.Join(paths.ConfigDir, "configs")

	// If config doesn't exist, create it from embedded default
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		data, err := defaultConfig.ReadFile("configs")
		if err != nil {
			return err
		}

		return os.WriteFile(configPath, data, 0644)
	}

	return nil
}

func readConfigs(configsPath string) (Config, error) {
	// init config
	config := Config{
		"file": configsPath,
	}

	// return error if config filepath is not provided
	if len(configsPath) == 0 {
		return nil, fmt.Errorf("invalid configs path `%s`", configsPath)
	}

	// open configs file
	file, err := os.Open(configsPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// instatiate new reader
	reader := bufio.NewReader(file)

	// parse through config file
	for {
		line, err := reader.ReadString('\n')

		// check line for '=' delimiter
		if equal := strings.Index(line, "="); equal >= 0 {
			// extract key
			if key := strings.TrimSpace(line[:equal]); len(key) > 0 {
				// init value
				value := ""
				if len(line) > equal {
					// assign value if not empty
					value = strings.TrimSpace(line[equal+1:])
				}

				// assign the config map
				config[key] = value
			}
		}

		// process errors
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}

	return config, nil
}

func (cm *ConfigManager) GetConfig(key string) (string, bool) {
	cm.configMutex.RLock()
	defer cm.configMutex.RUnlock()

	value, exists := cm.configs[key]
	return value, exists
}

func (cm *ConfigManager) GetConfigWithDefault(key string, defaultValue string) string {
	if value, exists := cm.GetConfig(key); exists {
		return value
	}
	return defaultValue
}

func (cm *ConfigManager) GetAllConfigs() Config {
	cm.configMutex.RLock()
	defer cm.configMutex.RUnlock()

	// Return a copy to prevent external modification
	configsCopy := make(Config)
	maps.Copy(configsCopy, cm.configs)
	return configsCopy
}

// Config reload method
func (cm *ConfigManager) ReloadConfig(path string) error {
	cm.configMutex.Lock()
	defer cm.configMutex.Unlock()

	newConfigs, err := readConfigs(path)
	if err != nil {
		return err
	}

	cm.configs = newConfigs

	return nil
}

// GetConfigDuration parses a duration string from config with default fallback
func (cm *ConfigManager) GetConfigDuration(key string, defaultValue time.Duration) time.Duration {
	valueStr := cm.GetConfigWithDefault(key, defaultValue.String())
	duration, err := time.ParseDuration(valueStr)
	if err != nil {
		fmt.Printf("Invalid duration '%s' for key '%s', using default %v\n", valueStr, key, defaultValue)
		return defaultValue
	}
	return duration
}

// GetConfigInt parses an integer from config with validation
func (cm *ConfigManager) GetConfigInt(key string, defaultValue int, min int, max int) int {
	valueStr := cm.GetConfigWithDefault(key, fmt.Sprintf("%d", defaultValue))
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		fmt.Printf("Invalid integer '%s' for key '%s', using default %d\n", valueStr, key, defaultValue)
		return defaultValue
	}
	if value < min || value > max {
		fmt.Printf("Value %d for key '%s' out of range [%d, %d], using default %d\n", value, key, min, max, defaultValue)
		return defaultValue
	}
	return value
}

// GetConfigInt64 parses an int64 from config with validation
func (cm *ConfigManager) GetConfigInt64(key string, defaultValue int64, min int64, max int64) int64 {
	valueStr := cm.GetConfigWithDefault(key, fmt.Sprintf("%d", defaultValue))
	value, err := strconv.ParseInt(valueStr, 10, 64)
	if err != nil {
		fmt.Printf("Invalid int64 '%s' for key '%s', using default %d\n", valueStr, key, defaultValue)
		return defaultValue
	}
	if value < min || value > max {
		fmt.Printf("Value %d for key '%s' out of range [%d, %d], using default %d\n", value, key, min, max, defaultValue)
		return defaultValue
	}
	return value
}

// GetConfigFloat64 parses a float64 from config with validation
func (cm *ConfigManager) GetConfigFloat64(key string, defaultValue float64, min float64, max float64) float64 {
	valueStr := cm.GetConfigWithDefault(key, fmt.Sprintf("%.1f", defaultValue))
	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		fmt.Printf("Invalid float '%s' for key '%s', using default %.1f\n", valueStr, key, defaultValue)
		return defaultValue
	}
	if value < min || value > max {
		fmt.Printf("Value %.1f for key '%s' out of range [%.1f, %.1f], using default %.1f\n", value, key, min, max, defaultValue)
		return defaultValue
	}
	return value
}

// GetConfigBytes parses a byte size from config (supports units like KB, MB, GB)
func (cm *ConfigManager) GetConfigBytes(key string, defaultValue int64) int64 {
	valueStr := cm.GetConfigWithDefault(key, fmt.Sprintf("%d", defaultValue))
	
	// Try to parse as plain number first
	if value, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
		return value
	}
	
	// Parse with units (case insensitive)
	valueStr = strings.ToLower(strings.TrimSpace(valueStr))
	
	multipliers := map[string]int64{
		"b":  1,
		"kb": 1024,
		"mb": 1024 * 1024,
		"gb": 1024 * 1024 * 1024,
	}
	
	for suffix, multiplier := range multipliers {
		if strings.HasSuffix(valueStr, suffix) {
			numStr := strings.TrimSuffix(valueStr, suffix)
			if num, err := strconv.ParseFloat(numStr, 64); err == nil {
				return int64(num * float64(multiplier))
			}
		}
	}
	
	fmt.Printf("Invalid byte size '%s' for key '%s', using default %d\n", valueStr, key, defaultValue)
	return defaultValue
}
