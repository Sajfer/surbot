package core

import (
	"context"
	"runtime/debug"
	"strings"
	"sync"
	"unicode"

	"github.com/sajfer/discordgo"

	"github.com/sajfer/surbot/internal/logger"
)

const failureReply = "Something went wrong."

type router struct {
	prefix   string
	registry *registry
	send     func(s *discordgo.Session, channelID, text string) error

	mu       sync.Mutex
	inflight int
	closing  bool
	idle     chan struct{}
}

func newRouter(prefix string, reg *registry) *router {
	return &router{
		prefix:   prefix,
		registry: reg,
		send:     SendText,
		idle:     make(chan struct{}),
	}
}

// parse splits "<prefix><name> <args>". A space right after the prefix is not a command.
func parse(prefix, content string) (name, args string, ok bool) {
	if !strings.HasPrefix(content, prefix) {
		return "", "", false
	}
	rest := content[len(prefix):]
	if rest == "" || unicode.IsSpace([]rune(rest)[0]) {
		return "", "", false
	}
	end := strings.IndexFunc(rest, unicode.IsSpace)
	if end < 0 {
		return rest, "", true
	}
	return rest[:end], strings.TrimSpace(rest[end:]), true
}

func (r *router) dispatch(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignoring every bot author also covers the bot's own messages.
	if m == nil || m.Message == nil || m.Author == nil || m.Author.Bot {
		return
	}
	name, args, ok := parse(r.prefix, m.Content)
	if !ok {
		return
	}
	e, ok := r.registry.lookup(name)
	if !ok {
		return
	}
	if !r.begin(ctx) {
		return
	}
	defer r.end()

	c := &Context{
		Session: s,
		Message: m,
		Args:    args,
		send:    func(channelID, text string) error { return r.send(s, channelID, text) },
	}
	r.run(ctx, e, c)
}

func (r *router) run(ctx context.Context, e entry, c *Context) {
	defer func() {
		if rec := recover(); rec != nil {
			logger.Log.Errorf("recovered from panic in command %q (%s): %v\n%s", e.cmd.Name, e.owner, rec, debug.Stack())
			r.replyFailure(c)
		}
	}()
	if err := e.cmd.Handler(ctx, c); err != nil {
		logger.Log.WithError(err).Warningf("command %q (%s) failed", e.cmd.Name, e.owner)
		r.replyFailure(c)
	}
}

func (r *router) replyFailure(c *Context) {
	if err := c.Reply(failureReply); err != nil {
		logger.Log.WithError(err).Warning("could not send failure reply")
	}
}

func (r *router) begin(ctx context.Context) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closing || ctx.Err() != nil {
		return false
	}
	r.inflight++
	return true
}

func (r *router) end() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inflight--
	r.closeIdleLocked()
}

func (r *router) closeIdleLocked() {
	if !r.closing || r.inflight != 0 {
		return
	}
	select {
	case <-r.idle:
	default:
		close(r.idle)
	}
}

// drain stops accepting commands and waits for the running ones. It returns
// false if ctx ends first; abandoned handlers keep running until they return.
func (r *router) drain(ctx context.Context) bool {
	r.mu.Lock()
	r.closing = true
	r.closeIdleLocked()
	r.mu.Unlock()

	select {
	case <-r.idle:
		return true
	case <-ctx.Done():
		r.mu.Lock()
		remaining := r.inflight
		r.mu.Unlock()
		logger.Log.Warningf("shutdown: %d command(s) still running, abandoning them", remaining)
		return false
	}
}
