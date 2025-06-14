package config

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary directory for config files
	tempDir, err := os.MkdirTemp("", "config_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Suppress log output during tests
	originalLogger := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(originalLogger)

	t.Run("ValidConfigFile", func(t *testing.T) {
		configContent := `
apiPort: 8081
`
		configFile := filepath.Join(tempDir, "app.yml")
		if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
			t.Fatalf("Failed to write temp config file: %v", err)
		}

		cfg, err := LoadConfig(tempDir)
		if err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}
		if cfg.APIPort != 8081 {
			t.Errorf("Expected APIPort 8081, got %d", cfg.APIPort)
		}
	})

	t.Run("NonExistentConfigFileUsesDefault", func(t *testing.T) {
		// Ensure no app.yml exists in this specific path for the test
		nonExistentConfigDir := filepath.Join(tempDir, "non_existent_config")
		os.Mkdir(nonExistentConfigDir, 0755) // Create the dir but no file

		cfg, err := LoadConfig(nonExistentConfigDir)
		if err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}
		if cfg.APIPort != 8080 { // Default port
			t.Errorf("Expected default APIPort 8080, got %d", cfg.APIPort)
		}
	})

	t.Run("EnvironmentVariableOverride", func(t *testing.T) {
		configContent := `
apiPort: 8081
`
		configFile := filepath.Join(tempDir, "app_env.yml")
		// Create a new viper instance path for this test
		envTestDir := filepath.Join(tempDir, "env_test_dir")
		os.Mkdir(envTestDir, 0755)
		configFileInEnvTestDir := filepath.Join(envTestDir, "app.yml")

		if err := os.WriteFile(configFileInEnvTestDir, []byte(configContent), 0644); err != nil {
			t.Fatalf("Failed to write temp config file: %v", err)
		}

		t.Setenv("APIPORT", "9090") // Viper by default uses uppercase for env vars

		cfg, err := LoadConfig(envTestDir)
		if err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}
		if cfg.APIPort != 9090 {
			t.Errorf("Expected APIPort 9090 from env var, got %d", cfg.APIPort)
		}
		t.Setenv("APIPORT", "") // Clean up env var
	})

	t.Run("MalformedConfigFile", func(t *testing.T) {
		configContent := `
apiPort: 8081
thisis: not: valid: yaml
`
		configFile := filepath.Join(tempDir, "malformed_app.yml")
		malformedConfigDir := filepath.Join(tempDir, "malformed_dir")
		os.Mkdir(malformedConfigDir, 0755)
		malformedConfigFile := filepath.Join(malformedConfigDir, "app.yml")

		if err := os.WriteFile(malformedConfigFile, []byte(configContent), 0644); err != nil {
			t.Fatalf("Failed to write temp config file: %v", err)
		}

		_, err := LoadConfig(malformedConfigDir)
		if err == nil {
			t.Errorf("Expected error for malformed config file, got nil")
		}
		// More specific error checking could be done here if desired
		// e.g., strings.Contains(err.Error(), "unmarshal errors")
	})

	t.Run("NoPortSpecifiedUsesDefault", func(t *testing.T) {
		configContent := `` // Empty config
		configFile := filepath.Join(tempDir, "empty_app.yml")
		emptyConfigDir := filepath.Join(tempDir, "empty_dir")
		os.Mkdir(emptyConfigDir, 0755)
		emptyConfigFile := filepath.Join(emptyConfigDir, "app.yml")

		if err := os.WriteFile(emptyConfigFile, []byte(configContent), 0644); err != nil {
			t.Fatalf("Failed to write temp config file: %v", err)
		}

		cfg, err := LoadConfig(emptyConfigDir)
		if err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}
		if cfg.APIPort != 8080 { // Default port
			t.Errorf("Expected default APIPort 8080 for empty config, got %d", cfg.APIPort)
		}
	})
}

// Note: Testing Init() directly is harder due to its reliance on viper's global state
// and finding "app.yml" in the execution path. LoadConfig is more testable.
// If Init() is the primary way config is loaded, consider refactoring it
// to be more testable or ensure integration tests cover its behavior.
