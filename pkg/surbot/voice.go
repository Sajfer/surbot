package surbot

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/sajfer/dca"
	"gitlab.com/sajfer/surbot/internal/logger"
	"gitlab.com/sajfer/surbot/internal/utils"
	"gitlab.com/sajfer/surbot/pkg/music"
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
		return err
	}
	voice.VoiceChannel = vc
	voice.voiceChannelID = channelId
	voice.voiceGuildID = guildId
	return nil
}

func (voice *Voice) Disconnect() error {
	logger.Log.Debug("voice.Disconnect")

	if voice.Playing {
		voice.done <- errVoiceStoppedManually
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
	logger.Log.Debug("voice.PlayVideo")

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
		err = voice.play()
		if err != nil {
			logger.Log.Warningf("could not play song, err=%s", err)
			return err
		}
	}
	return nil
}

func (voice *Voice) play() error {
	logger.Log.Debug("voice.Play")

	if !voice.Playing {
		voice.Playing = true

		if len(voice.music.Queue) == 0 {
			return fmt.Errorf("queue is empty")
		}
		song := voice.music.Queue[0]
		voice.music.Queue = voice.music.Queue[1:]
		err := voice.Session.UpdateListeningStatus(song.Title)
		if err != nil {
			voice.Playing = false
			voice.music.Queue = nil
			return err
		}

		if voice.timer.running {
			voice.timer.stopTimer()
		}

		voice.music.CurrentSong = song
		voice.NowPlaying()
		msg, err := voice.playRaw(*song)
		if msg != nil {
			if msg == errVoiceStoppedManually {
				voice.Playing = false
				err := voice.Session.UpdateListeningStatus("")
				voice.music.CurrentSong = nil
				go voice.timer.initTimer(time.Duration(timeout)*time.Minute, *voice)
				return err
			}
		}
		if err != nil {
			voice.Playing = false
			switch err {
			case io.ErrUnexpectedEOF:
				if msg != errVoiceSkippedManually {
					_ = voice.Session.UpdateListeningStatus("")
					voice.music.CurrentSong = nil
					go voice.timer.initTimer(time.Duration(timeout)*time.Minute, *voice)
					return err
				}
			case dca.ErrVoiceConnClosed:
				if msg != errVoiceSkippedManually {
					_ = voice.Session.UpdateListeningStatus("")
					voice.music.CurrentSong = nil
					err := voice.Connect(voice.channelID, voice.VoiceChannel.GuildID, false, true)
					if err != nil {
						logger.Log.Warningf("could not join voice channel, err=%s", err)
						return err
					}
				}
			default:
				return err
			}
		}
		if len(voice.music.Queue) > 0 {
			return voice.play()
		} else {
			err := voice.Session.UpdateListeningStatus("")
			if err != nil {
				return err
			}
			voice.music.CurrentSong = nil
			go voice.timer.initTimer(time.Duration(timeout)*time.Minute, *voice)
			return nil
		}

	}
	return nil
}

func (voice *Voice) playRaw(song music.Song) (error, error) {
	logger.Log.Debug("voice.PlayRaw")
	var err error

	options := dca.StdEncodeOptions
	options.RawOutput = true
	options.Bitrate = 384
	options.Application = "lowdelay"
	options.Volume = 10

	voice.EncodingSession, err = dca.EncodeFile(song.StreamURL, options)
	if err != nil {
		logger.Log.Warningf("Could not encode file, err=%s", err)
		return nil, err
	}

	voice.done = make(chan error)
	voice.StreamingSession = dca.NewStream(voice.EncodingSession, voice.VoiceChannel, voice.done)
	msg := <-voice.done
	if err != nil && err != io.EOF {
		logger.Log.Warningf("error while playing audio, err=%s", err)
		return nil, err
	}
	voice.Playing = false
	if voice.StreamingSession != nil {
		_, err = voice.StreamingSession.Finished()
		if err != nil {
			logger.Log.Warningf("error while stopping stream session, err=%s", err)
		}
		voice.StreamingSession = nil
	}

	voice.EncodingSession.Cleanup()
	voice.EncodingSession = nil
	return msg, err
}

func (voice *Voice) Stop() error {
	voice.done <- errVoiceStoppedManually

	if err := voice.EncodingSession.Stop(); err != nil {
		return err
	}

	voice.EncodingSession.Cleanup()
	voice.Playing = false

	go voice.timer.initTimer(time.Duration(timeout)*time.Minute, *voice)

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
		embed.AddField("No songs queued", "Use !play <youtube link> to queue a song")
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
	embed.AddField("Queue have been cleared", "Use !play <youtube link> to queue a song")
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
		embed.AddField("Currently not playing", "Use !play <youtube link> to queue a song")
	}
	_, err := voice.Session.ChannelMessageSendEmbed(voice.channelID, embed.MessageEmbed)
	if err != nil {
		logger.Log.Warningf("failed to send message, err=%s", err.Error())
	}
}

func (voice *Voice) Skip() error {
	voice.done <- errVoiceSkippedManually

	if err := voice.EncodingSession.Stop(); err != nil {
		return err
	}

	return nil
}
