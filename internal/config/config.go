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
	Database       struct {
		Path       string `mapstructure:"DB_PATH"`
		SocketPath string `mapstructure:"DB_SOCKET_PATH"`
		WALMode    bool   `mapstructure:"DB_WAL_MODE"`
		MaxRetries int    `mapstructure:"DB_MAX_RETRIES"`
		RetryDelay int    `mapstructure:"DB_RETRY_DELAY"`
	} `mapstructure:"database"`
	Domains struct {
		Portal string `mapstructure:"DOMAIN_PORTAL"`
		API    string `mapstructure:"DOMAIN_API"`
		Secure bool   `mapstructure:"DOMAIN_SECURE"`
	} `mapstructure:"domains"`
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

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	log.Printf("Configuration loaded successfully")
	return &cfg, nil
}
