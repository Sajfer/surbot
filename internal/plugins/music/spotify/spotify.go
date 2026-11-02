// Package spotify resolves Spotify links to track names.
package spotify

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/sirupsen/logrus"
	spotifyapi "github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

const (
	// MaxTracks is the most tracks one album or playlist link returns.
	MaxTracks = 1000
	// lookupTimeout bounds one link lookup, all pages included.
	lookupTimeout = 30 * time.Second
	// tokenTimeout bounds a token refresh, which runs outside any lookup's context.
	tokenTimeout = 10 * time.Second
)

// linkRegex matches track, album and playlist links, with or without a
// localised path (/intl-sv/) and a query string (?si=...).
var linkRegex = regexp.MustCompile(`^https?://open\.spotify\.com/(?:intl-[a-zA-Z-]+/)?(track|album|playlist)/([a-zA-Z0-9]{22})(?:[/?#]\S*)?$`)

// Client talks to the Spotify Web API with client-credentials auth. It is
// safe for concurrent use.
type Client struct {
	client *spotifyapi.Client
	log    *logrus.Entry
}

// Song is one track: its name, first artist, length and cover art.
type Song struct {
	Artist    string
	Name      string
	Duration  time.Duration
	Thumbnail string
}

// New fetches a token and returns an error if Spotify rejects the
// credentials. ctx bounds only that first token request; later tokens are
// fetched as needed, shortly before the current one expires.
func New(ctx context.Context, clientID, clientSecret string, log *logrus.Entry) (*Client, error) {
	return newClient(ctx, &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     spotifyauth.TokenURL,
	}, log)
}

func newClient(ctx context.Context, config *clientcredentials.Config, log *logrus.Entry, opts ...spotifyapi.ClientOption) (*Client, error) {
	token, err := config.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve spotify token: %w", err)
	}

	// The transport refreshes the token on the first request after it
	// expires, without that request's context, so the refresh gets its own
	// timeout through the context the token source keeps.
	refreshCtx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Timeout: tokenTimeout})
	tokens := oauth2.ReuseTokenSource(token, config.TokenSource(refreshCtx))
	return &Client{client: spotifyapi.New(oauth2.NewClient(refreshCtx, tokens), opts...), log: log}, nil
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
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()

	switch k {
	case kindTrack:
		return c.track(ctx, spotifyapi.ID(id))
	case kindAlbum:
		return c.album(ctx, spotifyapi.ID(id))
	default:
		return c.playlist(ctx, spotifyapi.ID(id))
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

func (c *Client) track(ctx context.Context, id spotifyapi.ID) ([]Song, error) {
	t, err := c.client.GetTrack(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get track %s: %w", id, err)
	}
	if song, ok := toSong(t.SimpleTrack, t.Album.Images); ok {
		return []Song{song}, nil
	}
	return nil, nil
}

func (c *Client) album(ctx context.Context, id spotifyapi.ID) ([]Song, error) {
	a, err := c.client.GetAlbum(ctx, id)
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
		if done, err := c.pageDone(c.client.NextPage(ctx, page), songs); done {
			return songs, err
		}
	}
}

func (c *Client) playlist(ctx context.Context, id spotifyapi.ID) ([]Song, error) {
	// The playlist object holds the first page of tracks; later pages
	// follow the next link Spotify returns.
	p, err := c.client.GetPlaylist(ctx, id)
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
		if done, err := c.pageDone(c.client.NextPage(ctx, page), songs); done {
			return songs, err
		}
	}
}

// pageDone reports whether paging stops after NextPage returned err. A
// failed page after some songs were found keeps those songs.
func (c *Client) pageDone(err error, songs []Song) (bool, error) {
	switch {
	case err == nil:
		return false, nil
	case errors.Is(err, spotifyapi.ErrNoMorePages):
		return true, nil
	case len(songs) > 0:
		c.log.WithError(err).Warningf("spotify: stopped after %d tracks", len(songs))
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
