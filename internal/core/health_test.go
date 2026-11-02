package core

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sajfer/surbot/internal/config"
)

func TestHealth(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		open     bool
		lastAck  time.Time
		wantCode int
		wantBody string
	}{
		{name: "still connecting", open: false, wantCode: 200, wantBody: "starting"},
		{name: "recent heartbeat", open: true, lastAck: now.Add(-45 * time.Second), wantCode: 200, wantBody: "ok"},
		{name: "just inside the limit", open: true, lastAck: now.Add(-healthStaleAfter), wantCode: 200, wantBody: "ok"},
		{name: "no heartbeat for too long", open: true, lastAck: now.Add(-4 * time.Minute), wantCode: 503, wantBody: "no gateway heartbeat for 4m0s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &health{lastAck: func() time.Time { return tt.lastAck }, now: func() time.Time { return now }}
			h.open.Store(tt.open)
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

			if rec.Code != tt.wantCode || strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Errorf("got %d %q, want %d %q", rec.Code, strings.TrimSpace(rec.Body.String()), tt.wantCode, tt.wantBody)
			}
		})
	}
}

func TestStartHealthServesHealthzUntilStopped(t *testing.T) {
	h := &health{lastAck: time.Now, now: time.Now}
	h.open.Store(true)

	srv, addr, err := startHealth("127.0.0.1:0", h)
	if err != nil {
		t.Fatalf("startHealth: %v", err)
	}
	url := "http://" + addr.String()

	resp, err := http.Get(url + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || strings.TrimSpace(string(body)) != "ok" {
		t.Errorf("GET /healthz = %d %q, want 200 ok", resp.StatusCode, body)
	}
	if resp, err := http.Get(url + "/other"); err == nil {
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET /other = %d, want 404", resp.StatusCode)
		}
	}

	stopHealth(srv)
	if resp, err := http.Get(url + "/healthz"); err == nil {
		_ = resp.Body.Close()
		t.Error("health endpoint still answers after stopHealth")
	}
}

func TestStartHealthOnABusyPortFails(t *testing.T) {
	srv, addr, err := startHealth("127.0.0.1:0", &health{})
	if err != nil {
		t.Fatalf("startHealth: %v", err)
	}
	defer stopHealth(srv)

	if _, _, err := startHealth(addr.String(), &health{}); err == nil {
		t.Error("second startHealth on the same port returned no error")
	}
}

func TestRunWithHealthOnABusyPortFailsBeforeAnyPluginStarts(t *testing.T) {
	busy, addr, err := startHealth("127.0.0.1:0", &health{})
	if err != nil {
		t.Fatalf("startHealth: %v", err)
	}
	defer stopHealth(busy)
	log := &eventLog{}
	bot, err := newBot(&fakeGateway{log: log}, &config.Config{}, "!", []Plugin{&fakePlugin{name: "A", log: log}})
	if err != nil {
		t.Fatalf("newBot: %v", err)
	}
	bot.healthAddr, bot.health = addr.String(), &health{lastAck: time.Now, now: time.Now}

	if err := bot.Run(context.Background()); err == nil {
		t.Fatal("Run returned no error with the health port taken")
	}
	if got := log.snapshot(); len(got) != 0 {
		t.Errorf("events = %v, want none: nothing may start", got)
	}
}

func TestRunReportsHealthyOnlyAfterTheGatewayOpens(t *testing.T) {
	log := &eventLog{}
	ctx, cancel := context.WithCancel(context.Background())
	h := &health{lastAck: time.Now, now: time.Now}
	var openBeforeGateway, openAfterGateway bool
	gw := &fakeGateway{log: log, onOpen: func() {
		openBeforeGateway = h.open.Load()
		go func() {
			for !h.open.Load() {
				time.Sleep(time.Millisecond)
			}
			openAfterGateway = true
			cancel()
		}()
	}}
	bot, err := newBot(gw, &config.Config{}, "!", nil)
	if err != nil {
		t.Fatalf("newBot: %v", err)
	}
	bot.healthAddr, bot.health = "127.0.0.1:0", h

	if err := bot.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if openBeforeGateway || !openAfterGateway {
		t.Errorf("open during Open = %v, after = %v; want false, then true", openBeforeGateway, openAfterGateway)
	}
}
