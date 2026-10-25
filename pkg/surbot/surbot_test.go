package surbot

import (
	"sync"
	"testing"
)

func TestCheckServerConcurrentFirstMessagesShareOneServer(t *testing.T) {
	s := &Surbot{}
	got := make([]*Server, 20)
	var wg sync.WaitGroup

	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got[i] = s.checkServer("guild1")
		}()
	}
	wg.Wait()

	for i, srv := range got {
		if srv != got[0] {
			t.Errorf("caller %d got a different Server than caller 0", i)
		}
	}
}

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

			got := s.HandleRoll(nil, nil, "chan1", tt.args)

			if got != "!roll d6 or !roll d20" {
				t.Errorf("HandleRoll(%q) = %q, want the usage text", tt.args, got)
			}
		})
	}
}

func TestCheckServerReturnsSameServerPerGuild(t *testing.T) {
	s := &Surbot{}

	first := s.checkServer("guild1")
	again := s.checkServer("guild1")
	other := s.checkServer("guild2")

	if first != again {
		t.Error("checkServer(guild1) returned different servers on repeat calls")
	}
	if first == other {
		t.Error("checkServer returned the same server for different guilds")
	}
}
