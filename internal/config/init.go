package config

import (
	"github.com/spf13/viper"
)

func Init() (*Config, error) { // Changed return type
	viper.SetDefault("config_dir", ".") // Assuming app.yml is in the execution directory
	viper.AutomaticEnv()

	viper.AddConfigPath(viper.GetString("config_dir"))
	viper.SetConfigName("app") // Name of config file (without extension)
	viper.SetConfigType("yml") // Type of config file (yaml for app.yml)

	if err := viper.ReadInConfig(); err != nil {
		// It's better to return the error and let the caller decide if it's fatal
		// or if defaults should be used.
		// For now, we'll return the error.
		return nil, err // Changed return
	}

	var c Config // Use AppConfig here

	err := viper.Unmarshal(&c)
	if err != nil {
		return nil, err // Changed return
	}

	// Set default port if not specified, similar to LoadConfig
	if c.APIPort == 0 {
		c.APIPort = 8080 // Default port
	}

	return &c, nil
}
