package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/kawaewanko/mogkoe/internal/config"
	"github.com/kawaewanko/mogkoe/internal/discord"
	"github.com/kawaewanko/mogkoe/internal/voicevox"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{}))
	cfg := config.Load()
	engine := voicevox.New(cfg.VoicevoxURL, cfg.CacheDir, cfg.AudioCacheTTL, logger)
	manager := discord.NewManager(cfg, engine, logger)
	if err := manager.Run(context.Background()); err != nil {
		logger.Error("mogkoe stopped", "error", err)
		os.Exit(1)
	}
}
