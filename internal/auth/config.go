package auth

import (
	"time"

	"github.com/spf13/viper"
)

// Config holds the authentication configuration
type Config struct {
	JWTSecretKey     string
	JWTTokenDuration time.Duration
	APIKeyPrefix     string
	MinPasswordLen   int
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		JWTSecretKey:     "your-secret-key", // Should be overridden in production
		JWTTokenDuration: 24 * time.Hour,
		APIKeyPrefix:     "ms_",
		MinPasswordLen:   8,
	}
}

// LoadConfig loads the configuration from environment variables and config files
func LoadConfig() (*Config, error) {
	config := DefaultConfig()

	// Load from environment variables
	viper.SetEnvPrefix("AUTH")
	viper.AutomaticEnv()

	if secret := viper.GetString("JWT_SECRET_KEY"); secret != "" {
		config.JWTSecretKey = secret
	}

	if duration := viper.GetDuration("JWT_TOKEN_DURATION"); duration > 0 {
		config.JWTTokenDuration = duration
	}

	if prefix := viper.GetString("API_KEY_PREFIX"); prefix != "" {
		config.APIKeyPrefix = prefix
	}

	if minLen := viper.GetInt("MIN_PASSWORD_LEN"); minLen > 0 {
		config.MinPasswordLen = minLen
	}

	return config, nil
}
