package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/MediSynth-io/medisynth/internal/config"
	"github.com/stretchr/testify/assert"
)

// Mock for config.Init
func mockConfigInit() (*config.Config, error) {
	return nil, errors.New("mock error")
}

func TestInitializeAPI(t *testing.T) {
	// Create a temporary directory for the config file
	dir := t.TempDir()
	configPath := filepath.Join(dir, "app.yml")

	// Write test config
	configContent := []byte(`
apiPort: 8080
synthea:
  path: /path/to/synthea
  output: /path/to/output
`)
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Set config directory before any Viper operations
	os.Setenv("CONFIG_DIR", dir)
	defer os.Unsetenv("CONFIG_DIR")

	// Test successful initialization
	api, err := initializeAPI()
	assert.NoError(t, err)
	assert.NotNil(t, api)

	// Test error handling
	originalConfigInit := configInit
	defer func() { configInit = originalConfigInit }()

	configInit = func() (*config.Config, error) {
		return nil, assert.AnError
	}

	api, err = initializeAPI()
	assert.Error(t, err)
	assert.Nil(t, api)
}
