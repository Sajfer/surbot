// Package music plays songs from YouTube and Spotify links in Discord voice
// channels, with one queue and player per guild.
package music

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sajfer/discordgo"
	"github.com/sirupsen/logrus"

	"gitlab.com/sajfer/surbot/internal/core"
	"gitlab.com/sajfer/surbot/internal/plugins/music/spotify"
	"gitlab.com/sajfer/surbot/internal/plugins/music/youtube"
)

const (
	idleTimeout  = 5 * time.Minute
	spotifyLogin = 10 * time.Second
)

// Plugin implements core.Plugin for music playback.
type Plugin struct {
	resolve   resolver
	newPlayer func(ctx context.Context, guildID string, s *discordgo.Session) *player
	log       *logrus.Entry

	mu      sync.Mutex // guards everything below
	ctx     context.Context
	cancel  context.CancelFunc
	closed  bool
	players map[string]*player
}

// New returns the plugin; it does no I/O until Setup.
func New() *Plugin { return &Plugin{} }

// Name implements core.Plugin.
func (p *Plugin) Name() string { return "music" }

// Setup implements core.Plugin. It does not need a connected gateway: players
// are created when the first music command of a guild arrives.
func (p *Plugin) Setup(ctx context.Context, r core.Registrar) error {
	svc := r.Services()
	yt := youtube.New(svc.Config.Get("YOUTUBE_API"), svc.HTTP)

	var sp trackAPI
	id, secret := svc.Config.Get("SPOTIFY_CLIENTID"), svc.Config.Get("SPOTIFY_CLIENTSECRET")
	if id == "" || secret == "" {
		svc.Log.Warning("spotify disabled: no credentials")
	} else {
		loginCtx, cancel := context.WithTimeout(ctx, spotifyLogin)
		client, err := spotify.New(loginCtx, id, secret)
		cancel()
		if err != nil {
			// Without working Spotify credentials the bot still runs; only
			// Spotify links are unavailable.
			svc.Log.WithError(err).Warning("spotify disabled, spotify links will not work")
		} else {
			sp = client
		}
	}

	log := svc.Log
	p.start(newResolver(yt, sp), func(ctx context.Context, guildID string, s *discordgo.Session) *player {
		return newPlayer(ctx, guildID, discordVoice{s: s}, discordAnnouncer{s: s, log: log}, idleTimeout, log.WithField("guild", guildID))
	}, log)

	for _, spec := range commandSpecs {
		err := r.Command(core.Command{
			Name:        spec.name,
			Usage:       spec.usage,
			Description: spec.description,
			Handler:     p.adapt(spec.handler),
		})
		if err != nil {
			p.cancel()
			return fmt.Errorf("register %s: %w", spec.name, err)
		}
	}
	return nil
}

// start sets the plugin's own lifetime, independent of the run context core
// cancels first, so Teardown decides when players stop.
func (p *Plugin) start(res resolver, factory func(context.Context, string, *discordgo.Session) *player, log *logrus.Entry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ctx, p.cancel = context.WithCancel(context.Background())
	p.resolve, p.newPlayer, p.log = res, factory, log
	p.players = make(map[string]*player)
}

// getOrCreate returns the guild's player, creating it on first use.
func (p *Plugin) getOrCreate(guildID string, s *discordgo.Session) (*player, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, errClosed
	}
	pl, ok := p.players[guildID]
	if !ok {
		pl = p.newPlayer(p.ctx, guildID, s)
		p.players[guildID] = pl
	}
	return pl, nil
}

// lookup returns the guild's player or nil.
func (p *Plugin) lookup(guildID string) *player {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.players[guildID]
}

// Teardown implements core.Plugin. It stops every player, which stops
// yt-dlp and ffmpeg and leaves voice, and waits until all loops have exited or
// ctx ends.
func (p *Plugin) Teardown(ctx context.Context) error {
	p.mu.Lock()
	p.closed = true
	if p.cancel != nil {
		p.cancel()
	}
	players := make(map[string]*player, len(p.players))
	for id, pl := range p.players {
		players[id] = pl
	}
	p.mu.Unlock()

	var errs []error
	for id, pl := range players {
		if err := pl.Wait(ctx); err != nil {
			errs = append(errs, fmt.Errorf("guild %s: %w", id, err))
		}
	}
	return errors.Join(errs...)
}
