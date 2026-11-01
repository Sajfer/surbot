// Package main provides the main functionality for Surbot.
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"

	"github.com/sajfer/surbot/internal/config"
	"github.com/sajfer/surbot/internal/core"
	"github.com/sajfer/surbot/internal/logger"
	"github.com/sajfer/surbot/internal/plugins"
)

func main() {
	os.Exit(run())
}

func run() int {
	// SUR_DEBUG turns on debug logs, for surbot and for discordgo's voice code.
	if os.Getenv("SUR_DEBUG") != "" {
		logger.Log.SetLevel(logrus.DebugLevel)
	}
	prefix := flag.String("p", "!", "Bot Prefix")
	flag.Parse()

	cfg, err := config.Load(".env")
	if err != nil {
		logger.Log.WithError(err).Error("could not load configuration")
		return 1
	}

	logger.Log.Infof("Starting Surbot with prefix: %s", *prefix)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Once shutdown starts, restore default signal handling so a second
	// Ctrl-C force-exits instead of waiting out the drain.
	go func() {
		<-ctx.Done()
		stop()
	}()

	bot, err := core.New(cfg, *prefix, plugins.Catalog())
	if err != nil {
		logger.Log.WithError(err).Error("could not create bot")
		return 1
	}
	if err := bot.Run(ctx); err != nil {
		logger.Log.WithError(err).Error("bot stopped with an error")
		return 1
	}
	return 0
}
