package config

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test cases
	tests := []struct {
		name        string
		configData  string
		envVars     map[string]string
		expectError bool
	}{
		{
			name: "Valid config file",
			configData: `
				apiPort = 8080
				host = "localhost"
			`,
			expectError: false,
		},
		{
			name: "Invalid config file",
			configData: `
				apiPort = "invalid"
			`,
			expectError: true,
		},
		{
			name: "Environment variables override",
			configData: `
				apiPort = 8080
				host = "localhost"
			`,
			envVars: map[string]string{
				"APIPORT": "9090",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create config file
			configPath := filepath.Join(tempDir, "config.toml")
			if err := os.WriteFile(configPath, []byte(tt.configData), 0644); err != nil {
				t.Fatalf("Failed to write config file: %v", err)
			}

			// Set environment variables
			for k, v := range tt.envVars {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}

			// Load config
			cfg, err := LoadConfig(configPath)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Verify config values
			if tt.envVars["APIPORT"] != "" {
				expectedPort, _ := strconv.Atoi(tt.envVars["APIPORT"])
				if cfg.APIPort != expectedPort {
					t.Errorf("Expected port %d, got %d", expectedPort, cfg.APIPort)
				}
			}
		})
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test with non-existent file
	configPath := filepath.Join(tempDir, "nonexistent.toml")
	_, err = LoadConfig(configPath)
	if err == nil {
		t.Error("Expected error for non-existent file, got none")
	}
}

func TestLoadConfigInvalidFile(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create an invalid config file
	configPath := filepath.Join(tempDir, "invalid.toml")
	invalidConfig := `apiPort = "invalid"`

	if err := os.WriteFile(configPath, []byte(invalidConfig), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Try to load the invalid config
	_, err = LoadConfig(configPath)
	if err == nil {
		t.Error("Expected error for invalid config, got none")
	}
}

// Note: Testing Init() directly is harder due to its reliance on viper's global state
// and finding "app.yml" in the execution path. LoadConfig is more testable.
// If Init() is the primary way config is loaded, consider refactoring it
// to be more testable or ensure integration tests cover its behavior.
