package surbot

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gitlab.com/sajfer/surbot/src/utils"

	"github.com/bwmarrin/discordgo"
)

// Surbot contain basic information about the bot
type Surbot struct {
	token   string
	version string
	prefix  string
}

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

	if strings.HasPrefix(m.Content, surbot.prefix) == false {
		return
	}

	message := strings.TrimPrefix(m.Content, surbot.prefix)

	// If the message is "ping" reply with "Pong!"
	if message == "ping" {
		s.ChannelMessageSend(m.ChannelID, "Pong!")
	}

	if message == "version" {
		s.ChannelMessageSend(m.ChannelID, "Version "+surbot.version)
	}

	if message == "help" {
		utils.PrintHelp(s, m.ChannelID)
	}

	if message == "chuck" {
		utils.GetWebsite("http://api.icndb.com/jokes/random")
	}

}

func (surbot Surbot) changedChannel(s *discordgo.Session, m *discordgo.VoiceStateUpdate) {

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
		s.ChannelMessageSendEmbed(logChannel.ID, embed)
		return
	}

	permissions := channel.PermissionOverwrites

	allowedToSee := true

	for _, permission := range permissions {
		role, err := utils.GetRole(s.State, m.VoiceState.GuildID, permission.ID)
		if err != nil {
			fmt.Println("Could not get Role name")
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

		s.ChannelMessageSendEmbed(logChannel.ID, embed)
	}
}

// StartServer connect the server to discord
func (surbot Surbot) StartServer() {
	discord, err := discordgo.New("Bot " + surbot.token)
	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}

	// Register the messageCreate func as a callback for MessageCreate events.
	discord.AddHandler(surbot.messageReceived)

	discord.AddHandler(surbot.changedChannel)

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
