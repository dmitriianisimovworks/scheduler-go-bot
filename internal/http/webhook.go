package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	tele "gopkg.in/telebot.v3"

	"meeting-bot/internal/config"
	"meeting-bot/internal/platform/logger"
)

func registerWebhookRoutes(router chi.Router, cfg config.Config, log *logger.Logger, telegram TelegramProcessor) {
	router.Post("/telegram/webhook", func(w http.ResponseWriter, r *http.Request) {
		// Fail closed: an unset secret means the endpoint accepts nothing,
		// never everything. Without this the handler would trust any POST and
		// let a forged update spoof any telegram_id — including an admin's.
		if cfg.TelegramWebhookSecret == "" || r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != cfg.TelegramWebhookSecret {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		var update tele.Update
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			log.Printf("decode telegram update: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		telegram.ProcessUpdate(update)
		w.WriteHeader(http.StatusOK)
	})
}
