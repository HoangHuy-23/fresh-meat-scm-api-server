// server/config/config.go
package config

import (
	"github.com/spf13/viper"
)

// --- Định nghĩa các struct con để khớp với cấu trúc của config.yaml ---

type ServerConfig struct {
	Port string `mapstructure:"port"`
}

type SuperAdminConfig struct {
	OrgName  string `mapstructure:"orgName"`
	UserName string `mapstructure:"userName"`
	CertPath string `mapstructure:"certPath"`
	KeyDir   string `mapstructure:"keyDir"`
}

type CAConfig struct {
	URL         string `mapstructure:"url"`
	CaName      string `mapstructure:"caName"`
	TlsCertPath string `mapstructure:"tlsCertPath"`
}

type FabricConfig struct {
	ChannelName       string           `mapstructure:"channelName"`
	ChaincodeName     string           `mapstructure:"chaincodeName"`
	ConnectionProfile string           `mapstructure:"connectionProfile"`
	SuperAdmin        SuperAdminConfig `mapstructure:"superAdmin"`
	CA                CAConfig         `mapstructure:"ca"`
}

// --- Struct Config chính, chứa tất cả các struct con ---

type Config struct {
	Server ServerConfig `mapstructure:"server"`
	Fabric FabricConfig `mapstructure:"fabric"`
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

	// Unmarshal toàn bộ file config vào struct Config chính
	err = viper.Unmarshal(&config)
	return
}