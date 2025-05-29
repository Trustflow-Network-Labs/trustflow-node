package utils

import (
	"os"
	"path/filepath"
	"runtime"
)

type AppPaths struct {
	ConfigDir string
	DataDir   string
	LogDir    string
	CacheDir  string
}

func GetAppPaths(appName string) *AppPaths {
	paths := &AppPaths{}

	if appName == "" {
		appName = "Trustflow Node"
	}

	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		paths.ConfigDir = filepath.Join(home, "Library", "Application Support", appName)
		paths.DataDir = filepath.Join(home, "Library", "Application Support", appName)
		paths.LogDir = filepath.Join(home, "Library", "Logs", appName)
		paths.CacheDir = filepath.Join(home, "Library", "Caches", appName)
	case "windows":
		appData := os.Getenv("APPDATA")
		paths.ConfigDir = filepath.Join(appData, appName)
		paths.DataDir = filepath.Join(appData, appName)
		paths.LogDir = filepath.Join(appData, appName, "logs")
		paths.CacheDir = filepath.Join(os.Getenv("LOCALAPPDATA"), appName, "cache")
	case "linux":
		home, _ := os.UserHomeDir()

		if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
			paths.ConfigDir = filepath.Join(xdgConfig, appName)
		} else {
			paths.ConfigDir = filepath.Join(home, ".config", appName)
		}

		if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
			paths.DataDir = filepath.Join(xdgData, appName)
		} else {
			paths.DataDir = filepath.Join(home, ".local", "share", appName)
		}

		paths.LogDir = filepath.Join(paths.DataDir, "logs")

		if xdgCache := os.Getenv("XDG_CACHE_HOME"); xdgCache != "" {
			paths.CacheDir = filepath.Join(xdgCache, appName)
		} else {
			paths.CacheDir = filepath.Join(home, ".cache", appName)
		}
	}

	// Create directories
	os.MkdirAll(paths.ConfigDir, 0755)
	os.MkdirAll(paths.DataDir, 0755)
	os.MkdirAll(paths.LogDir, 0755)
	os.MkdirAll(paths.CacheDir, 0755)

	return paths
}
