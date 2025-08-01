package utils

import (
	"bufio"
	"embed"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
