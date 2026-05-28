// Package surbot contains the main functionality for Surbot.
package surbot

import "errors"

// Typed error values for voice state management (type-safe comparisons)
var (
	errVoiceSkippedManually = errors.New("voice skipped")
	errVoiceStoppedManually = errors.New("voice stopped")
)

// Check if an error is a voice-specific error by checking its Error() string value
// This avoids fragile type assertions since our errors are string-based
func IsVoiceSkipped(err error) bool {
	return err != nil && errors.Is(err, errVoiceSkippedManually)
}

func IsVoiceStopped(err error) bool {
	return err != nil && errors.Is(err, errVoiceStoppedManually)
}
