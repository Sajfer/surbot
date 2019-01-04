package main

import (
	"flag"

	"gitlab.com/sajfer/surbot/src/surbot"
)

// Variables used for command line parameters
var (
	Token   string
	Version string
	Prefix  string
)

func init() {

	flag.StringVar(&Token, "t", "", "Bot Token")
	flag.StringVar(&Prefix, "p", "!", "Bot Prefix")
	flag.Parse()

	Version = "1.1.0"
}

func main() {
	surbot := surbot.NewSurbot(Token, Prefix, Version)
	surbot.StartServer()
}
