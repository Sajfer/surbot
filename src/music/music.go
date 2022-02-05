package music

import (
	"fmt"

	"gitlab.com/sajfer/surbot/src/logger"
	spotifyClient "gitlab.com/sajfer/surbot/src/spotify"
	"gitlab.com/sajfer/surbot/src/utils"
	"gitlab.com/sajfer/surbot/src/youtube"
)

type Song struct {
	Title     string
	Artist    string
	Duration  float64
	Thumbnail string
	ID        string
	StreamURL string
}

type Playlist struct {
	Title    string
	Uploader string
	Songs    []Song
}

type Clients struct {
	Youtube *youtube.Youtube
	Spotify *spotifyClient.Client
}

func (c *Clients) FetchSong(query string) (*Playlist, error) {
	logger.Log.Debug("music.FetchSong")

	if utils.IsYoutubeUrl(query) {
		return c.fetchYoutubeSong(query)
	} else if utils.IsSpotifyUrl(query) {
		return c.fetchSpotifySong(query)
	}
	url := c.Youtube.SearchVideo(query).Path
	info, err := youtube.GetVideoInfo(url)
	if err != nil {
		logger.Log.Warningf("could not fetch video information for %s, err= %s", url, err)
		return &Playlist{}, err
	}
	song := Song{
		Title:     info.Video.Title,
		Duration:  info.Video.Duration,
		Thumbnail: info.Video.Thumbnail,
		ID:        info.Video.ID,
		StreamURL: info.Video.Formats[0].URL,
	}

	return &Playlist{Songs: []Song{song}}, nil
}

func (c *Clients) fetchSpotifySong(query string) (*Playlist, error) {
	logger.Log.Debug("music.fetchSpotifySong")
	songs, err := c.Spotify.Search(query)
	if err != nil {
		logger.Log.Warningf("Could not search for song, err=%v", err)
		return &Playlist{}, err
	}
	playlist := &Playlist{}
	if songs.Title != "" && songs.Uploader != "" {
		playlist.Title = songs.Title
		playlist.Uploader = songs.Uploader
	}
	for _, song := range songs.Songs {
		url := c.Youtube.SearchVideo(fmt.Sprintf("%s - %s", song.Artist, song.Name))
		info, err := youtube.GetVideoInfo(url.Path)
		if err != nil {
			logger.Log.Warningf("could not fetch video information for %s, err= %s", url, err)
			continue
		}

		playlist.Songs = append(playlist.Songs, Song{
			Title:     info.Video.Title,
			Duration:  info.Video.Duration,
			Thumbnail: info.Video.Thumbnail,
			ID:        info.Video.ID,
			StreamURL: info.Video.Formats[0].URL,
		})
	}

	return playlist, nil
}

func (c *Clients) fetchYoutubeSong(query string) (*Playlist, error) {
	logger.Log.Debug("music.fetchSpotifySong")
	songs, err := c.Spotify.Search(query)
	if err != nil {
		logger.Log.Warningf("Could not search for song, err=%v", err)
		return &Playlist{}, err
	}
	playlist := &Playlist{}
	if songs.Title != "" && songs.Uploader != "" {
		playlist.Title = songs.Title
		playlist.Uploader = songs.Uploader
	}
	for _, song := range songs.Songs {
		url := c.Youtube.SearchVideo(fmt.Sprintf("%s - %s", song.Artist, song.Name))
		info, err := youtube.GetVideoInfo(url.Path)
		if err != nil {
			logger.Log.Warningf("could not fetch video information for %s, err= %s", url, err)
			continue
		}

		playlist.Songs = append(playlist.Songs, Song{
			Title:     info.Video.Title,
			Duration:  info.Video.Duration,
			Thumbnail: info.Video.Thumbnail,
			ID:        info.Video.ID,
			StreamURL: info.Video.Formats[0].URL,
		})
	}

	return playlist, nil
}
