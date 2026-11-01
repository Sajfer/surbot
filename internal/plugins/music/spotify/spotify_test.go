package spotify

import (
	"context"
	"testing"
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

func TestSearchNonLinkReturnsEmptyPlaylistWithoutNetwork(t *testing.T) {
	c := &Client{}

	got, err := c.Search(context.Background(), "never gonna give you up")

	if err != nil {
		t.Fatalf("Search returned error %v, want nil", err)
	}
	if len(got.Songs) != 0 {
		t.Errorf("Search returned %d songs, want 0", len(got.Songs))
	}
}
