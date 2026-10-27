package rajd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sajfer/discordgo"
	"github.com/sirupsen/logrus"

	"gitlab.com/sajfer/surbot/internal/config"
	"gitlab.com/sajfer/surbot/internal/core"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
}

func formatted(ds []time.Time) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.Format("Monday 01/02"))
	}
	return out
}

func TestDaysRunThroughTheNextTuesday(t *testing.T) {
	tests := []struct {
		name string
		from time.Time
		want []string
	}{
		{name: "monday", from: date(2026, time.October, 26), want: []string{"Monday 10/26", "Tuesday 10/27"}},
		{name: "tuesday", from: date(2026, time.October, 27), want: []string{"Tuesday 10/27"}},
		{name: "wednesday", from: date(2026, time.October, 28), want: []string{
			"Wednesday 10/28", "Thursday 10/29", "Friday 10/30", "Saturday 10/31",
			"Sunday 11/01", "Monday 11/02", "Tuesday 11/03",
		}},
		{name: "sunday", from: date(2026, time.November, 1), want: []string{"Sunday 11/01", "Monday 11/02", "Tuesday 11/03"}},
		{name: "across the year end", from: date(2025, time.December, 31), want: []string{
			"Wednesday 12/31", "Thursday 01/01", "Friday 01/02", "Saturday 01/03",
			"Sunday 01/04", "Monday 01/05", "Tuesday 01/06",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatted(days(tt.from))

			if !slices.Equal(got, tt.want) {
				t.Errorf("days = %v, want %v", got, tt.want)
			}
		})
	}
}

type fakePoster struct {
	calls     []string
	failSendN int
	sendErr   error
	failReact bool
	afterSend func()
	sends     int
}

func (f *fakePoster) ChannelMessageSend(channelID, content string, _ ...discordgo.RequestOption) (*discordgo.Message, error) {
	f.sends++
	f.calls = append(f.calls, fmt.Sprintf("send:%s:%s", channelID, content))
	if f.afterSend != nil {
		f.afterSend()
	}
	if f.failSendN == f.sends {
		return nil, f.sendErr
	}
	return &discordgo.Message{ID: fmt.Sprintf("m%d", f.sends)}, nil
}

func (f *fakePoster) MessageReactionAdd(channelID, messageID, emojiID string, _ ...discordgo.RequestOption) error {
	f.calls = append(f.calls, fmt.Sprintf("react:%s:%s:%s", channelID, messageID, emojiID))
	if f.failReact {
		return errors.New("reaction rejected")
	}
	return nil
}

func quietLog() *logrus.Entry {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return logrus.NewEntry(l)
}

func newPlugin(channel string) *Plugin {
	return &Plugin{channelID: channel, now: time.Now, log: quietLog()}
}

func TestPostSendsEachDayWithBothReactions(t *testing.T) {
	f := &fakePoster{}

	err := newPlugin("c1").post(context.Background(), f, date(2026, time.October, 26))

	if err != nil {
		t.Fatalf("post returned %v", err)
	}
	want := []string{
		"send:c1:Monday 10/26", "react:c1:m1:✅", "react:c1:m1:❌",
		"send:c1:Tuesday 10/27", "react:c1:m2:✅", "react:c1:m2:❌",
	}
	if !slices.Equal(f.calls, want) {
		t.Errorf("calls = %v, want %v", f.calls, want)
	}
}

func TestPostStillSendsEveryDayWhenReactionsFail(t *testing.T) {
	f := &fakePoster{failReact: true}

	err := newPlugin("c1").post(context.Background(), f, date(2026, time.October, 26))

	if err != nil {
		t.Errorf("post returned %v, want nil", err)
	}
	if f.sends != 2 {
		t.Errorf("sends = %d, want 2", f.sends)
	}
}

