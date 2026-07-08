package telegram

import (
	"context"

	"meeting-bot/internal/config"
	"meeting-bot/internal/integrations/sheets"
	"meeting-bot/internal/platform/clock"
	"meeting-bot/internal/platform/logger"
	"meeting-bot/internal/repository/postgres"
)

type Bot struct {
	config config.Config
}

func New(
	cfg config.Config,
	_ *logger.Logger,
	_ clock.Clock,
	_ *postgres.Repository,
	_ *sheets.Client,
) *Bot {
	return &Bot{config: cfg}
}

func (b *Bot) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}
