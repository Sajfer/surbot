package music

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

const waitTimeout = 2 * time.Second

// fakeVoice is the Discord voice boundary. Every conn it hands out shares its
// channels, so a test sees one ordered stream of what happened.
type fakeVoice struct {
	t *testing.T

	started chan string // song title, sent when a conn's Play begins
	finish  chan error  // a test sends here to end the current Play
	leaves  chan struct{}

	joinEntered chan struct{} // set together with joinBlock
	joinBlock   chan struct{} // when non-nil, Join blocks until it is closed

	mu         sync.Mutex
	failJoins  int
	panicOn    string
	autoFinish bool // Play returns nil at once
	ignoreStop bool // Play ignores ctx cancellation until finish is sent
	events     []string
	inFlight   atomic.Int32
}

func newFakeVoice(t *testing.T) *fakeVoice {
	return &fakeVoice{
		t:       t,
		started: make(chan string, 1000),
		finish:  make(chan error),
		leaves:  make(chan struct{}, 1000),
	}
}

func (v *fakeVoice) record(e string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.events = append(v.events, e)
}

func (v *fakeVoice) log() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.events...)
}

func (v *fakeVoice) joins() []string {
	var out []string
	for _, e := range v.log() {
		if len(e) > 5 && e[:5] == "join:" {
			out = append(out, e[5:])
		}
	}
	return out
}

func (v *fakeVoice) Join(guildID, channelID string) (voiceConn, error) {
	v.record("join:" + guildID + "/" + channelID)
	if v.joinBlock != nil {
		v.joinEntered <- struct{}{}
		<-v.joinBlock
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.failJoins > 0 {
		v.failJoins--
		return nil, errors.New("join failed")
	}
	return &fakeConn{v: v}, nil
}

type fakeConn struct{ v *fakeVoice }

func (c *fakeConn) Play(ctx context.Context, song *Song) error {
	v := c.v
	if v.inFlight.Add(1) > 1 {
		v.t.Errorf("two Play calls overlap (second song %q)", song.Title)
	}
	defer v.inFlight.Add(-1)
	defer v.record("return:" + song.Title)

	v.mu.Lock()
	panicOn, auto, ignore := v.panicOn, v.autoFinish, v.ignoreStop
	v.mu.Unlock()

	v.record("play:" + song.Title)
	v.started <- song.Title
	if song.Title == panicOn {
		panic("boom")
	}
	if auto {
		return nil
	}
	if ignore {
		return <-v.finish
	}
	select {
	case err := <-v.finish:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *fakeConn) Leave() error {
	c.v.record("leave")
	c.v.leaves <- struct{}{}
	return nil
}

// fakeAnnouncer records what the player tells Discord.
type fakeAnnouncer struct {
	mu        sync.Mutex
	nowPlay   []string
	texts     []string
	listening []string
	idle      chan struct{} // one send per Listening("")
}

func newFakeAnnouncer() *fakeAnnouncer { return &fakeAnnouncer{idle: make(chan struct{}, 1000)} }

func (a *fakeAnnouncer) NowPlaying(channelID string, song *Song) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.nowPlay = append(a.nowPlay, channelID+":"+song.Title)
}

func (a *fakeAnnouncer) Text(channelID, text string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.texts = append(a.texts, channelID+":"+text)
}

func (a *fakeAnnouncer) Listening(title string) {
	a.mu.Lock()
	a.listening = append(a.listening, title)
	a.mu.Unlock()
	if title == "" {
		a.idle <- struct{}{}
	}
}

func (a *fakeAnnouncer) snapshot() (nowPlay, texts, listening []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.nowPlay...), append([]string(nil), a.texts...), append([]string(nil), a.listening...)
}

type harness struct {
	t      *testing.T
	v      *fakeVoice
	a      *fakeAnnouncer
	p      *player
	cancel context.CancelFunc
}

