package app

import (
	"context"

	"meeting-bot/internal/config"
	apphttp "meeting-bot/internal/http"
	"meeting-bot/internal/integrations/calendar"
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
	ctx := context.Background()

	repo, err := postgres.New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	sheetsClient := sheets.New(cfg)
	calendarClient := calendar.New(cfg)
	tg := telegram.New(cfg, log, clk, repo, repo, repo, sheetsClient, calendarClient)
	httpServer := apphttp.NewServer(cfg, log, tg)

	return &App{
		httpServer: httpServer,
		telegram:   tg,
	}, nil
}
