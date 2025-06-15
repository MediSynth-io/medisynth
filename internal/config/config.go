package config

import (
	"log"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	APIPort        int    `mapstructure:"API_PORT"`
	APIURL         string `mapstructure:"API_URL"`
	APIInternalURL string `mapstructure:"API_INTERNAL_URL"`

	// Database configuration (flattened)
	DatabasePath       string `mapstructure:"DB_PATH"`
	DatabaseSocketPath string `mapstructure:"DB_SOCKET_PATH"`
	DatabaseWALMode    bool   `mapstructure:"DB_WAL_MODE"`
	DatabaseMaxRetries int    `mapstructure:"DB_MAX_RETRIES"`
	DatabaseRetryDelay int    `mapstructure:"DB_RETRY_DELAY"`

	// Domain configuration (flattened)
	DomainPortal string `mapstructure:"DOMAIN_PORTAL"`
	DomainAPI    string `mapstructure:"DOMAIN_API"`
	DomainSecure bool   `mapstructure:"DOMAIN_SECURE"`
}

// Database returns a database config struct for backward compatibility
func (c *Config) Database() DatabaseConfig {
	return DatabaseConfig{
		Path:       c.DatabasePath,
		SocketPath: c.DatabaseSocketPath,
		WALMode:    c.DatabaseWALMode,
		MaxRetries: c.DatabaseMaxRetries,
		RetryDelay: c.DatabaseRetryDelay,
	}
}

// Domains returns a domains config struct for backward compatibility
func (c *Config) Domains() DomainsConfig {
	return DomainsConfig{
		Portal: c.DomainPortal,
		API:    c.DomainAPI,
		Secure: c.DomainSecure,
	}
}

type DatabaseConfig struct {
	Path       string
	SocketPath string
	WALMode    bool
	MaxRetries int
	RetryDelay int
}

type DomainsConfig struct {
	Portal string
	API    string
	Secure bool
}

// LoadConfig loads the configuration from environment variables.
func LoadConfig() (*Config, error) {
	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Set defaults
	v.SetDefault("API_PORT", 8081)
	v.SetDefault("DB_PATH", "/data/medisynth.db")
	v.SetDefault("DB_SOCKET_PATH", "/data/sqlite.sock")
	v.SetDefault("DB_WAL_MODE", true)
	v.SetDefault("DB_MAX_RETRIES", 5)
	v.SetDefault("DB_RETRY_DELAY", 5)
	v.SetDefault("DOMAIN_PORTAL", "portal.medisynth.io")
	v.SetDefault("DOMAIN_API", "api.medisynth.io")
	v.SetDefault("DOMAIN_SECURE", true)
	v.SetDefault("API_URL", "https://api.medisynth.io")
	v.SetDefault("API_INTERNAL_URL", "http://medisynth-api-svc:8081")

	// Explicitly bind environment variables
	envVars := []string{
		"API_PORT", "API_URL", "API_INTERNAL_URL",
		"DB_PATH", "DB_SOCKET_PATH", "DB_WAL_MODE", "DB_MAX_RETRIES", "DB_RETRY_DELAY",
		"DOMAIN_PORTAL", "DOMAIN_API", "DOMAIN_SECURE",
	}

	for _, envVar := range envVars {
		if err := v.BindEnv(envVar); err != nil {
			log.Printf("Warning: failed to bind environment variable %s: %v", envVar, err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	log.Printf("Configuration loaded successfully")
	return &cfg, nil
}
