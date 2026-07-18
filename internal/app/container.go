package app

import (
	"context"
	"fmt"

	"meeting-bot/internal/config"
	apphttp "meeting-bot/internal/http"
	"meeting-bot/internal/integrations/calendar"
	"meeting-bot/internal/integrations/sheets"
	"meeting-bot/internal/notifier"
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

	// The bot receives updates only via webhook, so an empty secret leaves the
	// public /telegram/webhook endpoint unauthenticated. Refuse to boot rather
	// than run in that state.
	if cfg.TelegramWebhookSecret == "" {
		return nil, fmt.Errorf("TELEGRAM_WEBHOOK_SECRET is required: without it the webhook endpoint is unauthenticated")
	}

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
	notif := notifier.New(repo, repo, clk, tg, log)

	return &App{
		httpServer: httpServer,
		telegram:   tg,
		notifier:   notif,
	}, nil
}
