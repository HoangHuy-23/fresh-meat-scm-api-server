// config/config.go
package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	ServerPort        string `mapstructure:"port"`
	ChannelName       string `mapstructure:"channelName"`
	ChaincodeName     string `mapstructure:"chaincodeName"`
	OrgName           string `mapstructure:"orgName"`
	UserName          string `mapstructure:"userName"`
	ConnectionProfile string `mapstructure:"connectionProfile"`
	CryptoPath        string `mapstructure:"cryptoPath"`
	UserCertPath      string `mapstructure:"userCertPath"`
	UserKeyDir        string `mapstructure:"userKeyDir"`
}

func LoadConfig(path string) (config Config, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	viper.AutomaticEnv()

	err = viper.ReadInConfig()
	if err != nil {
		return
	}

	err = viper.UnmarshalKey("server", &config)
	if err != nil {
		return
	}
	err = viper.UnmarshalKey("fabric", &config)
	return
}
