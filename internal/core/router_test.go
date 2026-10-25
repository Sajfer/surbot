package core

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sajfer/discordgo"
)

type sentMessage struct{ channelID, text string }

type recorder struct {
	mu   sync.Mutex
	sent []sentMessage
}

func (r *recorder) send(_ *discordgo.Session, channelID, text string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, sentMessage{channelID, text})
	return nil
}

func (r *recorder) messages() []sentMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]sentMessage(nil), r.sent...)
}

// fixture is a router with the built-in help, a scope to register commands
// through the public Registrar, and a recorder in place of Discord REST.
type fixture struct {
	router *router
	scope  *scope
	sent   *recorder
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	reg := newRegistry()
	if err := reg.add(coreOwner, helpCommand("!", reg)); err != nil {
		t.Fatalf("register help: %v", err)
	}
	rt := newRouter("!", reg)
	rec := &recorder{}
	rt.send = rec.send
	return &fixture{router: rt, scope: newScope("fake", reg, &fakeGateway{}, nil, nil), sent: rec}
}

func (f *fixture) register(t *testing.T, cmd Command) {
	t.Helper()
	if err := f.scope.Command(cmd); err != nil {
		t.Fatalf("register %q: %v", cmd.Name, err)
	}
}

func (f *fixture) dispatch(content string) {
	f.dispatchFrom(&discordgo.User{ID: "u1"}, content)
}

func (f *fixture) dispatchFrom(author *discordgo.User, content string) {
	m := &discordgo.MessageCreate{Message: &discordgo.Message{
		ChannelID: "chan1", Content: content, Author: author,
	}}
	f.router.dispatch(context.Background(), &discordgo.Session{}, m)
}

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantName string
		wantArgs string
		wantOK   bool
	}{
		{name: "bare command", content: "!ping", wantName: "ping", wantOK: true},
		{name: "command with argument", content: "!roll d6", wantName: "roll", wantArgs: "d6", wantOK: true},
		{name: "arguments keep inner spacing and lose outer", content: "!play  some  song ", wantName: "play", wantArgs: "some  song", wantOK: true},
		{name: "no space means one long name", content: "!rolld6", wantName: "rolld6", wantOK: true},
		{name: "case is kept", content: "!clearQueue", wantName: "clearQueue", wantOK: true},
		{name: "prefix only", content: "!"},
		{name: "prefix and space", content: "! "},
		{name: "space after prefix", content: "! ping"},
		{name: "missing prefix", content: "ping"},
		{name: "other prefix", content: "?ping"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, args, ok := parse("!", tt.content)

			if name != tt.wantName || args != tt.wantArgs || ok != tt.wantOK {
				t.Errorf("parse(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.content, name, args, ok, tt.wantName, tt.wantArgs, tt.wantOK)
			}
		})
	}
}

func TestDispatchPassesTrimmedArgsToHandler(t *testing.T) {
	f := newFixture(t)
	var got string
	f.register(t, Command{Name: "fake", Handler: func(_ context.Context, c *Context) error {
		got = c.Args
		return nil
	}})

	f.dispatch("!fake  a b ")

	if got != "a b" {
		t.Errorf("Args = %q, want %q", got, "a b")
	}
}

func TestDispatchKeepsAllWordsAfterCommandForHandler(t *testing.T) {
	f := newFixture(t)
	var got string
	f.register(t, Command{Name: "roll", Handler: func(_ context.Context, c *Context) error {
		got = c.Args
		return nil
	}})

	f.dispatch("!roll d6 extra")

	if got != "d6 extra" {
		t.Errorf("Args = %q, want %q so the command can reject the extra word", got, "d6 extra")
	}
}

func TestDispatchIgnoresMessagesThatCannotBeUserCommands(t *testing.T) {
	tests := []struct {
		name   string
		author *discordgo.User
	}{
		{name: "bot author", author: &discordgo.User{ID: "b1", Bot: true}},
		{name: "missing author", author: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			ran := false
			f.register(t, Command{Name: "fake", Handler: func(context.Context, *Context) error {
				ran = true
				return nil
			}})

			f.dispatchFrom(tt.author, "!fake")

			if ran {
				t.Error("handler ran for a message that is not from a user")
			}
		})
	}
}

func TestDispatchUnknownCommandIsSilent(t *testing.T) {
	f := newFixture(t)
	f.register(t, Command{Name: "fake", Handler: func(context.Context, *Context) error { return nil }})

	f.dispatch("!nope")
	f.dispatch("!Fake")

	if got := f.sent.messages(); len(got) != 0 {
		t.Errorf("sent %v, want nothing", got)
	}
}

func TestDispatchRecoversPanicAndKeepsServing(t *testing.T) {
	f := newFixture(t)
	f.register(t, Command{Name: "boom", Handler: func(context.Context, *Context) error { panic("kaboom") }})
	f.register(t, Command{Name: "ok", Handler: func(_ context.Context, c *Context) error { return c.Reply("fine") }})

	f.dispatch("!boom")
	f.dispatch("!ok")

	want := []sentMessage{{"chan1", "Something went wrong."}, {"chan1", "fine"}}
	got := f.sent.messages()
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("sent %v, want %v", got, want)
	}
}

func TestDispatchHandlerErrorRepliesWithFailureMessage(t *testing.T) {
	f := newFixture(t)
	f.register(t, Command{Name: "bad", Handler: func(context.Context, *Context) error { return errors.New("nope") }})

	f.dispatch("!bad")

	got := f.sent.messages()
	if len(got) != 1 || got[0] != (sentMessage{"chan1", "Something went wrong."}) {
		t.Errorf("sent %v, want one failure reply to chan1", got)
	}
}

