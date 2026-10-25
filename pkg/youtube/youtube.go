// Package youtube provides youtube playback functionality.
package youtube

import (
	"context"
	"fmt"
	"strings"

	ytdl "github.com/kkdai/youtube/v2"
	"gitlab.com/sajfer/surbot/internal/logger"
	"gitlab.com/sajfer/surbot/internal/utils"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

type Youtube struct {
	ytdl   ytdl.Client
	devKey string
}

type SearchResult struct {
	VideoID    string
	VideoTitle string
	Duration   string
	Path       string
}

type Video struct {
	Title     string
	Duration  float64
	Thumbnail string
	ID        string
	StreamUrl string
}

type Playlist struct {
	Title    string
	Uploader string
	Songs    []*Video
}

func NewYoutube(key string) *Youtube {
	return &Youtube{devKey: key, ytdl: ytdl.Client{}}
}

func (yt *Youtube) SearchVideo(query string) *SearchResult {
	logger.Log.Debug("youtube.SearchVideo")
	ctx := context.Background()

	service, err := youtube.NewService(ctx, option.WithAPIKey(yt.devKey))
	if err != nil {
		logger.Log.Errorf("failed to create youtube service: %v", err)
		return nil
	}

	search := service.Search.List([]string{"id, snippet"}).Q(query).MaxResults(1)

	response, err := search.Do()
	if err != nil {
		logger.Log.Errorf("search error: %v", err)
		return nil
	}

	for _, item := range response.Items {
		logger.Log.Debugf("found video: %s", item.Snippet.Title)
		switch item.Id.Kind {
		case "youtube#video":
			newTitle := utils.FormatVideoTitle(item.Snippet.Title)
			return &SearchResult{
				VideoID:    item.Id.VideoId,
				VideoTitle: newTitle,
				Duration:   yt.GetDurationByID(item.Id.VideoId),
				Path:       fmt.Sprintf("youtube.com/watch?v=%s", item.Id.VideoId),
			}
		default:
			return &SearchResult{}
		}
	}
	return nil
}

func (yt *Youtube) GetDurationByID(id string) string {
	logger.Log.Debug("youtube.GetDurationByID")
	ctx := context.Background()

	service, err := youtube.NewService(ctx, option.WithAPIKey(yt.devKey))
	if err != nil {
		logger.Log.Errorf("failed to create youtube service: %v", err)
		return ""
	}

	search := service.Videos.List([]string{"id,contentDetails"}).Id(id)
	resp, err := search.Do()
	if err != nil {
		logger.Log.Errorf("duration lookup error: %v", err)
	}

	for _, item := range resp.Items {
		return utils.ParseISO8601(item.ContentDetails.Duration)
	}
	return ""
}

// GetInfo gets the info of a particular video or playlist
func (yt *Youtube) GetVideoInfo(url string) (*Playlist, error) {
	logger.Log.Debug("youtube.GetVideoInfo")

	playlist := &Playlist{}

	ytVideo, err := yt.ytdl.GetVideo(url)
	if err != nil {
		return playlist, err
	}
	getStreamURL := func(video *ytdl.Video) (string, error) {
		formats := video.Formats.WithAudioChannels()
		if len(formats) == 0 {
			return "", fmt.Errorf("no audio formats for video %s", video.ID)
		}
		// Prefer formats with both audio and video (index 1+), fall back to audio-only.
		if len(formats) > 1 {
			url, err := yt.ytdl.GetStreamURL(video, &formats[1])
			if err != nil {
				logger.Log.Warningf("failed to get stream URL (audio+video), trying audio-only: %v", err)
			}
			if url != "" {
				return url, nil
			}
		}
		// Fallback: first available format.
		url, err := yt.ytdl.GetStreamURL(video, &formats[0])
		if err != nil {
			return "", fmt.Errorf("failed to get stream URL for video %s: %w", video.ID, err)
		}
		return url, nil
	}

	if strings.Contains(url, "list=") {
		youtubePlaylist, err := yt.ytdl.GetPlaylist(url)
		if err != nil {
			return playlist, err
		}
		playlist.Title = youtubePlaylist.Title
		playlist.Uploader = youtubePlaylist.Author
		for _, song := range youtubePlaylist.Videos {
			tmp, err := yt.ytdl.VideoFromPlaylistEntry(song)
			if err != nil {
				continue
			}
			streamUrl, err := getStreamURL(tmp)
			if err != nil {
				logger.Log.Warningf("skipping playlist song: %v", err)
				continue
			}

			video := &Video{
				Title:     tmp.Title,
				Duration:  tmp.Duration.Seconds(),
				Thumbnail: thumbnailURL(tmp),
				ID:        tmp.ID,
				StreamUrl: streamUrl,
			}
			playlist.Songs = append(playlist.Songs, video)
		}
	} else {
		streamUrl, err := getStreamURL(ytVideo)
		if err != nil {
			return playlist, err
		}
		logger.Log.Debugf("got stream URL for %s", ytVideo.Title)

		video := &Video{
			Title:     ytVideo.Title,
			Duration:  ytVideo.Duration.Seconds(),
			Thumbnail: thumbnailURL(ytVideo),
			ID:        ytVideo.ID,
			StreamUrl: streamUrl,
		}
		playlist.Songs = append(playlist.Songs, video)
	}

	return playlist, nil
}

// thumbnailURL returns the video's first thumbnail, or "" if it has none.
func thumbnailURL(video *ytdl.Video) string {
	if len(video.Thumbnails) == 0 {
		return ""
	}
	return video.Thumbnails[0].URL
}
