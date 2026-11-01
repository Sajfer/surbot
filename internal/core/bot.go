package core

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sajfer/discordgo"

	"github.com/sajfer/surbot/internal/config"
	"github.com/sajfer/surbot/internal/logger"
)

const (
	drainTimeout    = 10 * time.Second
	teardownTimeout = 10 * time.Second
)

// gateway is the part of *discordgo.Session the bot lifecycle needs.
type gateway interface {
	Open() error
	Close() error
	AddHandler(handler any) func()
}

// Bot ties the gateway, the command router and the enabled plugins together.
type Bot struct {
	gw      gateway
	plugins []Plugin
	scopes  []*scope
	router  *router
}

// New builds a Bot with the plugins selected by cfg.Plugins (all of catalog when unset).
func New(cfg *config.Config, prefix string, catalog []Plugin) (*Bot, error) {
	session, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("create Discord session: %w", err)
	}
	return newBot(session, cfg, prefix, catalog)
}

func newBot(gw gateway, cfg *config.Config, prefix string, catalog []Plugin) (*Bot, error) {
	plugins, err := selectPlugins(cfg.Plugins, catalog)
	if err != nil {
		return nil, err
	}

	reg := newRegistry()
	if err := reg.add(coreOwner, helpCommand(prefix, reg)); err != nil {
		return nil, err
	}
	httpClient := &http.Client{Timeout: httpTimeout}
	scopes := make([]*scope, len(plugins))
	for i, p := range plugins {
		scopes[i] = newScope(p.Name(), reg, gw, cfg, httpClient)
	}
	return &Bot{
		gw:      gw,
		plugins: plugins,
		scopes:  scopes,
		router:  newRouter(prefix, reg),
	}, nil
}

func selectPlugins(names []string, catalog []Plugin) ([]Plugin, error) {
	if names == nil {
		return catalog, nil
	}
	byName := make(map[string]Plugin, len(catalog))
	available := make([]string, len(catalog))
	for i, p := range catalog {
		byName[p.Name()] = p
		available[i] = p.Name()
	}
	selected := make([]Plugin, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		p, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("unknown plugin %q (available: %s)", name, strings.Join(available, ", "))
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate plugin %q in plugin list", name)
		}
		seen[name] = true
		selected = append(selected, p)
	}
	return selected, nil
}

// Run sets up the plugins, connects, and blocks until ctx is cancelled. It then
// stops routing, waits for running commands (bounded), tears the plugins down in
// reverse order and closes the gateway. A nil error means a clean shutdown.
func (b *Bot) Run(ctx context.Context) error {
	ready := 0
	for i, p := range b.plugins {
		if err := p.Setup(ctx, b.scopes[i]); err != nil {
			// The failing plugin never finished Setup, so it gets no Teardown.
			b.scopes[i].removeHandlers()
			return errors.Join(fmt.Errorf("setup plugin %s: %w", p.Name(), err), b.teardown(ready))
		}
		ready++
		logger.Log.Infof("plugin %s ready", p.Name())
	}

	removeRouter := b.gw.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		b.router.dispatch(ctx, s, m)
	})

	if err := b.gw.Open(); err != nil {
		removeRouter()
		// Close is nil-safe on a session that never connected and stops any
		// goroutines a half-finished Open left behind.
		return errors.Join(fmt.Errorf("open Discord connection: %w", err), b.teardown(ready), b.gw.Close())
	}

	logger.Log.Info("Bot is now running. Press CTRL-C to exit.")
	<-ctx.Done()
	logger.Log.Info("Shutting down")

	removeRouter()
	drainCtx, cancel := context.WithTimeout(context.Background(), drainTimeout)
	defer cancel()
	if b.router.drain(drainCtx) {
		logger.Log.Info("Commands drained")
	}

	teardownErr := b.teardown(ready)
	var closeErr error
	if err := b.gw.Close(); err != nil {
		closeErr = fmt.Errorf("close Discord connection: %w", err)
	}
	return errors.Join(teardownErr, closeErr)
}

// teardown stops the first n plugins in reverse order. The run context is
// already cancelled by then, so each plugin gets a fresh bounded one.
func (b *Bot) teardown(n int) error {
	var errs []error
	for i := n - 1; i >= 0; i-- {
		b.scopes[i].removeHandlers()
		if err := b.teardownPlugin(b.plugins[i]); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (b *Bot) teardownPlugin(p Plugin) error {
	ctx, cancel := context.WithTimeout(context.Background(), teardownTimeout)
	defer cancel()
	if err := p.Teardown(ctx); err != nil {
		logger.Log.WithError(err).Warningf("teardown plugin %s failed", p.Name())
		return fmt.Errorf("teardown plugin %s: %w", p.Name(), err)
	}
	logger.Log.Infof("plugin %s stopped", p.Name())
	return nil
}
