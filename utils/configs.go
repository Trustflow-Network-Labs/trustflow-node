package utils

import (
	"bufio"
	"io"
	"os"
	"strings"
)

type ConfigManager struct {
	configsPath string
}

func NewConfigManager(path string) *ConfigManager {
	if path == "" {
		path = "configs"
	}
	return &ConfigManager{
		configsPath: path,
	}
}

type Config map[string]string

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
