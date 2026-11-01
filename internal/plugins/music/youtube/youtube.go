// Package youtube looks up YouTube videos and streams their audio through yt-dlp.
package youtube

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	ytdl "github.com/kkdai/youtube/v2"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"

	"gitlab.com/sajfer/surbot/internal/logger"
)

// Youtube resolves search terms and links to videos.
type Youtube struct {
	devKey     string
	httpClient *http.Client
}

// Video is one playable YouTube video.
type Video struct {
	Title     string
	Duration  float64
	Thumbnail string
	ID        string
	StreamUrl string
}

// Playlist is the result of a lookup; a single video is a playlist of one.
type Playlist struct {
	Title    string
	Uploader string
	Songs    []*Video
}

// New returns a client that uses devKey for searches and httpClient (may be
// nil for the default) for video metadata requests.
func New(devKey string, httpClient *http.Client) *Youtube {
	return &Youtube{devKey: devKey, httpClient: httpClient}
}

// SearchVideo returns the watch URL of the first video matching query.
func (yt *Youtube) SearchVideo(ctx context.Context, query string) (string, error) {
	service, err := youtube.NewService(ctx, option.WithAPIKey(yt.devKey))
	if err != nil {
		return "", fmt.Errorf("create youtube service: %w", err)
	}

	response, err := service.Search.List([]string{"id"}).Q(query).Type("video").MaxResults(1).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("youtube search: %w", err)
	}

	for _, item := range response.Items {
		if item.Id != nil && item.Id.Kind == "youtube#video" {
			return "youtube.com/watch?v=" + item.Id.VideoId, nil
		}
	}
	return "", errors.New("youtube search returned no video")
}

// GetVideoInfo looks up a video or, when the URL has a list= parameter, every
// video of the playlist.
//
// Each lookup uses its own metadata client: the library's client is not safe
// for concurrent use, and sharing one behind a lock would make one guild's
// large playlist block every other guild's lookups. The cost is that
// per-client state (visitor ID, player cache) is fetched again per lookup.
func (yt *Youtube) GetVideoInfo(ctx context.Context, url string) (*Playlist, error) {
	client := &ytdl.Client{HTTPClient: yt.httpClient}
	playlist := &Playlist{}

	ytVideo, err := client.GetVideoContext(ctx, url)
	if err != nil {
		return playlist, err
	}

	if !strings.Contains(url, "list=") {
		video, err := toVideo(ctx, client, ytVideo)
		if err != nil {
			return playlist, err
		}
		playlist.Songs = append(playlist.Songs, video)
		return playlist, nil
	}

	youtubePlaylist, err := client.GetPlaylistContext(ctx, url)
	if err != nil {
		return playlist, err
	}
	playlist.Title = youtubePlaylist.Title
	playlist.Uploader = youtubePlaylist.Author
	for _, entry := range youtubePlaylist.Videos {
		if ctx.Err() != nil {
			return playlist, ctx.Err()
		}
		entryVideo, err := client.VideoFromPlaylistEntryContext(ctx, entry)
		if err != nil {
			continue
		}
		video, err := toVideo(ctx, client, entryVideo)
		if err != nil {
			logger.Log.Warningf("skipping playlist song: %v", err)
			continue
		}
		playlist.Songs = append(playlist.Songs, video)
	}
	return playlist, nil
}

func toVideo(ctx context.Context, client *ytdl.Client, video *ytdl.Video) (*Video, error) {
	streamURL, err := streamURL(ctx, client, video)
	if err != nil {
		return nil, err
	}
	return &Video{
		Title:     video.Title,
		Duration:  video.Duration.Seconds(),
		Thumbnail: thumbnailURL(video),
		ID:        video.ID,
		StreamUrl: streamURL,
	}, nil
}

func streamURL(ctx context.Context, client *ytdl.Client, video *ytdl.Video) (string, error) {
	formats := video.Formats.WithAudioChannels()
	if len(formats) == 0 {
		return "", fmt.Errorf("no audio formats for video %s", video.ID)
	}
	// Prefer formats with both audio and video (index 1+), fall back to audio-only.
	if len(formats) > 1 {
		u, err := client.GetStreamURLContext(ctx, video, &formats[1])
		if err != nil {
			logger.Log.Warningf("failed to get stream URL (audio+video), trying audio-only: %v", err)
		}
		if u != "" {
			return u, nil
		}
	}
	u, err := client.GetStreamURLContext(ctx, video, &formats[0])
	if err != nil {
		return "", fmt.Errorf("failed to get stream URL for video %s: %w", video.ID, err)
	}
	return u, nil
}

// thumbnailURL returns the video's first thumbnail, or "" if it has none.
func thumbnailURL(video *ytdl.Video) string {
	if len(video.Thumbnails) == 0 {
		return ""
	}
	return video.Thumbnails[0].URL
}
