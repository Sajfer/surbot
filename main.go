package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gitlab.com/sajfer/surbot/utils"

	"github.com/bwmarrin/discordgo"
)

// Variables used for command line parameters
var (
	Token   string
	Version string
	Prefix  string
)

func init() {

	flag.StringVar(&Token, "t", "", "Bot Token")
	flag.StringVar(&Prefix, "p", "", "Bot Prefix")
	flag.Parse()

	Version = "0.0.1"
}

// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the autenticated bot has access to.
func messageReceived(s *discordgo.Session, m *discordgo.MessageCreate) {

	// Ignore all messages created by the bot itself
	// This isn't required in this specific example but it's a good practice.
	if m.Author.ID == s.State.User.ID {
		return
	}

	if strings.HasPrefix(m.Content, Prefix) == false {
		return
	}

	message := strings.TrimPrefix(m.Content, Prefix)

	// If the message is "ping" reply with "Pong!"
	if message == "ping" {
		s.ChannelMessageSend(m.ChannelID, "Pong!")
	}

	if message == "version" {
		s.ChannelMessageSend(m.ChannelID, "Version "+Version)
	}

	if message == "help" {
		utils.PrintHelp(s, m.ChannelID)
	}

	if message == "chuck" {
		utils.GetWebsite("http://api.icndb.com/jokes/random")
	}

}

func changedChannel(s *discordgo.Session, m *discordgo.VoiceStateUpdate) {

	t := time.Now()

	user, err := s.User(m.UserID)
	if err != nil {
		fmt.Println("error found no user,", err)
		return
	}

	server, err := s.Guild(m.VoiceState.GuildID)
	if err != nil {
		fmt.Println("error found no server,", err)
		return
	}

	logChannel := utils.GetChannel(server, "log")
	if logChannel == nil {
		fmt.Println("No channel found")
		return
	}

	channel, err := s.Channel(m.VoiceState.ChannelID)
	if err != nil {
		s.ChannelMessageSend(logChannel.ID, "("+t.Format("15:04")+") **"+user.Username+"** have **disconnected**")
		return
	}

	s.ChannelMessageSend(logChannel.ID, "("+t.Format("15:04")+") **"+user.Username+"** have moved to **"+channel.Name+"**")
}

func main() {
	discord, err := discordgo.New("Bot " + Token)
	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}

	// Register the messageCreate func as a callback for MessageCreate events.
	discord.AddHandler(messageReceived)

	discord.AddHandler(changedChannel)

	// Open a websocket connection to Discord and begin listening.
	err = discord.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	discord.Close()
}
