// Package surbot contains the main functionality for Surbot.
package surbot

import (
	"crypto/rand"
	"log"
	"math/big"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/sajfer/discordgo"
	"gitlab.com/sajfer/surbot/internal/logger"
	"gitlab.com/sajfer/surbot/internal/utils"
	"gitlab.com/sajfer/surbot/pkg/music"
)

// Surbot contain basic information about the bot
type Surbot struct {
	token        string
	prefix       string
	musicClients *music.MusicClients
	servers      []*Server
}

type Server struct {
	id    string
	voice *Voice
}

// NewSurbot return an instance of surbot
func NewSurbot(token, youtubeAPI, clientID, clientSecret, prefix string) Surbot {
	logger.Log.Debug("NewSurbot")
	musicClients := music.NewMusicClients(youtubeAPI, clientID, clientSecret)
	return Surbot{token: token, prefix: prefix, musicClients: musicClients}
}

// checkServer returns the server configuration of current server
func (surbot *Surbot) checkServer(serverID string) *Server {
	for _, t := range surbot.servers {
		if t.id == serverID {
			return t
		}
	}
	musicClient := music.NewMusic()
	voice := NewVoice(musicClient)
	server := &Server{id: serverID, voice: voice}
	surbot.servers = append(surbot.servers, server)
	return server
}

// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the autenticated bot has access to.
func (surbot *Surbot) messageReceived(s *discordgo.Session, m *discordgo.MessageCreate) {

	// Ignore all messages created by the bot itself
	// This isn't required in this specific example but it's a good practice.
	if m.Author.ID == s.State.User.ID {
		return
	}

	if !strings.HasPrefix(m.Content, surbot.prefix) {
		return
	}

	server := surbot.checkServer(m.GuildID)
	message := strings.TrimPrefix(m.Content, surbot.prefix)

	// If the message is "ping" reply with "Pong!"
	if message == "ping" {
		_, err := s.ChannelMessageSend(m.ChannelID, "Pong!")
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
		voice := server.voice
		voice.channelID = m.ChannelID
		voice.SetSession(s)
		voice.NowPlaying()
		return
	}

	if message == "shuffle" {
		server.voice.music.Shuffle()
		return
	}

	if strings.HasPrefix(message, "play") {
		logger.Log.Debugln("Playing music")
		voice := server.voice
		voice.channelID = m.ChannelID
		voice.SetSession(s)
		query := strings.TrimPrefix(message, "play")
		query = strings.ReplaceAll(query, " ", "")

		playlist, err := surbot.musicClients.FetchSong(query)
		if err != nil {
			logger.Log.Warningf("could not fetch song information, err=%v", err)
		}
		err = voice.music.AddToQueue(*playlist)
		if err != nil {
			logger.Log.Warningf("could not add songs to playlist, err=%v", err)
		}

		err = voice.Start(m)
		if err != nil {
			logger.Log.Warningf("could not play song, err=%s", err)
		}
		return
	}

	if message == "stop" {
		voice := server.voice
		voice.channelID = m.ChannelID
		voice.SetSession(s)
		err := voice.Stop()
		if err != nil {
			logger.Log.Warningf("could not stop playing song, err=%s", err)
		}
	}

	if message == "queue" {
		voice := server.voice
		voice.channelID = m.ChannelID
		voice.SetSession(s)
		err := voice.ShowQueue()
		if err != nil {
			logger.Log.Warningf("could not show queue, err=%s", err)
		}
		return
	}

	if message == "skip" {
		voice := server.voice
		voice.channelID = m.ChannelID
		voice.SetSession(s)
		err := voice.Skip()
		if err != nil {
			logger.Log.Warning("could not skip song")
		}
		return
	}

	if message == "disconnect" {
		voice := server.voice
		voice.channelID = m.ChannelID
		voice.SetSession(s)
		err := voice.Disconnect()
		if err != nil {
			logger.Log.Warning("could not disconnect")
		}
	}

	if message == "clearQueue" {
		voice := server.voice
		voice.channelID = m.ChannelID
		voice.SetSession(s)
		err := voice.ClearQueue()
		if err != nil {
			logger.Log.Warning("could not clear queue")
		}
	}
	if message == "rajd" {
		currentTime := time.Now()
		rajdChannel := "1006248135737221251"
		for {
			msg, err := s.ChannelMessageSend(rajdChannel, currentTime.Format("Monday 01/02"))
			if err != nil {
				logger.Log.Warning("could not send message,", err)
			}
			err = s.MessageReactionAdd(rajdChannel, msg.ID, "✅")
			if err != nil {
				logger.Log.Warning("could not add emote,", err)
			}
			err = s.MessageReactionAdd(rajdChannel, msg.ID, "❌")
			if err != nil {
				logger.Log.Warning("could not add emote,", err)
			}
			currentTime = currentTime.AddDate(0, 0, 1)
			if currentTime.Format("Monday") == "Wednesday" {
				break
			}
		}
		return
	}
	if strings.HasPrefix(message, "roll") {
		dice := strings.TrimPrefix(message, "roll")
		dice = strings.ReplaceAll(dice, " ", "")
		var roll *big.Int
		var err error
		switch dice {
		case "d6":
			roll, err = rand.Int(rand.Reader, big.NewInt(6)) // #nosec G404
			if err != nil {
				logger.Log.Warning("could not generate random number,", err)
			}
		case "d10":
			roll, err = rand.Int(rand.Reader, big.NewInt(10)) // #nosec G404
			if err != nil {
				logger.Log.Warning("could not generate random number,", err)
			}
		case "d20":
			roll, err = rand.Int(rand.Reader, big.NewInt(20)) // #nosec G404
			if err != nil {
				logger.Log.Warning("could not generate random number,", err)
			}
		case "d100":
			roll, err = rand.Int(rand.Reader, big.NewInt(100)) // #nosec G404
			if err != nil {
				logger.Log.Warning("could not generate random number,", err)
			}
		}
		_, err = s.ChannelMessageSend(m.ChannelID, roll.String())
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
