package music

import (
	"math/rand"
	"time"
)

type Song struct {
	Title     string
	Artist    string
	Duration  float64
	Thumbnail string
	ID        string
	StreamURL string
}

type Playlist struct {
	Title    string
	Uploader string
	Songs    []*Song
}

type Music struct {
	CurrentSong *Song
	Queue       []*Song
}

func NewMusic() *Music {
	music := &Music{}
	return music
}

func (m *Music) AddToQueue(playlist Playlist) error {
	m.Queue = append(m.Queue, playlist.Songs...)
	return nil
}

func (m *Music) Shuffle() {
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(m.Queue), func(i, j int) { m.Queue[i], m.Queue[j] = m.Queue[j], m.Queue[i] })
}
