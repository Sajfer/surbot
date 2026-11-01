package music

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sajfer/discordgo"

	"gitlab.com/sajfer/surbot/internal/core"
)

const (
	maxQueueLines = 20
	// maxQueueTitle keeps 20 lines well under Discord's 2000 characters:
	// YouTube titles can be 100 characters, which would not fit.
	maxQueueTitle = 80
	noSongsText   = "No songs queued. " + playHint
)

// request is one music command with everything the handlers need, so they can
// be tested without a Discord connection.
type request struct {
	session      *discordgo.Session // nil in tests
	guildID      string
	textChannel  string
	voiceChannel string // author's current voice channel, "" if none
	args         string
	reply        func(text string) error
	replyEmbed   func(*discordgo.MessageEmbed) error
}

type commandSpec struct {
	name, usage, description string
	handler                  func(*Plugin, context.Context, request) error
}

var commandSpecs = []commandSpec{
	{"play", "<youtube link|spotify link|search>", "Queue a song and start playing in your voice channel", (*Plugin).play},
	{"playing", "", "Show the current song", (*Plugin).playing},
	{"queue", "", "Show the queue", (*Plugin).queue},
	{"shuffle", "", "Shuffle the queue", (*Plugin).shuffle},
	{"skip", "", "Skip the current song", (*Plugin).skip},
	{"stop", "", "Stop playing", (*Plugin).stop},
	{"clearQueue", "", "Clear the queue", (*Plugin).clearQueue},
	{"disconnect", "", "Leave the voice channel", (*Plugin).disconnect},
}

// adapt turns a request handler into a core command handler.
func (p *Plugin) adapt(fn func(*Plugin, context.Context, request) error) core.HandlerFunc {
	return func(ctx context.Context, c *core.Context) error {
		m := c.Message
		if m.GuildID == "" {
			return c.Reply("Music commands only work in a server.")
		}
		voiceChannel := ""
		if m.Author != nil {
			if vs, err := c.Session.State.VoiceState(m.GuildID, m.Author.ID); err == nil {
				voiceChannel = vs.ChannelID
			}
		}
		return fn(p, ctx, request{
			session:      c.Session,
			guildID:      m.GuildID,
			textChannel:  m.ChannelID,
			voiceChannel: voiceChannel,
			args:         c.Args,
			reply:        c.Reply,
			replyEmbed: func(e *discordgo.MessageEmbed) error {
				_, err := c.Session.ChannelMessageSendEmbed(m.ChannelID, e)
				return err
			},
		})
	}
}

// existing returns the guild's player, if it has one, and points its
// announcements at the channel of this command.
func (p *Plugin) existing(req request) *player {
	pl := p.lookup(req.guildID)
	if pl != nil {
		pl.SetTextChannel(req.textChannel)
	}
	return pl
}

func (p *Plugin) play(ctx context.Context, req request) error {
	if req.args == "" {
		return req.reply("Usage: play <youtube link|spotify link|search>")
	}
	pl, err := p.getOrCreate(req.guildID, req.session)
	if err != nil {
		return nil
	}
	pl.SetTextChannel(req.textChannel)
	if !pl.Active() && req.voiceChannel == "" {
		return req.reply("Join a voice channel first.")
	}

	songs, err := p.resolve.Resolve(ctx, req.args)
	if err != nil {
		p.log.WithError(err).Warningf("could not resolve %q", req.args)
		if ctx.Err() != nil {
			return nil
		}
		return req.reply("Failed to fetch song information")
	}

	err = pl.Play(songs, req.voiceChannel, req.textChannel)
	switch {
	case err == nil, errors.Is(err, errClosed):
		return nil
	case errors.Is(err, errNoVoiceChannel):
		return req.reply("Join a voice channel first.")
	}
	return err
}

func (p *Plugin) playing(_ context.Context, req request) error {
	pl := p.existing(req)
	if pl == nil {
		return req.replyEmbed(notPlayingEmbed())
	}
	current, _ := pl.Snapshot()
	if current == nil {
		return req.replyEmbed(notPlayingEmbed())
	}
	return req.replyEmbed(nowPlayingEmbed(current))
}

func (p *Plugin) queue(_ context.Context, req request) error {
	pl := p.existing(req)
	if pl == nil {
		return req.reply(queueText(nil, nil))
	}
	current, queue := pl.Snapshot()
	return req.reply(queueText(current, queue))
}

func (p *Plugin) shuffle(_ context.Context, req request) error {
	if pl := p.existing(req); pl != nil {
		pl.Shuffle()
	}
	return nil
}

func (p *Plugin) skip(_ context.Context, req request) error {
	if pl := p.existing(req); pl != nil {
		pl.Skip()
	}
	return nil
}

func (p *Plugin) stop(_ context.Context, req request) error {
	if pl := p.existing(req); pl != nil {
		pl.Stop()
	}
	return nil
}

func (p *Plugin) disconnect(_ context.Context, req request) error {
	if pl := p.existing(req); pl != nil {
		pl.Disconnect()
	}
	return nil
}

func (p *Plugin) clearQueue(_ context.Context, req request) error {
	if pl := p.existing(req); pl != nil {
		pl.ClearQueue()
	}
	return req.replyEmbed(queueClearedEmbed())
}

// queueText lists the current song first, then the queue, at most
// maxQueueLines lines in total.
func queueText(current *Song, queue []Song) string {
	if current == nil && len(queue) == 0 {
		return noSongsText
	}
	titles := make([]string, 0, len(queue)+1)
	if current != nil {
		titles = append(titles, current.Title)
	}
	for _, s := range queue {
		titles = append(titles, s.Title)
	}

	var b strings.Builder
	b.WriteString("Queue:")
	for i, title := range titles {
		if i == maxQueueLines {
			fmt.Fprintf(&b, "\n… and %d more", len(titles)-maxQueueLines)
			break
		}
		fmt.Fprintf(&b, "\n%d. %s", i+1, shorten(title, maxQueueTitle))
	}
	return b.String()
}

// shorten cuts s to at most limit characters, ending in "…" when cut.
func shorten(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit-1]) + "…"
}
