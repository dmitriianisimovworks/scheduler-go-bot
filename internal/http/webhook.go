package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func registerWebhookRoutes(router chi.Router) {
	router.Post("/telegram/webhook", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}
