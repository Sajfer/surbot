package music

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/sajfer/discordgo"

	"github.com/sajfer/surbot/internal/core"
)

// recorder collects what a request handler replied.
type recorder struct {
	texts  []string
	embeds []*discordgo.MessageEmbed
	reacts []string
}

func (r *recorder) request(guild, textChannel, voiceChannel, args string) request {
	return request{
		guildID: guild, textChannel: textChannel, voiceChannel: voiceChannel, args: args,
		reply:      func(text string) error { r.texts = append(r.texts, text); return nil },
		replyEmbed: func(e *discordgo.MessageEmbed) error { r.embeds = append(r.embeds, e); return nil },
		react:      func(emoji string) { r.reacts = append(r.reacts, emoji) },
	}
}

func TestPlayResolvesTheQueryAsTypedJoinsTheAuthorsChannelAndReturnsWhilePlaying(t *testing.T) {
	res := &fakeResolver{songs: songs("A")}
	tp := newTestPlugin(t, res)

	tp.playIn("g1", "v1", "never gonna give")

	f := tp.fakes("g1")
	waitStarted(t, f, "A")
	if got, want := res.seen(), []string{"never gonna give"}; !reflect.DeepEqual(got, want) {
		t.Errorf("resolver queries = %v, want %v", got, want)
	}
	if got, want := f.v.joins(), []string{"g1/v1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("joins = %v, want %v", got, want)
	}
	if cur, _ := tp.lookup("g1").Snapshot(); cur == nil || cur.Title != "A" {
		t.Errorf("current song = %v, want A still playing after play returned", cur)
	}
}

func TestPlayWhileNotInVoiceAndBotIdleRepliesWithoutLookingAnythingUp(t *testing.T) {
	res := &fakeResolver{songs: songs("A")}
	tp := newTestPlugin(t, res)

	replies := tp.playIn("g1", "", "x")

	if want := []string{"Join a voice channel first."}; !reflect.DeepEqual(replies, want) {
		t.Errorf("replies = %v, want %v", replies, want)
	}
	if got := res.seen(); len(got) != 0 {
		t.Errorf("resolver was called: %v", got)
	}
}

func TestPlayFromAuthorNotInVoiceQueuesWhenBotIsActive(t *testing.T) {
	res := &fakeResolver{songs: songs("A")}
	tp := newTestPlugin(t, res)
	tp.playIn("g1", "v1", "first")
	waitStarted(t, tp.fakes("g1"), "A")
	res.mu.Lock()
	res.songs = songs("X")
	res.mu.Unlock()

	replies := tp.playIn("g1", "", "x")

	if len(replies) != 0 {
		t.Errorf("replies = %v, want none", replies)
	}
	if _, q := tp.lookup("g1").Snapshot(); !reflect.DeepEqual(queueTitles(q), []string{"X"}) {
		t.Errorf("queue = %v, want [X]", queueTitles(q))
	}
}

func TestPlayWhenResolverFailsRepliesAndQueuesNothing(t *testing.T) {
	res := &fakeResolver{err: errors.New("nothing found")}
	tp := newTestPlugin(t, res)

	replies := tp.playIn("g1", "v1", "zzz")

	if want := []string{"Failed to fetch song information"}; !reflect.DeepEqual(replies, want) {
		t.Errorf("replies = %v, want %v", replies, want)
	}
	if got := tp.fakes("g1").v.joins(); len(got) != 0 {
		t.Errorf("joins = %v, want none", got)
	}
}

func TestPlayWithoutArgumentsRepliesUsageWithoutLookup(t *testing.T) {
	res := &fakeResolver{songs: songs("A")}
	tp := newTestPlugin(t, res)

	replies := tp.playIn("g1", "v1", "")

	if want := []string{"Usage: play <youtube link|spotify link|search>"}; !reflect.DeepEqual(replies, want) {
		t.Errorf("replies = %v, want %v", replies, want)
	}
	if got := res.seen(); len(got) != 0 {
		t.Errorf("resolver was called: %v", got)
	}
}

func TestControlsWithoutPlayerSayNothingIsPlaying(t *testing.T) {
	tp := newTestPlugin(t, &fakeResolver{})
	ctx := context.Background()

	for name, tt := range map[string]struct {
		run  func(context.Context, request) error
		want string
	}{
		"skip":       {tp.skip, notPlaying},
		"stop":       {tp.stop, notPlaying},
		"disconnect": {tp.disconnect, notPlaying},
		"shuffle":    {tp.shuffle, noSongsText},
	} {
		var rec recorder
		if err := tt.run(ctx, rec.request("g1", "t1", "", "")); err != nil {
			t.Errorf("%s returned %v", name, err)
		}
		if !reflect.DeepEqual(rec.texts, []string{tt.want}) || len(rec.reacts) != 0 {
			t.Errorf("%s replied %v and reacted %v, want only %q", name, rec.texts, rec.reacts, tt.want)
		}
	}
}

