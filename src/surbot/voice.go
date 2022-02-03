package surbot

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/sajfer/dca"
	"gitlab.com/sajfer/surbot/src/logger"
	spotifyClient "gitlab.com/sajfer/surbot/src/spotify"
	"gitlab.com/sajfer/surbot/src/utils"
	"gitlab.com/sajfer/surbot/src/youtube"
)

type Metadata struct {
	Title     string
	Duration  float64
	Thumbnail string
	ID        string
}

type Song struct {
	Metadata  *Metadata
	StreamURL string
}

type Voice struct {
	VoiceChannel     *discordgo.VoiceConnection
	EncodingSession  *dca.EncodeSession
	StreamingSession *dca.StreamingSession
	Session          *discordgo.Session
	Playing          bool
	CurrentSong      *Song
	Queue            []*Song
	ChannelID        string
	done             chan error
	timer            *Timer
}

var (
	timeout = 5
)

func NewVoice() *Voice {
	return &Voice{timer: &Timer{stop: make(chan bool), running: false}}
}

func (voice *Voice) SetTextChannel(channel string) {
	voice.ChannelID = channel
}

func (voice *Voice) SetSession(session *discordgo.Session) {
	voice.Session = session
}

func (voice *Voice) Connect(userId, guildId string, mute, deaf bool) error {
	logger.Log.Debug("voice.Connect")
	guild, err := voice.Session.State.Guild(guildId)
	var channelId = ""
	if err != nil {
		return err
	}
	for _, person := range guild.VoiceStates {
		if person.UserID == userId {
			logger.Log.Debugf("Voice channel: %s", person.ChannelID)
			channelId = person.ChannelID
			break
		}
	}
	if channelId == "" {
		return errors.New("user not in a channel")
	}
	vc, err := voice.Session.ChannelVoiceJoin(guildId, channelId, mute, deaf)
	if err != nil {
		return err
	}
	voice.VoiceChannel = vc
	return nil
}

func (voice *Voice) Disconnect() error {
	logger.Log.Debug("voice.Disconnect")

	if voice.Playing {
		voice.done <- errVoiceStoppedManually
	}
	voice.Playing = false

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

func (voice *Voice) PlayVideo(url string, m *discordgo.MessageCreate) error {
	info, err := youtube.GetVideoInfo(url)
	if err != nil {
		logger.Log.Warningf("could not fetch video information for %s, err= %s", url, err)
		return err
	}
	if info.Playlist != nil {
		err := voice.AddPlaylistToQueue(*info.Playlist)
		if err != nil {
			logger.Log.Warningf("could not add playlist to queue, err=%s", err)
			return err
		}
	} else {
		err := voice.AddSongToQueue(*info.Video)
		if err != nil {
			logger.Log.Warningf("could not add song to queue, err=%s", err)
			return err
		}
	}

	if !voice.Playing {
		err = voice.Connect(m.Author.ID, m.GuildID, false, true)
		if err != nil {
			logger.Log.Warningf("could not join voice channel, err=%s", err)
			return err
		}
		err = voice.Play()
		if err != nil {
			logger.Log.Warningf("could not play song, err=%s", err)
			return err
		}
	}
	return nil
}

func (voice *Voice) Play() error {
	logger.Log.Debug("voice.Play")

	if !voice.Playing {
		voice.Playing = true

		song := voice.Queue[0]
		voice.Queue = voice.Queue[1:]
		err := voice.Session.UpdateListeningStatus(song.Metadata.Title)
		if err != nil {
			return err
		}

		if voice.timer.running {
			voice.timer.stopTimer()
		}
		// voice.resetTimer(time.Duration(int(song.Metadata.Duration))*time.Second + time.Duration(timeout)*time.Minute)

		voice.CurrentSong = song
		voice.NowPlaying()
		msg, err := voice.playRaw(*song)
		if msg != nil {
			if msg == errVoiceStoppedManually {
				voice.Playing = false
				err := voice.Session.UpdateListeningStatus("")
				voice.CurrentSong = nil
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
					voice.CurrentSong = nil
					go voice.timer.initTimer(time.Duration(timeout)*time.Minute, *voice)
					return err
				}
			default:
				return err
			}
		}
		if len(voice.Queue) > 0 {
			return voice.Play()
		} else {
			err := voice.Session.UpdateListeningStatus("")
			if err != nil {
				return err
			}
			voice.CurrentSong = nil
			go voice.timer.initTimer(time.Duration(timeout)*time.Minute, *voice)
			return nil
		}

	}
	return nil
}

