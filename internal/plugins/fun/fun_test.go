package fun

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"gitlab.com/sajfer/surbot/internal/core"
)

func quietLog() *logrus.Entry {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return logrus.NewEntry(l)
}

func newJokePlugin(url string, client *http.Client) *Plugin {
	p := New()
	p.jokeURL = url
	p.http = client
	p.log = quietLog()
	return p
}

func TestRollD6ReturnsOneToSixAndReachesBothEnds(t *testing.T) {
	seen := map[int]bool{}

	for range 1000 {
		text, err := roll("d6")
		if err != nil {
			t.Fatalf("roll(d6) error: %v", err)
		}
		n, err := strconv.Atoi(text)
		if err != nil {
			t.Fatalf("roll(d6) = %q, want an integer", text)
		}
		if n < 1 || n > 6 {
			t.Fatalf("roll(d6) = %d, want 1..6", n)
		}
		seen[n] = true
	}

	if !seen[1] || !seen[6] {
		t.Errorf("1000 rolls of d6 never produced both 1 and 6: seen %v", seen)
	}
}

func TestRollOtherDiceStayInRange(t *testing.T) {
	tests := []struct {
		args string
		max  int
	}{
		{args: "d10", max: 10},
		{args: "d20", max: 20},
		{args: "d100", max: 100},
	}
	for _, tt := range tests {
		t.Run(tt.args, func(t *testing.T) {
			for range 500 {
				text, err := roll(tt.args)
				if err != nil {
					t.Fatalf("roll(%s) error: %v", tt.args, err)
				}
				n, err := strconv.Atoi(text)
				if err != nil || n < 1 || n > tt.max {
					t.Fatalf("roll(%s) = %q, want an integer in 1..%d", tt.args, text, tt.max)
				}
			}
		})
	}
}

func TestRollUnsupportedArgsAnswerWithUsage(t *testing.T) {
	for _, args := range []string{"d6 extra", "", "d7", "d6x", "D6"} {
		t.Run("args="+args, func(t *testing.T) {
			text, err := roll(args)

			if err != nil {
				t.Fatalf("roll(%q) error: %v", args, err)
			}
			if text != "!roll d6 or !roll d20" {
				t.Errorf("roll(%q) = %q, want the usage text", args, text)
			}
		})
	}
}

func TestJokeReturnsTheValueFromTheAPI(t *testing.T) {
	var hits atomic.Int32
	var gotMethod, gotPath atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		gotMethod.Store(r.Method)
		gotPath.Store(r.URL.Path)
		_, _ = io.WriteString(w, `{"categories":[],"id":"x","value":"Chuck Norris counted to infinity. Twice."}`)
	}))
	defer srv.Close()
	p := newJokePlugin(srv.URL+"/jokes/random", srv.Client())

	got := p.joke(context.Background())

	if got != "Chuck Norris counted to infinity. Twice." {
		t.Errorf("joke = %q, want the API value", got)
	}
	if hits.Load() != 1 || gotMethod.Load() != http.MethodGet || gotPath.Load() != "/jokes/random" {
		t.Errorf("server saw %d requests, last %v %v; want one GET /jokes/random", hits.Load(), gotMethod.Load(), gotPath.Load())
	}
}

func TestJokeFallsBackWhenTheAPIMisbehaves(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "server error", status: http.StatusInternalServerError, body: `{"value":"ignored"}`},
		{name: "not json", status: http.StatusOK, body: `<html>oops</html>`},
		{name: "empty value", status: http.StatusOK, body: `{"value":""}`},
		{name: "blank value", status: http.StatusOK, body: `{"value":"   "}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer srv.Close()
			p := newJokePlugin(srv.URL, srv.Client())

			got := p.joke(context.Background())

			if !slices.Contains(fallbackJokes, got) {
				t.Errorf("joke = %q, want one of the fallback jokes", got)
			}
		})
	}
}

func TestJokeFallsBackWhenTheAPIIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	p := newJokePlugin(url, &http.Client{Timeout: 2 * time.Second})

	got := p.joke(context.Background())

	if !slices.Contains(fallbackJokes, got) {
		t.Errorf("joke = %q, want one of the fallback jokes", got)
	}
}

func TestJokeRequestIsCancelledWithTheCommandContext(t *testing.T) {
	released := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		close(released)
	}))
	defer srv.Close()
	p := newJokePlugin(srv.URL, srv.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan string, 1)
	go func() { done <- p.joke(ctx) }()

	select {
	case got := <-done:
		if !slices.Contains(fallbackJokes, got) {
			t.Errorf("joke = %q, want a fallback joke", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("joke did not return after its context ended")
	}
	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatal("the server never saw the request being cancelled")
	}
}

type rejectingRegistrar struct {
	services *core.Services
	failOn   string
}

func (r *rejectingRegistrar) Command(cmd core.Command) error {
	if cmd.Name == r.failOn {
		return errors.New("rejected")
	}
	return nil
}

func (r *rejectingRegistrar) AddHandler(any) error { return nil }

func (r *rejectingRegistrar) Services() *core.Services { return r.services }

func TestSetupReportsWhichCommandFailedToRegister(t *testing.T) {
	r := &rejectingRegistrar{
		services: &core.Services{Log: quietLog(), HTTP: http.DefaultClient},
		failOn:   "chuck",
	}

	err := New().Setup(context.Background(), r)

	if err == nil || !strings.Contains(err.Error(), "register chuck") {
		t.Errorf("Setup error = %v, want it to mention \"register chuck\"", err)
	}
}
