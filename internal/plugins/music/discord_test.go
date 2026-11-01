package music

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sajfer/dca"
	"github.com/sajfer/discordgo"
)

func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
}

// installFakeYtdlp puts a yt-dlp script first on PATH and returns the file the
// script writes its pid to.
func installFakeYtdlp(t *testing.T, script string) (pidFile string) {
	t.Helper()
	dir := t.TempDir()
	pidFile = filepath.Join(dir, "pid")
	body := "#!/bin/sh\necho $$ > \"$FAKE_YTDLP_PIDFILE\"\n" + script + "\n"
	if err := os.WriteFile(filepath.Join(dir, "yt-dlp"), []byte(body), 0o755); err != nil { // #nosec G306 -- test script must be executable
		t.Fatalf("write fake yt-dlp: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_YTDLP_PIDFILE", pidFile)
	return pidFile
}

func requireProcessGone(t *testing.T, pidFile string) {
	t.Helper()
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read pid file: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("parse pid %q: %v", raw, err)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Errorf("yt-dlp process %d still exists after Play returned (kill -0: %v)", pid, err)
	}
}

// drainedVoice returns a voice connection whose opus frames are consumed and
// counted until the test ends.
func drainedVoice(t *testing.T) (*discordgo.VoiceConnection, <-chan struct{}) {
	t.Helper()
	vc := &discordgo.VoiceConnection{OpusSend: make(chan []byte, 16)}
	frames := make(chan struct{}, 1<<16)
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	go func() {
		for {
			select {
			case <-vc.OpusSend:
				select {
				case frames <- struct{}{}:
				default:
				}
			case <-stop:
				return
			}
		}
	}()
	return vc, frames
}

type playResult struct {
	err     error
	elapsed time.Duration
}

func startPlay(ctx context.Context, vc *discordgo.VoiceConnection, song *Song) <-chan playResult {
	out := make(chan playResult, 1)
	go func() {
		start := time.Now()
		err := (&discordConn{vc: vc}).Play(ctx, song)
		out <- playResult{err: err, elapsed: time.Since(start)}
	}()
	return out
}

func TestPlayCancelledMidSongReturnsCanceledAndReapsYtdlp(t *testing.T) {
	requireFFmpeg(t)
	pidFile := installFakeYtdlp(t, "exec ffmpeg -loglevel quiet -f lavfi -i sine=frequency=440 -f wav -")
	vc, frames := drainedVoice(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result := startPlay(ctx, vc, &Song{ID: "dQw4w9WgXcQ", Title: "tone"})
	for range 3 {
		select {
		case <-frames:
		case <-time.After(10 * time.Second):
			t.Fatal("no opus frames arrived")
		}
	}
	cancel()

	select {
	case r := <-result:
		if !errors.Is(r.err, context.Canceled) {
			t.Errorf("Play returned %v, want context.Canceled", r.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Play did not return within 3s of cancel")
	}
	requireProcessGone(t, pidFile)
}

func TestPlayCancelledImmediatelyDoesNotHang(t *testing.T) {
	requireFFmpeg(t)
	installFakeYtdlp(t, "exec ffmpeg -loglevel quiet -f lavfi -i sine=frequency=440 -f wav -")
	vc, _ := drainedVoice(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := startPlay(ctx, vc, &Song{ID: "dQw4w9WgXcQ", Title: "tone"})

	select {
	case r := <-result:
		if !errors.Is(r.err, context.Canceled) {
			t.Errorf("Play returned %v, want context.Canceled", r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Play did not return after an immediate cancel")
	}
}

func TestPlayStreamErrorWithStalledYtdlpReturnsTheErrorAndDoesNotHang(t *testing.T) {
	requireFFmpeg(t)
	pidFile := installFakeYtdlp(t, "ffmpeg -loglevel quiet -f lavfi -i sine=frequency=440 -t 6 -f wav -\nexec sleep 300")
	vc := &discordgo.VoiceConnection{OpusSend: make(chan []byte)} // nobody reads: sends time out

	result := startPlay(context.Background(), vc, &Song{ID: "dQw4w9WgXcQ", Title: "tone"})

	select {
	case r := <-result:
		if !errors.Is(r.err, dca.ErrVoiceConnClosed) {
			t.Errorf("Play returned %v, want dca.ErrVoiceConnClosed", r.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Play hung after a stream error while yt-dlp stalled")
	}
	requireProcessGone(t, pidFile)
}

func TestPlaySongToTheEndReturnsNil(t *testing.T) {
	requireFFmpeg(t)
	installFakeYtdlp(t, "exec ffmpeg -loglevel quiet -f lavfi -i sine=frequency=440:duration=1 -f wav -")
	vc, frames := drainedVoice(t)

	result := startPlay(context.Background(), vc, &Song{ID: "dQw4w9WgXcQ", Title: "tone"})

	select {
	case r := <-result:
		if r.err != nil {
			t.Errorf("Play returned %v, want nil at end of file", r.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Play did not finish a 1s song")
	}
	if len(frames) == 0 {
		t.Error("no opus frames were sent")
	}
}

func TestPlaySongWithQuerySearchesInTheSameYtdlpRun(t *testing.T) {
	requireFFmpeg(t)
	argsFile := filepath.Join(t.TempDir(), "args")
	installFakeYtdlp(t, "printf '%s\\n' \"$@\" > "+argsFile+"\nexec ffmpeg -loglevel quiet -f lavfi -i sine=frequency=440:duration=1 -f wav -")
	vc, _ := drainedVoice(t)

	result := startPlay(context.Background(), vc, &Song{Title: "X - Y", Query: "X - Y"})

	select {
	case r := <-result:
		if r.err != nil {
			t.Fatalf("Play returned %v, want nil", r.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Play did not finish a 1s song")
	}
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	args := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if n := len(args); n < 2 || args[n-2] != "--" || args[n-1] != "ytsearch1:X - Y" {
		t.Errorf("yt-dlp args end in %q, want -- ytsearch1:X - Y", args)
	}
}

func TestPlaySongWithoutIDFailsAtOnce(t *testing.T) {
	vc, _ := drainedVoice(t)

	result := startPlay(context.Background(), vc, &Song{Title: "nothing"})

	select {
	case r := <-result:
		if r.err == nil {
			t.Error("Play returned nil for a song with nothing to stream")
		}
	case <-time.After(time.Second):
		t.Fatal("Play did not return")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{name: "under a minute", d: 7 * time.Second, want: "00:07"},
		{name: "minutes", d: 3*time.Minute + 5*time.Second, want: "03:05"},
		{name: "hours", d: time.Hour + 2*time.Minute + 5*time.Second, want: "1:02:05"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatDuration(tt.d); got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestNowPlayingEmbedTruncatesLongTitlesToDiscordsLimit(t *testing.T) {
	long := strings.Repeat("é", 1500)

	embed := nowPlayingEmbed(&Song{Title: long, Thumbnail: "http://x/y.png"})

	if got := len([]rune(embed.Fields[0].Value)); got != 1024 {
		t.Errorf("title field has %d characters, want 1024", got)
	}
	if embed.Thumbnail == nil || embed.Thumbnail.URL != "http://x/y.png" {
		t.Errorf("thumbnail = %+v, want http://x/y.png", embed.Thumbnail)
	}
}
