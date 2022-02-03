package spotifyClient

import (
	"context"
	"log"

	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"gitlab.com/sajfer/surbot/src/logger"
	"gitlab.com/sajfer/surbot/src/utils"
	"golang.org/x/oauth2/clientcredentials"
)

type Client struct {
	client *spotify.Client
}

type Song struct {
	Artist string
	Name   string
}

type Playlist struct {
	Title    string
	Uploader string
	Songs    []Song
}

func NewSpotifyClient(clientID, clientSecret string) *Client {

	ctx := context.Background()
	config := &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     spotifyauth.TokenURL,
	}
	token, err := config.Token(ctx)
	if err != nil {
		log.Fatalf("couldn't get token: %v", err)
	}

	httpClient := spotifyauth.New().Client(ctx, token)
	client := spotify.New(httpClient)
	// search for playlists and albums containing "holiday"

	return &Client{client: client}
}

func (c *Client) Search(url string) (*Playlist, error) {
	logger.Log.Debug("spotify.Search")

	id := utils.GetSpotifyID(url)

	if utils.IsSpotifyTrackUrl(url) {
		return c.GetTrack(id)
	} else if utils.IsSpotifyAlbumUrl(url) {
		return c.GetAlbum(id)
	} else if utils.IsSpotifyPlaylistUrl(url) {
		return c.GetPlaylist(id)
	}
	return &Playlist{}, nil
}

func (c *Client) GetTrack(query string) (*Playlist, error) {
	logger.Log.Debug("spotify.GetTrack")
	ctx := context.Background()
	results, err := c.client.GetTrack(ctx, spotify.ID(query), spotify.Limit(1))
	if err != nil {
		logger.Log.Warningf("Could not search for spotify track, err=%v", err)
		return &Playlist{}, err
	}
	logger.Log.Debugf("%s", results.SimpleTrack.Name)
	return &Playlist{Songs: []Song{Song{Name: results.SimpleTrack.Name, Artist: results.SimpleTrack.Artists[0].Name}}}, nil
}

func (c *Client) GetPlaylist(query string) (*Playlist, error) {
	logger.Log.Debug("spotify.GetPlaylist")
	ctx := context.Background()
	results, err := c.client.GetPlaylist(ctx, spotify.ID(query), spotify.Limit(1))
	if err != nil {
		logger.Log.Warningf("Could not search for spotify track, err=%v", err)
		return &Playlist{}, err
	}
	playlist := &Playlist{Title: results.Name, Uploader: results.SimplePlaylist.Owner.DisplayName}
	for _, item := range results.Tracks.Tracks {
		playlist.Songs = append(playlist.Songs, Song{Name: item.Track.Name, Artist: item.Track.Artists[0].Name})
	}
	return playlist, nil
}

func (c *Client) GetAlbum(query string) (*Playlist, error) {
	logger.Log.Debug("spotify.GetAlbum")
	ctx := context.Background()
	results, err := c.client.GetAlbum(ctx, spotify.ID(query), spotify.Limit(1))
	if err != nil {
		logger.Log.Warningf("Could not search for spotify track, err=%v", err)
		return &Playlist{}, err
	}
	playlist := &Playlist{}
	for _, item := range results.Tracks.Tracks {
		playlist.Songs = append(playlist.Songs, Song{Name: item.Name, Artist: item.Artists[0].Name})
	}
	return playlist, nil
}
