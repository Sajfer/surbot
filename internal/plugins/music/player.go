package music

import (
	"context"
	"errors"
	"math/rand"
	"runtime/debug"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// voiceJoiner joins a guild voice channel; Join blocks until the connection is usable.
type voiceJoiner interface {
	Join(guildID, channelID string) (voiceConn, error)
}

// voiceConn is one joined voice channel.
type voiceConn interface {
	// Play streams song and blocks until it ends, fails or ctx is cancelled
	// (then it returns ctx.Err()). When Play returns, the yt-dlp and ffmpeg
	// processes it started have exited and its audio goroutine is done.
	Play(ctx context.Context, song *Song) error
	// Leave disconnects from the channel. It never panics.
	Leave() error
}

// announcer posts player updates to Discord. Implementations log their errors.
type announcer interface {
	NowPlaying(channelID string, song *Song)
	Text(channelID, text string)
	Listening(title string) // "" clears the status
}

var (
	errClosed         = errors.New("music player is shut down")
	errNoVoiceChannel = errors.New("not in a voice channel")
)

// player owns one guild's queue and the single goroutine that joins voice and
// plays it. Every method is safe for concurrent use and none blocks on I/O
// except Wait.
type player struct {
	guildID     string
	ctx         context.Context // plugin lifetime; cancelled by Teardown
	voice       voiceJoiner
	out         announcer
	idleTimeout time.Duration
	log         *logrus.Entry

	mu          sync.Mutex // guards everything below
	queue       []*Song
	current     *Song
	textChannel string // where Now playing and errors go
	wantChannel string // voice channel the loop should be connected to
	connChannel string // channel conn was joined in
	conn        voiceConn
	stopped     bool // !stop: keep the queue, do not start the next song
	leave       bool // !disconnect: the running loop must leave voice and exit
	cancelTrack context.CancelFunc
	running     bool
	done        chan struct{} // closed when the latest loop has fully exited
	wake        chan struct{} // capacity 1, non-blocking sends
}

func newPlayer(ctx context.Context, guildID string, v voiceJoiner, out announcer, idle time.Duration, log *logrus.Entry) *player {
	return &player{
		guildID:     guildID,
		ctx:         ctx,
		voice:       v,
		out:         out,
		idleTimeout: idle,
		log:         log,
		wake:        make(chan struct{}, 1),
	}
}

// Play queues songs. voiceChannelID is the channel to join when the player is
// not already in voice; it may be empty while the player is active.
func (p *player) Play(songs []*Song, voiceChannelID, textChannelID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctx.Err() != nil {
		return errClosed
	}
	active := p.running && !p.leave
	if !active && voiceChannelID == "" {
		return errNoVoiceChannel
	}

	// The bot follows the requester only when nothing is playing and nothing
	// is about to: never between songs of a running queue.
	idle := p.current == nil && (p.stopped || len(p.queue) == 0)
	p.queue = append(p.queue, songs...)
	if textChannelID != "" {
		p.textChannel = textChannelID
	}
	if voiceChannelID != "" && (!active || idle) {
		p.wantChannel = voiceChannelID
	}
	p.stopped = false

	if active {
		p.signal()
		return nil
	}
	// A loop that is still leaving voice must finish first: discordgo keeps one
	// voice connection per guild, so its Leave would drop the new loop's join.
	p.leave = false
	p.running = true
	prev := p.done
	p.done = make(chan struct{})
	go p.loop(prev, p.done)
	return nil
}

func (p *player) signal() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// Active reports whether a loop is joining, playing or idling in voice.
func (p *player) Active() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running && !p.leave
}

// SetTextChannel sets where announcements go.
func (p *player) SetTextChannel(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.textChannel = id
}

// Skip ends the current song; the next one starts.
func (p *player) Skip() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancelTrack != nil {
		p.cancelTrack()
	}
}

// Stop ends the current song and keeps the queue; nothing plays until the next Play.
func (p *player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return
	}
	p.stopped = true
	if p.cancelTrack != nil {
		p.cancelTrack()
	}
	p.signal()
}

// Disconnect ends the current song and leaves voice, keeping the queue.
func (p *player) Disconnect() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return
	}
	p.leave = true
	if p.cancelTrack != nil {
		p.cancelTrack()
	}
	p.signal()
}

// Shuffle reorders the queued songs; the current song is not part of the queue.
func (p *player) Shuffle() {
	p.mu.Lock()
	defer p.mu.Unlock()
	// #nosec G404 -- shuffling a song queue, no need for crypto/rand
	rand.Shuffle(len(p.queue), func(i, j int) { p.queue[i], p.queue[j] = p.queue[j], p.queue[i] })
}

// ClearQueue drops the queued songs; the current song keeps playing.
func (p *player) ClearQueue() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.queue = nil
}

// Snapshot returns copies of the current song and the queue, safe to read unlocked.
func (p *player) Snapshot() (current *Song, queue []Song) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current != nil {
		c := *p.current
		current = &c
	}
	queue = make([]Song, len(p.queue))
	for i, s := range p.queue {
		queue[i] = *s
	}
	return current, queue
}

