// server/config/config.go
package config

import (
	// "strings"
	"github.com/spf13/viper"
)

// --- Các struct con, phản ánh cấu trúc của YAML ---

type ServerConfig struct {
	Port string `mapstructure:"port"`
}

type MongoConfig struct {
	URI    string `mapstructure:"uri"`
	DBName string `mapstructure:"dbName"`
}

type JWTConfig struct {
	Secret      string `mapstructure:"secret"`
	Expiration  string `mapstructure:"expiration"`
}

type FabricConfig struct {
	ChannelName       string `mapstructure:"channelName"`
	ChaincodeName     string `mapstructure:"chaincodeName"`
	OrgName           string `mapstructure:"orgName"`
	UserName          string `mapstructure:"userName"`
	ConnectionProfile string `mapstructure:"connectionProfile"`
	UserCertPath      string `mapstructure:"userCertPath"`
	UserKeyDir        string `mapstructure:"userKeyDir"`
}

type S3Config struct {
	Bucket          string `mapstructure:"bucket"`
	Region          string `mapstructure:"region"`
	AccessKeyID     string `mapstructure:"accessKeyID"`
	SecretAccessKey string `mapstructure:"secretAccessKey"`
	CloudFrontDomain string `mapstructure:"cloudFrontDomain"`
}

type N8NConfig struct {
	DispatchWebhookURL string `mapstructure:"dispatchWebhookURL"`
}

// --- Struct Config chính, bao gồm tất cả các struct con ---

type Config struct {
	Server ServerConfig `mapstructure:"server"`
	N8N    N8NConfig    `mapstructure:"n8n"`
	Mongo  MongoConfig  `mapstructure:"mongo"`
	JWT    JWTConfig    `mapstructure:"jwt"`
	Fabric FabricConfig `mapstructure:"fabric"`
	S3     S3Config     `mapstructure:"s3"`
}

// LoadConfig đọc cấu hình từ file và ghi đè bằng các biến môi trường.
func LoadConfig(path string) (config Config, err error) {
	// Thiết lập đường dẫn và tên file config
	viper.AddConfigPath(path)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	// --- Cấu hình Viper để đọc từ biến môi trường ---
	// 1. Bật tính năng tự động đọc biến môi trường
	viper.AutomaticEnv()

	// 2. Thiết lập bộ thay thế để ánh xạ key
	// Ví dụ: key "mongo.uri" trong YAML sẽ được ánh xạ tới biến môi trường "MONGO_URI"
	// viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	// -------------------------------------------------
	// 3. Thiết lập tiền tố cho biến môi trường (nếu cần)
	// viper.SetEnvPrefix("FMSCM") // Ví dụ: FMSCM_MONGO_URI
	// -------------------------------------------------
	// 4. Đọc biến môi trường với tiền tố (nếu đã thiết lập)
	// viper.AllowEmptyEnv(true) // Cho phép biến môi trường rỗng
	// -------------------------------------------------
	//
	viper.BindEnv("mongo.uri", "MONGO_URI")
	viper.BindEnv("mongo.dbName", "MONGO_DBNAME")
	viper.BindEnv("server.port", "SERVER_PORT")
	viper.BindEnv("jwt.secret", "JWT_SECRET")
	viper.BindEnv("jwt.expiration", "JWT_EXPIRATION")
	viper.BindEnv("s3.bucket", "S3_BUCKET")
	viper.BindEnv("s3.region", "S3_REGION")
	viper.BindEnv("s3.accessKeyID", "S3_ACCESS_KEY_ID")
	viper.BindEnv("s3.secretAccessKey", "S3_SECRET_ACCESS_KEY")
	viper.BindEnv("s3.cloudFrontDomain", "S3_CLOUDFRONT_DOMAIN")
	viper.BindEnv("n8n.dispatchWebhookURL", "N8N_DISPATCH_WEBHOOK_URL")
	// -------------------------------------------------

	// Đọc file config.yaml
	// Nếu file không tồn tại, Viper sẽ chỉ sử dụng các biến môi trường.
	err = viper.ReadInConfig()
	if err != nil {
		// Chỉ trả về lỗi nếu đó không phải là lỗi "không tìm thấy file"
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return
		}
	}

	// Unmarshal toàn bộ cấu hình đã được kết hợp (từ file và env) vào struct Config
	err = viper.Unmarshal(&config)
	if err != nil {
		return
	}

	return
}