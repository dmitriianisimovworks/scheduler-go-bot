package config

import "os"

type Config struct {
	AppEnv                string
	AppPort               string
	AppTimezone           string
	AppDomain             string
	TelegramBotToken      string
	TelegramWebhookURL    string
	TelegramWebhookSecret string
	PostgresHost          string
	PostgresPort          string
	PostgresDB            string
	PostgresUser          string
	PostgresPassword      string
	GoogleSheetsID        string
	GoogleServiceJSON     string
}

func Load() Config {
	return Config{
		AppEnv:                getenv("APP_ENV", "development"),
		AppPort:               getenv("APP_PORT", "8080"),
		AppTimezone:           getenv("APP_TIMEZONE", "Europe/Moscow"),
		AppDomain:             getenv("APP_DOMAIN", ""),
		TelegramBotToken:      getenv("TELEGRAM_BOT_TOKEN", ""),
		TelegramWebhookURL:    getenv("TELEGRAM_WEBHOOK_URL", ""),
		TelegramWebhookSecret: getenv("TELEGRAM_WEBHOOK_SECRET", ""),
		PostgresHost:          getenv("POSTGRES_HOST", "postgres"),
		PostgresPort:          getenv("POSTGRES_PORT", "5432"),
		PostgresDB:            getenv("POSTGRES_DB", "meeting_bot"),
		PostgresUser:          getenv("POSTGRES_USER", "meeting_bot"),
		PostgresPassword:      getenv("POSTGRES_PASSWORD", ""),
		GoogleSheetsID:        getenv("GOOGLE_SHEETS_ID", ""),
		GoogleServiceJSON:     getenv("GOOGLE_SERVICE_ACCOUNT_JSON", ""),
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
