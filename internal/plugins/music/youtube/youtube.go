// Package youtube looks up YouTube videos and streams their audio, all through yt-dlp.
package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// lookupTimeout bounds one yt-dlp metadata run. A flat playlist of a few
// thousand videos takes well under this.
const lookupTimeout = 60 * time.Second

// Video is one playable YouTube video.
type Video struct {
	Title     string
	Duration  time.Duration
	Thumbnail string
	ID        string
}

// Client looks up videos by running yt-dlp. Each lookup is its own process, so
// the client is safe for concurrent use.
type Client struct{}

// New returns a client.
func New() *Client { return &Client{} }

// Search returns the first video matching query.
func (c *Client) Search(ctx context.Context, query string) (*Video, error) {
	videos, err := c.Lookup(ctx, "ytsearch1:"+query)
	if err != nil {
		return nil, err
	}
	if len(videos) == 0 {
		return nil, errors.New("youtube search returned no video")
	}
	return videos[0], nil
}

// matchCandidates is how many search results Match compares.
const matchCandidates = 5

// Match searches for query and returns the first of the top results whose
// length is close to want (within 5s, or 5% for long tracks). A plain audio
// upload usually matches a Spotify track's length, while a music video with
// an intro, a live version or an extended mix does not. When none is close,
// or want is zero, it returns the top result.
func (c *Client) Match(ctx context.Context, query string, want time.Duration) (*Video, error) {
	videos, err := c.Lookup(ctx, fmt.Sprintf("ytsearch%d:%s", matchCandidates, query))
	if err != nil {
		return nil, err
	}
	if len(videos) == 0 {
		return nil, errors.New("youtube search returned no video")
	}
	return closestLength(videos, want), nil
}

func closestLength(videos []*Video, want time.Duration) *Video {
	if want <= 0 {
		return videos[0]
	}
	tolerance := max(5*time.Second, want/20)
	for _, v := range videos {
		diff := v.Duration - want
		if v.Duration > 0 && diff <= tolerance && diff >= -tolerance {
			return v
		}
	}
	return videos[0]
}

// Lookup returns the video behind url, or every video of the playlist when the
// URL has a list= parameter. Unavailable playlist entries are left out.
func (c *Client) Lookup(ctx context.Context, url string) ([]*Video, error) {
	bin, err := ytdlpBinary()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()

	// --flat-playlist lists a playlist's videos without opening each one.
	// "--" keeps a target starting with "-" from being read as an option.
	// #nosec G204 -- bin comes from PATH lookup of a fixed name; url is one argument
	cmd := exec.CommandContext(ctx, bin, "--quiet", "--no-warnings", "--no-cache-dir",
		"--flat-playlist", "--dump-single-json",
		"--extractor-args", "youtube:player_client=default,-tv_samsung",
		"--", url)
	cmd.WaitDelay = 2 * time.Second
	stderr := &syncBuffer{}
	cmd.Stderr = stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("yt-dlp %s: %w: %s", url, err, msg)
		}
		return nil, fmt.Errorf("yt-dlp %s: %w", url, err)
	}
	return parseInfo(out)
}

// info is the part of yt-dlp's JSON surbot uses. A video has no entries; a
// playlist or search result lists its videos as entries.
type info struct {
	Type      string  `json:"_type"`
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Duration  float64 `json:"duration"`
	Thumbnail string  `json:"thumbnail"`
	Entries   []info  `json:"entries"`
}

func parseInfo(data []byte) ([]*Video, error) {
	var root info
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse yt-dlp output: %w", err)
	}
	items := root.Entries
	if root.Type != "playlist" {
		items = []info{root}
	}
	videos := make([]*Video, 0, len(items))
	for _, it := range items {
		if !videoIDPattern.MatchString(it.ID) || unavailable(it.Title) {
			continue
		}
		thumb := it.Thumbnail
		if thumb == "" {
			// Flat playlist entries carry no single thumbnail; this URL
			// exists for every video.
			thumb = "https://i.ytimg.com/vi/" + it.ID + "/hqdefault.jpg"
		}
		videos = append(videos, &Video{
			Title:     it.Title,
			Duration:  time.Duration(it.Duration * float64(time.Second)),
			Thumbnail: thumb,
			ID:        it.ID,
		})
	}
	return videos, nil
}

// unavailable reports the placeholder titles YouTube gives removed videos in playlists.
func unavailable(title string) bool {
	switch title {
	case "[Private video]", "[Deleted video]", "[Unavailable video]":
		return true
	}
	return false
}
