// server/config/config.go
package config

import (
	"github.com/spf13/viper"
)

// Config struct holds all configuration for the application.
type Config struct {
	ServerPort        string `mapstructure:"port"`
	ChannelName       string `mapstructure:"channelName"`
	ChaincodeName     string `mapstructure:"chaincodeName"`
	OrgName           string `mapstructure:"orgName"`
	UserName          string `mapstructure:"userName"`
	ConnectionProfile string `mapstructure:"connectionProfile"`
	UserCertPath      string `mapstructure:"userCertPath"`
	UserKeyDir        string `mapstructure:"userKeyDir"`
}

// LoadConfig reads configuration from file or environment variables.
func LoadConfig(path string) (config Config, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	viper.AutomaticEnv()

	err = viper.ReadInConfig()
	if err != nil {
		return
	}

	// Unmarshal the config into the struct
	var serverConfig struct {
		Server Config `mapstructure:"server"`
	}
	err = viper.Unmarshal(&serverConfig)
	if err != nil {
		return
	}

	var fabricConfig struct {
		Fabric Config `mapstructure:"fabric"`
	}
	err = viper.Unmarshal(&fabricConfig)
	if err != nil {
		return
	}
	
	// Combine the structs
	config = serverConfig.Server
	config.ChannelName = fabricConfig.Fabric.ChannelName
	config.ChaincodeName = fabricConfig.Fabric.ChaincodeName
	config.OrgName = fabricConfig.Fabric.OrgName
	config.UserName = fabricConfig.Fabric.UserName
	config.ConnectionProfile = fabricConfig.Fabric.ConnectionProfile
	config.UserCertPath = fabricConfig.Fabric.UserCertPath
	config.UserKeyDir = fabricConfig.Fabric.UserKeyDir

	return
}