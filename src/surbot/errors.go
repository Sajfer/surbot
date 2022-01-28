package surbot

import "errors"

var (
	errVoiceSkippedManually = errors.New("voice: skipped audio manually")
	errVoiceStoppedManually = errors.New("voice: stopped audio manually")
)
