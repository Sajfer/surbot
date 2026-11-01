package youtube

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeYtdlp puts a yt-dlp script first on PATH that writes its arguments, one
// per line, to the returned file and then runs script.
func fakeYtdlp(t *testing.T, script string) (argsFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + argsFile + "\"\n" + script + "\n"
	if err := os.WriteFile(filepath.Join(dir, "yt-dlp"), []byte(body), 0o755); err != nil { // #nosec G306 -- test script must be executable
		t.Fatalf("write fake yt-dlp: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argsFile
}

func readArgs(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	return strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
}

func TestLookupVideo(t *testing.T) {
	args := fakeYtdlp(t, `echo '{"_type":"video","id":"dQw4w9WgXcQ","title":"Never Gonna Give You Up","duration":212.5,"thumbnail":"https://i.ytimg.com/x.jpg"}'`)

	videos, err := New().Lookup(context.Background(), "https://youtu.be/dQw4w9WgXcQ")

	if err != nil {
		t.Fatalf("Lookup returned error %v", err)
	}
	if len(videos) != 1 {
		t.Fatalf("Lookup returned %d videos, want 1", len(videos))
	}
	v := videos[0]
	if v.ID != "dQw4w9WgXcQ" || v.Title != "Never Gonna Give You Up" || v.Duration != 212500*time.Millisecond || v.Thumbnail != "https://i.ytimg.com/x.jpg" {
		t.Errorf("video = %+v", v)
	}
	got := readArgs(t, args)
	if n := len(got); n < 2 || got[n-2] != "--" || got[n-1] != "https://youtu.be/dQw4w9WgXcQ" {
		t.Errorf("yt-dlp args end in %v, want -- and the URL", got)
	}
}

func TestLookupPlaylistKeepsOrderAndSkipsUnavailableEntries(t *testing.T) {
	fakeYtdlp(t, `echo '{"_type":"playlist","id":"PL1","title":"mix","entries":[
		{"_type":"url","id":"aaaaaaaaaaa","title":"one","duration":60},
		{"_type":"url","id":"bbbbbbbbbbb","title":"[Private video]","duration":null},
		{"_type":"url","id":"ccccccccccc","title":"three","duration":null},
		{"_type":"url","id":"not-an-id","title":"broken"},
		{"_type":"url","id":"ddddddddddd","title":"[Deleted video]"}
	]}'`)

	videos, err := New().Lookup(context.Background(), "https://www.youtube.com/playlist?list=PL1")

	if err != nil {
		t.Fatalf("Lookup returned error %v", err)
	}
	var ids []string
	for _, v := range videos {
		ids = append(ids, v.ID)
	}
	if strings.Join(ids, ",") != "aaaaaaaaaaa,ccccccccccc" {
		t.Errorf("Lookup IDs = %v, want [aaaaaaaaaaa ccccccccccc]", ids)
	}
	if want := "https://i.ytimg.com/vi/aaaaaaaaaaa/hqdefault.jpg"; videos[0].Thumbnail != want {
		t.Errorf("thumbnail = %q, want %q", videos[0].Thumbnail, want)
	}
}

func TestSearchAsksForOneResult(t *testing.T) {
	args := fakeYtdlp(t, `echo '{"_type":"playlist","entries":[{"_type":"url","id":"dQw4w9WgXcQ","title":"hit"}]}'`)

	v, err := New().Search(context.Background(), "-rick astley")

	if err != nil {
		t.Fatalf("Search returned error %v", err)
	}
	if v.ID != "dQw4w9WgXcQ" {
		t.Errorf("Search ID = %q, want dQw4w9WgXcQ", v.ID)
	}
	got := readArgs(t, args)
	if last := got[len(got)-1]; last != "ytsearch1:-rick astley" {
		t.Errorf("yt-dlp target = %q, want %q", last, "ytsearch1:-rick astley")
	}
}

func TestSearchWithoutResultFails(t *testing.T) {
	fakeYtdlp(t, `echo '{"_type":"playlist","entries":[]}'`)

	if v, err := New().Search(context.Background(), "zzzz"); err == nil {
		t.Fatalf("Search returned %+v, want an error", v)
	}
}

func TestLookupFailureIncludesYtdlpStderr(t *testing.T) {
	fakeYtdlp(t, `echo 'ERROR: Video unavailable' >&2; exit 1`)

	_, err := New().Lookup(context.Background(), "https://youtu.be/dQw4w9WgXcQ")

	if err == nil || !strings.Contains(err.Error(), "Video unavailable") {
		t.Errorf("Lookup error = %v, want it to include yt-dlp's message", err)
	}
}

func TestLookupStopsWhenContextIsCancelled(t *testing.T) {
	fakeYtdlp(t, `exec sleep 30`)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := New().Lookup(ctx, "https://youtu.be/dQw4w9WgXcQ")

	if err == nil {
		t.Fatal("Lookup returned no error after cancel")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Lookup took %v after cancel", elapsed)
	}
}
