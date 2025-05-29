package utils

import (
	"bufio"
	"embed"
	"io"
	"os"
	"path/filepath"
	"strings"
)

//go:embed configs
var defaultConfig embed.FS

type ConfigManager struct {
	configsPath string
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

	return &ConfigManager{
		configsPath: path,
	}
}

type Config map[string]string

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

func (cm *ConfigManager) ReadConfigs() (Config, error) {
	// init config
	config := Config{
		"file": cm.configsPath,
	}

	// return error if config filepath is not provided
	if len(cm.configsPath) == 0 {
		return config, nil
	}

	// open configs file
	file, err := os.Open(cm.configsPath)
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
