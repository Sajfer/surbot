package youtube

import (
	"os/exec"
	"strings"

	jsoniter "github.com/json-iterator/go"

	"gitlab.com/sajfer/surbot/src/logger"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

// Download will download either the given url or the youtubeDl.VideoUrl from youtube
func Download(url string, path string) error {
	logger.Log.Debug("youtube.Download")

	cmd := exec.Command("youtube-dl", "-x", "--audio-format", "mp3", "-o", path, url)
	err := cmd.Run()
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			logger.Log.Debug(e.Stderr)
		}
		return err
	}
	return nil
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
