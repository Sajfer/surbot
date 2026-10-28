package core

import (
	"unicode/utf16"

	"github.com/sajfer/discordgo"
)

// MaxMessageLength is the most characters Discord accepts in a message.
const MaxMessageLength = 2000

// SendText posts text to a channel with every mention turned off, so text from
// outside (a video title, a joke) cannot ping @everyone, roles or users. Text
// over MaxMessageLength is cut and ends in "…" instead of being rejected.
func SendText(s *discordgo.Session, channelID, text string) error {
	_, err := s.ChannelMessageSendComplex(channelID, textMessage(text))
	return err
}

func textMessage(text string) *discordgo.MessageSend {
	return &discordgo.MessageSend{
		Content: fitText(text, MaxMessageLength),
		// An empty (not nil) Parse list is sent as "parse": [], which tells
		// Discord to resolve no mentions at all.
		AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}},
	}
}

// fitText returns text cut to at most limit characters, ending in "…" when
// cut. Characters are counted in UTF-16 units, which is never fewer than
// Discord counts, so the result always fits.
func fitText(text string, limit int) string {
	if utf16Len(text) <= limit {
		return text
	}
	const ellipsis = "…" // one UTF-16 unit
	n := 0
	for i, r := range text {
		w := utf16.RuneLen(r)
		if n+w > limit-1 {
			return text[:i] + ellipsis
		}
		n += w
	}
	return text
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}
