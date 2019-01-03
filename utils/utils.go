package utils

import (
	"fmt"
	"net/http"

	"github.com/bwmarrin/discordgo"
)

// GetWebsite returns the content of a website
func GetWebsite(addr string) string {
	response, err := http.Get(addr)
	if err != nil {
		fmt.Println("error found no user,", err)
		return ""
	}
	if response.Status != "200 OK" {
		fmt.Println("error status != 200,", response.Status)
		return ""
	}
	defer response.Body.Close()

	return ""
}

// GetChannel return the channel object of a channel
func GetChannel(server *discordgo.Guild, name string) *discordgo.Channel {
	for _, element := range server.Channels {
		if element.Name == name {
			return element
		}
	}
	return nil
}

// GetRole return the name of a role
func GetRole(state *discordgo.State, guildID, roleID string) (string, error) {
	role, err := state.Role(guildID, roleID)
	if err != nil {
		return "", err
	}
	return role.Name, nil
}

// PrintHelp print the avalible command for the bot
func PrintHelp(s *discordgo.Session, channel string) {
	s.ChannelMessageSend(channel,
		"Avalible commands: \n"+
			"**help**: Show this command\n"+
			"**ping**: Respods with pong!\n"+
			"**version**: Responds with bot version")
}
