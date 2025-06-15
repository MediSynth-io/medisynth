package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Init initializes the configuration based on the environment
func Init() (*Config, error) {
	env := os.Getenv("MEDISYNTH_ENV")
	if env == "" {
		env = "dev" // Default to development environment
	}

	configFile := "app.yml"
	if env == "prod" {
		configFile = "app.prod.yml"
	}

	// Get the executable directory
	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}
	execDir := filepath.Dir(execPath)

	// Try to load config from different possible locations
	configPaths := []string{
		filepath.Join(execDir, configFile),          // Next to the binary
		filepath.Join(execDir, "..", configFile),    // One level up
		filepath.Join(".", configFile),              // Current directory
		filepath.Join("/etc/medisynth", configFile), // System-wide config
	}

	var loadErr error
	for _, path := range configPaths {
		cfg, err := LoadConfig(path)
		if err == nil {
			return cfg, nil
		}
		loadErr = err
	}

	return nil, fmt.Errorf("failed to load config from any location: %w", loadErr)
}
