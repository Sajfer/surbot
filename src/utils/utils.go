package utils

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"

	"github.com/bwmarrin/discordgo"
)

type chuckResponse struct {
	Type  string `json:"type"`
	Value inner  `json:"value"`
}

type inner struct {
	ID         int      `json:"id"`
	Joke       string   `json:"joke"`
	Categories []string `json:"categories"`
}

func RandomString(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

	s := make([]rune, n)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}

// GetChuckJoke return a chuck norris joke
func GetChuckJoke() string {

	body := GetWebsite("http://api.icndb.com/jokes/random")

	chuckresp := chuckResponse{}
	err := json.Unmarshal(body, &chuckresp)
	if err != nil {
		log.Println("Could not get chuck norris joke,", err)
		return ""
	}

	return chuckresp.Value.Joke
}

// GetWebsite returns the content of a website
func GetWebsite(addr string) []byte {
	response, err := http.Get(addr)
	if err != nil {
		log.Println("Could not get website,", err)
		return []byte{}
	}
	if response.Status != "200 OK" {
		log.Println("error status != 200,", response.Status)
		return []byte{}
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Println("Could not read website body,", err)
		return []byte{}
	}
	defer response.Body.Close()

	return body
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
	_, err := s.ChannelMessageSend(channel,
		"Avalible commands: \n"+
			"**help**: Show this command\n"+
			"**ping**: Respods with pong!\n"+
			"**version**: Responds with bot version\n"+
			"**chuck**: Responds with chuck norris joke")
	if err != nil {
		log.Println("error sending message,", err)
	}
}
