package http

import (
	"net/http"

	"meeting-bot/internal/config"
	"meeting-bot/internal/platform/logger"
)

type Server struct {
	config config.Config
	logger *logger.Logger
}

func NewServer(cfg config.Config, log *logger.Logger) *Server {
	return &Server{
		config: cfg,
		logger: log,
	}
}

func (s *Server) Start() error {
	return http.ListenAndServe(":"+s.config.AppPort, http.NewServeMux())
}
