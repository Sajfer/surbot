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
		wantURL  bool
		wantKind kind
		wantID   string
	}{
		{name: "track with query", url: "https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT?si=abc", wantURL: true, wantKind: kindTrack, wantID: id},
		{name: "album with query", url: "https://open.spotify.com/album/4cOdK2wGLETKBW3PvgPWqT?si=abc", wantURL: true, wantKind: kindAlbum, wantID: id},
		{name: "playlist with query", url: "https://open.spotify.com/playlist/4cOdK2wGLETKBW3PvgPWqT?si=abc", wantURL: true, wantKind: kindPlaylist, wantID: id},
		{name: "track without query is a Spotify link of no known kind", url: "https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT", wantURL: true, wantKind: kindNone, wantID: id},
		{name: "plain text", url: "never gonna give you up", wantURL: false, wantKind: kindNone, wantID: ""},
		{name: "youtube link", url: "https://www.youtube.com/watch?v=dQw4w9WgXcQ", wantURL: false, wantKind: kindNone, wantID: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsURL(tt.url); got != tt.wantURL {
				t.Errorf("IsURL(%q) = %v, want %v", tt.url, got, tt.wantURL)
			}
			if got := linkKind(tt.url); got != tt.wantKind {
				t.Errorf("linkKind(%q) = %v, want %v", tt.url, got, tt.wantKind)
			}
			if got := getID(tt.url); got != tt.wantID {
				t.Errorf("getID(%q) = %q, want %q", tt.url, got, tt.wantID)
			}
		})
	}
}

func TestSearchLinkWithoutQueryReturnsEmptyPlaylistWithoutNetwork(t *testing.T) {
	c := &Client{}

	got, err := c.Search(context.Background(), "https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT")

	if err != nil {
		t.Fatalf("Search returned error %v, want nil", err)
	}
	if len(got.Songs) != 0 {
		t.Errorf("Search returned %d songs, want 0", len(got.Songs))
	}
}
