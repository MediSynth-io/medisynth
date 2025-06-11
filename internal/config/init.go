package config

import (
	"github.com/spf13/viper"
)

var Cfg AppConfig // This uses AppConfig correctly

// Init should return *AppConfig to be consistent with how LoadConfig was structured
// and to avoid unnecessary copying of the struct.
func Init() (*AppConfig, error) { // Changed return type
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

	var c AppConfig // Use AppConfig here

	err := viper.Unmarshal(&c)
	if err != nil {
		return nil, err // Changed return
	}

	// Set default port if not specified, similar to LoadConfig
	if c.APIPort == 0 {
		c.APIPort = 8080 // Default port
	}

	Cfg = c // Store the loaded config in the global Cfg variable
	return &c, nil
}

// LoadSomething was a placeholder, if you intend to use the global Cfg,
// you might not need this function, or it could be simplified.
// For now, let's assume it's meant to return the globally initialized config.
func LoadSomething() (*AppConfig, error) {
	// This assumes Init() has been called and Cfg is populated.
	// Error handling might be needed if Cfg isn't guaranteed to be initialized.
	return &Cfg, nil
}
