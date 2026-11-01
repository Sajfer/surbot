package spotify

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	spotifyapi "github.com/zmb3/spotify/v2"
	"golang.org/x/oauth2"
)

func TestLinkParsing(t *testing.T) {
	const id = "4cOdK2wGLETKBW3PvgPWqT"
	tests := []struct {
		name     string
		url      string
		wantKind kind
		wantID   string
	}{
		{name: "track with query", url: "https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT?si=abc", wantKind: kindTrack, wantID: id},
		{name: "album with query", url: "https://open.spotify.com/album/4cOdK2wGLETKBW3PvgPWqT?si=abc", wantKind: kindAlbum, wantID: id},
		{name: "playlist with query", url: "https://open.spotify.com/playlist/4cOdK2wGLETKBW3PvgPWqT?si=abc", wantKind: kindPlaylist, wantID: id},
		{name: "track without query", url: "https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT", wantKind: kindTrack, wantID: id},
		{name: "localised track", url: "https://open.spotify.com/intl-sv/track/4cOdK2wGLETKBW3PvgPWqT?si=abc", wantKind: kindTrack, wantID: id},
		{name: "localised album without query", url: "https://open.spotify.com/intl-pt-BR/album/4cOdK2wGLETKBW3PvgPWqT", wantKind: kindAlbum, wantID: id},
		{name: "trailing slash", url: "https://open.spotify.com/playlist/4cOdK2wGLETKBW3PvgPWqT/", wantKind: kindPlaylist, wantID: id},
		{name: "http", url: "http://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT", wantKind: kindTrack, wantID: id},
		{name: "short ID", url: "https://open.spotify.com/track/4cOdK2wGLE", wantKind: kindNone},
		{name: "artist page", url: "https://open.spotify.com/artist/4cOdK2wGLETKBW3PvgPWqT", wantKind: kindNone},
		{name: "link followed by text", url: "https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT please", wantKind: kindNone},
		{name: "plain text", url: "never gonna give you up", wantKind: kindNone},
		{name: "youtube link", url: "https://www.youtube.com/watch?v=dQw4w9WgXcQ", wantKind: kindNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, want := IsURL(tt.url), tt.wantKind != kindNone; got != want {
				t.Errorf("IsURL(%q) = %v, want %v", tt.url, got, want)
			}
			gotKind, gotID := parseLink(tt.url)
			if gotKind != tt.wantKind || gotID != tt.wantID {
				t.Errorf("parseLink(%q) = %v, %q, want %v, %q", tt.url, gotKind, gotID, tt.wantKind, tt.wantID)
			}
		})
	}
}