func newHarness(t *testing.T, idle time.Duration) *harness {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	v, a := newFakeVoice(t), newFakeAnnouncer()
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	h := &harness{t: t, v: v, a: a, cancel: cancel,
		p: newPlayer(ctx, "g1", v, a, idle, logrus.NewEntry(logger))}
	t.Cleanup(func() {
		cancel()
		wctx, wcancel := context.WithTimeout(context.Background(), waitTimeout)
		defer wcancel()
		if err := h.p.Wait(wctx); err != nil {
			t.Errorf("player did not exit at cleanup: %v", err)
		}
	})
	return h
}

func song(title string) *Song { return &Song{Title: title} }

func songs(titles ...string) []*Song {
	out := make([]*Song, len(titles))
	for i, title := range titles {
		out[i] = song(title)
	}
	return out
}

func (h *harness) play(voiceChannel string, titles ...string) {
	h.t.Helper()
	if err := h.p.Play(songs(titles...), voiceChannel, "t1"); err != nil {
		h.t.Fatalf("Play(%v) returned %v", titles, err)
	}
}

func (h *harness) expectStarted(want string) {
	h.t.Helper()
	select {
	case got := <-h.v.started:
		if got != want {
			h.t.Fatalf("started song %q, want %q", got, want)
		}
	case <-time.After(waitTimeout):
		h.t.Fatalf("song %q did not start within %v", want, waitTimeout)
	}
}

func (h *harness) finish(err error) {
	h.t.Helper()
	select {
	case h.v.finish <- err:
	case <-time.After(waitTimeout):
		h.t.Fatal("no song was playing to finish")
	}
}

func (h *harness) expectLeave() {
	h.t.Helper()
	select {
	case <-h.v.leaves:
	case <-time.After(waitTimeout):
		h.t.Fatal("Leave was not called")
	}
}

func (h *harness) expectIdle() {
	h.t.Helper()
	select {
	case <-h.a.idle:
	case <-time.After(waitTimeout):
		h.t.Fatal("player did not go idle")
	}
}

func (h *harness) wait() {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	if err := h.p.Wait(ctx); err != nil {
		h.t.Fatalf("Wait: %v", err)
	}
}

func queueTitles(q []Song) []string {
	out := make([]string, len(q))
	for i, s := range q {
		out[i] = s.Title
	}
	return out
}

func TestPlayReturnsBeforeSongEndsAndStartsNextWithoutRejoining(t *testing.T) {
	h := newHarness(t, time.Hour)

	h.play("v1", "A", "B")
	h.expectStarted("A")
	h.finish(nil)
	h.expectStarted("B")

	if got, want := h.v.joins(), []string{"g1/v1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("joins = %v, want %v", got, want)
	}
	nowPlay, _, _ := h.a.snapshot()
	if want := []string{"t1:A", "t1:B"}; !reflect.DeepEqual(nowPlay, want) {
		t.Errorf("now playing announcements = %v, want %v", nowPlay, want)
	}
}

func TestPlayFromAnotherChannelWhilePlayingQueuesWithoutMovingTheBot(t *testing.T) {
	h := newHarness(t, time.Hour)
	h.play("v1", "A")
	h.expectStarted("A")

	h.play("v2", "C")
	h.finish(nil)
	h.expectStarted("C")

	if got, want := h.v.joins(), []string{"g1/v1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("joins = %v, want %v", got, want)
	}
	if got := len(h.v.leaves); got != 0 {
		t.Errorf("Leave calls = %d, want 0", got)
	}
}

func TestPlayFromAnotherChannelWhenIdleMovesTheBotBeforePlaying(t *testing.T) {
	h := newHarness(t, time.Hour)
	h.play("v1", "A")
	h.expectStarted("A")
	h.finish(nil)
	h.expectIdle()

	h.play("v2", "B")
	h.expectStarted("B")

	want := []string{"join:g1/v1", "play:A", "return:A", "leave", "join:g1/v2", "play:B"}
	if got := h.v.log(); !reflect.DeepEqual(got, want) {
		t.Errorf("event order = %v, want %v", got, want)
	}
}

