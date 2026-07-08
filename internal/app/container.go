package app

import (
	"context"

	"meeting-bot/internal/config"
	apphttp "meeting-bot/internal/http"
	"meeting-bot/internal/integrations/sheets"
	"meeting-bot/internal/platform/clock"
	"meeting-bot/internal/platform/logger"
	"meeting-bot/internal/repository/postgres"
	"meeting-bot/internal/telegram"
)

type HTTPServer interface {
	Start() error
	Shutdown(ctx context.Context) error
}

type TelegramRunner interface {
	Run(ctx context.Context) error
}

func buildContainer() (*App, error) {
	cfg := config.Load()
	log := logger.New(cfg.AppEnv)
	clk := clock.New()

	repo := postgres.New(cfg)
	sheetsClient := sheets.New(cfg)
	httpServer := apphttp.NewServer(cfg, log)
	tg := telegram.New(cfg, log, clk, repo, sheetsClient)

	return &App{
		httpServer: httpServer,
		telegram:   tg,
	}, nil
}
