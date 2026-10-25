// Package surbot contains the main functionality for Surbot.
package surbot

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/sajfer/dca"
	"github.com/sajfer/discordgo"
	"gitlab.com/sajfer/surbot/internal/logger"
	"gitlab.com/sajfer/surbot/internal/utils"
	"gitlab.com/sajfer/surbot/pkg/music"
	"gitlab.com/sajfer/surbot/pkg/youtube"
)

type Voice struct {
	VoiceChannel     *discordgo.VoiceConnection
	EncodingSession  *dca.EncodeSession
	StreamingSession *dca.StreamingSession
	Session          *discordgo.Session
	Playing          bool
	voiceGuildID     string
	voiceChannelID   string
	channelID        string
	done             chan error
	timer            *Timer
	music            *music.Music
}

var (
	timeout = 5
)

func NewVoice(music *music.Music) *Voice {
	return &Voice{timer: &Timer{stop: make(chan bool), running: false}, music: music}
}

func (voice *Voice) SetTextChannel(channel string) {
	voice.channelID = channel
}

func (voice *Voice) SetSession(session *discordgo.Session) {
	voice.Session = session
}

func (voice *Voice) Connect(channelId, guildId string, mute, deaf bool) error {
	logger.Log.Debug("voice.Connect")

	if channelId == "" {
		return errors.New("user not in a channel")
	}
	vc, err := voice.Session.ChannelVoiceJoin(guildId, channelId, mute, deaf)
	if err != nil {
		if _, ok := voice.Session.VoiceConnections[guildId]; ok {
			vc = voice.Session.VoiceConnections[guildId]
		} else {
			return err
		}
	}
	voice.VoiceChannel = vc
	voice.voiceChannelID = channelId
	voice.voiceGuildID = guildId
	return nil
}

func (voice *Voice) Disconnect() error {
	logger.Log.Debug("voice.Disconnect")

	if voice.Playing && voice.done != nil {
		select {
		case voice.done <- errVoiceStoppedManually:
		default:
		}
	}
	voice.Playing = false
	voice.voiceChannelID = ""
	voice.voiceGuildID = ""
	if voice.StreamingSession != nil {
		_, err := voice.StreamingSession.Finished()
		if err != nil {
			return err
		}
		voice.StreamingSession = nil
	}

	if voice.EncodingSession != nil {
		err := voice.EncodingSession.Stop()
		if err != nil {
			return err
		}
		voice.EncodingSession.Cleanup()
		voice.EncodingSession = nil
	}
	if voice.VoiceChannel != nil {
		err := voice.VoiceChannel.Disconnect()
		if err != nil {
			return err
		}
		voice.VoiceChannel = nil
	}
	return nil
}

func (voice *Voice) Start(m *discordgo.MessageCreate) error {
	logger.Log.Debug("voice.Start")

	if !voice.Playing {
		guild, err := voice.Session.State.Guild(m.GuildID)
		if err != nil {
			return err
		}
		channelId := ""
		for _, person := range guild.VoiceStates {
			if person.UserID == m.Author.ID {
				logger.Log.Debugf("Voice channel: %s", person.ChannelID)
				channelId = person.ChannelID
				break
			}
		}
		err = voice.Connect(channelId, m.GuildID, false, true)
		if err != nil {
			logger.Log.Warningf("could not join voice channel, err=%s", err)
			return err
		}
		// Wait for voice connection to be fully ready before streaming.
		time.Sleep(500 * time.Millisecond)
		err = voice.play()
		if err != nil {
			logger.Log.Warningf("could not play song, err=%s", err)
			return err
		}
	}
	return nil
}

func (voice *Voice) stopPlaying() error {
	err := voice.Session.UpdateListeningStatus("")
	voice.music.CurrentSong = nil
	go voice.timer.initTimer(time.Duration(timeout)*time.Minute, voice)
	return err
}

func (voice *Voice) getSongFromQueue() (*music.Song, error) {
	if len(voice.music.Queue) == 0 {
		return nil, fmt.Errorf("queue is empty")
	}
	song := voice.music.Queue[0]
	voice.music.Queue = voice.music.Queue[1:]
	err := voice.Session.UpdateListeningStatus(song.Title)
	if err != nil {
		voice.Playing = false
		voice.music.Queue = nil
		return nil, err
	}
	return song, nil
}

func (voice *Voice) play() error {
	logger.Log.Debug("voice.Play")

	if voice.Playing {
		return nil
	}

	voice.Playing = true

	if voice.timer.running {
		voice.timer.stopTimer()
	}
	song, err := voice.getSongFromQueue()
	if err != nil {
		return err
	}

	voice.music.CurrentSong = song
	voice.NowPlaying()
	logger.Log.Infof("Now playing: %s - %s", song.Artist, song.Title)
	msg, err := voice.playRaw(*song)
	if msg == errVoiceStoppedManually {
		voice.Playing = false
		return voice.stopPlaying()
	}
	if IsVoiceSkipped(err) {
		_ = voice.Session.UpdateListeningStatus("")
		voice.music.CurrentSong = nil
		if len(voice.music.Queue) > 0 {
			return voice.play()
		}
		return nil
	}
	if len(voice.music.Queue) > 0 {
		return voice.play()
	} else {
		err := voice.Session.UpdateListeningStatus("")
		if err != nil {
			return err
		}
		voice.music.CurrentSong = nil
		go voice.timer.initTimer(time.Duration(timeout)*time.Minute, voice)
		return nil
	}
}