// Wait blocks until the latest loop has exited, or ctx ends.
func (p *player) Wait(ctx context.Context) error {
	p.mu.Lock()
	done := p.done
	p.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// workLocked reports whether the loop has something to start.
func (p *player) workLocked() bool { return !p.stopped && len(p.queue) > 0 }

// endLocked marks the loop finished unless a newer loop has taken over.
func (p *player) endLocked(done chan struct{}) {
	if p.done == done {
		p.running = false
	}
}

// loop is the only goroutine a player starts. It exits when the plugin shuts
// down, on Disconnect, after the idle timeout, or when a join fails, and it
// always leaves voice before closing done.
func (p *player) loop(prev, done chan struct{}) {
	if prev != nil {
		<-prev
	}
	defer close(done)
	defer p.exit(done)

	timer := time.NewTimer(p.idleTimeout)
	timer.Stop()
	defer timer.Stop()

	for {
		p.mu.Lock()
		if p.ctx.Err() != nil || p.leave || p.done != done {
			p.endLocked(done)
			p.mu.Unlock()
			return
		}

		if p.workLocked() {
			if p.conn != nil && p.connChannel != p.wantChannel {
				p.leaveOldLocked()
				continue
			}
			if p.conn == nil {
				if !p.joinLocked(done) {
					return
				}
				continue
			}
			song, trackCtx, cancel := p.popLocked()
			conn := p.conn
			text := p.textChannel
			p.mu.Unlock()
			p.playTrack(trackCtx, cancel, conn, song, text)
			continue
		}

		p.mu.Unlock()
		p.out.Listening("")
		timer.Reset(p.idleTimeout)
		select {
		case <-p.wake:
		case <-p.ctx.Done():
		case <-timer.C:
			p.mu.Lock()
			if p.workLocked() || len(p.wake) > 0 || p.ctx.Err() != nil || p.leave {
				p.mu.Unlock()
				continue
			}
			p.endLocked(done)
			p.mu.Unlock()
			p.log.Info("Idle timeout, leaving channel")
			return
		}
	}
}

// leaveOldLocked is called with mu held and returns with it released. The loop
// then re-evaluates, so a Disconnect or shutdown during Leave is honoured and
// the join targets the latest wantChannel.
func (p *player) leaveOldLocked() {
	old := p.conn
	p.mu.Unlock()
	if err := old.Leave(); err != nil {
		p.log.WithError(err).Warning("could not leave voice channel")
	}
	p.mu.Lock()
	p.conn = nil
	p.connChannel = ""
	p.mu.Unlock()
}

// joinLocked is called with mu held and returns with it released. It reports
// false when the loop must exit.
func (p *player) joinLocked(done chan struct{}) bool {
	channel := p.wantChannel
	p.mu.Unlock()
	c, err := p.voice.Join(p.guildID, channel)
	p.mu.Lock()
	if err != nil {
		abandoned := p.ctx.Err() != nil || p.leave || p.done != done
		text := p.textChannel
		p.endLocked(done)
		p.mu.Unlock()
		if !abandoned {
			p.log.WithError(err).Warning("could not join voice channel")
			p.out.Text(text, "Failed to play song")
		}
		return false
	}
	// The join ran unlocked and can take seconds; the decision at the top of
	// the loop is made again with whatever Disconnect, Stop or shutdown did
	// meanwhile. The connection is kept so the exit path leaves it.
	p.conn = c
	p.connChannel = channel
	p.mu.Unlock()
	return true
}

func (p *player) popLocked() (*Song, context.Context, context.CancelFunc) {
	song := p.queue[0]
	p.queue[0] = nil
	p.queue = p.queue[1:]
	p.current = song
	trackCtx, cancel := context.WithCancel(p.ctx)
	p.cancelTrack = cancel
	return song, trackCtx, cancel
}

func (p *player) playTrack(ctx context.Context, cancel context.CancelFunc, conn voiceConn, song *Song, textChannel string) {
	defer func() {
		cancel()
		p.mu.Lock()
		p.current = nil
		p.cancelTrack = nil
		p.mu.Unlock()
	}()
	p.out.NowPlaying(textChannel, song)
	p.out.Listening(song.Title)
	p.log.Infof("Now playing: %s", song.Title)
	if err := conn.Play(ctx, song); err != nil && !errors.Is(err, context.Canceled) {
		p.log.WithError(err).Warningf("could not play %s", song.Title)
	}
}

// exit runs when the loop ends, normally or by panic. It leaves voice and
// resets the per-connection state before done closes.
func (p *player) exit(done chan struct{}) {
	if r := recover(); r != nil {
		p.log.Errorf("music player panic: %v\n%s", r, debug.Stack())
	}

	p.mu.Lock()
	p.endLocked(done)
	conn := p.conn
	p.mu.Unlock()

	if conn != nil {
		if err := conn.Leave(); err != nil {
			p.log.WithError(err).Warning("could not leave voice channel")
		}
	}
	p.out.Listening("")

	p.mu.Lock()
	p.conn = nil
	p.connChannel = ""
	p.current = nil
	p.cancelTrack = nil
	if p.done == done {
		p.leave = false
	}
	p.mu.Unlock()
}
