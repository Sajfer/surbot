package core

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/sajfer/discordgo"

	"github.com/sajfer/surbot/internal/config"
)

type eventLog struct {
	mu     sync.Mutex
	events []string
}

func (l *eventLog) add(event string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, event)
}

func (l *eventLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...)
}

// fakeGateway records lifecycle calls. Handlers are labelled by their event
// type; the router's MessageCreate handler is labelled "router".
type fakeGateway struct {
	log     *eventLog
	openErr error
	onOpen  func()
}

func (g *fakeGateway) Open() error {
	g.log.add("open")
	if g.onOpen != nil {
		g.onOpen()
	}
	return g.openErr
}

func (g *fakeGateway) Close() error {
	g.log.add("close")
	return nil
}

func (g *fakeGateway) AddHandler(handler any) func() {
	label := "any"
	if event := reflect.TypeOf(handler).In(1); event.Kind() == reflect.Pointer {
		label = strings.ToLower(event.Elem().Name())
	}
	if label == "messagecreate" {
		label = "router"
	}
	g.log.add("add:" + label)
	return func() { g.log.add("remove:" + label) }
}

type fakePlugin struct {
	name        string
	log         *eventLog
	setupErr    error
	teardownErr error
	listener    any
}

func (p *fakePlugin) Name() string { return p.name }

func (p *fakePlugin) Setup(_ context.Context, r Registrar) error {
	p.log.add("setup:" + p.name)
	if p.listener != nil {
		if err := r.AddHandler(p.listener); err != nil {
			return err
		}
	}
	return p.setupErr
}

func (p *fakePlugin) Teardown(context.Context) error {
	p.log.add("teardown:" + p.name)
	return p.teardownErr
}

func readyListener() any      { return func(*discordgo.Session, *discordgo.Ready) {} }
func disconnectListener() any { return func(*discordgo.Session, *discordgo.Disconnect) {} }

func runBot(t *testing.T, gw *fakeGateway, cfg *config.Config, plugins ...Plugin) error {
	t.Helper()
	bot, err := newBot(gw, cfg, "!", plugins)
	if err != nil {
		t.Fatalf("newBot() error = %v", err)
	}
	return bot.Run(context.Background())
}

func cancelOnOpen(log *eventLog) (*fakeGateway, context.Context) {
	ctx, cancel := context.WithCancel(context.Background())
	return &fakeGateway{log: log, onOpen: cancel}, ctx
}

func TestRunShutsDownInOrder(t *testing.T) {
	log := &eventLog{}
	gw, ctx := cancelOnOpen(log)
	a := &fakePlugin{name: "A", log: log, listener: readyListener()}
	b := &fakePlugin{name: "B", log: log}
	bot, err := newBot(gw, &config.Config{}, "!", []Plugin{a, b})
	if err != nil {
		t.Fatalf("newBot() error = %v", err)
	}

	err = bot.Run(ctx)

	if err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}
	want := []string{
		"setup:A", "add:ready", "setup:B", "add:router", "open",
		"remove:router", "teardown:B", "remove:ready", "teardown:A", "close",
	}
	if got := log.snapshot(); !reflect.DeepEqual(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
}

func TestRunSetupFailureUnwindsWithoutConnecting(t *testing.T) {
	log := &eventLog{}
	setupErr := errors.New("boom")
	a := &fakePlugin{name: "A", log: log}
	b := &fakePlugin{name: "B", log: log, listener: disconnectListener(), setupErr: setupErr}

	err := runBot(t, &fakeGateway{log: log}, &config.Config{}, a, b)

	if !errors.Is(err, setupErr) {
		t.Errorf("Run() error = %v, want it to wrap %v", err, setupErr)
	}
	want := []string{"setup:A", "setup:B", "add:disconnect", "remove:disconnect", "teardown:A"}
	if got := log.snapshot(); !reflect.DeepEqual(got, want) {
		t.Errorf("events = %v, want %v (no teardown:B, no open)", got, want)
	}
}

func TestRunTeardownErrorDoesNotStopOtherTeardownsOrClose(t *testing.T) {
	log := &eventLog{}
	gw, ctx := cancelOnOpen(log)
	teardownErr := errors.New("stuck")
	a := &fakePlugin{name: "A", log: log, teardownErr: teardownErr}
	b := &fakePlugin{name: "B", log: log}
	bot, err := newBot(gw, &config.Config{}, "!", []Plugin{a, b})
	if err != nil {
		t.Fatalf("newBot() error = %v", err)
	}

	err = bot.Run(ctx)

	if !errors.Is(err, teardownErr) {
		t.Errorf("Run() error = %v, want it to wrap %v", err, teardownErr)
	}
	want := []string{"setup:A", "setup:B", "add:router", "open", "remove:router", "teardown:B", "teardown:A", "close"}
	if got := log.snapshot(); !reflect.DeepEqual(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
}

func TestRunOpenFailureTearsDownThenClosesBestEffort(t *testing.T) {
	log := &eventLog{}
	openErr := errors.New("no network")
	gw := &fakeGateway{log: log, openErr: openErr}
	a := &fakePlugin{name: "A", log: log}
	b := &fakePlugin{name: "B", log: log}

	err := runBot(t, gw, &config.Config{}, a, b)

	if !errors.Is(err, openErr) {
		t.Errorf("Run() error = %v, want it to wrap %v", err, openErr)
	}
	want := []string{
		"setup:A", "setup:B", "add:router", "open",
		"remove:router", "teardown:B", "teardown:A", "close",
	}
	if got := log.snapshot(); !reflect.DeepEqual(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
}

func TestNewBotSelectsPluginsFromConfig(t *testing.T) {
	tests := []struct {
		name      string
		selected  []string
		wantSetup []string
		wantErr   string
	}{
		{name: "unset enables all in catalog order", selected: nil, wantSetup: []string{"setup:a", "setup:b"}},
		{name: "list order wins", selected: []string{"b", "a"}, wantSetup: []string{"setup:b", "setup:a"}},
		{name: "subset", selected: []string{"b"}, wantSetup: []string{"setup:b"}},
		{name: "unknown name", selected: []string{"nope"}, wantErr: "nope"},
		{name: "duplicate name", selected: []string{"a", "a"}, wantErr: "duplicate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := &eventLog{}
			gw, ctx := cancelOnOpen(log)
			catalog := []Plugin{&fakePlugin{name: "a", log: log}, &fakePlugin{name: "b", log: log}}

			bot, err := newBot(gw, &config.Config{Plugins: tt.selected}, "!", catalog)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("newBot() error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("newBot() error = %v", err)
			}
			if err := bot.Run(ctx); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			var gotSetup []string
			for _, e := range log.snapshot() {
				if strings.HasPrefix(e, "setup:") {
					gotSetup = append(gotSetup, e)
				}
			}
			if !reflect.DeepEqual(gotSetup, tt.wantSetup) {
				t.Errorf("setup order = %v, want %v", gotSetup, tt.wantSetup)
			}
		})
	}
}