func TestPostReturnsTheSendFailureNamingTheChannel(t *testing.T) {
	restErr := errors.New("404 unknown channel")
	f := &fakePoster{failSendN: 2, sendErr: restErr}

	err := newPlugin("c1").post(context.Background(), f, date(2026, time.October, 28))

	if !errors.Is(err, restErr) {
		t.Fatalf("post error = %v, want it to wrap the REST error", err)
	}
	if !strings.Contains(err.Error(), "c1") {
		t.Errorf("post error = %q, want it to name the channel", err)
	}
	want := []string{
		"send:c1:Wednesday 10/28", "react:c1:m1:✅", "react:c1:m1:❌",
		"send:c1:Thursday 10/29",
	}
	if !slices.Equal(f.calls, want) {
		t.Errorf("calls = %v, want %v", f.calls, want)
	}
}

func TestPostStopsQuietlyWhenShutdownCancelsBetweenDays(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakePoster{afterSend: cancel}

	err := newPlugin("c1").post(ctx, f, date(2026, time.October, 28))

	if err != nil {
		t.Errorf("post returned %v, want nil for a deliberate stop", err)
	}
	if f.sends != 1 {
		t.Errorf("sends = %d, want 1", f.sends)
	}
}

func TestPostStopsQuietlyWhenShutdownInterruptsASend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakePoster{afterSend: cancel, failSendN: 1, sendErr: context.Canceled}

	err := newPlugin("c1").post(ctx, f, date(2026, time.October, 28))

	if err != nil {
		t.Errorf("post returned %v, want nil for a deliberate stop", err)
	}
	if f.sends != 1 {
		t.Errorf("sends = %d, want 1 (the interrupted send)", f.sends)
	}
}

type fakeRegistrar struct {
	services *core.Services
	names    []string
}

func (r *fakeRegistrar) Command(cmd core.Command) error {
	r.names = append(r.names, cmd.Name)
	return nil
}

func (r *fakeRegistrar) AddHandler(any) error { return nil }

func (r *fakeRegistrar) Services() *core.Services { return r.services }

func newRegistrar(t *testing.T, channel string) *fakeRegistrar {
	t.Helper()
	t.Setenv("SUR_TOKEN", "token")
	t.Setenv("SUR_RAJD_CHANNEL", channel)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "missing.env"))
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return &fakeRegistrar{services: &core.Services{Config: cfg, Log: quietLog()}}
}

func TestSetupChannelSelectsWhereRajdPosts(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		wantMsg string
	}{
		{name: "unset uses the default", env: "", wantMsg: "send:1006248135737221251:Tuesday 10/27"},
		{name: "configured id", env: "123456", wantMsg: "send:123456:Tuesday 10/27"},
		{name: "surrounding space is ignored", env: " 123456 ", wantMsg: "send:123456:Tuesday 10/27"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			if err := p.Setup(context.Background(), newRegistrar(t, tt.env)); err != nil {
				t.Fatalf("Setup returned %v", err)
			}
			f := &fakePoster{}

			err := p.post(context.Background(), f, date(2026, time.October, 27))

			if err != nil {
				t.Fatalf("post returned %v", err)
			}
			if len(f.calls) == 0 || f.calls[0] != tt.wantMsg {
				t.Errorf("calls = %v, want first call %q", f.calls, tt.wantMsg)
			}
		})
	}
}

func TestSetupRejectsANonNumericChannelAndRegistersNothing(t *testing.T) {
	for _, env := range []string{"general", "12a4", "-5", "12 34"} {
		t.Run(env, func(t *testing.T) {
			r := newRegistrar(t, env)

			err := New().Setup(context.Background(), r)

			if err == nil || !strings.Contains(err.Error(), "SUR_RAJD_CHANNEL") {
				t.Errorf("Setup error = %v, want it to mention SUR_RAJD_CHANNEL", err)
			}
			if len(r.names) != 0 {
				t.Errorf("registered %v, want no commands", r.names)
			}
		})
	}
}
