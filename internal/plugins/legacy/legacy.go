// Package legacy serves the remaining non-music commands (ping, chuck, roll,
// rajd) from pkg/surbot through the plugin interface.
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
	{Name: "roll", Usage: "d6|d10|d20|d100", Description: "Roll a die"},
	{Name: "rajd", Description: "Post this week's rajd days with reactions"},
}

// Setup implements core.Plugin.
func (p *Plugin) Setup(_ context.Context, r core.Registrar) error {
	p.bot = surbot.NewSurbot()

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

// Teardown implements core.Plugin.
func (p *Plugin) Teardown(context.Context) error { return nil }
