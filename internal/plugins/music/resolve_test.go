package music

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"gitlab.com/sajfer/surbot/internal/plugins/music/spotify"
	"gitlab.com/sajfer/surbot/internal/plugins/music/youtube"
)

type fakeVideoAPI struct {
	searchResult string
	searchErr    error
	searched     []string
	videos       map[string][]*youtube.Video
	infoErr      error
	infoCalls    []string
}

func (f *fakeVideoAPI) SearchVideo(_ context.Context, query string) (string, error) {
	f.searched = append(f.searched, query)
	return f.searchResult, f.searchErr
}

func (f *fakeVideoAPI) GetVideoInfo(_ context.Context, url string) (*youtube.Playlist, error) {
	f.infoCalls = append(f.infoCalls, url)
	if f.infoErr != nil {
		return &youtube.Playlist{}, f.infoErr
	}
	return &youtube.Playlist{Songs: f.videos[url]}, nil
}

type fakeTrackAPI struct {
	songs []spotify.Song
	err   error
}

func (f *fakeTrackAPI) Search(context.Context, string) (*spotify.Playlist, error) {
	return &spotify.Playlist{Songs: f.songs}, f.err
}

func titles(songs []*Song) []string {
	out := make([]string, len(songs))
	for i, s := range songs {
		out[i] = s.Title
	}
	return out
}

func TestResolveYouTubeURLReturnsEveryVideoInOrder(t *testing.T) {
	const url = "https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=PL1234567890"
	yt := &fakeVideoAPI{videos: map[string][]*youtube.Video{url: {
		{Title: "one", ID: "aaaaaaaaaaa"}, {Title: "two", ID: "bbbbbbbbbbb"}, {Title: "three", ID: "ccccccccccc"},
	}}}

	songs, err := newResolver(yt, nil).Resolve(context.Background(), url)

	if err != nil {
		t.Fatalf("Resolve returned error %v", err)
	}
	if want := []string{"one", "two", "three"}; !reflect.DeepEqual(titles(songs), want) {
		t.Errorf("Resolve titles = %v, want %v", titles(songs), want)
	}
	if songs[1].ID != "bbbbbbbbbbb" {
		t.Errorf("second song ID = %q, want bbbbbbbbbbb", songs[1].ID)
	}
	if len(yt.searched) != 0 {
		t.Errorf("YouTube search was called for a link: %v", yt.searched)
	}
}

func TestResolveFreeTextSearchesAndReturnsFirstVideoOnly(t *testing.T) {
	yt := &fakeVideoAPI{
		searchResult: "youtube.com/watch?v=xyz",
		videos:       map[string][]*youtube.Video{"youtube.com/watch?v=xyz": {{Title: "hit", Duration: 90.5}, {Title: "extra"}}},
	}

	songs, err := newResolver(yt, nil).Resolve(context.Background(), "never gonna give")

	if err != nil {
		t.Fatalf("Resolve returned error %v", err)
	}
	if want := []string{"hit"}; !reflect.DeepEqual(titles(songs), want) {
		t.Errorf("Resolve titles = %v, want %v", titles(songs), want)
	}
	if want := []string{"never gonna give"}; !reflect.DeepEqual(yt.searched, want) {
		t.Errorf("searched = %v, want %v", yt.searched, want)
	}
	if songs[0].Duration.Seconds() != 90.5 {
		t.Errorf("Duration = %v, want 1m30.5s", songs[0].Duration)
	}
}

func TestResolveSearchWithoutResultReturnsError(t *testing.T) {
	yt := &fakeVideoAPI{searchErr: errors.New("no result")}

	songs, err := newResolver(yt, nil).Resolve(context.Background(), "zzzz")

	if err == nil {
		t.Fatalf("Resolve returned songs %v, want an error", titles(songs))
	}
	if len(yt.infoCalls) != 0 {
		t.Errorf("GetVideoInfo was called after a failed search: %v", yt.infoCalls)
	}
}

func TestResolveSpotifyLinkWithSpotifyDisabledReturnsErrorWithoutCallingYouTube(t *testing.T) {
	yt := &fakeVideoAPI{}

	_, err := newResolver(yt, nil).Resolve(context.Background(), "https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT?si=a")

	if err == nil {
		t.Fatal("Resolve returned no error with Spotify disabled")
	}
	if len(yt.searched) != 0 || len(yt.infoCalls) != 0 {
		t.Errorf("YouTube was called: searched=%v info=%v", yt.searched, yt.infoCalls)
	}
}

func TestResolveSpotifyTrackSearchesYouTubeForArtistAndNameAndReturnsOneSong(t *testing.T) {
	yt := &fakeVideoAPI{
		searchResult: "youtube.com/watch?v=abc",
		videos:       map[string][]*youtube.Video{"youtube.com/watch?v=abc": {{Title: "Y by X"}}},
	}
	sp := &fakeTrackAPI{songs: []spotify.Song{{Artist: "X", Name: "Y"}, {Artist: "Z", Name: "W"}}}

	songs, err := newResolver(yt, sp).Resolve(context.Background(), "https://open.spotify.com/playlist/4cOdK2wGLETKBW3PvgPWqT?si=a")

	if err != nil {
		t.Fatalf("Resolve returned error %v", err)
	}
	if want := []string{"X - Y"}; !reflect.DeepEqual(yt.searched, want) {
		t.Errorf("searched = %v, want %v", yt.searched, want)
	}
	if want := []string{"Y by X"}; !reflect.DeepEqual(titles(songs), want) {
		t.Errorf("Resolve titles = %v, want %v", titles(songs), want)
	}
}

func TestResolveSpotifyLinkWithoutTracksReturnsError(t *testing.T) {
	yt := &fakeVideoAPI{}

	_, err := newResolver(yt, &fakeTrackAPI{}).Resolve(context.Background(), "https://open.spotify.com/album/4cOdK2wGLETKBW3PvgPWqT?si=a")

	if err == nil {
		t.Fatal("Resolve returned no error for a link without tracks")
	}
}

func TestResolveZeroVideosReturnsErrorNotEmptySlice(t *testing.T) {
	yt := &fakeVideoAPI{videos: map[string][]*youtube.Video{}}

	songs, err := newResolver(yt, nil).Resolve(context.Background(), "https://youtu.be/dQw4w9WgXcQ")

	if err == nil {
		t.Fatalf("Resolve returned %d songs and no error", len(songs))
	}
}
