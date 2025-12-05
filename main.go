// Package main provides the main fuctionality for Surbot.
package main

import (
	"flag"
	"fmt"
	"os"

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

func readFromEnvFile() {
	fmt.Println("Reading .env file")
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("could not read config, %v\n", err.Error())
	}
}

func readEnv(envConfig *envConfig) {
	viper.SetEnvPrefix("sur")
	viper.AutomaticEnv()
	err := viper.BindEnv("token")
	if err != nil {
		fmt.Printf("could not bind variable, %v\n", err.Error())
	}
	err = viper.BindEnv("youtube_api")
	if err != nil {
		fmt.Printf("could not bind variable, %v\n", err.Error())
	}
	err = viper.BindEnv("spotify_clientid")
	if err != nil {
		fmt.Printf("could not bind variable, %v\n", err.Error())
	}
	err = viper.BindEnv("spotify_clientsecret")
	if err != nil {
		fmt.Printf("could not bind variable, %v\n", err.Error())
	}
	envConfig.Token = viper.GetString("token")
	envConfig.YoutubeAPI = viper.GetString("youtube_api")
	envConfig.SpotifyClientID = viper.GetString("spotify_clientid")
	envConfig.SpotifyClientSecret = viper.GetString("spotify_clientsecret")
}

func main() {
	EnvConfigs = new(envConfig)
	if _, err := os.Stat(".env"); err == nil {
		readFromEnvFile()
		if err := viper.Unmarshal(EnvConfigs); err != nil {
			fmt.Printf("could not read envs, %v\n", err.Error())
		}
	} else {
		readEnv(EnvConfigs)
	}
	flag.StringVar(&Prefix, "p", "!", "Bot Prefix")
	flag.Parse()
	fmt.Printf("token: %v\n", EnvConfigs.Token)
	bot := surbot.NewSurbot(EnvConfigs.Token, EnvConfigs.YoutubeAPI, EnvConfigs.SpotifyClientID, EnvConfigs.SpotifyClientSecret, Prefix)
	bot.StartServer()
}
