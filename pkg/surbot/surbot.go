// Package surbot contains the main functionality for Surbot.
package surbot

import (
	"crypto/rand"
	"fmt"
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

// Surbot contains basic information about the bot
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

// NewSurbot returns an instance of Surbot
func NewSurbot(token, youtubeAPI, clientID, clientSecret, prefix string) Surbot {
	logger.Log.Debug("NewSurbot")
	musicClients := music.NewMusicClients(youtubeAPI, clientID, clientSecret)
	return Surbot{token: token, prefix: prefix, musicClients: musicClients}
}

// checkServer returns the server configuration for current server
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

// messageReceived handles incoming Discord messages
func (surbot *Surbot) messageReceived(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Check if message has our prefix
	if !strings.HasPrefix(m.Content, surbot.prefix) {
		return
	}

	server := surbot.checkServer(m.GuildID)
	// Ensure the voice has a reference to the Discord session.
	server.voice.SetSession(s)
	command := strings.TrimPrefix(m.Content, surbot.prefix)

	surbot.dispatchCommand(s, m.ChannelID, server.voice, m, command)
}

// dispatchCommand routes the command to its appropriate handler and sends responses
// All handlers return either a message string (empty = no response needed) or error.
func (surbot *Surbot) dispatchCommand(s *discordgo.Session, channelID string, voice *Voice, m *discordgo.MessageCreate, command string) {
	switch {
	case command == "ping":
		surbot.HandlePing(voice, s, channelID)
	case command == "help":
		surbot.HandleHelp()
	case command == "chuck":
		message := surbot.HandleChuck(voice, s, channelID)
		if message != "" {
			logger.Log.Debug("sending chuck response:", message)
			_, _ = s.ChannelMessageSend(channelID, message)
		}
	case command == "playing":
		surbot.HandlePlayingStatus(voice, s)
	case command == "shuffle":
		surbot.HandleShuffle(voice)
	case strings.HasPrefix(command, "play"):
		message := surbot.HandlePlay(voice, s, channelID, m, command)
		if message != "" {
			logger.Log.Debug("sending play response:", message)
			_, _ = s.ChannelMessageSend(channelID, message)
		}
	case command == "stop":
		surbot.HandleStop(voice)
	case command == "queue":
		message := surbot.HandleQueue(voice)
		if message != "" {
			logger.Log.Debug("sending queue response:", message)
			_, _ = s.ChannelMessageSend(channelID, message)
		}
	case command == "skip":
		surbot.HandleSkip(voice)
	case command == "disconnect":
		surbot.HandleDisconnect(voice)
	case command == "clearQueue":
		surbot.HandleClearQueue(voice)
	case command == "rajd":
		surbot.HandleRajd(s, channelID, voice)
	case strings.HasPrefix(command, "roll"):
		message := surbot.HandleRoll(voice, s, channelID, command)
		if message != "" {
			logger.Log.Debug("sending roll response:", message)
			_, _ = s.ChannelMessageSend(channelID, message)
		}
	}
}

// HandlePing returns the pong message
func (surbot *Surbot) HandlePing(voice *Voice, s *discordgo.Session, channelID string) {
	logger.Log.Debug("handling ping command")
	message := "Pong!"
	_, _ = s.ChannelMessageSend(channelID, message)
}

// HandleHelp shows the help menu to users
func (surbot *Surbot) HandleHelp() {
	logger.Log.Debug("showing help command")
	utils.PrintHelp(nil, "")
}

// HandleChuck returns a Chuck Norris joke
func (surbot *Surbot) HandleChuck(voice *Voice, s *discordgo.Session, channelID string) string {
	logger.Log.Debug("handling chuck command")
	return utils.GetChuckJoke()
}

// HandlePlayingStatus checks and displays playing status
func (surbot *Surbot) HandlePlayingStatus(voice *Voice, s *discordgo.Session) {
	logger.Log.Debug("checking playing status")
	// voice.channelID already set when connected to channel
	voice.SetSession(s)
	voice.NowPlaying()
}

// HandleShuffle shuffles the music queue
func (surbot *Surbot) HandleShuffle(voice *Voice) {
	logger.Log.Debug("shuffling queue")
	voice.music.Shuffle()
}

// HandlePlay fetches song and adds to queue, returns error message if failed
func (surbot *Surbot) HandlePlay(voice *Voice, s *discordgo.Session, channelID string, m *discordgo.MessageCreate, command string) string {
	logger.Log.Debug("handling play command")

	query := strings.TrimPrefix(command, "play")
	query = strings.ReplaceAll(query, " ", "")

	playlist, err := surbot.musicClients.FetchSong(query)
	if err != nil {
		return "Failed to fetch song information"
	}

	err = voice.music.AddToQueue(*playlist)
	if err != nil {
		return "Failed to add songs to playlist"
	}

	err = voice.Start(m)
	if err != nil {
		return "Failed to play song"
	}

	return "" // Success, no message needed
}

// HandleStop stops the current playback
func (surbot *Surbot) HandleStop(voice *Voice) {
	logger.Log.Debug("stopping music")
	err := voice.Stop()
	if err != nil {
		logger.Log.Warningf("failed to stop song: %v", err)
	}
}

// HandleQueue displays the current queue and returns formatted message
func (surbot *Surbot) HandleQueue(voice *Voice) string {
	logger.Log.Debug("showing queue")

	songList := ""
	var index = 1

	if voice.music.CurrentSong != nil {
		songList += fmt.Sprintf("%d. %s - %s", index, voice.music.CurrentSong.Title, voice.music.CurrentSong.Artist)
		index++
	} else if len(voice.music.Queue) == 0 {
		return "No songs queued. Use !play <youtube link|spotify link> to queue a song"
	}

	for i, song := range voice.music.Queue {
		if i > 18 {
			break
		}
		songList += fmt.Sprintf("%d. %s", index, song.Title)
		index++
	}

	return "Queue: " + songList
}

// HandleSkip skips the current song
func (surbot *Surbot) HandleSkip(voice *Voice) {
	logger.Log.Debug("skipping song")
	err := voice.Skip()
	if err != nil {
		logger.Log.Warning("failed to skip song")
	}
}

// HandleDisconnect disconnects from voice channel
func (surbot *Surbot) HandleDisconnect(voice *Voice) {
	logger.Log.Debug("disconnecting from voice channel")
	err := voice.Disconnect()
	if err != nil {
		logger.Log.Warning("failed to disconnect")
	}
}

// HandleClearQueue clears the music queue
func (surbot *Surbot) HandleClearQueue(voice *Voice) {
	logger.Log.Debug("clearing queue")
	voice.music.Queue = nil
	err := voice.ClearQueue()
	if err != nil {
		logger.Log.Warning("failed to clear queue")
	}
}

// HandleRajd posts the current date and reactions in Rajda channel (Wednesdays only)
func (surbot *Surbot) HandleRajd(s *discordgo.Session, channelID string, voice *Voice) {
	logger.Log.Debug("posting rajda message")

	currentTime := time.Now()
	rajdChannel := "1006248135737221251"

	for {
		msg, err := s.ChannelMessageSend(rajdChannel, currentTime.Format("Monday 01/02"))
		if err != nil {
			logger.Log.Warningf("failed to send message to Rajda: %v", err)
			return
		}

		err = s.MessageReactionAdd(rajdChannel, msg.ID, "✅")
		if err != nil {
			logger.Log.Warningf("failed to add reaction ✅: %v", err)
		}
		err = s.MessageReactionAdd(rajdChannel, msg.ID, "❌")
		if err != nil {
			logger.Log.Warningf("failed to add reaction ❌: %v", err)
		}

		currentTime = currentTime.AddDate(0, 0, 1)
		if currentTime.Format("Monday") == "Wednesday" {
			break
		}
	}
}

// HandleRoll rolls dice of specified type (d6, d10, d20, or d100) and returns the result
func (surbot *Surbot) HandleRoll(voice *Voice, s *discordgo.Session, channelID string, command string) string {
	logger.Log.Debug("handling roll command")

	// Parse die type from command (e.g., "d6", "d20")
	diceType := strings.TrimPrefix(command, "roll")

	switch diceType {
	case "d6":
		result, err := rand.Int(rand.Reader, big.NewInt(6))
		if err != nil {
			logger.Log.Warningf("could not generate random number: %v", err)
			return "failed to roll dice!"
		}
		return result.String()
	case "d10":
		result, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			logger.Log.Warningf("could not generate random number: %v", err)
			return "failed to roll dice!"
		}
		return result.String()
	case "d20":
		result, err := rand.Int(rand.Reader, big.NewInt(20))
		if err != nil {
			logger.Log.Warningf("could not generate random number: %v", err)
			return "failed to roll dice!"
		}
		return result.String()
	case "d100":
		result, err := rand.Int(rand.Reader, big.NewInt(100))
		if err != nil {
			logger.Log.Warningf("could not generate random number: %v", err)
			return "failed to roll dice!"
		}
		return result.String()
	default:
		logger.Log.Warningf("unknown die type: %s", diceType)
		return "!roll d6 or !roll d20"
	}
}

// StartServer starts the Discord bot
func (surbot Surbot) StartServer() {
	discord, err := discordgo.New("Bot " + surbot.token)
	if err != nil {
		logger.Log.WithError(err).Fatal("failed to create Discord session")
		return
	}

	// Register the messageCreate handler
	discord.AddHandler(surbot.messageReceived)

	// Open a websocket connection to Discord and begin listening
	err = discord.Open()
	if err != nil {
		logger.Log.WithError(err).Fatal("failed to open Discord connection")
		return
	}

	// Wait here until CTRL-C or other term signal is received
	logger.Log.Info("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Cleanly close down the Discord session
	err = discord.Close()
	if err != nil {
		logger.Log.WithError(err).Warning("failed to close connection")
		return
	}
}