func TestPlayFromAnotherChannelWhenStoppedMovesTheBotAndKeepsTheQueue(t *testing.T) {
	h := newHarness(t, time.Hour)
	h.play("v1", "A", "B")
	h.expectStarted("A")
	h.p.Stop()
	h.expectIdle()

	h.play("v2", "C")
	h.expectStarted("B")

	want := []string{"join:g1/v1", "play:A", "return:A", "leave", "join:g1/v2", "play:B"}
	if got := h.v.log(); !reflect.DeepEqual(got, want) {
		t.Errorf("event order = %v, want %v", got, want)
	}
	if _, q := h.p.Snapshot(); !reflect.DeepEqual(queueTitles(q), []string{"C"}) {
		t.Errorf("queue = %v, want [C]", queueTitles(q))
	}
}

func TestPlayFromTheSameChannelWhenIdleDoesNotRejoin(t *testing.T) {
	h := newHarness(t, time.Hour)
	h.play("v1", "A")
	h.expectStarted("A")
	h.finish(nil)
	h.expectIdle()

	h.play("v1", "B")
	h.expectStarted("B")

	want := []string{"join:g1/v1", "play:A", "return:A", "play:B"}
	if got := h.v.log(); !reflect.DeepEqual(got, want) {
		t.Errorf("event order = %v, want %v", got, want)
	}
}

func TestMoveRejoinFailureKeepsQueueAndEndsLoop(t *testing.T) {
	h := newHarness(t, time.Hour)
	h.play("v1", "A")
	h.expectStarted("A")
	h.finish(nil)
	h.expectIdle()
	h.v.mu.Lock()
	h.v.failJoins = 1
	h.v.mu.Unlock()

	h.play("v2", "B")
	h.wait()

	_, texts, _ := h.a.snapshot()
	if want := []string{"t1:Failed to play song"}; !reflect.DeepEqual(texts, want) {
		t.Errorf("texts = %v, want %v", texts, want)
	}
	if _, q := h.p.Snapshot(); !reflect.DeepEqual(queueTitles(q), []string{"B"}) {
		t.Errorf("queue = %v, want [B]", queueTitles(q))
	}
	if got := len(h.v.leaves); got != 1 {
		t.Errorf("Leave calls = %d, want 1 (old channel only)", got)
	}
}

func TestPlayWithoutVoiceChannelAndNoLoopQueuesNothing(t *testing.T) {
	h := newHarness(t, time.Hour)

	err := h.p.Play(songs("A"), "", "t1")

	if !errors.Is(err, errNoVoiceChannel) {
		t.Fatalf("Play error = %v, want errNoVoiceChannel", err)
	}
	if _, q := h.p.Snapshot(); len(q) != 0 {
		t.Errorf("queue = %v, want empty", queueTitles(q))
	}
}

func TestPlayAfterShutdownReturnsErrClosedAndJoinsNothing(t *testing.T) {
	h := newHarness(t, time.Hour)
	h.cancel()

	err := h.p.Play(songs("A"), "v1", "t1")

	if !errors.Is(err, errClosed) {
		t.Fatalf("Play error = %v, want errClosed", err)
	}
	if got := h.v.joins(); len(got) != 0 {
		t.Errorf("joins = %v, want none", got)
	}
}

func TestSkipEndsCurrentSongAndStartsNext(t *testing.T) {
	h := newHarness(t, time.Hour)
	h.play("v1", "A", "B")
	h.expectStarted("A")

	h.p.Skip()

	h.expectStarted("B")
}

func TestControlsWithoutLoopDoNothing(t *testing.T) {
	h := newHarness(t, time.Hour)

	h.p.Skip()
	h.p.Stop()
	h.p.Disconnect()
	h.p.Shuffle()
	h.p.ClearQueue()

	if cur, q := h.p.Snapshot(); cur != nil || len(q) != 0 {
		t.Errorf("Snapshot = %v, %v; want nothing", cur, queueTitles(q))
	}
	if h.p.Active() {
		t.Error("Active() = true with no loop")
	}
}

