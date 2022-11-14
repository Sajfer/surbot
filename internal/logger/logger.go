package logger

import (
	"github.com/sirupsen/logrus"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
)

var Log = setupLogger()

func setupLogger() *logrus.Logger {

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	prefix_formatter := &prefixed.TextFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
		FullTimestamp:   true,
		ForceFormatting: true,
	}

	prefix_formatter.SetColorScheme(&prefixed.ColorScheme{
		PrefixStyle:    "blue+b",
		TimestampStyle: "white",
	})

	logger.Formatter = prefix_formatter
	return logger
}
