package youtube

import "testing"

func TestOpenAudioStreamRejectsInvalidIDs(t *testing.T) {
	for _, id := range []string{"", "short", "--exec=rm", "dQw4w9WgXcQ; ls", "dQw4w9WgXcQx"} {
		if stream, err := OpenAudioStream(id); err == nil {
			_ = stream.Close()
			t.Errorf("OpenAudioStream(%q) returned no error", id)
		}
	}
}

func TestOpenSearchStreamRejectsAnEmptyQuery(t *testing.T) {
	for _, q := range []string{"", "   "} {
		if stream, err := OpenSearchStream(q); err == nil {
			_ = stream.Close()
			t.Errorf("OpenSearchStream(%q) returned no error", q)
		}
	}
}