func TestRegisterRejectsInvalidAndDuplicateCommands(t *testing.T) {
	handler := func(context.Context, *Context) error { return nil }
	tests := []struct {
		name    string
		cmd     Command
		wantErr string
	}{
		{name: "empty name", cmd: Command{Handler: handler}, wantErr: "empty"},
		{name: "nil handler", cmd: Command{Name: "x"}, wantErr: "no handler"},
		{name: "taken by core", cmd: Command{Name: "help", Handler: handler}, wantErr: "registered by core"},
		{name: "taken by plugin", cmd: Command{Name: "play", Handler: handler}, wantErr: "registered by fake"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.register(t, Command{Name: "play", Handler: handler})

			err := f.scope.Command(tt.cmd)

			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Command() error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestHelpListsCommandsSortedWithUsage(t *testing.T) {
	f := newFixture(t)
	f.register(t, Command{Name: "b", Usage: "<x>", Description: "Does b", Handler: func(context.Context, *Context) error { return nil }})
	f.register(t, Command{Name: "a", Description: "Does a", Handler: func(context.Context, *Context) error { return nil }})

	f.dispatch("!help")

	want := "Available commands:\n" +
		"**!a** — Does a\n" +
		"**!b** <x> — Does b\n" +
		"**!help** — Show this list"
	got := f.sent.messages()
	if len(got) != 1 || got[0].text != want {
		t.Errorf("help reply = %v, want %q", got, want)
	}
}

func TestAddHandlerAcceptsOnlyDiscordgoHandlerShapes(t *testing.T) {
	type namedHandler func(*discordgo.Session, *discordgo.MessageCreate)
	type notDiscordgo struct{}
	tests := []struct {
		name    string
		handler any
		wantErr bool
	}{
		{name: "message create", handler: func(*discordgo.Session, *discordgo.MessageCreate) {}},
		{name: "any event", handler: func(*discordgo.Session, interface{}) {}},
		{name: "wrong signature", handler: func(string) {}, wantErr: true},
		{name: "not a func", handler: "handler", wantErr: true},
		{name: "nil", handler: nil, wantErr: true},
		{name: "returns a value", handler: func(*discordgo.Session, *discordgo.MessageCreate) error { return nil }, wantErr: true},
		{name: "first arg is not a session", handler: func(string, *discordgo.MessageCreate) {}, wantErr: true},
		{name: "event is not a discordgo type", handler: func(*discordgo.Session, *notDiscordgo) {}, wantErr: true},
		{name: "named func type", handler: namedHandler(func(*discordgo.Session, *discordgo.MessageCreate) {}), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)

			err := f.scope.AddHandler(tt.handler)

			if (err != nil) != tt.wantErr {
				t.Errorf("AddHandler() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// blockingCommand registers "slow", which signals started and then waits for release.
func blockingCommand(t *testing.T, f *fixture) (started, release chan struct{}) {
	t.Helper()
	started, release = make(chan struct{}), make(chan struct{})
	f.register(t, Command{Name: "slow", Handler: func(context.Context, *Context) error {
		close(started)
		<-release
		return nil
	}})
	return started, release
}

func dispatchAsync(f *fixture, content string) (done chan struct{}) {
	done = make(chan struct{})
	go func() {
		defer close(done)
		f.dispatch(content)
	}()
	return done
}

func waitFor(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestDrainWithNothingInFlightReturnsTrue(t *testing.T) {
	f := newFixture(t)

	if !f.router.drain(context.Background()) {
		t.Error("drain() = false, want true when idle")
	}
}

func TestDrainTimesOutWhileHandlerIsBlocked(t *testing.T) {
	f := newFixture(t)
	started, release := blockingCommand(t, f)
	done := dispatchAsync(f, "!slow")
	waitFor(t, started, "handler start")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	got := f.router.drain(ctx)

	if got {
		t.Error("drain() = true, want false while a handler is still running")
	}
	close(release)
	waitFor(t, done, "handler exit")
}

func TestDrainReturnsTrueOnceHandlerFinishes(t *testing.T) {
	f := newFixture(t)
	started, release := blockingCommand(t, f)
	done := dispatchAsync(f, "!slow")
	waitFor(t, started, "handler start")
	go func() {
		time.Sleep(20 * time.Millisecond)
		close(release)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got := f.router.drain(ctx)

	if !got {
		t.Error("drain() = false, want true after the handler was released")
	}
	waitFor(t, done, "handler exit")
}

func TestDispatchAfterDrainStartedDoesNotRunHandler(t *testing.T) {
	f := newFixture(t)
	ran := false
	f.register(t, Command{Name: "fake", Handler: func(context.Context, *Context) error {
		ran = true
		return nil
	}})
	f.router.drain(context.Background())

	f.dispatch("!fake")

	if ran {
		t.Error("handler ran after drain started")
	}
}

func TestDispatchWithCancelledContextDoesNotRunHandler(t *testing.T) {
	f := newFixture(t)
	ran := false
	f.register(t, Command{Name: "fake", Handler: func(context.Context, *Context) error {
		ran = true
		return nil
	}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m := &discordgo.MessageCreate{Message: &discordgo.Message{
		ChannelID: "chan1", Content: "!fake", Author: &discordgo.User{ID: "u1"},
	}}

	f.router.dispatch(ctx, &discordgo.Session{}, m)

	if ran {
		t.Error("handler ran with a cancelled context")
	}
}
