// Package rajd posts this week's rajd days with yes/no reactions to a configured channel.
package rajd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sajfer/discordgo"
	"github.com/sirupsen/logrus"

	"gitlab.com/sajfer/surbot/internal/core"
)

const defaultChannel = "1006248135737221251"

// poster is the part of *discordgo.Session the command needs.
type poster interface {
	ChannelMessageSend(channelID, content string, options ...discordgo.RequestOption) (*discordgo.Message, error)
	MessageReactionAdd(channelID, messageID, emojiID string, options ...discordgo.RequestOption) error
}

var _ poster = (*discordgo.Session)(nil)

// Plugin serves the rajd command.
type Plugin struct {
	channelID string
	now       func() time.Time
	log       *logrus.Entry
}

// New returns the plugin; it does no I/O until Setup.
func New() *Plugin { return &Plugin{now: time.Now} }

// Name implements core.Plugin.
func (p *Plugin) Name() string { return "rajd" }

// Setup implements core.Plugin. It fails when SUR_RAJD_CHANNEL is set to
// something other than a numeric channel ID.
func (p *Plugin) Setup(_ context.Context, r core.Registrar) error {
	svc := r.Services()
	p.log = svc.Log

	ch := strings.TrimSpace(svc.Config.Get("RAJD_CHANNEL"))
	if ch == "" {
		ch = defaultChannel
	}
	if !digitsOnly(ch) {
		return fmt.Errorf("SUR_RAJD_CHANNEL %q is not a Discord channel ID", ch)
	}
	p.channelID = ch
	p.log.Infof("posting to channel %s", ch)

	err := r.Command(core.Command{
		Name:        "rajd",
		Description: "Post this week's rajd days with reactions",
		Handler: func(ctx context.Context, c *core.Context) error {
			return p.post(ctx, c.Session, p.now())
		},
	})
	if err != nil {
		return fmt.Errorf("register rajd: %w", err)
	}
	return nil
}

// Teardown implements core.Plugin.
func (p *Plugin) Teardown(context.Context) error { return nil }

func digitsOnly(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// days lists from and every following day up to and including the next Tuesday.
func days(from time.Time) []time.Time {
	out := []time.Time{from}
	for d := from.AddDate(0, 0, 1); d.Weekday() != time.Wednesday; d = d.AddDate(0, 0, 1) {
		out = append(out, d)
	}
	return out
}

// post sends one message per day with a yes and a no reaction. Being cut short
// by cancellation is a deliberate stop, not a failure.
func (p *Plugin) post(ctx context.Context, d poster, from time.Time) error {
	for _, day := range days(from) {
		if ctx.Err() != nil {
			p.log.Info("rajd stopped before all days were posted")
			return nil
		}
		text := day.Format("Monday 01/02")
		msg, err := d.ChannelMessageSend(p.channelID, text, discordgo.WithContext(ctx))
		if err != nil {
			if ctx.Err() != nil {
				p.log.WithError(err).Info("rajd stopped before all days were posted")
				return nil
			}
			return fmt.Errorf("post %s to channel %s: %w", text, p.channelID, err)
		}
		for _, emoji := range []string{"✅", "❌"} {
			if err := d.MessageReactionAdd(p.channelID, msg.ID, emoji, discordgo.WithContext(ctx)); err != nil {
				p.log.WithError(err).Warningf("react %s to %s", emoji, text)
			}
		}
	}
	return nil
}
