package main

import (
	"flag"
	"os"

	"gitlab.com/sajfer/surbot/pkg/surbot"
)

// Variables used for command line parameters
var (
	Token               string
	Prefix              string
	YoutubeAPI          string
	spotifyClientID     string
	spotifyClientSecret string
)

func main() {
	flag.StringVar(&Token, "t", "", "Bot Token")
	flag.StringVar(&YoutubeAPI, "y", "", "Youtube API token")
	flag.StringVar(&spotifyClientID, "cid", "", "Spotify clientID")
	flag.StringVar(&spotifyClientSecret, "cs", "", "Spotify clientSecretID")
	flag.StringVar(&Prefix, "p", "!", "Bot Prefix")
	flag.Parse()

	if Token == "" {
		Token = os.Getenv("TOKEN")
	}

	if YoutubeAPI == "" {
		YoutubeAPI = os.Getenv("YOUTUBE_API")
	}

	if spotifyClientID == "" {
		spotifyClientID = os.Getenv("SPOTIFY_CLIENTID")
	}

	if spotifyClientSecret == "" {
		spotifyClientSecret = os.Getenv("SPOTIFY_CLIENTSECRET")
	}

	bot := surbot.NewSurbot(Token, YoutubeAPI, spotifyClientID, spotifyClientSecret, Prefix)
	bot.StartServer()
}
