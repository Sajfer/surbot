package music

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sajfer/discordgo"
	"github.com/sirupsen/logrus"

	"gitlab.com/sajfer/surbot/internal/config"
	"gitlab.com/sajfer/surbot/internal/core"
)

type guildFakes struct {
	v *fakeVoice
	a *fakeAnnouncer
}

// testPlugin is a started Plugin whose players talk to per-guild fakes.
type testPlugin struct {
	*Plugin
	t *testing.T

	mu     sync.Mutex
	guilds map[string]*guildFakes
}

type fakeResolver struct {
	mu      sync.Mutex
	queries []string
	songs   []*Song
	err     error
}

func (f *fakeResolver) Resolve(_ context.Context, query string) ([]*Song, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, query)
	return f.songs, f.err
}

func (f *fakeResolver) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.queries...)
}

func quietLog() *logrus.Entry {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return logrus.NewEntry(l)
}

func newTestPlugin(t *testing.T, res resolver) *testPlugin {
	t.Helper()
	tp := &testPlugin{Plugin: New(), t: t, guilds: make(map[string]*guildFakes)}
	tp.start(res, func(ctx context.Context, guildID string, _ *discordgo.Session) *player {
		tp.mu.Lock()
		defer tp.mu.Unlock()
		f := &guildFakes{v: newFakeVoice(t), a: newFakeAnnouncer()}
		tp.guilds[guildID] = f
		return newPlayer(ctx, guildID, f.v, f.a, time.Hour, quietLog())
	}, quietLog())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
		defer cancel()
		_ = tp.Teardown(ctx)
	})
	return tp
}

func (tp *testPlugin) fakes(guildID string) *guildFakes {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	return tp.guilds[guildID]
}

func (tp *testPlugin) playIn(guildID, voiceChannel, args string) []string {
	tp.t.Helper()
	var replies []string
	err := tp.play(context.Background(), request{
		guildID: guildID, textChannel: "t1", voiceChannel: voiceChannel, args: args,
		reply: func(text string) error { replies = append(replies, text); return nil },
	})
	if err != nil {
		tp.t.Fatalf("play returned %v", err)
	}
	return replies
}

func waitStarted(t *testing.T, f *guildFakes, want string) {
	t.Helper()
	select {
	case got := <-f.v.started:
		if got != want {
			t.Fatalf("started %q, want %q", got, want)
		}
	case <-time.After(waitTimeout):
		t.Fatalf("%q did not start", want)
	}
}

func TestTeardownReturnsOnlyAfterEveryGuildStoppedPlayingAndLeftVoice(t *testing.T) {
	tp := newTestPlugin(t, &fakeResolver{songs: songs("A")})
	tp.playIn("g1", "v1", "x")
	tp.playIn("g2", "v9", "x")
	waitStarted(t, tp.fakes("g1"), "A")
	waitStarted(t, tp.fakes("g2"), "A")
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()

	err := tp.Teardown(ctx)

	if err != nil {
		t.Fatalf("Teardown returned %v", err)
	}
	for _, guild := range []string{"g1", "g2"} {
		f := tp.fakes(guild)
		if got, want := f.v.log()[1:], []string{"play:A", "return:A", "leave"}; !reflect.DeepEqual(got, want) {
			t.Errorf("%s events after join = %v, want %v", guild, got, want)
		}
		_, _, listening := f.a.snapshot()
		if last := listening[len(listening)-1]; last != "" {
			t.Errorf("%s listening status after Teardown = %q, want cleared", guild, last)
		}
	}
}

func TestTeardownWithStuckPlayerReturnsErrorNamingTheGuild(t *testing.T) {
	tp := newTestPlugin(t, &fakeResolver{songs: songs("A")})
	tp.playIn("g7", "v1", "x")
	f := tp.fakes("g7")
	f.v.mu.Lock()
	f.v.ignoreStop = true
	f.v.mu.Unlock()
	waitStarted(t, f, "A")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := tp.Teardown(ctx)

	if err == nil || !strings.Contains(err.Error(), "g7") {
		t.Fatalf("Teardown error = %v, want one naming guild g7", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Teardown error = %v, want it to wrap context.DeadlineExceeded", err)
	}
	f.v.finish <- nil
	wctx, wcancel := context.WithTimeout(context.Background(), waitTimeout)
	defer wcancel()
	if err := tp.Teardown(wctx); err != nil {
		t.Errorf("second Teardown after releasing the player returned %v", err)
	}
}

func TestPlayAfterTeardownJoinsNothingAndSendsNoReply(t *testing.T) {
	res := &fakeResolver{songs: songs("A")}
	tp := newTestPlugin(t, res)
	if err := tp.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown returned %v", err)
	}

	replies := tp.playIn("g1", "v1", "x")

	if len(replies) != 0 {
		t.Errorf("replies = %v, want none", replies)
	}
	if tp.fakes("g1") != nil {
		t.Error("a player was created after Teardown")
	}
}

