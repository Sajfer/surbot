package utils

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"strconv"

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

func zeroPad(str string) (result string) {
	if len(str) < 2 {
		result = "0" + str
	} else {
		result = str
	}
	return
}

func SecondsToHuman(input float64) (result string) {
	hours := math.Floor(float64(input) / 60 / 60)
	seconds := int(input) % (60 * 60)
	minutes := math.Floor(float64(seconds) / 60)
	seconds = int(input) % 60

	if hours > 0 {
		result = strconv.Itoa(int(hours)) + ":" + zeroPad(strconv.Itoa(int(minutes))) + ":" + zeroPad(strconv.Itoa(int(seconds)))
	} else {
		result = zeroPad(strconv.Itoa(int(minutes))) + ":" + zeroPad(strconv.Itoa(int(seconds)))
	}

	return
}

func CheckFileExists(filename string) bool {
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return true
}

func FloatToString(input_num float64) string {
	// to convert a float number to a string
	return strconv.FormatFloat(input_num, 'f', 6, 64)
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
			"**chuck**: Responds with chuck norris joke\n"+
			"**play**: Play a youtube link\n"+
			"**stop**: Stop playing music\n"+
			"**queue**: Show the queue of music")
	if err != nil {
		log.Println("error sending message,", err)
	}
}
