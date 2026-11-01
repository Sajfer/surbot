package music

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/sajfer/dca"
	"github.com/sajfer/discordgo"
	"github.com/sirupsen/logrus"

	"gitlab.com/sajfer/surbot/internal/core"
	"gitlab.com/sajfer/surbot/internal/logger"
	"gitlab.com/sajfer/surbot/internal/plugins/music/youtube"
)

const (
	embedFieldLimit = 1024
	playHint        = "Use !play <youtube link|spotify link> to queue a song"
)

// discordVoice joins voice channels through a discordgo session.
type discordVoice struct {
	s *discordgo.Session
}

// Join returns once the voice connection is ready, or an error. A failed join
// leaves the channel again so the bot does not linger in it.
func (d discordVoice) Join(guildID, channelID string) (voiceConn, error) {
	setVoiceLogLevel(d.s, guildID)
	vc, err := d.s.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		d.s.RLock()
		existing := d.s.VoiceConnections[guildID]
		d.s.RUnlock()
		if existing != nil && voiceReady(existing) {
			vc = existing
		} else {
			if existing != nil {
				_ = (&discordConn{vc: existing}).Leave()
			}
			return nil, err
		}
	}
	// Wait for voice connection to be fully ready before streaming.
	time.Sleep(500 * time.Millisecond)
	return &discordConn{vc: vc}, nil
}

func voiceReady(vc *discordgo.VoiceConnection) bool {
	vc.RLock()
	defer vc.RUnlock()
	return vc.Ready
}

// setVoiceLogLevel makes discordgo log the voice handshake when surbot runs at
// debug level. A voice connection has its own log level, so it has to be
// created with one before ChannelVoiceJoin starts the handshake.
func setVoiceLogLevel(session *discordgo.Session, guildID string) {
	if !logger.Log.IsLevelEnabled(logrus.DebugLevel) {
		return
	}
	session.Lock()
	defer session.Unlock()
	if _, ok := session.VoiceConnections[guildID]; !ok {
		session.VoiceConnections[guildID] = &discordgo.VoiceConnection{LogLevel: discordgo.LogDebug}
	}
}

// discordConn is one joined voice channel.
type discordConn struct {
	vc *discordgo.VoiceConnection
}

// Leave disconnects from the channel. Disconnect writes to the gateway socket
// without a nil check, so a gone socket would panic.
func (c *discordConn) Leave() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("leave voice: %v", r)
		}
	}()
	return c.vc.Disconnect()
}

func encodeOptions() *dca.EncodeOptions {
	return &dca.EncodeOptions{
		Volume:           0.10,
		Channels:         2,
		FrameRate:        48000,
		FrameDuration:    20,
		Bitrate:          384,
		Application:      dca.AudioApplicationLowDelay,
		CompressionLevel: 10,
		PacketLoss:       1,
		BufferedFrames:   100,
		VBR:              true,
		RawOutput:        true,
	}
}

// openEncoder starts the encoder for song. audio is nil when yt-dlp is not
// used; otherwise the caller must Close it.
func openEncoder(song *Song) (enc *dca.EncodeSession, audio *youtube.AudioStream, err error) {
	options := encodeOptions()
	if song.ID != "" {
		audio, err = youtube.OpenAudioStream(song.ID)
		if err == nil {
			enc, err = dca.EncodeMem(audio, options)
			if err != nil {
				_ = audio.Close()
				return nil, nil, fmt.Errorf("encode %s: %w", song.Title, err)
			}
			return enc, audio, nil
		}
		logger.Log.Warningf("could not start yt-dlp for %s, trying the youtube library's stream URL: %v", song.Title, err)
		audio = nil
	}
	if song.StreamURL == "" {
		return nil, nil, fmt.Errorf("stream URL is empty for song: %s", song.Title)
	}
	enc, err = dca.EncodeFile(song.StreamURL, options)
	if err != nil {
		return nil, nil, fmt.Errorf("encode %s: %w", song.Title, err)
	}
	return enc, nil, nil
}

