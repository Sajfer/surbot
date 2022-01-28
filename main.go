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
	Token   string
	Version string
	Prefix  string
)

func main() {

	rand.Seed(time.Now().UTC().UnixNano())

	flag.StringVar(&Token, "t", "", "Bot Token")
	flag.StringVar(&Prefix, "p", "!", "Bot Prefix")
	flag.Parse()

	Version = "1.2.0"

	if Token == "" {
		Token = os.Getenv("TOKEN")
	}

	surbot := surbot.NewSurbot(Token, Prefix, Version)
	surbot.StartServer()
}
