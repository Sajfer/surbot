// Package spotify resolves Spotify links to track names.
package spotify

import (
	"context"
	"errors"
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

const (
	// MaxTracks is the most tracks one album or playlist link returns.
	MaxTracks = 1000
	// lookupTimeout bounds one link lookup, all pages included.
	lookupTimeout = 30 * time.Second
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

// Song is one track: its name, first artist, length and cover art.
type Song struct {
	Artist    string
	Name      string
	Duration  time.Duration
	Thumbnail string
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

// Search returns the tracks behind a Spotify link in order, at most
// MaxTracks. Tracks without an artist, local files and podcast episodes are
// left out. Text that is not a Spotify link yields no songs.
func (c *Client) Search(ctx context.Context, url string) ([]Song, error) {
	k, id := parseLink(url)
	if k == kindNone {
		return nil, nil
	}
	api, err := c.api(ctx)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()

	switch k {
	case kindTrack:
		return track(ctx, api, spotifyapi.ID(id))
	case kindAlbum:
		return album(ctx, api, spotifyapi.ID(id))
	default:
		return playlist(ctx, api, spotifyapi.ID(id))
	}
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

func track(ctx context.Context, api *spotifyapi.Client, id spotifyapi.ID) ([]Song, error) {
	t, err := api.GetTrack(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get track %s: %w", id, err)
	}
	if song, ok := toSong(t.SimpleTrack, t.Album.Images); ok {
		return []Song{song}, nil
	}
	return nil, nil
}

func album(ctx context.Context, api *spotifyapi.Client, id spotifyapi.ID) ([]Song, error) {
	a, err := api.GetAlbum(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get album %s: %w", id, err)
	}
	// Album tracks carry no cover art of their own.
	var songs []Song
	page := &a.Tracks
	for {
		for _, t := range page.Tracks {
			if song, ok := toSong(t, a.Images); ok {
				songs = append(songs, song)
			}
		}
		if len(songs) >= MaxTracks {
			return songs[:MaxTracks], nil
		}
		if done, err := pageDone(api.NextPage(ctx, page), songs); done {
			return songs, err
		}
	}
}

func playlist(ctx context.Context, api *spotifyapi.Client, id spotifyapi.ID) ([]Song, error) {
	// The playlist object holds the first page of tracks; later pages
	// follow the next link Spotify returns.
	p, err := api.GetPlaylist(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get playlist %s: %w", id, err)
	}
	var songs []Song
	page := &p.Tracks
	for {
		for _, item := range page.Tracks {
			// A removed track or a podcast episode has no artists and is
			// skipped by toSong.
			if item.IsLocal {
				continue
			}
			if song, ok := toSong(item.Track.SimpleTrack, item.Track.Album.Images); ok {
				songs = append(songs, song)
			}
		}
		if len(songs) >= MaxTracks {
			return songs[:MaxTracks], nil
		}
		if done, err := pageDone(api.NextPage(ctx, page), songs); done {
			return songs, err
		}
	}
}

// pageDone reports whether paging stops after NextPage returned err. A
// failed page after some songs were found keeps those songs.
func pageDone(err error, songs []Song) (bool, error) {
	switch {
	case err == nil:
		return false, nil
	case errors.Is(err, spotifyapi.ErrNoMorePages):
		return true, nil
	case len(songs) > 0:
		logger.Log.WithError(err).Warningf("spotify: stopped after %d tracks", len(songs))
		return true, nil
	}
	return true, fmt.Errorf("next page: %w", err)
}

func toSong(t spotifyapi.SimpleTrack, images []spotifyapi.Image) (Song, bool) {
	if len(t.Artists) == 0 || t.Name == "" {
		return Song{}, false
	}
	song := Song{
		Name:     t.Name,
		Artist:   t.Artists[0].Name,
		Duration: time.Duration(t.Duration) * time.Millisecond,
	}
	// Spotify lists images largest first.
	if len(images) > 0 {
		song.Thumbnail = images[0].URL
	}
	return song, true
}
