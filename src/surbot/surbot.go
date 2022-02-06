package surbot

import (
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"gitlab.com/sajfer/surbot/src/logger"
	"gitlab.com/sajfer/surbot/src/music"
	"gitlab.com/sajfer/surbot/src/utils"
)

// Surbot contain basic information about the bot
type Surbot struct {
	token   string
	version string
	prefix  string
}

var (
	voiceData    *Voice
	musicClients *music.Music
)

// NewSurbot return an instance of surbot
func NewSurbot(token, youtubeAPI, clientID, clientSecret, prefix, version string) Surbot {
	logger.Log.Debug("NewSurbot")
	musicClients = music.NewMusic(youtubeAPI, clientID, clientSecret)
	voiceData = NewVoice(musicClients)
	return Surbot{token: token, prefix: prefix, version: version}
}

// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the autenticated bot has access to.
func (surbot Surbot) messageReceived(s *discordgo.Session, m *discordgo.MessageCreate) {

	// Ignore all messages created by the bot itself
	// This isn't required in this specific example but it's a good practice.
	if m.Author.ID == s.State.User.ID {
		return
	}

	if !strings.HasPrefix(m.Content, surbot.prefix) {
		return
	}

	message := strings.TrimPrefix(m.Content, surbot.prefix)

	// If the message is "ping" reply with "Pong!"
	if message == "ping" {
		_, err := s.ChannelMessageSend(m.ChannelID, "Pong!")
		if err != nil {
			logger.Log.Warning("could not send message,", err)
		}
		return
	}

	if message == "version" {
		_, err := s.ChannelMessageSend(m.ChannelID, "Version "+surbot.version)
		if err != nil {
			logger.Log.Warning("could not send message,", err)
		}
		return
	}

	if message == "help" {
		utils.PrintHelp(s, m.ChannelID)
	}

	if message == "chuck" {
		joke := utils.GetChuckJoke()
		_, err := s.ChannelMessageSend(m.ChannelID, joke)
		if err != nil {
			logger.Log.Warning("could not send message,", err)
		}
		return
	}

	if message == "playing" {
		voiceData.channelID = m.ChannelID
		voiceData.SetSession(s)
		voiceData.NowPlaying()
		return
	}

	if strings.HasPrefix(message, "play") {
		logger.Log.Debugln("Playing music")
		voiceData.channelID = m.ChannelID
		voiceData.SetSession(s)
		query := strings.TrimPrefix(message, "play")
		query = strings.ReplaceAll(query, " ", "")

		err := musicClients.FetchSong(query)
		if err != nil {
			logger.Log.Warningf("could not fetch song information, err=%v", err)
		}

		err = voiceData.Start(m)
		if err != nil {
			logger.Log.Warningf("could not play song, err=%s", err)
		}
		return
	}

	if message == "stop" {
		voiceData.channelID = m.ChannelID
		voiceData.SetSession(s)
		err := voiceData.Stop()
		if err != nil {
			logger.Log.Warningf("could not stop playing song, err=%s", err)
		}
	}

	if message == "queue" {
		voiceData.channelID = m.ChannelID
		voiceData.SetSession(s)
		err := voiceData.ShowQueue()
		if err != nil {
			logger.Log.Warningf("could not show queue, err=%s", err)
		}
		return
	}

	if message == "skip" {
		voiceData.channelID = m.ChannelID
		voiceData.SetSession(s)
		err := voiceData.Skip()
		if err != nil {
			logger.Log.Warning("could not skip song")
		}
		return
	}

	if message == "disconnect" {
		voiceData.channelID = m.ChannelID
		voiceData.SetSession(s)
		err := voiceData.Disconnect()
		if err != nil {
			logger.Log.Warning("could not disconnect")
		}
	}

	if message == "clearQueue" {
		voiceData.channelID = m.ChannelID
		voiceData.SetSession(s)
		err := voiceData.ClearQueue()
		if err != nil {
			logger.Log.Warning("could not clear queue")
		}
	}

	if strings.HasPrefix(message, "roll") {
		dice := strings.TrimPrefix(message, "roll")
		dice = strings.ReplaceAll(dice, " ", "")
		var roll = 0
		switch dice {
		case "d6":
			roll = rand.Intn(6)
		case "d10":
			roll = rand.Intn(10)
		case "d20":
			roll = rand.Intn(20)
		case "d100":
			roll = rand.Intn(100)
		}
		_, err := s.ChannelMessageSend(m.ChannelID, strconv.Itoa(roll))
		if err != nil {
			logger.Log.Warning("could not send message,", err)
		}
		return
	}
}

// StartServer connect the server to discord
func (surbot Surbot) StartServer() {
	discord, err := discordgo.New("Bot " + surbot.token)
	if err != nil {
		logger.Log.Fatal("error creating Discord session,", err)
		return
	}

	// Register the messageCreate func as a callback for MessageCreate events.
	discord.AddHandler(surbot.messageReceived)

	//discord.AddHandler(surbot.changedChannel)

	// Open a websocket connection to Discord and begin listening.
	err = discord.Open()
	if err != nil {
		logger.Log.Fatal("error opening connection,", err)
		return
	}

	// Wait here until CTRL-C or other term signal is received.
	logger.Log.Info("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Cleanly close down the Discord session.
	err = discord.Close()
	if err != nil {
		log.Println("error closing connection,", err)
		return
	}
}
