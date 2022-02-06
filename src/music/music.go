package music

import (
	"fmt"
	"math/rand"
	"time"

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
	Songs    []*Song
}

type Music struct {
	Youtube     *youtube.Youtube
	Spotify     *spotifyClient.Client
	CurrentSong *Song
	Queue       []*Song
}

func NewMusic(youtubeAPI, spotifyClientID, spotifyClientSecret string) *Music {
	music := &Music{}
	music.Youtube = youtube.NewYoutube(youtubeAPI)
	music.Spotify = spotifyClient.NewSpotifyClient(spotifyClientID, spotifyClientSecret)
	return music
}

func (m *Music) FetchSong(query string) error {
	logger.Log.Debug("music.FetchSong")

	if utils.IsYoutubeUrl(query) {
		playlist, err := m.fetchYoutubeSong(query)
		if err != nil {
			logger.Log.Warningf("Could not fetch youtube songs, err=%v", err)
			return err
		}
		m.Queue = append(m.Queue, playlist.Songs...)
		return nil
	} else if utils.IsSpotifyUrl(query) {
		playlist, err := m.fetchSpotifySong(query)
		if err != nil {
			logger.Log.Warningf("Could not fetch spotify songs, err=%v", err)
			return err
		}
		m.Queue = append(m.Queue, playlist.Songs...)
		return nil
	}
	url := m.Youtube.SearchVideo(query).Path
	video, err := m.Youtube.GetVideoInfo(url)
	if err != nil {
		logger.Log.Warningf("could not fetch video information for %s, err= %s", url, err)
		return err
	}
	song := Song{
		Title:     video.Songs[0].Title,
		Duration:  video.Songs[0].Duration,
		Thumbnail: video.Songs[0].Thumbnail,
		ID:        video.Songs[0].ID,
		StreamURL: video.Songs[0].StreamUrl,
	}

	m.Queue = append(m.Queue, &song)

	return nil
}

func (m *Music) fetchSpotifySong(query string) (*Playlist, error) {
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

	if len(songs.Songs) > 1 {
		go func() {
			for _, song := range songs.Songs[1:] {
				url := m.Youtube.SearchVideo(fmt.Sprintf("%s - %s", song.Artist, song.Name))
				video, err := m.Youtube.GetVideoInfo(url.Path)
				if err != nil {
					logger.Log.Warningf("could not fetch video information for %s, err= %s", url, err)
					continue
				}

				m.Queue = append(m.Queue, &Song{
					Title:     video.Songs[0].Title,
					Duration:  video.Songs[0].Duration,
					Thumbnail: video.Songs[0].Thumbnail,
					ID:        video.Songs[0].ID,
					StreamURL: video.Songs[0].StreamUrl,
				})
			}
		}()
	}

	return playlist, nil
}

func (m *Music) fetchYoutubeSong(query string) (*Playlist, error) {
	logger.Log.Debug("music.fetchYoutubeSong")

	video, err := m.Youtube.GetVideoInfo(query)
	if err != nil {
		logger.Log.Warningf("could not fetch video information for %s, err= %s", query, err)
		return &Playlist{}, err
	}
	playlist := &Playlist{}
	if len(video.Songs) > 1 {
		res, err := m.addYoutubePlaylist(video)
		if err != nil {
			logger.Log.Warningf("Could not fetch youtube playlist information, err=%v", err)
			return playlist, err
		}
		return res, nil
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

func (m *Music) addYoutubePlaylist(playlist *youtube.Playlist) (*Playlist, error) {
	logger.Log.Debug("music.addYoutubePlaylist")

	pl := &Playlist{}
	pl.Title = playlist.Title
	pl.Uploader = playlist.Uploader

	if len(playlist.Songs) == 0 {
		return pl, fmt.Errorf("playlist contain no songs")
	}

	for _, song := range playlist.Songs {
		m.Queue = append(m.Queue, &Song{
			Title:     song.Title,
			Duration:  song.Duration,
			Thumbnail: song.Thumbnail,
			ID:        song.ID,
			StreamURL: song.StreamUrl,
		})
	}
	return pl, nil
}

func (m *Music) Shuffle() {
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(m.Queue), func(i, j int) { m.Queue[i], m.Queue[j] = m.Queue[j], m.Queue[i] })
}
