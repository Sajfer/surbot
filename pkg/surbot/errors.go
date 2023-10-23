// Package surbot contains the main functionality for Surbot.
package surbot

import "errors"

var (
	errVoiceSkippedManually = errors.New("voice: skipped audio manually")
	errVoiceStoppedManually = errors.New("voice: stopped audio manually")
)
