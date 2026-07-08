package http

import (
	"context"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"

	"meeting-bot/internal/config"
	"meeting-bot/internal/platform/logger"
)

type Server struct {
	config config.Config
	logger *logger.Logger
	http   *http.Server
}

func NewServer(cfg config.Config, log *logger.Logger) *Server {
	router := chi.NewRouter()
	registerHealthRoutes(router)
	registerWebhookRoutes(router)

	return &Server{
		config: cfg,
		logger: log,
		http: &http.Server{
			Addr:    net.JoinHostPort("", cfg.AppPort),
			Handler: router,
		},
	}
}

func (s *Server) Start() error {
	s.logger.Printf("http server starting on :%s", s.config.AppPort)
	err := s.http.ListenAndServe()
	if err == nil || err == http.ErrServerClosed {
		return nil
	}

	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Printf("http server shutting down")
	return s.http.Shutdown(ctx)
}
