package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/sajfer/surbot/internal/logger"
)

const (
	// DefaultHealthAddr is where /healthz listens unless SUR_HEALTH_ADDR says otherwise.
	DefaultHealthAddr = ":8080"
	// healthStaleAfter is how long without a gateway heartbeat ACK counts as
	// down. Discord asks for a heartbeat about every 41s and discordgo
	// reconnects after missed ACKs, so 3 minutes means reconnecting has
	// been failing for a while and a restart is the remaining fix.
	healthStaleAfter = 3 * time.Minute
)

// health answers /healthz for the Kubernetes liveness probe.
type health struct {
	lastAck func() time.Time // the gateway's last heartbeat ACK
	now     func() time.Time
	open    atomic.Bool // set once the gateway connection is open
}

func (h *health) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	if !h.open.Load() {
		// Still connecting. A failed Open ends the process, so there is
		// nothing for a restart to fix yet.
		_, _ = fmt.Fprintln(w, "starting")
		return
	}
	if age := h.now().Sub(h.lastAck()); age > healthStaleAfter {
		http.Error(w, fmt.Sprintf("no gateway heartbeat for %s", age.Round(time.Second)), http.StatusServiceUnavailable)
		return
	}
	_, _ = fmt.Fprintln(w, "ok")
}

// startHealth listens on addr and serves h at /healthz until Shutdown. A port
// that cannot be bound is an error at once, not later in the background.
func startHealth(addr string, h http.Handler) (*http.Server, net.Addr, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("health endpoint: %w", err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /healthz", h)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Log.WithError(err).Error("health endpoint stopped")
		}
	}()
	return srv, ln.Addr(), nil
}

func stopHealth(srv *http.Server) {
	if srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
