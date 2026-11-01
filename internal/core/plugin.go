// Package core runs the bot: gateway lifecycle, command routing and plugin
// Setup/Teardown. It knows nothing about individual features.
package core

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/sajfer/discordgo"
	"github.com/sirupsen/logrus"

	"gitlab.com/sajfer/surbot/internal/config"
	"gitlab.com/sajfer/surbot/internal/logger"
)

// Plugin is one feature. Constructors must not do I/O; Setup does the work.
type Plugin interface {
	Name() string
	Setup(ctx context.Context, r Registrar) error
	Teardown(ctx context.Context) error
}

// Registrar is what a plugin uses to hook into the bot during Setup.
type Registrar interface {
	// Command registers a prefix command. It fails on an empty name, a nil
	// handler or a name that is already taken (including "help").
	Command(cmd Command) error
	// AddHandler registers a discordgo event handler. Core removes it before
	// the plugin's Teardown.
	AddHandler(handler any) error
	Services() *Services
}

// Command is a prefix command such as "!ping".
type Command struct {
	Name        string // matched exactly, case-sensitive
	Usage       string // argument hint shown by help, e.g. "<link|search>"
	Description string
	Handler     HandlerFunc
}

// HandlerFunc handles one command invocation. A returned error is logged and
// answered with a generic failure reply; call Context.Reply for specific messages.
type HandlerFunc func(ctx context.Context, c *Context) error

// Context describes one command invocation.
type Context struct {
	Session *discordgo.Session
	Message *discordgo.MessageCreate
	Args    string // text after the command name, trimmed
	send    func(channelID, text string) error
}

// Reply sends text to the channel the command came from, with mentions turned
// off and cut to fit Discord's limit (see SendText).
func (c *Context) Reply(text string) error {
	return c.send(c.Message.ChannelID, text)
}

// Services are the shared dependencies handed to each plugin.
type Services struct {
	Config *config.Config
	Log    *logrus.Entry // prefixed with the plugin name
	HTTP   *http.Client  // shared by plugins that call external services
}

const httpTimeout = 10 * time.Second

type scope struct {
	plugin   string
	registry *registry
	gw       gateway
	services *Services
	removers []func()
}

func newScope(plugin string, reg *registry, gw gateway, cfg *config.Config, httpClient *http.Client) *scope {
	return &scope{
		plugin:   plugin,
		registry: reg,
		gw:       gw,
		services: &Services{
			Config: cfg,
			Log:    logger.Log.WithField("prefix", plugin),
			HTTP:   httpClient,
		},
	}
}

func (s *scope) Command(cmd Command) error { return s.registry.add(s.plugin, cmd) }

func (s *scope) Services() *Services { return s.services }

func (s *scope) AddHandler(handler any) error {
	if !validHandlerShape(handler) {
		return fmt.Errorf("plugin %s: invalid event handler type %T", s.plugin, handler)
	}
	s.removers = append(s.removers, s.gw.AddHandler(handler))
	return nil
}

func (s *scope) removeHandlers() {
	for i := len(s.removers) - 1; i >= 0; i-- {
		s.removers[i]()
	}
	s.removers = nil
}

var (
	sessionPtrType = reflect.TypeOf((*discordgo.Session)(nil))
	discordgoPkg   = reflect.TypeOf(discordgo.Session{}).PkgPath()
)

// validHandlerShape mirrors discordgo's own check, which is unexported and on a
// mismatch only logs and returns a no-op remover. discordgo matches exact
// unnamed func types, so named func types are rejected too. It is an
// approximation: any pointer to a discordgo struct passes, even one that has
// no event handler.
func validHandlerShape(handler any) bool {
	t := reflect.TypeOf(handler)
	if t == nil || t.Kind() != reflect.Func || t.PkgPath() != "" {
		return false
	}
	if t.NumIn() != 2 || t.NumOut() != 0 || t.In(0) != sessionPtrType {
		return false
	}
	event := t.In(1)
	if event.Kind() == reflect.Interface {
		return event.NumMethod() == 0
	}
	return event.Kind() == reflect.Pointer && event.Elem().Kind() == reflect.Struct &&
		event.Elem().PkgPath() == discordgoPkg
}
