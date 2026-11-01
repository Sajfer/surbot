// Package fun provides small chat commands: ping, a Chuck Norris joke and dice rolls.
package fun

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	mrand "math/rand/v2"
	"net/http"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	"gitlab.com/sajfer/surbot/internal/core"
)

const jokeAPI = "https://api.chucknorris.io/jokes/random"

const rollUsage = "!roll d6 or !roll d20"

var fallbackJokes = []string{
	"Why did the Chuck Norris joke go to therapy? It had too many existential crises.",
	"Chuck Norris doesn't read jokes. He stares at them until they confess.",
	"What's Chuck Norris's favorite type of music? Heavy metal, because he can crush it.",
}

// Plugin serves ping, chuck and roll.
type Plugin struct {
	jokeURL string
	http    *http.Client
	log     *logrus.Entry
}

// New returns the plugin; it does no I/O until Setup.
func New() *Plugin { return &Plugin{jokeURL: jokeAPI} }

// Name implements core.Plugin.
func (p *Plugin) Name() string { return "fun" }

// Setup implements core.Plugin.
func (p *Plugin) Setup(_ context.Context, r core.Registrar) error {
	svc := r.Services()
	p.http, p.log = svc.HTTP, svc.Log

	commands := []core.Command{
		{Name: "ping", Description: "Responds with Pong!", Handler: func(_ context.Context, c *core.Context) error {
			return c.Reply("Pong!")
		}},
		{Name: "chuck", Description: "Responds with a Chuck Norris joke", Handler: func(ctx context.Context, c *core.Context) error {
			return c.Reply(p.joke(ctx))
		}},
		{Name: "roll", Usage: "d6|d10|d20|d100", Description: "Roll a die", Handler: func(_ context.Context, c *core.Context) error {
			text, err := roll(c.Args)
			if err != nil {
				return err
			}
			return c.Reply(text)
		}},
	}
	for _, cmd := range commands {
		if err := r.Command(cmd); err != nil {
			return fmt.Errorf("register %s: %w", cmd.Name, err)
		}
	}
	return nil
}

// Teardown implements core.Plugin.
func (p *Plugin) Teardown(context.Context) error { return nil }

func roll(args string) (string, error) {
	sides, ok := map[string]int64{"d6": 6, "d10": 10, "d20": 20, "d100": 100}[args]
	if !ok {
		return rollUsage, nil
	}
	n, err := rand.Int(rand.Reader, big.NewInt(sides))
	if err != nil {
		return "", fmt.Errorf("roll %s: %w", args, err)
	}
	return strconv.FormatInt(n.Int64()+1, 10), nil
}

// joke returns a joke from the API, or a built-in one when the API fails.
func (p *Plugin) joke(ctx context.Context) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.jokeURL, nil)
	if err != nil {
		p.log.WithError(err).Warning("build joke request")
		return fallbackJoke()
	}
	resp, err := p.http.Do(req)
	if err != nil {
		p.log.WithError(err).Warning("fetch joke")
		return fallbackJoke()
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			p.log.WithError(err).Debug("close joke response body")
		}
	}()
	if resp.StatusCode != http.StatusOK {
		p.log.Warningf("joke API answered %s", resp.Status)
		return fallbackJoke()
	}

	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body); err != nil {
		p.log.WithError(err).Warning("decode joke")
		return fallbackJoke()
	}
	if strings.TrimSpace(body.Value) == "" {
		p.log.Warning("joke API returned an empty joke")
		return fallbackJoke()
	}
	return body.Value
}

func fallbackJoke() string {
	return fallbackJokes[mrand.IntN(len(fallbackJokes))] // #nosec G404
}
