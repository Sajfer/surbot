package youtube

import (
	"context"
	"fmt"
	"strings"

	ytdl "github.com/kkdai/youtube/v2"
	"gitlab.com/sajfer/surbot/src/logger"
	"gitlab.com/sajfer/surbot/src/utils"
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
	logger.Log.Info("youtube.SearchVideo")
	ctx := context.Background()

	service, err := youtube.NewService(ctx, option.WithAPIKey(yt.devKey))
	if err != nil {
		print(err)
		return nil
	}

	search := service.Search.List([]string{"id, snippet"}).Q(query).MaxResults(1)

	response, err := search.Do()
	if err != nil {
		logger.Log.Error(err)
		return nil
	}

	for _, item := range response.Items {
		logger.Log.Info(item.Snippet.Title)
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
	ctx := context.Background()

	service, err := youtube.NewService(ctx, option.WithAPIKey(yt.devKey))
	if err != nil {
		print(err)
		return ""
	}

	search := service.Videos.List([]string{"id,contentDetails"}).Id(id)
	resp, err := search.Do()
	if err != nil {
		logger.Log.Error(err)
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
			formats := tmp.Formats.WithAudioChannels()
			streamUrl, _ := yt.ytdl.GetStreamURL(tmp, &formats[1])

			video := &Video{
				Title:     tmp.Title,
				Duration:  tmp.Duration.Seconds(),
				Thumbnail: tmp.Thumbnails[0].URL,
				ID:        tmp.ID,
				StreamUrl: streamUrl,
			}
			playlist.Songs = append(playlist.Songs, video)
		}
	} else {
		formats := ytVideo.Formats.WithAudioChannels()
		streamUrl, _ := yt.ytdl.GetStreamURL(ytVideo, &formats[1])

		video := &Video{
			Title:     ytVideo.Title,
			Duration:  ytVideo.Duration.Seconds(),
			Thumbnail: ytVideo.Thumbnails[0].URL,
			ID:        ytVideo.ID,
			StreamUrl: streamUrl,
		}
		playlist.Songs = append(playlist.Songs, video)
	}

	return playlist, nil
}
