// Package music provides music playback functionality.
package music

import (
	"fmt"

	"gitlab.com/sajfer/surbot/internal/logger"
	"gitlab.com/sajfer/surbot/internal/utils"
	spotifyClient "gitlab.com/sajfer/surbot/pkg/spotify"
	"gitlab.com/sajfer/surbot/pkg/youtube"
)

type MusicClients struct {
	Youtube *youtube.Youtube
	Spotify *spotifyClient.Client
}

func NewMusicClients(youtubeAPI, spotifyClientID, spotifyClientSecret string) *MusicClients {
	music := &MusicClients{}
	music.Youtube = youtube.NewYoutube(youtubeAPI)
	music.Spotify = spotifyClient.NewSpotifyClient(spotifyClientID, spotifyClientSecret)
	return music
}

func (m *MusicClients) FetchSong(query string) (*Playlist, error) {
	logger.Log.Debug("music.FetchSong")

	if utils.IsYoutubeUrl(query) {
		playlist, err := m.fetchYoutubeSong(query)
		if err != nil {
			logger.Log.Warningf("Could not fetch youtube songs, err=%v", err)
			return nil, err
		}
		return playlist, nil
	} else if utils.IsSpotifyUrl(query) {
		playlist, err := m.fetchSpotifySong(query)
		if err != nil {
			logger.Log.Warningf("Could not fetch spotify songs, err=%v", err)
			return nil, err
		}
		return playlist, nil
	}
	url := m.Youtube.SearchVideo(query).Path
	video, err := m.Youtube.GetVideoInfo(url)
	if err != nil {
		logger.Log.Warningf("could not fetch video information for %s, err= %s", url, err)
		return nil, err
	}
	song := Song{
		Title:     video.Songs[0].Title,
		Duration:  video.Songs[0].Duration,
		Thumbnail: video.Songs[0].Thumbnail,
		ID:        video.Songs[0].ID,
		StreamURL: video.Songs[0].StreamUrl,
	}

	playlist := Playlist{Title: "", Uploader: "", Songs: []*Song{&song}}
	return &playlist, nil
}

func (m *MusicClients) fetchSpotifySong(query string) (*Playlist, error) {
	logger.Log.Debug("music.fetchSpotifySong")
	songs, err := m.Spotify.Search(query)
	if err != nil {
		logger.Log.Warningf("Could not search for song, err=%v", err)
		return &Playlist{}, err
	}
	playlist := &Playlist{}
	if songs.Title != "" && songs.Uploader != "" {
		playlist.Title = songs.Title
		playlist.Uploader = songs.Uploader
	}
	if len(songs.Songs) == 0 {
		return playlist, fmt.Errorf("did not find any songs")
	}

	url := m.Youtube.SearchVideo(fmt.Sprintf("%s - %s", songs.Songs[0].Artist, songs.Songs[0].Name))
	video, err := m.Youtube.GetVideoInfo(url.Path)
	if err != nil {
		logger.Log.Warningf("could not fetch video information for %s, err= %s", url, err)
		return playlist, fmt.Errorf("failed to fetch song information")
	}

	playlist.Songs = append(playlist.Songs, &Song{
		Title:     video.Songs[0].Title,
		Duration:  video.Songs[0].Duration,
		Thumbnail: video.Songs[0].Thumbnail,
		ID:        video.Songs[0].ID,
		StreamURL: video.Songs[0].StreamUrl,
	})
	return playlist, nil
}

func (m *MusicClients) fetchYoutubeSong(query string) (*Playlist, error) {
	logger.Log.Debug("music.fetchYoutubeSong")

	video, err := m.Youtube.GetVideoInfo(query)
	if err != nil {
		logger.Log.Warningf("could not fetch video information for %s, err= %s", query, err)
		return &Playlist{}, err
	}
	playlist := &Playlist{}
	if len(video.Songs) > 1 {

		for _, song := range video.Songs {
			playlist.Songs = append(playlist.Songs, &Song{
				Title:     song.Title,
				Duration:  song.Duration,
				Thumbnail: song.Thumbnail,
				ID:        song.ID,
				StreamURL: song.StreamUrl,
			})
		}
		return playlist, nil
	}

	playlist.Songs = append(playlist.Songs, &Song{
		Title:     video.Songs[0].Title,
		Duration:  video.Songs[0].Duration,
		Thumbnail: video.Songs[0].Thumbnail,
		ID:        video.Songs[0].ID,
		StreamURL: video.Songs[0].StreamUrl,
	})

	return playlist, nil
}
