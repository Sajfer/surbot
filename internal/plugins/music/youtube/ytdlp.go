package youtube

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

var videoIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// AudioStream is a YouTube video's audio, downloaded by yt-dlp to its stdout.
// The stream URLs from the youtube library are refused (403) for many
// videos, and yt-dlp's own URLs only work with the headers it sends, so
// yt-dlp does the download itself.
type AudioStream struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	stderr *syncBuffer
	cancel context.CancelFunc
	once   sync.Once
	err    error
}

// OpenAudioStream starts yt-dlp for the video. Close stops it.
func OpenAudioStream(videoID string) (*AudioStream, error) {
	if !videoIDPattern.MatchString(videoID) {
		return nil, fmt.Errorf("invalid video id %q", videoID)
	}
	return openStream("https://www.youtube.com/watch?v=" + videoID)
}

func openStream(target string) (*AudioStream, error) {
	bin, err := ytdlpBinary()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	// #nosec G204 -- bin comes from PATH lookup of a fixed name; target is one argument after "--"
	cmd := exec.CommandContext(ctx, bin, "--quiet", "--no-warnings", "--no-cache-dir", "--no-playlist",
		// The tv_samsung client's stream URLs only work after a delay, so
		// yt-dlp sleeps 5s before downloading. The other default clients
		// return URLs that work at once.
		"--extractor-args", "youtube:player_client=default,-tv_samsung",
		"--format", "bestaudio/best", "--output", "-", "--", target)
	// Bounds Close when a child yt-dlp spawned (deno, for the JS challenge)
	// still holds stderr after yt-dlp itself is killed.
	cmd.WaitDelay = 2 * time.Second
	stderr := &syncBuffer{}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("could not start %s: %w", bin, err)
	}
	return &AudioStream{cmd: cmd, stdout: stdout, stderr: stderr, cancel: cancel}, nil
}

func (a *AudioStream) Read(p []byte) (int, error) {
	return a.stdout.Read(p)
}

// Close stops yt-dlp if it is still running and waits for it to exit.
func (a *AudioStream) Close() error {
	a.once.Do(func() {
		a.cancel()
		a.err = a.cmd.Wait()
	})
	return a.err
}

// Stderr returns what yt-dlp wrote to stderr, for error messages.
func (a *AudioStream) Stderr() string {
	return strings.TrimSpace(a.stderr.String())
}

func ytdlpBinary() (string, error) {
	path, err := exec.LookPath("yt-dlp")
	if err != nil {
		return "", errors.New("yt-dlp is not installed")
	}
	return path, nil
}

// syncBuffer keeps the first 16 KiB written to it; yt-dlp writes to stderr
// from another goroutine than the one reading it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room := 16<<10 - b.buf.Len(); room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
		} else {
			b.buf.Write(p)
		}
	}
	return len(p), nil
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
