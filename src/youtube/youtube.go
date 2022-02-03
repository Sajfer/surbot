package youtube

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	jsoniter "github.com/json-iterator/go"

	"gitlab.com/sajfer/surbot/src/logger"
	"gitlab.com/sajfer/surbot/src/utils"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type Youtube struct {
	devKey string
}

type SearchResult struct {
	VideoID    string
	VideoTitle string
	Duration   string
	Path       string
}

func NewYoutube(key string) *Youtube {
	return &Youtube{devKey: key}
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
func GetVideoInfo(url string) (Info, error) {
	logger.Log.Debug("youtube.GetVideoInfo")
	info := Info{}
	cmd := exec.Command("youtube-dl", "-J", "--flat-playlist", url)

	stdOut, err := cmd.StdoutPipe()

	if err != nil {
		return Info{}, err
	}

	if err := cmd.Start(); err != nil {
		return Info{}, err
	}

	if strings.Contains(url, "list=") {
		if err := json.NewDecoder(stdOut).Decode(&info.Playlist); err != nil {
			return Info{}, err
		}
		temp := strings.Split(url, "&")
		temp = strings.Split(temp[0], "=")
		videoID := temp[1]
		for i, id := range info.Playlist.Entries {
			if videoID == id.ID {
				info.Video = &info.Playlist.Entries[i]
				break
			}
		}
	} else {
		if err := json.NewDecoder(stdOut).Decode(&info.Video); err != nil {
			return Info{}, err
		}
	}

	return info, cmd.Wait()
}