func TestControlsWhilePlayingReactInsteadOfReplying(t *testing.T) {
	tp := newTestPlugin(t, &fakeResolver{songs: songs("A", "B", "C")})
	tp.playIn("g1", "v1", "x")
	waitStarted(t, tp.fakes("g1"), "A")

	for name, run := range map[string]func(context.Context, request) error{
		"shuffle": tp.shuffle, "skip": tp.skip,
	} {
		var rec recorder
		if err := run(context.Background(), rec.request("g1", "t1", "", "")); err != nil {
			t.Fatalf("%s returned %v", name, err)
		}
		if len(rec.texts) != 0 || !reflect.DeepEqual(rec.reacts, []string{ackEmoji}) {
			t.Errorf("%s replied %v and reacted %v, want only %s", name, rec.texts, rec.reacts, ackEmoji)
		}
	}
}

func TestPlayOneSongReactsAndManySongsSayHowMany(t *testing.T) {
	tp := newTestPlugin(t, &fakeResolver{songs: songs("A")})
	var one recorder
	if err := tp.play(context.Background(), one.request("g1", "t1", "v1", "a")); err != nil {
		t.Fatalf("play returned %v", err)
	}
	if len(one.texts) != 0 || !reflect.DeepEqual(one.reacts, []string{ackEmoji}) {
		t.Errorf("one song: replied %v and reacted %v, want only %s", one.texts, one.reacts, ackEmoji)
	}

	tp2 := newTestPlugin(t, &fakeResolver{songs: songs("A", "B", "C")})
	var many recorder
	if err := tp2.play(context.Background(), many.request("g1", "t1", "v1", "list")); err != nil {
		t.Fatalf("play returned %v", err)
	}
	if want := []string{"Queued 3 songs."}; !reflect.DeepEqual(many.texts, want) {
		t.Errorf("three songs: replies = %v, want %v", many.texts, want)
	}
}

func TestPlayMoreThanTheQueueHoldsSaysHowManyWereLeftOut(t *testing.T) {
	titles := make([]string, maxQueue+5)
	for i := range titles {
		titles[i] = fmt.Sprint(i)
	}
	tp := newTestPlugin(t, &fakeResolver{songs: songs(titles...)})

	replies := tp.playIn("g1", "v1", "huge playlist")

	want := fmt.Sprintf("Queued %d of %d songs; the queue holds at most %d.", maxQueue, maxQueue+5, maxQueue)
	if !reflect.DeepEqual(replies, []string{want}) {
		t.Errorf("replies = %v, want [%s]", replies, want)
	}
}

func TestQueueWithoutPlayerRepliesWithTheHint(t *testing.T) {
	tp := newTestPlugin(t, &fakeResolver{})
	var rec recorder

	err := tp.queue(context.Background(), rec.request("g1", "t1", "", ""))

	want := []string{"No songs queued. Use !play <youtube link|spotify link> to queue a song"}
	if err != nil || !reflect.DeepEqual(rec.texts, want) {
		t.Errorf("queue replied %v (err %v), want %v", rec.texts, err, want)
	}
}

func TestPlayingWithoutPlayerRepliesNotPlayingEmbed(t *testing.T) {
	tp := newTestPlugin(t, &fakeResolver{})
	var rec recorder

	err := tp.playing(context.Background(), rec.request("g1", "t1", "", ""))

	if err != nil || len(rec.embeds) != 1 || rec.embeds[0].Fields[0].Name != "Currently not playing" {
		t.Errorf("playing replied %+v (err %v), want the not-playing embed", rec.embeds, err)
	}
}

func TestPlayingShowsTheCurrentSongWithDuration(t *testing.T) {
	res := &fakeResolver{songs: []*Song{{Title: "A", Duration: 3725 * 1e9}}}
	tp := newTestPlugin(t, res)
	tp.playIn("g1", "v1", "x")
	waitStarted(t, tp.fakes("g1"), "A")
	var rec recorder

	err := tp.playing(context.Background(), rec.request("g1", "t1", "", ""))

	if err != nil || len(rec.embeds) != 1 {
		t.Fatalf("playing replied %+v (err %v), want one embed", rec.embeds, err)
	}
	fields := rec.embeds[0].Fields
	if fields[0].Name != "Now playing" || fields[0].Value != "A" || fields[1].Name != "Duration" || fields[1].Value != "1:02:05" {
		t.Errorf("embed fields = %+v %+v, want Now playing=A and Duration=1:02:05", fields[0], fields[1])
	}
}

