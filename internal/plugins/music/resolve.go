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
	StreamURL string // direct URL used when yt-dlp is unavailable
}

var youtubeURLRegex = regexp.MustCompile(`^(?:https?\:\/\/)?(?:www\.)?(?:(?:youtube\.com\/watch\?v=)|(?:youtu\.be\/))([a-zA-Z0-9\-_]{11})+.*$|^(?:https:\/\/www\.youtube\.com\/playlist\?list=)([a-zA-Z0-9\-_].*).*$`)

func isYouTubeURL(text string) bool { return youtubeURLRegex.MatchString(text) }

type videoAPI interface {
	// SearchVideo returns the watch URL of the first match, or an error when there is none.
	SearchVideo(ctx context.Context, query string) (string, error)
	GetVideoInfo(ctx context.Context, url string) (*youtube.Playlist, error)
}

type trackAPI interface {
	Search(ctx context.Context, url string) (*spotify.Playlist, error)
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
		return c.fromVideoURL(ctx, query, true)
	case spotify.IsURL(query):
		return c.fromSpotify(ctx, query)
	}
	return c.fromSearch(ctx, query)
}

func (c *clients) fromSearch(ctx context.Context, query string) ([]*Song, error) {
	url, err := c.yt.SearchVideo(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("search %q: %w", query, err)
	}
	return c.fromVideoURL(ctx, url, false)
}

func (c *clients) fromSpotify(ctx context.Context, query string) ([]*Song, error) {
	if c.sp == nil {
		return nil, errors.New("spotify is not available")
	}
	tracks, err := c.sp.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("spotify lookup: %w", err)
	}
	if len(tracks.Songs) == 0 {
		return nil, errors.New("spotify returned no songs")
	}
	return c.fromSearch(ctx, fmt.Sprintf("%s - %s", tracks.Songs[0].Artist, tracks.Songs[0].Name))
}

func (c *clients) fromVideoURL(ctx context.Context, url string, all bool) ([]*Song, error) {
	playlist, err := c.yt.GetVideoInfo(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("video info for %s: %w", url, err)
	}
	videos := playlist.Songs
	if len(videos) == 0 {
		return nil, fmt.Errorf("no videos found for %s", url)
	}
	if !all {
		videos = videos[:1]
	}
	songs := make([]*Song, len(videos))
	for i, v := range videos {
		songs[i] = &Song{
			Title:     v.Title,
			Duration:  time.Duration(v.Duration * float64(time.Second)),
			Thumbnail: v.Thumbnail,
			ID:        v.ID,
			StreamURL: v.StreamUrl,
		}
	}
	return songs, nil
}
