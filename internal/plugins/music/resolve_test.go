package music

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/sajfer/surbot/internal/plugins/music/spotify"
	"github.com/sajfer/surbot/internal/plugins/music/youtube"
)

type fakeVideoAPI struct {
	searchResult *youtube.Video
	searchErr    error
	searched     []string
	videos       map[string][]*youtube.Video
	lookupErr    error
	lookups      []string
}

func (f *fakeVideoAPI) Search(_ context.Context, query string) (*youtube.Video, error) {
	f.searched = append(f.searched, query)
	return f.searchResult, f.searchErr
}

func (f *fakeVideoAPI) Lookup(_ context.Context, url string) ([]*youtube.Video, error) {
	f.lookups = append(f.lookups, url)
	return f.videos[url], f.lookupErr
}

type fakeTrackAPI struct {
	songs []spotify.Song
	err   error
}

func (f *fakeTrackAPI) Search(context.Context, string) ([]spotify.Song, error) {
	return f.songs, f.err
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

func TestResolveFreeTextSearchesAndReturnsTheMatch(t *testing.T) {
	yt := &fakeVideoAPI{searchResult: &youtube.Video{Title: "hit", Duration: 90500 * time.Millisecond, ID: "xyzxyzxyzxy"}}

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
	if songs[0].Duration.Seconds() != 90.5 || songs[0].ID != "xyzxyzxyzxy" {
		t.Errorf("song = %+v, want 1m30.5s with ID xyzxyzxyzxy", songs[0])
	}
	if len(yt.lookups) != 0 {
		t.Errorf("Lookup was called for free text: %v", yt.lookups)
	}
}

func TestResolveSearchWithoutResultReturnsError(t *testing.T) {
	yt := &fakeVideoAPI{searchErr: errors.New("no result")}

	songs, err := newResolver(yt, nil).Resolve(context.Background(), "zzzz")

	if err == nil {
		t.Fatalf("Resolve returned songs %v, want an error", titles(songs))
	}
	if len(yt.lookups) != 0 {
		t.Errorf("Lookup was called after a failed search: %v", yt.lookups)
	}
}

func TestResolveSpotifyLinkWithSpotifyDisabledReturnsErrorWithoutCallingYouTube(t *testing.T) {
	yt := &fakeVideoAPI{}

	_, err := newResolver(yt, nil).Resolve(context.Background(), "https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT?si=a")

	if err == nil {
		t.Fatal("Resolve returned no error with Spotify disabled")
	}
	if len(yt.searched) != 0 || len(yt.lookups) != 0 {
		t.Errorf("YouTube was called: searched=%v lookups=%v", yt.searched, yt.lookups)
	}
}

func TestResolveSpotifyQueuesEveryTrackWithoutSearchingYet(t *testing.T) {
	yt := &fakeVideoAPI{}
	sp := &fakeTrackAPI{songs: []spotify.Song{
		{Artist: "X", Name: "Y", Duration: time.Minute, Thumbnail: "https://img/y.jpg"},
		{Artist: "Z", Name: "W"},
	}}

	songs, err := newResolver(yt, sp).Resolve(context.Background(), "https://open.spotify.com/playlist/4cOdK2wGLETKBW3PvgPWqT?si=a")

	if err != nil {
		t.Fatalf("Resolve returned error %v", err)
	}
	if want := []string{"X - Y", "Z - W"}; !reflect.DeepEqual(titles(songs), want) {
		t.Errorf("Resolve titles = %v, want %v", titles(songs), want)
	}
	first := songs[0]
	if first.Query != "X - Y" || first.ID != "" || first.Duration != time.Minute || first.Thumbnail != "https://img/y.jpg" {
		t.Errorf("first song = %+v, want query X - Y, no ID, Spotify duration and cover", first)
	}
	if len(yt.searched) != 0 || len(yt.lookups) != 0 {
		t.Errorf("YouTube was called at queue time: searched=%v lookups=%v", yt.searched, yt.lookups)
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

func TestResolveLookupErrorIsReturned(t *testing.T) {
	yt := &fakeVideoAPI{lookupErr: errors.New("yt-dlp failed")}

	if _, err := newResolver(yt, nil).Resolve(context.Background(), "https://youtu.be/dQw4w9WgXcQ"); err == nil {
		t.Fatal("Resolve returned no error when the lookup failed")
	}
}

func TestIsYouTubeURL(t *testing.T) {
	links := []string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"youtube.com/watch?v=dQw4w9WgXcQ",
		"https://www.youtube.com/watch?app=desktop&v=dQw4w9WgXcQ",
		"https://www.youtube.com/playlist?list=PLFgquLnL59alCl_2TQvOiD5Vgm1hCaGSI",
		"https://youtu.be/dQw4w9WgXcQ?si=abc",
		"https://m.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://music.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://youtube.com/shorts/dQw4w9WgXcQ",
		"https://www.youtube.com/live/dQw4w9WgXcQ",
	}
	for _, u := range links {
		if !isYouTubeURL(u) {
			t.Errorf("isYouTubeURL(%q) = false, want true", u)
		}
	}
	notLinks := []string{
		"never gonna give you up",
		"https://www.youtube.com/@channel",
		"https://www.youtube.com/",
		"https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ and more words",
	}
	for _, u := range notLinks {
		if isYouTubeURL(u) {
			t.Errorf("isYouTubeURL(%q) = true, want false", u)
		}
	}
}