func TestStopCancelsSongKeepsQueueAndStaysInVoice(t *testing.T) {
	h := newHarness(t, time.Hour)
	h.play("v1", "A", "B")
	h.expectStarted("A")

	h.p.Stop()
	h.expectIdle()

	cur, q := h.p.Snapshot()
	if cur != nil || !reflect.DeepEqual(queueTitles(q), []string{"B"}) {
		t.Errorf("after Stop: current=%v queue=%v, want nil and [B]", cur, queueTitles(q))
	}
	if len(h.v.leaves) != 0 {
		t.Error("Leave was called by Stop")
	}

	h.play("v1", "C")
	h.expectStarted("B")
	h.finish(nil)
	h.expectStarted("C")
	if got := h.v.joins(); len(got) != 1 {
		t.Errorf("joins = %v, want a single join", got)
	}
}

func TestDisconnectLeavesVoiceKeepsQueueAndLaterPlayJoinsNewChannel(t *testing.T) {
	h := newHarness(t, time.Hour)
	h.play("v1", "A", "B")
	h.expectStarted("A")

	h.p.Disconnect()
	h.expectLeave()
	h.wait()
	if _, q := h.p.Snapshot(); !reflect.DeepEqual(queueTitles(q), []string{"B"}) {
		t.Errorf("queue after Disconnect = %v, want [B]", queueTitles(q))
	}

	h.play("v2", "D")
	h.expectStarted("B")
	h.finish(nil)
	h.expectStarted("D")

	if got, want := h.v.joins(), []string{"g1/v1", "g1/v2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("joins = %v, want %v", got, want)
	}
}

func TestPlayRightAfterDisconnectLeavesOldChannelBeforeJoiningNew(t *testing.T) {
	h := newHarness(t, time.Hour)
	h.play("v1", "A")
	h.expectStarted("A")

	h.p.Disconnect()
	h.play("v2", "B")
	h.expectStarted("B")

	want := []string{"join:g1/v1", "play:A", "return:A", "leave", "join:g1/v2", "play:B"}
	if got := h.v.log(); !reflect.DeepEqual(got, want) {
		t.Errorf("event order = %v, want %v", got, want)
	}
}

func TestLeavesVoiceAfterIdleTimeout(t *testing.T) {
	tests := []struct {
		name string
		end  func(h *harness)
	}{
		{name: "last song finishes", end: func(h *harness) { h.finish(nil) }},
		{name: "last song skipped", end: func(h *harness) { h.p.Skip() }},
		{name: "playback stopped", end: func(h *harness) { h.p.Stop() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, 50*time.Millisecond)
			h.play("v1", "A")
			h.expectStarted("A")

			tt.end(h)

			h.expectLeave()
			h.wait()
		})
	}
}

func TestJoinFailureAnnouncesKeepsQueueAndNextPlayRetries(t *testing.T) {
	h := newHarness(t, time.Hour)
	h.v.failJoins = 1

	h.play("v1", "A")
	h.wait()

	_, texts, _ := h.a.snapshot()
	if want := []string{"t1:Failed to play song"}; !reflect.DeepEqual(texts, want) {
		t.Errorf("texts = %v, want %v", texts, want)
	}
	if len(h.v.leaves) != 0 {
		t.Error("Leave called although the join failed")
	}
	if _, q := h.p.Snapshot(); !reflect.DeepEqual(queueTitles(q), []string{"A"}) {
		t.Errorf("queue = %v, want [A]", queueTitles(q))
	}

	h.play("v1")
	h.expectStarted("A")
}

func TestSongErrorStillStartsNextSong(t *testing.T) {
	h := newHarness(t, time.Hour)
	h.play("v1", "A", "B")
	h.expectStarted("A")

	h.finish(errors.New("stream broke"))

	h.expectStarted("B")
}

func TestClearQueueKeepsCurrentSongAndDropsTheRest(t *testing.T) {
	h := newHarness(t, 50*time.Millisecond)
	h.play("v1", "A", "B", "C")
	h.expectStarted("A")

	h.p.ClearQueue()
	h.finish(nil)

	h.expectLeave()
	if got := len(h.v.started); got != 0 {
		t.Errorf("%d more songs started after ClearQueue", got)
	}
}

