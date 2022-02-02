package main

import (
	"flag"
	"math/rand"
	"os"
	"time"

	"gitlab.com/sajfer/surbot/src/surbot"
)

// Variables used for command line parameters
var (
	Token      string
	Version    string
	Prefix     string
	YoutubeAPI string
)

func main() {

	rand.Seed(time.Now().UTC().UnixNano())

	flag.StringVar(&Token, "t", "", "Bot Token")
	flag.StringVar(&YoutubeAPI, "y", "", "Youtube API token")
	flag.StringVar(&Prefix, "p", "!", "Bot Prefix")
	flag.Parse()

	Version = "1.2.0"

	if Token == "" {
		Token = os.Getenv("TOKEN")
	}

	if YoutubeAPI == "" {
		YoutubeAPI = os.Getenv("YOUTUBE_API")
	}

	surbot := surbot.NewSurbot(Token, YoutubeAPI, Prefix, Version)
	surbot.StartServer()
}
