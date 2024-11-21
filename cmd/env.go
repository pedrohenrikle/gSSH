package env

import (
	"log"

	"github.com/spf13/viper"
)

type Env struct {
	ClientAddress  string `mapstructure:"CLIENT_ADDRESS"`
	ClientPort     int    `mapstructure:"CLIENT_PORT"`
	ServerAddress  string `mapstructure:"SERVER_ADDRESS"`
	ServerPort     int    `mapstructure:"SERVER_PORT"`
	ServerCertPort int    `mapstructure:"SERVER_CERT_PORT"`
}

func NewEnv() *Env {
	env := Env{}
	viper.SetConfigFile(".env")

	err := viper.ReadInConfig()
	if err != nil {
		log.Fatalf("Couldn't find the file .env: %s", err)
	}

	err = viper.Unmarshal(&env)
	if err != nil {
		log.Fatalf("Environment couldn't be loaded: %s", err)
	}

	return &env
}