func (voice *Voice) playRaw(song Song) (error, error) {
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
	if voice.CurrentSong != nil {
		songList = songList + fmt.Sprintf("%d. %s\n", index, voice.CurrentSong.Metadata.Title)
		index = index + 1
	} else {
		embed.AddField("No songs queued", "Use !play <youtube link> to queue a song")
	}
	for i, song := range voice.Queue {
		if i > 18 {
			songList = songList + "-- Only showing the first 20 songs --\n"
			break
		}
		songList = songList + fmt.Sprintf("%d. %s\n", i+index, song.Metadata.Title)
	}
	if songList != "" {
		embed.AddField("---", songList)
	}
	_, err := voice.Session.ChannelMessageSendEmbed(voice.ChannelID, embed.MessageEmbed)
	if err != nil {
		return err
	}
	return nil
}

func (voice *Voice) ClearQueue() error {
	voice.Queue = nil
	embed := NewEmbed()
	embed.SetTitle("Queue")
	embed.AddField("Queue have been cleared", "Use !play <youtube link> to queue a song")
	_, err := voice.Session.ChannelMessageSendEmbed(voice.ChannelID, embed.MessageEmbed)
	if err != nil {
		return err
	}
	return nil
}

func (voice *Voice) NowPlaying() {
	embed := NewEmbed()
	if voice.CurrentSong != nil {
		embed.AddField("Now playing", voice.CurrentSong.Metadata.Title)
		embed.AddField("Duration", utils.SecondsToHuman(voice.CurrentSong.Metadata.Duration))
		embed.SetThumbnail(voice.CurrentSong.Metadata.Thumbnail)
	} else {
		embed.AddField("Currently not playing", "Use !play <youtube link> to queue a song")
	}
	_, err := voice.Session.ChannelMessageSendEmbed(voice.ChannelID, embed.MessageEmbed)
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

func (voice *Voice) AddSongToQueue(video youtube.Video) error {
	logger.Log.Debug("voice.AddSongToQueue")

	err := voice.addItemToQueue(video)
	if err != nil {
		return err
	}

	return nil
}

func (voice *Voice) addItemToQueue(video youtube.Video) error {
	logger.Log.Debug("voice.addItemToQueue")
	metadata := Metadata{Title: video.Title, Duration: video.Duration, Thumbnail: video.Thumbnail, ID: video.ID}
	song := Song{Metadata: &metadata, StreamURL: video.Formats[0].URL}
	voice.Queue = append(voice.Queue, &song)
	return nil
}

func (voice *Voice) AddPlaylistToQueue(playlist youtube.Playlist) error {
	logger.Log.Debug("voice.AddPlaylistToQueue")

	embed := NewEmbed()
	embed.AddField("Playlist added to queue", fmt.Sprintf("%s by %s", playlist.Title, playlist.Uploader))
	_, err := voice.Session.ChannelMessageSendEmbed(voice.ChannelID, embed.MessageEmbed)
	if err != nil {
		return err
	}
	firstVideo, err := youtube.GetVideoInfo(playlist.Entries[0].ID)
	if err != nil {
		return err
	}
	err = voice.addItemToQueue(*firstVideo.Video)
	if err != nil {
		return err
	}
	go voice.addPlaylistItemsToQueue(playlist)
	return nil
}

func (voice *Voice) AddSpotifyPlaylist(playlist spotifyClient.Playlist) error {
	logger.Log.Debug("voice.AddSpotifyPlaylist")
	embed := NewEmbed()
	if len(playlist.Songs) > 1 && playlist.Title != "" {
		embed.AddField("Playlist added to queue", fmt.Sprintf("%s by %s", playlist.Title, playlist.Uploader))
		_, err := voice.Session.ChannelMessageSendEmbed(voice.ChannelID, embed.MessageEmbed)
		if err != nil {
			return err
		}
	}

	url := yt.SearchVideo(fmt.Sprintf("%s - %s", playlist.Songs[0].Artist, playlist.Songs[0].Name))
	info, err := youtube.GetVideoInfo(url.Path)
	if err != nil {
		logger.Log.Warningf("could not fetch video information for %s, err= %s", url, err)
	}
	err = voice.addItemToQueue(*info.Video)
	if err != nil {
		logger.Log.Warningf("Could not add song to play queue, err=%v", err)
		return err
	}

	go func() {
		for _, song := range playlist.Songs[1:] {
			url := yt.SearchVideo(fmt.Sprintf("%s - %s", song.Artist, song.Name))
			info, err := youtube.GetVideoInfo(url.Path)
			if err != nil {
				logger.Log.Warningf("could not fetch video information for %s, err= %s", url, err)
				continue
			}
			err = voice.addItemToQueue(*info.Video)
			if err != nil {
				logger.Log.Warningf("Could not add song to play queue, err=%v", err)
				continue
			}
		}
	}()
	return nil
}

func (voice *Voice) addPlaylistItemsToQueue(playlist youtube.Playlist) {
	for _, video := range playlist.Entries[1:] {
		vid, err := youtube.GetVideoInfo(video.ID)
		if err != nil {
			continue
		}
		err = voice.addItemToQueue(*vid.Video)
		if err != nil {
			logger.Log.Warningf("Could not add song to queue, err=%s", err)
		}
	}
}
