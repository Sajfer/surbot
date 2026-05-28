// Package utils privides utility functions for Surbot.
package utils

import (
	"encoding/json"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

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

var (
	durationRegex = `P(?P<years>\d+Y)?(?P<months>\d+M)?(?P<days>\d+D)?T?(?P<hours>\d+H)?(?P<minutes>\d+M)?(?P<seconds>\d+S)?`

	ytUrlRegex         = `^(?:https?\:\/\/)?(?:www\.)?(?:(?:youtube\.com\/watch\?v=)|(?:youtu\.be\/))([a-zA-Z0-9\-_]{11})+.*$|^(?:https:\/\/www\.youtube\.com\/playlist\?list=)([a-zA-Z0-9\-_].*).*$`
	ytPlaylistUrlRegex = `^(?:https:\/\/www\.youtube\.com\/playlist\?list=)([a-zA-Z0-9\-_]{34}).*$`
	ytTrackUrlRegex    = `^(?:https?\:\/\/)?(?:www\.)?(?:(?:youtube\.com\/watch\?v=)|(?:youtu\.be\/))([a-zA-Z0-9\-_]{11})+.*$`

	spotifyHttpUrlRegex      = `^(?:https?:\/\/open\.spotify\.com\/(?:playlist\/|album\/|track\/)([a-zA-Z0-9]+))(?:.*)`
	spotifyHttpPlaylistRegex = `^(https:\/\/open\.spotify\.com\/playlist\/[[a-zA-Z0-9]{22}\?.*)$`
	spotifyHttpAlbumRegex    = `^(https:\/\/open\.spotify\.com\/album\/[[a-zA-Z0-9]{22}\?.*)$`
	spotifyHttpTrackRegex    = `^(https:\/\/open\.spotify\.com\/track\/[[a-zA-Z0-9]{22}\?.*)$`
)

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

func FormatVideoTitle(videoTitle string) string {
	newTitle := strings.TrimSpace(videoTitle)

	stringReplacer := strings.NewReplacer("/", "_", "-", "_", ",", "_", " ", "", "'", "")

	newTitle = stringReplacer.Replace(newTitle)
	//videoFileFullPath := path.Join(BASESONGPATH, newTitle)

	return newTitle
}

func IsYoutubeUrl(url string) bool {
	re := regexp.MustCompile(ytUrlRegex)
	return re.MatchString(url)
}

func IsYoutubeTrackUrl(url string) bool {
	re := regexp.MustCompile(ytTrackUrlRegex)
	return re.MatchString(url)
}

func IsYoutubePlaylistUrl(url string) bool {
	re := regexp.MustCompile(ytPlaylistUrlRegex)
	return re.MatchString(url)
}

func IsSpotifyUrl(url string) bool {
	re := regexp.MustCompile(spotifyHttpUrlRegex)
	return re.MatchString(url)
}

func IsSpotifyTrackUrl(url string) bool {
	re := regexp.MustCompile(spotifyHttpTrackRegex)
	return re.MatchString(url)
}

func IsSpotifyAlbumUrl(url string) bool {
	re := regexp.MustCompile(spotifyHttpAlbumRegex)
	return re.MatchString(url)
}

func IsSpotifyPlaylistUrl(url string) bool {
	re := regexp.MustCompile(spotifyHttpPlaylistRegex)
	return re.MatchString(url)
}

func GetSpotifyID(url string) string {
	re := regexp.MustCompile(spotifyHttpUrlRegex)
	matches := re.FindStringSubmatch(url)
	if matches == nil {
		return ""
	}
	return matches[1]
}

func ParseISO8601(duration string) string {
	r, err := regexp.Compile(durationRegex)
	if err != nil {
		logger.Log.WithError(err).Warningf("unable to parse duration")
		return ""
	}

	matches := r.FindStringSubmatch(duration)

	years := parseInt64(matches[1])
	months := parseInt64(matches[2])
	days := parseInt64(matches[3])
	hours := parseInt64(matches[4])
	minutes := parseInt64(matches[5])
	seconds := parseInt64(matches[6])

	hour := int64(time.Hour)
	minute := int64(time.Minute)
	second := int64(time.Second)

	return time.Duration(years*24*365*hour +
		months*30*24*hour + days*24*hour +
		hours*hour + minutes*minute + seconds*second).String()
}

func parseInt64(value string) int64 {
	if len(value) == 0 {
		return 0
	}

	parsed, err := strconv.Atoi(value[:len(value)-1])
	if err != nil {
		return 0
	}

	return int64(parsed)
}

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
			"**queue**: Show the queue of music\n"+
			"**shuffle**: Shuffle the songs in the queue")
	if err != nil {
		logger.Log.Warningf("error sending message: %v", err)
	}
}