// Play streams song to the channel and blocks until it ends, fails or ctx is
// cancelled (then it returns ctx.Err()). When it returns, yt-dlp and ffmpeg
// have exited and the dca streaming goroutine is done.
func (c *discordConn) Play(ctx context.Context, song *Song) error {
	enc, audio, err := openEncoder(song)
	if err != nil {
		logger.Log.Errorf("Could not encode stream: %v", err)
		return err
	}

	done := make(chan error, 1)
	dca.NewStream(enc, c.vc, done)

	var streamErr error
	received := false
	select {
	case streamErr = <-done:
		received = true
	case <-ctx.Done():
	}

	encoderDone := received && (streamErr == nil || errors.Is(streamErr, io.EOF))
	endPlayback(audio, enc, done, received, encoderDone)

	if ctx.Err() != nil && !received {
		return ctx.Err()
	}
	if streamErr != nil && !errors.Is(streamErr, io.EOF) {
		logger.Log.Errorf("error while playing audio: %v", streamErr)
		logger.Log.Errorf("ffmpeg output: %s", enc.FFMPEGMessages())
		return streamErr
	}
	// ffmpeg can fail without the stream noticing.
	if ffmpegErr := enc.Error(); ffmpegErr != nil {
		logger.Log.Errorf("ffmpeg encoding error: %v\nffmpeg output:\n%s", ffmpegErr, enc.FFMPEGMessages())
		if audio != nil {
			logger.Log.Errorf("yt-dlp output: %s", audio.Stderr())
		}
		return ffmpegErr
	}
	return nil
}

// endPlayback is the one exit sequence for every end of a song: natural end,
// stream error and cancel. The order matters. ffmpeg's stdin is the yt-dlp
// reader, and waiting for ffmpeg waits for that copy, so yt-dlp must die
// first; Cleanup then drains the frames so the streaming goroutine reaches EOF
// instead of playing buffered audio, and its result on done is received last.
// encoderDone means the stream hit EOF, so ffmpeg has already exited.
func endPlayback(audio *youtube.AudioStream, enc *dca.EncodeSession, done <-chan error, received, encoderDone bool) {
	if audio != nil {
		_ = audio.Close()
	}
	if !encoderDone {
		stopEncoder(enc)
	}
	enc.Cleanup()
	if !received {
		if err := <-done; err != nil && !errors.Is(err, io.EOF) {
			logger.Log.Debugf("stream ended with error after cancel: %v", err)
		}
	}
}

// stopEncoder kills ffmpeg. dca starts it from its own goroutine, so a cancel
// right after the encoder was created can find it not running yet; Stop is
// retried briefly for that case. When ffmpeg already exited there is nothing
// to wait for.
func stopEncoder(enc *dca.EncodeSession) {
	for range 5 {
		if enc.Stop() == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// discordAnnouncer posts to Discord; errors are logged, never returned.
type discordAnnouncer struct {
	s   *discordgo.Session
	log *logrus.Entry
}

func (a discordAnnouncer) NowPlaying(channelID string, song *Song) {
	if _, err := a.s.ChannelMessageSendEmbed(channelID, nowPlayingEmbed(song)); err != nil {
		a.log.WithError(err).Warning("failed to send now playing message")
	}
}

func (a discordAnnouncer) Text(channelID, text string) {
	if err := core.SendText(a.s, channelID, text); err != nil {
		a.log.WithError(err).Warning("failed to send message")
	}
}

func (a discordAnnouncer) Listening(title string) {
	if err := a.s.UpdateListeningStatus(title); err != nil {
		a.log.WithError(err).Warning("failed to update listening status")
	}
}

func nowPlayingEmbed(song *Song) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{Fields: []*discordgo.MessageEmbedField{
		{Name: "Now playing", Value: truncate(song.Title, embedFieldLimit)},
		{Name: "Duration", Value: formatDuration(song.Duration)},
	}}
	if song.Thumbnail != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: song.Thumbnail}
	}
	return embed
}

func notPlayingEmbed() *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{Fields: []*discordgo.MessageEmbedField{
		{Name: "Currently not playing", Value: playHint},
	}}
}

func queueClearedEmbed() *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:  "Queue",
		Fields: []*discordgo.MessageEmbedField{{Name: "Queue have been cleared", Value: playHint}},
	}
}

// truncate shortens s to at most limit characters.
func truncate(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}

// formatDuration renders d as M:SS, or H:MM:SS from one hour on.
func formatDuration(d time.Duration) string {
	total := int(d.Seconds())
	hours, minutes, seconds := total/3600, total%3600/60, total%60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}
