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
	DatabaseType       string `mapstructure:"DB_TYPE"`        // "sqlite" or "postgres"
	DatabasePath       string `mapstructure:"DB_PATH"`        // SQLite file path
	DatabaseSocketPath string `mapstructure:"DB_SOCKET_PATH"` // SQLite socket path
	DatabaseWALMode    bool   `mapstructure:"DB_WAL_MODE"`    // SQLite WAL mode
	DatabaseMaxRetries int    `mapstructure:"DB_MAX_RETRIES"` // SQLite retries
	DatabaseRetryDelay int    `mapstructure:"DB_RETRY_DELAY"` // SQLite retry delay

	// PostgreSQL configuration
	DatabaseHost            string `mapstructure:"DB_HOST"`                    // PostgreSQL host
	DatabasePort            string `mapstructure:"DB_PORT"`                    // PostgreSQL port
	DatabaseName            string `mapstructure:"DB_NAME"`                    // PostgreSQL database name
	DatabaseUser            string `mapstructure:"DB_USER"`                    // PostgreSQL username
	DatabasePassword        string `mapstructure:"DB_PASSWORD"`                // PostgreSQL password
	DatabaseSSLMode         string `mapstructure:"DB_SSL_MODE"`                // PostgreSQL SSL mode
	DatabaseMaxConns        int    `mapstructure:"DB_MAX_CONNECTIONS"`         // PostgreSQL max connections
	DatabaseMaxIdle         int    `mapstructure:"DB_MAX_IDLE_CONNECTIONS"`    // PostgreSQL max idle connections
	DatabaseConnMaxLifetime string `mapstructure:"DB_CONNECTION_MAX_LIFETIME"` // PostgreSQL connection max lifetime

	// Domain configuration (flattened)
	DomainPortal string `mapstructure:"DOMAIN_PORTAL"`
	DomainAPI    string `mapstructure:"DOMAIN_API"`
	DomainSecure bool   `mapstructure:"DOMAIN_SECURE"`

	// DigitalOcean Spaces configuration
	S3Endpoint        string `mapstructure:"S3_ENDPOINT"`          // e.g. https://nyc3.digitaloceanspaces.com
	S3Region          string `mapstructure:"S3_REGION"`            // e.g. nyc3, ams3, sgp1
	S3Bucket          string `mapstructure:"S3_BUCKET"`            // Your DigitalOcean Space name
	S3AccessKeyID     string `mapstructure:"S3_ACCESS_KEY_ID"`     // DigitalOcean Spaces Key
	S3SecretAccessKey string `mapstructure:"S3_SECRET_ACCESS_KEY"` // DigitalOcean Spaces Secret
	S3UseSSL          bool   `mapstructure:"S3_USE_SSL"`
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
	v.SetDefault("DB_TYPE", "sqlite")
	v.SetDefault("DB_PATH", "/data/medisynth.db")
	v.SetDefault("DB_SOCKET_PATH", "/data/sqlite.sock")
	v.SetDefault("DB_WAL_MODE", true)
	v.SetDefault("DB_MAX_RETRIES", 5)
	v.SetDefault("DB_RETRY_DELAY", 5)
	v.SetDefault("DB_HOST", "")
	v.SetDefault("DB_PORT", "")
	v.SetDefault("DB_NAME", "")
	v.SetDefault("DB_USER", "")
	v.SetDefault("DB_PASSWORD", "")
	v.SetDefault("DB_SSL_MODE", "")
	v.SetDefault("DB_MAX_CONNECTIONS", 10)
	v.SetDefault("DB_MAX_IDLE_CONNECTIONS", 5)
	v.SetDefault("DB_CONNECTION_MAX_LIFETIME", "0")
	v.SetDefault("DOMAIN_PORTAL", "portal.medisynth.io")
	v.SetDefault("DOMAIN_API", "api.medisynth.io")
	v.SetDefault("DOMAIN_SECURE", true)
	v.SetDefault("API_URL", "https://api.medisynth.io")
	v.SetDefault("API_INTERNAL_URL", "http://medisynth-api-svc:8081")
	v.SetDefault("S3_ENDPOINT", "https://nyc3.digitaloceanspaces.com")
	v.SetDefault("S3_REGION", "nyc3")
	v.SetDefault("S3_BUCKET", "medisynth-data")
	v.SetDefault("S3_ACCESS_KEY_ID", "")
	v.SetDefault("S3_SECRET_ACCESS_KEY", "")
	v.SetDefault("S3_USE_SSL", true)

	// Explicitly bind environment variables
	envVars := []string{
		"API_PORT", "API_URL", "API_INTERNAL_URL",
		"DB_TYPE", "DB_PATH", "DB_SOCKET_PATH", "DB_WAL_MODE", "DB_MAX_RETRIES", "DB_RETRY_DELAY",
		"DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD", "DB_SSL_MODE",
		"DB_MAX_CONNECTIONS", "DB_MAX_IDLE_CONNECTIONS", "DB_CONNECTION_MAX_LIFETIME",
		"DOMAIN_PORTAL", "DOMAIN_API", "DOMAIN_SECURE",
		"S3_ENDPOINT", "S3_REGION", "S3_BUCKET", "S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY", "S3_USE_SSL",
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
