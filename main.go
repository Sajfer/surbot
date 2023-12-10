// Package main provides the main fuctionality for Surbot.
package main

import (
	"flag"
	"fmt"

	"github.com/spf13/viper"

	"gitlab.com/sajfer/surbot/pkg/surbot"
)

type envConfig struct {
	Token               string `mapstructure:"TOKEN"`
	YoutubeAPI          string `mapstructure:"YOUTUBE_API"`
	SpotifyClientID     string `mapstructure:"SPOTIFY_CLIENTID"`
	SpotifyClientSecret string `mapstructure:"SPOTIFY_CLIENTSECRET"`
}

// Variables used for command line parameters
var (
	Prefix     string
	EnvConfigs *envConfig
)

func main() {
	EnvConfigs = new(envConfig)
	viper.SetConfigFile(".env")
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("could not read config, %v", err.Error())
	}
	if err := viper.Unmarshal(EnvConfigs); err != nil {
		fmt.Printf("could not read envs, %v", err.Error())
	}
	flag.StringVar(&Prefix, "p", "!", "Bot Prefix")
	flag.Parse()

	fmt.Printf("token: %v", EnvConfigs.Token)
	bot := surbot.NewSurbot(EnvConfigs.Token, EnvConfigs.YoutubeAPI, EnvConfigs.SpotifyClientID, EnvConfigs.SpotifyClientSecret, Prefix)
	bot.StartServer()
}