func TestShuffleKeepsTheSameSongs(t *testing.T) {
	h := newHarness(t, time.Hour)
	h.play("v1", "A")
	h.expectStarted("A")
	h.play("", "B", "C", "D", "E", "F", "G")

	h.p.Shuffle()

	_, q := h.p.Snapshot()
	got := queueTitles(q)
	sort.Strings(got)
	if want := []string{"B", "C", "D", "E", "F", "G"}; !reflect.DeepEqual(got, want) {
		t.Errorf("queue after Shuffle = %v, want the same songs %v", got, want)
	}
}

func TestPlayRacingIdleExitNeverStrandsASong(t *testing.T) {
	h := newHarness(t, time.Millisecond)
	h.v.autoFinish = true
	const total = 50

	for i := range total {
		h.play("v1", fmt.Sprintf("s%d", i))
		time.Sleep(time.Millisecond)
	}

	seen := make(map[string]bool)
	for range total {
		select {
		case title := <-h.v.started:
			seen[title] = true
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d of %d songs played", len(seen), total)
		}
	}
	if len(seen) != total {
		t.Errorf("%d distinct songs played, want %d", len(seen), total)
	}
}

func TestConcurrentControlsAreRaceFreeAndNeverOverlapPlayback(t *testing.T) {
	h := newHarness(t, time.Hour)
	h.v.autoFinish = true
	var wg sync.WaitGroup

	for g := range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 20 {
				switch (g + i) % 5 {
				case 0:
					_ = h.p.Play(songs("x", "y"), "v1", "t1")
				case 1:
					h.p.Skip()
				case 2:
					h.p.Shuffle()
				case 3:
					h.p.Snapshot()
				case 4:
					h.p.ClearQueue()
				}
			}
		}()
	}
	wg.Wait()
}

func TestPanicInPlaybackLeavesVoiceAndLaterPlayStartsNewLoop(t *testing.T) {
	h := newHarness(t, time.Hour)
	h.v.panicOn = "bomb"

	h.play("v1", "bomb")
	h.expectStarted("bomb")
	h.expectLeave()
	h.wait()

	h.v.mu.Lock()
	h.v.panicOn = ""
	h.v.mu.Unlock()
	h.play("v1", "after")
	h.expectStarted("after")
}

func TestDisconnectDuringJoinStartsNoSongAndLeavesTheNewConnection(t *testing.T) {
	h := newHarness(t, time.Hour)
	h.v.joinEntered = make(chan struct{}, 1)
	h.v.joinBlock = make(chan struct{})
	h.play("v1", "A")
	<-h.v.joinEntered

	h.p.Disconnect()
	close(h.v.joinBlock)

	h.expectLeave()
	h.wait()
	if got := len(h.v.started); got != 0 {
		t.Errorf("%d songs started after Disconnect during Join", got)
	}
	if _, q := h.p.Snapshot(); !reflect.DeepEqual(queueTitles(q), []string{"A"}) {
		t.Errorf("queue = %v, want [A] kept", queueTitles(q))
	}
}

func TestShutdownDuringJoinStartsNoSongAndLeavesTheNewConnection(t *testing.T) {
	h := newHarness(t, time.Hour)
	h.v.joinEntered = make(chan struct{}, 1)
	h.v.joinBlock = make(chan struct{})
	h.play("v1", "A")
	<-h.v.joinEntered

	h.cancel()
	close(h.v.joinBlock)

	h.expectLeave()
	h.wait()
	if got := len(h.v.started); got != 0 {
		t.Errorf("%d songs started after shutdown during Join", got)
	}
}

func TestStopDuringJoinHoldsBackTheQueue(t *testing.T) {
	h := newHarness(t, time.Hour)
	h.v.joinEntered = make(chan struct{}, 1)
	h.v.joinBlock = make(chan struct{})
	h.play("v1", "A")
	<-h.v.joinEntered

	h.p.Stop()
	close(h.v.joinBlock)
	h.expectIdle()

	if got := len(h.v.started); got != 0 {
		t.Errorf("%d songs started after Stop during Join", got)
	}
	if _, q := h.p.Snapshot(); !reflect.DeepEqual(queueTitles(q), []string{"A"}) {
		t.Errorf("queue = %v, want [A]", queueTitles(q))
	}
}
