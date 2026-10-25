// Package utils privides utility functions for Surbot.
package utils

import (
	"encoding/json"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"

	"github.com/sajfer/discordgo"
	"gitlab.com/sajfer/surbot/internal/logger"
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

const (
	chuckUrl = "https://api.chucknorris.io/jokes/random"
)

func CheckFileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

func FloatToString(input_num float64) string {
	// to convert a float number to a string
	return strconv.FormatFloat(input_num, 'f', 6, 64)
}

// GetChuckJoke return a chuck norris joke
func GetChuckJoke() string {
	body := GetWebsite(chuckUrl)

	if len(body) == 0 {
		logger.Log.Warning("could not fetch chuck norris joke from api")
		return getFallbackJoke()
	}

	chuckresp := chuckResponse{}
	err := json.Unmarshal(body, &chuckresp)
	if err != nil {
		logger.Log.Warningf("could not parse chuck norris response: %v", err)
		return getFallbackJoke()
	}

	if chuckresp.Value.Joke == "" {
		logger.Log.Warning("chuck joke was empty")
		return getFallbackJoke()
	}

	return chuckresp.Value.Joke
}

// getFallbackJoke returns a fallback joke when the API fails
func getFallbackJoke() string {
	fallbacks := []string{
		"Why did the Chuck Norris joke go to therapy? It had too many existential crises.",
		"Chuck Norris doesn't read jokes. He stares at them until they confess.",
		"What's Chuck Norris's favorite type of music? Heavy metal, because he can crush it.",
	}
	idx := rand.IntN(len(fallbacks)) // #nosec G404
	return fallbacks[idx]
}

// GetWebsite returns the content of a website
func GetWebsite(addr string) []byte {
	response, err := http.Get(addr) // #nosec G107
	if err != nil {
		logger.Log.Warningf("could not get website: %v", err)
		return []byte{}
	}
	if response.Status != "200 OK" {
		logger.Log.Errorf("response status: %s", response.Status)
		return []byte{}
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		logger.Log.Warningf("could not read website body: %v", err)
		return []byte{}
	}
	//nolint:all
	defer response.Body.Close() // #nosec G307

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
