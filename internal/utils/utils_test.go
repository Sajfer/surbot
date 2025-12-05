package utils

import (
	"reflect"
	"testing"

	"github.com/sajfer/discordgo"
)

func TestPrintHelp(t *testing.T) {
	type args struct {
		s       *discordgo.Session
		channel string
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			PrintHelp(tt.args.s, tt.args.channel)
		})
	}
}

func TestGetChannel(t *testing.T) {
	type args struct {
		server *discordgo.Guild
		name   string
	}
	tests := []struct {
		name string
		args args
		want *discordgo.Channel
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetChannel(tt.args.server, tt.args.name); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetChannel() = %v, want %v", got, tt.want)
			}
		})
	}
}
