package surbot

import (
	"time"

	"gitlab.com/sajfer/surbot/internal/logger"
)

type Timer struct {
	timer   time.Timer
	running bool
	stop    chan bool
}

func (timer *Timer) initTimer(timeout time.Duration, voice Voice) {
	logger.Log.Debug("timer.initTimer")
	timer.running = true
	timer.timer = *time.NewTimer(time.Duration(timeout))
	select {
	case <-timer.timer.C:
		logger.Log.Debug("Idle timeout, leaving channel")
		err := voice.Disconnect()
		if err != nil {
			logger.Log.Warningf("Could not disconnect from voice channel, err=%v", err)
		}
	case <-timer.stop:
		logger.Log.Debug("Timer cancelled")
	}
}

func (timer *Timer) stopTimer() {
	logger.Log.Debug("Stopping idle timer")
	select {
	case timer.stop <- true:
	default:
	}
	timer.timer.Stop()
	timer.running = false
}
