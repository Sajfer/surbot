package core

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTextMessageTurnsOffMentions(t *testing.T) {
	msg := textMessage("@everyone listen to <@&123> and <@456>")

	body, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(body), `"allowed_mentions":{"parse":[]`) {
		t.Errorf("message JSON does not disable mentions: %s", body)
	}
	if msg.Content != "@everyone listen to <@&123> and <@456>" {
		t.Errorf("content changed to %q", msg.Content)
	}
}

func TestTextMessageFitsDiscordLimit(t *testing.T) {
	msg := textMessage(strings.Repeat("a", MaxMessageLength+500))

	if n := utf16Len(msg.Content); n != MaxMessageLength {
		t.Errorf("content is %d characters, want %d", n, MaxMessageLength)
	}
	if !strings.HasSuffix(msg.Content, "…") {
		t.Errorf("cut content does not end in …")
	}
}

func TestFitText(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		limit int
		want  string
	}{
		{name: "short", text: "hello", limit: 10, want: "hello"},
		{name: "exactly the limit", text: "hello", limit: 5, want: "hello"},
		{name: "over the limit", text: "hello world", limit: 5, want: "hell…"},
		{name: "multibyte runes stay whole", text: "åäöåäö", limit: 4, want: "åäö…"},
		// 😀 is two UTF-16 units, so only one fits before the ellipsis.
		{name: "emoji count as two", text: "😀😀😀", limit: 4, want: "😀…"},
		{name: "emoji does not fit the last slot", text: "ab😀cd", limit: 3, want: "ab…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fitText(tt.text, tt.limit)
			if got != tt.want {
				t.Errorf("fitText(%q, %d) = %q, want %q", tt.text, tt.limit, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("fitText produced invalid UTF-8: %q", got)
			}
			if utf16Len(got) > tt.limit {
				t.Errorf("fitText result is %d units, over the limit %d", utf16Len(got), tt.limit)
			}
		})
	}
}
