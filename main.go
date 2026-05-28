// Package main provides the main functionality for Surbot.
package main

import (
	"flag"
	"os"

	"github.com/spf13/viper"

	"gitlab.com/sajfer/surbot/internal/logger"
	"gitlab.com/sajfer/surbot/pkg/surbot"
)

type envConfig struct {
	Token               string `mapstructure:"TOKEN"`
	YoutubeAPI          string `mapstructure:"YOUTUBE_API"`
	SpotifyClientID     string `mapstructure:"SPOTIFY_CLIENTID"`
	SpotifyClientSecret string `mapstructure:"SPOTIFY_CLIENTSECRET"`
}

// readAndInitConfig reads configuration from .env file or environment variables.
func readAndInitConfig() *envConfig {
	config := &envConfig{}

	viper.SetEnvPrefix("sur")
	viper.AutomaticEnv()
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
	viper.AddConfigPath(".")

	if _, err := os.Stat(".env"); err == nil {
		logger.Log.Debug("reading configuration from environment and/or .env file")
		if err := viper.ReadInConfig(); err != nil {
			logger.Log.Errorf("could not read config: %v", err)
			os.Exit(1)
		}
	}

	if err := viper.Unmarshal(config); err != nil {
		logger.Log.Errorf("could not parse config: %v", err)
		os.Exit(1)
	}

	return config
}

func main() {
	envConfigs := readAndInitConfig()

	var (
		prefix = flag.String("p", "!", "Bot Prefix")
	)
	flag.Parse()

	logger.Log.Infof("Starting Surbot with prefix: %s", *prefix)

	bot := surbot.NewSurbot(envConfigs.Token, envConfigs.YoutubeAPI,
		envConfigs.SpotifyClientID, envConfigs.SpotifyClientSecret, *prefix)
	bot.StartServer()
}