func TestSearchNonLinkReturnsNoSongsWithoutNetwork(t *testing.T) {
	c := &Client{}

	got, err := c.Search(context.Background(), "never gonna give you up")

	if err != nil {
		t.Fatalf("Search returned error %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("Search returned %d songs, want 0", len(got))
	}
}

// fakeAPI serves canned JSON by request path; "{base}" in a body becomes the
// server's base URL, for next links.
func fakeAPI(t *testing.T, routes map[string]string) *Client {
	t.Helper()
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path
		if r.URL.RawQuery != "" {
			key += "?" + r.URL.RawQuery
		}
		body, ok := routes[key]
		if !ok {
			body, ok = routes[r.URL.Path]
		}
		if !ok {
			http.Error(w, `{"error":{"status":404,"message":"not found"}}`, http.StatusNotFound)
			return
		}
		if body == "500" {
			http.Error(w, `{"error":{"status":500,"message":"boom"}}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, strings.ReplaceAll(body, "{base}", base))
	}))
	t.Cleanup(srv.Close)
	base = srv.URL + "/"
	return &Client{
		client: spotifyapi.New(srv.Client(), spotifyapi.WithBaseURL(base)),
		token:  &oauth2.Token{Expiry: time.Now().Add(time.Hour)},
	}
}

func trackJSON(name, artist string, ms int) string {
	return fmt.Sprintf(`{"type":"track","name":%q,"duration_ms":%d,"artists":[{"name":%q}],"album":{"images":[{"url":"https://img/%s.jpg"}]}}`, name, ms, artist, name)
}

func names(songs []Song) string {
	out := make([]string, len(songs))
	for i, s := range songs {
		out[i] = s.Artist + " - " + s.Name
	}
	return strings.Join(out, ", ")
}

const playlistURL = "https://open.spotify.com/playlist/4cOdK2wGLETKBW3PvgPWqT?si=a"

func TestSearchPlaylistFollowsPagesAndSkipsUnplayableItems(t *testing.T) {
	c := fakeAPI(t, map[string]string{
		"/playlists/4cOdK2wGLETKBW3PvgPWqT": `{"name":"mix","tracks":{"next":"{base}page2","items":[
			{"track":` + trackJSON("one", "A", 61000) + `},
			{"is_local":true,"track":` + trackJSON("local", "L", 1000) + `},
			{"track":null},
			{"track":{"type":"episode","name":"podcast"}}
		]}}`,
		"/page2": `{"next":"","items":[{"track":` + trackJSON("two", "B", 1000) + `}]}`,
	})

	songs, err := c.Search(context.Background(), playlistURL)

	if err != nil {
		t.Fatalf("Search returned error %v", err)
	}
	if got, want := names(songs), "A - one, B - two"; got != want {
		t.Errorf("Search songs = %s, want %s", got, want)
	}
	if songs[0].Duration != 61*time.Second || songs[0].Thumbnail != "https://img/one.jpg" {
		t.Errorf("first song = %+v, want 1m1s with its album image", songs[0])
	}
}

func TestSearchPlaylistKeepsTracksFoundBeforeAFailedPage(t *testing.T) {
	c := fakeAPI(t, map[string]string{
		"/playlists/4cOdK2wGLETKBW3PvgPWqT": `{"tracks":{"next":"{base}page2","items":[{"track":` + trackJSON("one", "A", 1000) + `}]}}`,
		"/page2":                            "500",
	})

	songs, err := c.Search(context.Background(), playlistURL)

	if err != nil || names(songs) != "A - one" {
		t.Errorf("Search = %s, %v; want A - one and no error", names(songs), err)
	}
}

func TestSearchPlaylistStopsAtMaxTracks(t *testing.T) {
	items := make([]string, 0, 100)
	for i := range 100 {
		items = append(items, `{"track":`+trackJSON(fmt.Sprint(i), "A", 1000)+`}`)
	}
	page := `{"next":"{base}more","items":[` + strings.Join(items, ",") + `]}`
	c := fakeAPI(t, map[string]string{
		"/playlists/4cOdK2wGLETKBW3PvgPWqT": `{"tracks":` + page + `}`,
		"/more":                             page, // endless: every page links to another
	})

	songs, err := c.Search(context.Background(), playlistURL)

	if err != nil {
		t.Fatalf("Search returned error %v", err)
	}
	if len(songs) != MaxTracks {
		t.Errorf("Search returned %d songs, want %d", len(songs), MaxTracks)
	}
}

func TestSearchAlbumUsesTheAlbumCoverForEveryTrack(t *testing.T) {
	c := fakeAPI(t, map[string]string{
		"/albums/4cOdK2wGLETKBW3PvgPWqT": `{"name":"LP","images":[{"url":"https://img/lp.jpg"}],"tracks":{"next":"{base}albumpage2","items":[
			{"name":"one","duration_ms":1000,"artists":[{"name":"A"}]}
		]}}`,
		"/albumpage2": `{"next":"","items":[{"name":"two","duration_ms":1000,"artists":[{"name":"A"}]}]}`,
	})

	songs, err := c.Search(context.Background(), "https://open.spotify.com/album/4cOdK2wGLETKBW3PvgPWqT")

	if err != nil {
		t.Fatalf("Search returned error %v", err)
	}
	if got, want := names(songs), "A - one, A - two"; got != want {
		t.Errorf("Search songs = %s, want %s", got, want)
	}
	for _, s := range songs {
		if s.Thumbnail != "https://img/lp.jpg" {
			t.Errorf("%s thumbnail = %q, want the album cover", s.Name, s.Thumbnail)
		}
	}
}

func TestSearchTrack(t *testing.T) {
	c := fakeAPI(t, map[string]string{
		"/tracks/4cOdK2wGLETKBW3PvgPWqT": trackJSON("song", "Artist", 200000),
	})

	songs, err := c.Search(context.Background(), "https://open.spotify.com/intl-sv/track/4cOdK2wGLETKBW3PvgPWqT")

	if err != nil || names(songs) != "Artist - song" {
		t.Errorf("Search = %s, %v; want Artist - song", names(songs), err)
	}
}

func TestSearchFirstRequestFailureIsAnError(t *testing.T) {
	c := fakeAPI(t, map[string]string{"/playlists/4cOdK2wGLETKBW3PvgPWqT": "500"})

	if songs, err := c.Search(context.Background(), playlistURL); err == nil {
		t.Errorf("Search returned %s and no error", names(songs))
	}
}
