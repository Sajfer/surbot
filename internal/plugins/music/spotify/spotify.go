// Package spotify resolves Spotify links to track names.
package spotify

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	spotifyapi "github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"gitlab.com/sajfer/surbot/internal/logger"
)

// linkRegex matches track, album and playlist links, with or without a
// localised path (/intl-sv/) and a query string (?si=...).
var linkRegex = regexp.MustCompile(`^https?://open\.spotify\.com/(?:intl-[a-zA-Z-]+/)?(track|album|playlist)/([a-zA-Z0-9]{22})(?:[/?#]\S*)?$`)

// Client talks to the Spotify Web API with client-credentials auth.
type Client struct {
	config *clientcredentials.Config

	mu     sync.Mutex // guards client and token
	client *spotifyapi.Client
	token  *oauth2.Token
}

// Song is a track name with its first artist.
type Song struct {
	Artist string
	Name   string
}

// Playlist is the tracks behind a Spotify link; a track link is a playlist of one.
type Playlist struct {
	Title    string
	Uploader string
	Songs    []Song
}

// New fetches a token and returns an error if Spotify rejects the credentials.
// ctx bounds only that first token request.
func New(ctx context.Context, clientID, clientSecret string) (*Client, error) {
	config := &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     spotifyauth.TokenURL,
	}

	client, token, err := newClient(ctx, config)
	if err != nil {
		return nil, err
	}

	return &Client{client: client, token: token, config: config}, nil
}

// newClient fetches a token under ctx. The API client is bound to
// context.Background() because oauth2 keeps the context it is given for every
// later token refresh.
func newClient(ctx context.Context, config *clientcredentials.Config) (*spotifyapi.Client, *oauth2.Token, error) {
	token, err := config.Token(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to retrieve spotify token: %w", err)
	}

	httpClient := spotifyauth.New().Client(context.Background(), token)
	return spotifyapi.New(httpClient), token, nil
}

// api returns a client with a valid token.
func (c *Client) api(ctx context.Context) (*spotifyapi.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token.Expiry.Before(time.Now()) {
		logger.Log.Info("Token have expired, generating new token")
		client, token, err := newClient(ctx, c.config)
		if err != nil {
			return nil, err
		}
		c.client, c.token = client, token
	}
	return c.client, nil
}

// IsURL reports whether text is a Spotify track, album or playlist link.
func IsURL(text string) bool { return linkRegex.MatchString(text) }

// Search returns the tracks behind a Spotify link. Text that is not a
// Spotify link yields an empty playlist.
func (c *Client) Search(ctx context.Context, url string) (*Playlist, error) {
	k, id := parseLink(url)
	switch k {
	case kindTrack:
		return c.GetTrack(ctx, id)
	case kindAlbum:
		return c.GetAlbum(ctx, id)
	case kindPlaylist:
		return c.GetPlaylist(ctx, id)
	}
	return &Playlist{}, nil
}

type kind int

const (
	kindNone kind = iota
	kindTrack
	kindAlbum
	kindPlaylist
)

// parseLink returns the kind and ID of a Spotify link, or kindNone.
func parseLink(url string) (kind, string) {
	m := linkRegex.FindStringSubmatch(url)
	if m == nil {
		return kindNone, ""
	}
	return map[string]kind{"track": kindTrack, "album": kindAlbum, "playlist": kindPlaylist}[m[1]], m[2]
}

// GetTrack returns the track with the given Spotify ID.
func (c *Client) GetTrack(ctx context.Context, id string) (*Playlist, error) {
	api, err := c.api(ctx)
	if err != nil {
		return &Playlist{}, err
	}
	results, err := api.GetTrack(ctx, spotifyapi.ID(id), spotifyapi.Limit(1))
	if err != nil {
		logger.Log.Warningf("Could not search for spotify track, err=%v", err)
		return &Playlist{}, err
	}
	song, ok := firstArtistSong(results.SimpleTrack)
	if !ok {
		return &Playlist{}, nil
	}
	return &Playlist{Songs: []Song{song}}, nil
}

// GetPlaylist returns the tracks of the playlist with the given Spotify ID.
func (c *Client) GetPlaylist(ctx context.Context, id string) (*Playlist, error) {
	api, err := c.api(ctx)
	if err != nil {
		return &Playlist{}, err
	}
	results, err := api.GetPlaylist(ctx, spotifyapi.ID(id), spotifyapi.Limit(1))
	if err != nil {
		logger.Log.Warningf("Could not search for spotify playlist, err=%v", err)
		return &Playlist{}, err
	}
	playlist := &Playlist{Title: results.Name, Uploader: results.Owner.DisplayName}
	for _, item := range results.Tracks.Tracks {
		if song, ok := firstArtistSong(item.Track.SimpleTrack); ok {
			playlist.Songs = append(playlist.Songs, song)
		}
	}
	return playlist, nil
}

// GetAlbum returns the tracks of the album with the given Spotify ID.
func (c *Client) GetAlbum(ctx context.Context, id string) (*Playlist, error) {
	api, err := c.api(ctx)
	if err != nil {
		return &Playlist{}, err
	}
	results, err := api.GetAlbum(ctx, spotifyapi.ID(id), spotifyapi.Limit(1))
	if err != nil {
		logger.Log.Warningf("Could not search for spotify album, err=%v", err)
		return &Playlist{}, err
	}
	playlist := &Playlist{}
	for _, item := range results.Tracks.Tracks {
		if song, ok := firstArtistSong(item); ok {
			playlist.Songs = append(playlist.Songs, song)
		}
	}
	return playlist, nil
}

func firstArtistSong(track spotifyapi.SimpleTrack) (Song, bool) {
	if len(track.Artists) == 0 {
		return Song{}, false
	}
	return Song{Name: track.Name, Artist: track.Artists[0].Name}, true
}