type fakeRegistrar struct {
	services *core.Services
	names    []string
	failOn   string
}

func (r *fakeRegistrar) Command(cmd core.Command) error {
	for _, n := range r.names {
		if n == cmd.Name {
			return fmt.Errorf("duplicate command %q", cmd.Name)
		}
	}
	if cmd.Handler == nil {
		return fmt.Errorf("command %q has no handler", cmd.Name)
	}
	if cmd.Name == r.failOn {
		return errors.New("rejected")
	}
	r.names = append(r.names, cmd.Name)
	return nil
}

func (r *fakeRegistrar) AddHandler(any) error { return nil }

func (r *fakeRegistrar) Services() *core.Services { return r.services }

type recordingHook struct {
	mu      sync.Mutex
	entries []string
}

func (h *recordingHook) Levels() []logrus.Level { return logrus.AllLevels }

func (h *recordingHook) Fire(e *logrus.Entry) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = append(h.entries, e.Level.String()+": "+e.Message)
	return nil
}

func newRegistrar(t *testing.T) (*fakeRegistrar, *recordingHook) {
	t.Helper()
	t.Setenv("SUR_TOKEN", "token")
	t.Setenv("SUR_YOUTUBE_API", "")
	t.Setenv("SUR_SPOTIFY_CLIENTID", "")
	t.Setenv("SUR_SPOTIFY_CLIENTSECRET", "")
	cfg, err := config.Load(filepath.Join(t.TempDir(), "missing.env"))
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	l := logrus.New()
	l.SetOutput(io.Discard)
	hook := &recordingHook{}
	l.AddHook(hook)
	return &fakeRegistrar{services: &core.Services{Config: cfg, Log: logrus.NewEntry(l)}}, hook
}

func TestSetupRegistersTheEightMusicCommandsOnce(t *testing.T) {
	r, _ := newRegistrar(t)
	p := New()

	err := p.Setup(context.Background(), r)

	if err != nil {
		t.Fatalf("Setup returned %v", err)
	}
	sort.Strings(r.names)
	want := []string{"clearQueue", "disconnect", "play", "playing", "queue", "shuffle", "skip", "stop"}
	if !reflect.DeepEqual(r.names, want) {
		t.Errorf("registered commands = %v, want %v", r.names, want)
	}
	if err := p.Teardown(context.Background()); err != nil {
		t.Errorf("Teardown after Setup returned %v", err)
	}
}

func TestSetupWithoutSpotifyCredentialsWarnsAndRejectsSpotifyLinksWithoutNetwork(t *testing.T) {
	r, hook := newRegistrar(t)
	p := New()
	if err := p.Setup(context.Background(), r); err != nil {
		t.Fatalf("Setup returned %v", err)
	}
	defer func() { _ = p.Teardown(context.Background()) }()

	_, err := p.resolve.Resolve(context.Background(), "https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT?si=a")

	if err == nil {
		t.Error("Resolve of a Spotify link succeeded with Spotify disabled")
	}
	hook.mu.Lock()
	defer hook.mu.Unlock()
	if want := []string{"warning: spotify disabled: no credentials"}; !reflect.DeepEqual(hook.entries, want) {
		t.Errorf("log entries = %v, want %v", hook.entries, want)
	}
}

func TestSetupWrapsCommandRegistrationErrorsAndStopsTheLifetime(t *testing.T) {
	r, _ := newRegistrar(t)
	r.failOn = "skip"

	err := New().Setup(context.Background(), r)

	if err == nil || !strings.Contains(err.Error(), "register skip") {
		t.Fatalf("Setup error = %v, want one wrapping \"register skip\"", err)
	}
}
