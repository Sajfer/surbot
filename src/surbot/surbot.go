package surbot

import (
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"gitlab.com/sajfer/surbot/src/logger"
	"gitlab.com/sajfer/surbot/src/utils"
	"gitlab.com/sajfer/surbot/src/youtube"
)

// Surbot contain basic information about the bot
type Surbot struct {
	token   string
	version string
	prefix  string
}

var (
	voiceData = Voice{}
)

// NewSurbot return an instance of surbot
func NewSurbot(token, prefix, version string) Surbot {
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
		voiceData.ChannelID = m.ChannelID
		voiceData.SetSession(s)
		voiceData.NowPlaying()
		return
	}

	if strings.HasPrefix(message, "play") {
		logger.Log.Debugln("Playing music")
		voiceData.ChannelID = m.ChannelID
		voiceData.SetSession(s)
		songUrl := strings.TrimPrefix(message, "play")
		songUrl = strings.ReplaceAll(songUrl, " ", "")

		info, err := youtube.GetVideoInfo(songUrl)
		if err != nil {
			logger.Log.Warningf("could not fetch video information for %s, err= %s", songUrl, err)
			return
		}
		if info.Playlist != nil {
			voiceData.AddPlaylistToQueue(*info.Playlist)
		} else {
			voiceData.AddSongToQueue(*info.Video)
		}

		err = voiceData.Connect(m.Author.ID, m.GuildID, false, true)
		if err != nil {
			logger.Log.Warningf("could not join voice channel, err=%s", err)
			return
		}
		if !voiceData.Playing {
			err = voiceData.Play()
			if err != nil {
				logger.Log.Warningf("Could not play song, err=%s", err)
			}
		}
		return
	}

	if message == "stop" {
		voiceData.ChannelID = m.ChannelID
		voiceData.SetSession(s)
		voiceData.Stop()
	}

	if message == "queue" {
		voiceData.ChannelID = m.ChannelID
		voiceData.SetSession(s)
		err := voiceData.ShowQueue()
		if err != nil {
			logger.Log.Warningf("could not show queue, err=%s", err)
		}
		return
	}

	if message == "skip" {
		voiceData.ChannelID = m.ChannelID
		voiceData.SetSession(s)
		err := voiceData.Skip()
		if err != nil {
			logger.Log.Warning("could not skip song")
		}
		return
	}

	if message == "disconnect" {
		voiceData.ChannelID = m.ChannelID
		voiceData.SetSession(s)
		err := voiceData.Disconnect()
		if err != nil {
			logger.Log.Warning("could not disconnect")
		}
	}

	if message == "clearQueue" {
		voiceData.ChannelID = m.ChannelID
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

func (surbot Surbot) changedChannel(s *discordgo.Session, m *discordgo.VoiceStateUpdate) {

	user, err := s.User(m.UserID)
	if err != nil {
		logger.Log.Warningf("could not find user %s, err=%s", m.UserID, err)
		return
	}

	server, err := s.Guild(m.VoiceState.GuildID)
	if err != nil {
		logger.Log.Warningf("could not find server %s, err=%s", m.VoiceState.GuildID, err)
		return
	}

	logChannel := utils.GetChannel(server, "log")
	if logChannel == nil {
		logger.Log.Warning("could not find log channel")
		return
	}

	avatar := user.AvatarURL("")

	channel, err := s.Channel(m.VoiceState.ChannelID)

	if err != nil {
		embed := &discordgo.MessageEmbed{
			Author: &discordgo.MessageEmbedAuthor{
				Name:    user.Username,
				IconURL: avatar,
			},
			Color:     0xf08080,                        // Red
			Timestamp: time.Now().Format(time.RFC3339), // Discord wants ISO8601; RFC3339 is an extension of ISO8601 and should be completely compatible.
			Title:     "Disconnected",
		}
		_, err = s.ChannelMessageSendEmbed(logChannel.ID, embed)
		if err != nil {
			logger.Log.Warning("could not send message,", err)
		}
		return
	}

	permissions := channel.PermissionOverwrites

	allowedToSee := true

	for _, permission := range permissions {
		role, err := utils.GetRole(s.State, m.VoiceState.GuildID, permission.ID)
		if err != nil {
			logger.Log.Warning("could not get role name")
		}
		if role == "@everyone" {
			if permission.Deny&0x00000400 > 0 { // Check if everyone is allowed to see channel
				allowedToSee = false
			}
		}
	}

	if allowedToSee {
		embed := &discordgo.MessageEmbed{
			Author: &discordgo.MessageEmbedAuthor{
				Name:    user.Username,
				IconURL: avatar,
			},
			Color:     0x00ff00,                        // Green
			Timestamp: time.Now().Format(time.RFC3339), // Discord wants ISO8601; RFC3339 is an extension of ISO8601 and should be completely compatible.
			Title:     "Moved to \"" + channel.Name + "\"",
		}

		_, err := s.ChannelMessageSendEmbed(logChannel.ID, embed)
		if err != nil {
			logger.Log.Warning("could not send message,", err)
		}
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
