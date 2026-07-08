package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func registerHealthRoutes(router chi.Router) {
	router.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}
