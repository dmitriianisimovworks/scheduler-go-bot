package app

import (
	"context"
	"os/signal"
	"syscall"
	"time"
)

type App struct {
	httpServer HTTPServer
	telegram   TelegramRunner
}

func New() (*App, error) {
	return buildContainer()
}

func (a *App) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 2)

	if a.telegram != nil {
		go func() {
			errCh <- a.telegram.Run(ctx)
		}()
	}

	if a.httpServer != nil {
		go func() {
			errCh <- a.httpServer.Start()
		}()
	}

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if a.httpServer != nil {
			return a.httpServer.Shutdown(shutdownCtx)
		}

		return nil
	case err := <-errCh:
		return err
	}
}
