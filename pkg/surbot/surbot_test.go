package surbot

import (
	"testing"
)

func TestHandleRollRejectsInputThatIsNotExactlyADieType(t *testing.T) {
	tests := []struct {
		name string
		args string
	}{
		{name: "extra word after die type", args: "d6 extra"},
		{name: "no die type", args: ""},
		{name: "unknown die type", args: "d7"},
		{name: "glued suffix", args: "d6x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Surbot{}

			got := s.HandleRoll(nil, "chan1", tt.args)

			if got != "!roll d6 or !roll d20" {
				t.Errorf("HandleRoll(%q) = %q, want the usage text", tt.args, got)
			}
		})
	}
}
