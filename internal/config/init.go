package config

import (
	"github.com/spf13/viper"
)

func Init() (Config, error) {
	viper.SetDefault("config_dir", ".")
	viper.AutomaticEnv()

	viper.AddConfigPath(viper.GetString("config_dir"))
	viper.SetConfigName("app")
	viper.SetConfigType("env")

	// if we error trying to read the config file it must need to be generated
	if err := viper.ReadInConfig(); err != nil {
		return Config{}, err
	}

	c := Config{}

	err := viper.Unmarshal(&c)
	if err != nil {
		return Config{}, err
	}

	return c, nil
}
