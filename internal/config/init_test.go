package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInit_Success(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "app.yml")
	os.WriteFile(configPath, []byte("apiPort: 1234\n"), 0644)

	os.Setenv("CONFIG_DIR", dir)
	defer os.Unsetenv("CONFIG_DIR")

	os.Setenv("APIPORT", "5678") // Should override file
	defer os.Unsetenv("APIPORT")

	// viper uses global state, so set config dir
	os.Setenv("config_dir", dir)
	defer os.Unsetenv("config_dir")

	cfg, err := Init()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.APIPort != 5678 {
		t.Errorf("expected APIPort 5678 from env, got %d", cfg.APIPort)
	}
}

func TestInit_MissingFile(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("config_dir", dir)
	defer os.Unsetenv("config_dir")

	_, err := Init()
	if err == nil {
		t.Error("expected error for missing config file, got nil")
	}
}

func TestInit_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "app.yml")
	os.WriteFile(configPath, []byte("apiPort: notanumber\n"), 0644)
	os.Setenv("config_dir", dir)
	defer os.Unsetenv("config_dir")

	_, err := Init()
	if err == nil {
		t.Error("expected error for invalid config file, got nil")
	}
}