func TestQueueShowsCurrentSongFirstThenQueue(t *testing.T) {
	res := &fakeResolver{songs: songs("A", "B")}
	tp := newTestPlugin(t, res)
	tp.playIn("g1", "v1", "x")
	waitStarted(t, tp.fakes("g1"), "A")
	var rec recorder

	err := tp.queue(context.Background(), rec.request("g1", "t1", "", ""))

	if want := []string{"Queue:\n1. A\n2. B"}; err != nil || !reflect.DeepEqual(rec.texts, want) {
		t.Errorf("queue replied %q (err %v), want %q", rec.texts, err, want)
	}
}

func TestClearQueueRepliesEvenWithoutPlayer(t *testing.T) {
	tp := newTestPlugin(t, &fakeResolver{})
	var rec recorder

	err := tp.clearQueue(context.Background(), rec.request("g1", "t1", "", ""))

	if err != nil || len(rec.embeds) != 1 || rec.embeds[0].Fields[0].Name != "Queue have been cleared" {
		t.Errorf("clearQueue replied %+v (err %v), want the cleared embed", rec.embeds, err)
	}
}

func TestSkipFromAnotherChannelMovesAnnouncementsThere(t *testing.T) {
	res := &fakeResolver{songs: songs("A", "B")}
	tp := newTestPlugin(t, res)
	tp.playIn("g1", "v1", "x")
	f := tp.fakes("g1")
	waitStarted(t, f, "A")
	var rec recorder

	if err := tp.skip(context.Background(), rec.request("g1", "t2", "", "")); err != nil {
		t.Fatalf("skip returned %v", err)
	}

	waitStarted(t, f, "B")
	nowPlay, _, _ := f.a.snapshot()
	if want := []string{"t1:A", "t2:B"}; !reflect.DeepEqual(nowPlay, want) {
		t.Errorf("now playing announcements = %v, want %v", nowPlay, want)
	}
}

func TestQueueText(t *testing.T) {
	tests := []struct {
		name    string
		current *Song
		queue   []Song
		want    string
	}{
		{name: "nothing queued", want: "No songs queued. Use !play <youtube link|spotify link> to queue a song"},
		{name: "current and queue", current: &Song{Title: "A"}, queue: []Song{{Title: "B"}, {Title: "C"}}, want: "Queue:\n1. A\n2. B\n3. C"},
		{name: "only queue", queue: []Song{{Title: "B"}}, want: "Queue:\n1. B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := queueText(tt.current, tt.queue); got != tt.want {
				t.Errorf("queueText = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestQueueTextCapsAtTwentyLinesAndCountsTheRest(t *testing.T) {
	queue := make([]Song, 25)
	for i := range queue {
		queue[i] = Song{Title: "s" + string(rune('a'+i))}
	}

	got := queueText(&Song{Title: "now"}, queue)

	lines := strings.Split(got, "\n")
	if len(lines) != 22 {
		t.Fatalf("queueText has %d lines, want header + 20 + overflow = 22:\n%s", len(lines), got)
	}
	if lines[1] != "1. now" || lines[20] != "20. ss" || lines[21] != "… and 6 more" {
		t.Errorf("lines 1, 20, 21 = %q, %q, %q; want \"1. now\", \"20. sr\", \"… and 6 more\"", lines[1], lines[20], lines[21])
	}
}

func TestQueueTextFitsDiscordLimitWithLongTitles(t *testing.T) {
	long := strings.Repeat("x", 100) // YouTube's longest title
	queue := make([]Song, 30)
	for i := range queue {
		queue[i] = Song{Title: long}
	}

	got := queueText(&Song{Title: long}, queue)

	if n := len([]rune(got)); n > core.MaxMessageLength {
		t.Fatalf("queueText is %d characters, over Discord's %d", n, core.MaxMessageLength)
	}
	lines := strings.Split(got, "\n")
	if len(lines) != 22 || lines[21] != "… and 11 more" {
		t.Errorf("want header + 20 songs + \"… and 11 more\", got %d lines ending %q", len(lines), lines[len(lines)-1])
	}
	if want := "1. " + strings.Repeat("x", maxQueueTitle-1) + "…"; lines[1] != want {
		t.Errorf("line 1 = %q, want the title cut to %d characters", lines[1], maxQueueTitle)
	}
}
