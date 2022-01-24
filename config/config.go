package config

import "github.com/spf13/viper"

var Default = &Config{}

type Config struct {
	Addr    string  `json:"addr" mapstructure:"addr"`
	WeChat  WeChat  `json:"wechat" mapstructure:"wechat"`
	Tencent Tencent `json:"tencent" mapstructure:"tencent"`
}

type WeChat struct {
	AppID          string `json:"app_id" mapstructure:"app_id"`
	AppSecret      string `json:"app_secret" mapstructure:"app_secret"`
	Token          string `json:"token" mapstructure:"token"`
	EncodingAESKey string `json:"encoding_aes_key" mapstructure:"encoding_aes_key"`
}

type Tencent struct {
	SecretID  string `json:"secret_id" mapstructure:"secret_id"`
	SecretKey string `json:"secret_key" mapstructure:"secret_key"`
}

func init() {
	viper.SetConfigType("json")
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err != nil {
		panic(any("read config: " + err.Error()))
	}

	if err := viper.Unmarshal(Default); err != nil {
		panic(any("unmarshal config: " + err.Error()))
	}
}
