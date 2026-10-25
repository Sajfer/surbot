// Package legacy exposes the original pkg/surbot commands through the plugin interface.
package legacy

import (
	"context"
	"fmt"

	"gitlab.com/sajfer/surbot/internal/core"
	"gitlab.com/sajfer/surbot/pkg/surbot"
)

// Plugin adapts pkg/surbot to core.Plugin.
type Plugin struct {
	bot *surbot.Surbot
}

// New returns the plugin; it does no I/O until Setup.
func New() *Plugin { return &Plugin{} }

// Name implements core.Plugin.
func (p *Plugin) Name() string { return "legacy" }

var commands = []core.Command{
	{Name: "ping", Description: "Responds with Pong!"},
	{Name: "chuck", Description: "Responds with a Chuck Norris joke"},
	{Name: "play", Usage: "<youtube link|spotify link|search>", Description: "Queue a song and start playing in your voice channel"},
	{Name: "playing", Description: "Show the current song"},
	{Name: "queue", Description: "Show the queue"},
	{Name: "shuffle", Description: "Shuffle the queue"},
	{Name: "skip", Description: "Skip the current song"},
	{Name: "stop", Description: "Stop playing"},
	{Name: "clearQueue", Description: "Clear the queue"},
	{Name: "disconnect", Description: "Leave the voice channel"},
	{Name: "roll", Usage: "d6|d10|d20|d100", Description: "Roll a die"},
	{Name: "rajd", Description: "Post this week's rajd days with reactions"},
}

// Setup implements core.Plugin. The Spotify login inside NewSurbot is not
// bounded by ctx.
func (p *Plugin) Setup(_ context.Context, r core.Registrar) error {
	cfg := r.Services().Config
	p.bot = surbot.NewSurbot(cfg.Get("YOUTUBE_API"), cfg.Get("SPOTIFY_CLIENTID"), cfg.Get("SPOTIFY_CLIENTSECRET"))

	for _, cmd := range commands {
		name := cmd.Name
		cmd.Handler = func(_ context.Context, c *core.Context) error {
			p.bot.HandleCommand(c.Session, c.Message, name, c.Args)
			return nil
		}
		if err := r.Command(cmd); err != nil {
			return fmt.Errorf("register %s: %w", name, err)
		}
	}
	return nil
}

// Teardown implements core.Plugin. Voice cleanup arrives with the music plugin.
func (p *Plugin) Teardown(context.Context) error { return nil }
