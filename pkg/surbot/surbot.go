// Package surbot contains the main functionality for Surbot.
package surbot

import (
	"crypto/rand"
	"math/big"
	"time"

	"github.com/sajfer/discordgo"
	"gitlab.com/sajfer/surbot/internal/logger"
	"gitlab.com/sajfer/surbot/internal/utils"
)

// Surbot contains basic information about the bot
type Surbot struct{}

// NewSurbot returns an instance of Surbot
func NewSurbot() *Surbot {
	logger.Log.Debug("NewSurbot")
	return &Surbot{}
}

// HandleCommand runs the named command for a message the caller has already
// matched against the prefix. Unknown names are ignored.
func (surbot *Surbot) HandleCommand(s *discordgo.Session, m *discordgo.MessageCreate, name, args string) {
	channelID := m.ChannelID

	switch name {
	case "ping":
		surbot.HandlePing(s, channelID)
	case "chuck":
		surbot.send(s, channelID, "chuck", surbot.HandleChuck(s, channelID))
	case "rajd":
		surbot.HandleRajd(s, channelID)
	case "roll":
		surbot.send(s, channelID, "roll", surbot.HandleRoll(s, channelID, args))
	}
}

// send posts a handler's response; an empty message means nothing to say.
func (surbot *Surbot) send(s *discordgo.Session, channelID, command, message string) {
	if message == "" {
		return
	}
	logger.Log.Debugf("sending %s response: %s", command, message)
	_, _ = s.ChannelMessageSend(channelID, message)
}

// HandlePing returns the pong message
func (surbot *Surbot) HandlePing(s *discordgo.Session, channelID string) {
	logger.Log.Debug("handling ping command")
	message := "Pong!"
	_, _ = s.ChannelMessageSend(channelID, message)
}

// HandleChuck returns a Chuck Norris joke
func (surbot *Surbot) HandleChuck(s *discordgo.Session, channelID string) string {
	logger.Log.Debug("handling chuck command")
	return utils.GetChuckJoke()
}

// HandleRajd posts the current date and reactions in Rajda channel (Wednesdays only)
func (surbot *Surbot) HandleRajd(s *discordgo.Session, channelID string) {
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
func (surbot *Surbot) HandleRoll(s *discordgo.Session, channelID string, diceType string) string {
	logger.Log.Debug("handling roll command")

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
