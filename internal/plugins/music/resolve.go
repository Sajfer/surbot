package music

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"gitlab.com/sajfer/surbot/internal/plugins/music/spotify"
	"gitlab.com/sajfer/surbot/internal/plugins/music/youtube"
)

// Song is one queued track.
type Song struct {
	Title     string
	Duration  time.Duration
	Thumbnail string
	ID        string // YouTube video ID, streamed through yt-dlp
	// Query is searched on YouTube when the song starts, for songs without
	// an ID, and the result closest to Duration is played. Spotify tracks
	// use it, so a long playlist queues at once.
	Query string
}

// youtubeURLRegex matches the YouTube links yt-dlp is asked about directly:
// videos, shorts, live streams and playlists, on any of YouTube's hosts.
// Anything else, a channel page included, is treated as search text.
var youtubeURLRegex = regexp.MustCompile(`^(?:https?://)?(?:(?:www|m|music)\.)?(?:youtube\.com/(?:watch\?|playlist\?|shorts/|live/|embed/)|youtu\.be/)\S+$`)

func isYouTubeURL(text string) bool { return youtubeURLRegex.MatchString(text) }

type videoAPI interface {
	// Search returns the first match, or an error when there is none.
	Search(ctx context.Context, query string) (*youtube.Video, error)
	// Lookup returns the video behind a link, or every video of a playlist.
	Lookup(ctx context.Context, url string) ([]*youtube.Video, error)
}

type trackAPI interface {
	Search(ctx context.Context, url string) ([]spotify.Song, error)
}

// resolver turns a !play query into songs. ctx bounds every network call.
type resolver interface {
	Resolve(ctx context.Context, query string) ([]*Song, error)
}

type clients struct {
	yt videoAPI
	sp trackAPI
}

// newResolver returns the resolver for the given APIs. sp must be a nil
// interface (not a typed nil) when Spotify is disabled.
func newResolver(yt videoAPI, sp trackAPI) *clients {
	return &clients{yt: yt, sp: sp}
}

// Resolve returns at least one song or an error.
func (c *clients) Resolve(ctx context.Context, query string) ([]*Song, error) {
	switch {
	case isYouTubeURL(query):
		return c.fromLink(ctx, query)
	case spotify.IsURL(query):
		return c.fromSpotify(ctx, query)
	}
	return c.fromSearch(ctx, query)
}

func (c *clients) fromSearch(ctx context.Context, query string) ([]*Song, error) {
	video, err := c.yt.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("search %q: %w", query, err)
	}
	return []*Song{toSong(video)}, nil
}

func (c *clients) fromSpotify(ctx context.Context, query string) ([]*Song, error) {
	if c.sp == nil {
		return nil, errors.New("spotify is not available")
	}
	tracks, err := c.sp.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("spotify lookup: %w", err)
	}
	if len(tracks) == 0 {
		return nil, errors.New("spotify returned no songs")
	}
	songs := make([]*Song, len(tracks))
	for i, t := range tracks {
		name := fmt.Sprintf("%s - %s", t.Artist, t.Name)
		songs[i] = &Song{Title: name, Duration: t.Duration, Thumbnail: t.Thumbnail, Query: name}
	}
	return songs, nil
}

func (c *clients) fromLink(ctx context.Context, url string) ([]*Song, error) {
	videos, err := c.yt.Lookup(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("look up %s: %w", url, err)
	}
	if len(videos) == 0 {
		return nil, fmt.Errorf("no videos found for %s", url)
	}
	songs := make([]*Song, len(videos))
	for i, v := range videos {
		songs[i] = toSong(v)
	}
	return songs, nil
}

func toSong(v *youtube.Video) *Song {
	return &Song{Title: v.Title, Duration: v.Duration, Thumbnail: v.Thumbnail, ID: v.ID}
}