func (voice *Voice) playRaw(song music.Song) (error, error) {
	logger.Log.Debug("voice.PlayRaw")
	var err error

	options := &dca.EncodeOptions{
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

	// Keep our own references: Stop, Skip and Disconnect can change the
	// shared fields from another goroutine while we wait on done.
	var encodingSession *dca.EncodeSession
	var stream *youtube.AudioStream
	if song.ID != "" {
		stream, err = youtube.OpenAudioStream(song.ID)
		if err != nil {
			logger.Log.Warningf("could not start yt-dlp for %s, trying the youtube library's stream URL: %v", song.Title, err)
			stream = nil
		} else {
			defer func() { _ = stream.Close() }()
			encodingSession, err = dca.EncodeMem(stream, options)
		}
	}
	if stream == nil {
		if song.StreamURL == "" {
			return nil, fmt.Errorf("stream URL is empty for song: %s", song.Title)
		}
		encodingSession, err = dca.EncodeFile(song.StreamURL, options)
	}
	if err != nil {
		logger.Log.Errorf("Could not encode stream: %v", err)
		return nil, err
	}
	voice.EncodingSession = encodingSession

	done := make(chan error)
	voice.done = done
	streamingSession := dca.NewStream(encodingSession, voice.VoiceChannel, done)
	voice.StreamingSession = streamingSession
	msg := <-done
	voice.Playing = false
	defer func() {
		encodingSession.Cleanup()
		if voice.EncodingSession == encodingSession {
			voice.EncodingSession = nil
		}
		if voice.StreamingSession == streamingSession {
			voice.StreamingSession = nil
		}
	}()

	if msg != nil && msg != io.EOF {
		logger.Log.Errorf("error while playing audio: %v", msg)
		logger.Log.Errorf("ffmpeg output: %s", encodingSession.FFMPEGMessages())
		return msg, err
	}
	// Check if ffmpeg actually produced any data or failed silently.
	if ffmpegErr := encodingSession.Error(); ffmpegErr != nil {
		logger.Log.Errorf("ffmpeg encoding error: %v\nffmpeg output:\n%s", ffmpegErr, encodingSession.FFMPEGMessages())
		if stream != nil {
			logger.Log.Errorf("yt-dlp output: %s", stream.Stderr())
		}
		return msg, ffmpegErr
	}
	if _, err = streamingSession.Finished(); err != nil {
		logger.Log.Warningf("error while stopping stream session, err=%s", err)
	}
	return msg, err
}

func (voice *Voice) Stop() error {
	if voice.done != nil {
		select {
		case voice.done <- errVoiceStoppedManually:
		default:
		}
	}

	if voice.EncodingSession != nil {
		if err := voice.EncodingSession.Stop(); err != nil {
			return err
		}
		voice.EncodingSession.Cleanup()
	}
	voice.Playing = false

	go voice.timer.initTimer(time.Duration(timeout)*time.Minute, voice)

	return nil
}

func (voice *Voice) ShowQueue() error {
	embed := NewEmbed()
	embed.SetTitle("Queue")
	songList := ""
	var index = 1
	if voice.music.CurrentSong != nil {
		songList = songList + fmt.Sprintf("%d. %s\n", index, voice.music.CurrentSong.Title)
		index = index + 1
	} else {
		embed.AddField("No songs queued", "Use !play <youtube link|spotify link> to queue a song")
	}
	for i, song := range voice.music.Queue {
		if i > 18 {
			songList = songList + "-- Only showing the first 20 songs --\n"
			break
		}
		songList = songList + fmt.Sprintf("%d. %s\n", i+index, song.Title)
	}
	if songList != "" {
		embed.AddField("---", songList)
	}
	_, err := voice.Session.ChannelMessageSendEmbed(voice.channelID, embed.MessageEmbed)
	if err != nil {
		return err
	}
	return nil
}

func (voice *Voice) ClearQueue() error {
	voice.music.Queue = nil
	embed := NewEmbed()
	embed.SetTitle("Queue")
	embed.AddField("Queue have been cleared", "Use !play <youtube link|spotify link> to queue a song")
	_, err := voice.Session.ChannelMessageSendEmbed(voice.channelID, embed.MessageEmbed)
	if err != nil {
		return err
	}
	return nil
}

func (voice *Voice) NowPlaying() {
	embed := NewEmbed()
	if voice.music.CurrentSong != nil {
		embed.AddField("Now playing", voice.music.CurrentSong.Title)
		embed.AddField("Duration", utils.SecondsToHuman(voice.music.CurrentSong.Duration))
		embed.SetThumbnail(voice.music.CurrentSong.Thumbnail)
	} else {
		embed.AddField("Currently not playing", "Use !play <youtube link|spotify link> to queue a song")
	}
	_, err := voice.Session.ChannelMessageSendEmbed(voice.channelID, embed.MessageEmbed)
	if err != nil {
		logger.Log.Warningf("failed to send message, err=%s", err.Error())
	}
}

func (voice *Voice) Skip() error {
	if voice.done != nil {
		select {
		case voice.done <- errVoiceSkippedManually:
		default:
		}
	}

	if voice.EncodingSession != nil {
		if err := voice.EncodingSession.Stop(); err != nil {
			return err
		}
	}

	return nil
}
