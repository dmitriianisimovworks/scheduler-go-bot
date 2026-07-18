# LD Calendar Bot

Telegram-бот для планирования встреч внутри команды. Сделан в рамках тестового задания (Часть 1).

Бот: [@ldcalendar_bot](https://t.me/ldcalendar_bot)

## Что умеет

- Регистрация сотрудников через Telegram.
- Создание встречи через удобный inline-интерфейс: календарная сетка, выбор времени слотами, мультивыбор участников.
- Проверка конфликтов: бот не даст назначить встречу, если время у участника занято; прошедшее время выбрать нельзя.
- Раздел «Мои встречи» с просмотром и отменой встреч прямо из списка.
- Интеграция с Google Calendar: встреча создаётся в календаре, участникам приходит ссылка на Google Meet.
- Синхронизация встреч в Google Sheets.
- Уведомления о встречах в реальном времени.
- Роль администратора: массовое назначение встреч, лимит встреч в день.

## Стек

Go, chi, telebot, PostgreSQL, Nginx, Google Calendar API, Google Sheets API, Docker Compose.

## Структура проекта

```
cmd/bot           — точка входа
cmd/oauth-setup   — утилита для получения Google OAuth refresh token
internal/telegram — хендлеры, клавиатуры, флоу бота
internal/usecase  — бизнес-логика (встречи, конфликты, участники)
internal/repository — работа с Postgres
internal/integrations — Google Calendar и Google Sheets
internal/http     — webhook-эндпоинт и healthcheck
internal/db/migrations — SQL-миграции
deploy/nginx      — конфиг Nginx
```

## Запуск

```bash
cp .env.example .env   # заполнить значения (см. ниже)
make compose-up        # docker compose up --build -d
```

Локально без Docker: `make run` (нужен запущенный Postgres и заполненный `.env`).

## Заметки для деплоя

### 1. Переменные окружения (`.env`)

| Переменная | Что указать |
|---|---|
| `APP_ENV` | `production` |
| `APP_PORT` | Порт приложения внутри контейнера (по умолчанию `8080`) |
| `APP_TIMEZONE` | Таймзона встреч, например `Europe/Moscow` |
| `APP_DOMAIN` | Домен сервера (для webhook и SSL) |
| `TELEGRAM_BOT_TOKEN` | Токен бота от @BotFather |
| `TELEGRAM_WEBHOOK_URL` | `https://<домен>/telegram/webhook` — бот работает через webhook, без него обновления приходить не будут |
| `TELEGRAM_WEBHOOK_SECRET` | Обязательно на проде: случайная строка, защищает webhook-эндпоинт от подделки апдейтов. Если пусто — эндпоинт принимает любые запросы |
| `POSTGRES_*` | Хост `postgres` (имя сервиса в compose), имя БД, пользователь, пароль |
| `GOOGLE_SHEETS_ID` | ID таблицы из её URL |
| `GOOGLE_SERVICE_ACCOUNT_JSON` | JSON сервисного аккаунта (таблицу нужно расшарить на его email) |
| `GOOGLE_OAUTH_CLIENT_ID` / `GOOGLE_OAUTH_CLIENT_SECRET` | OAuth-клиент из Google Cloud Console (для Calendar + Meet) |
| `GOOGLE_OAUTH_REFRESH_TOKEN` | Получается один раз утилитой `go run ./cmd/oauth-setup` |
| `GOOGLE_CALENDAR_ID` | ID календаря (обычно email аккаунта или `primary`) |
| `ADMIN_TELEGRAM_IDS` | Telegram ID администраторов через запятую |

### 2. Google API

- В Google Cloud Console включить **Google Calendar API** и **Google Sheets API**.
- Для Sheets — создать сервисный аккаунт и дать ему доступ к таблице.
- Для Calendar — создать OAuth-клиент, затем получить refresh token через `cmd/oauth-setup`.

### 3. База данных

Миграции лежат в `internal/db/migrations` (формат golang-migrate). Применить к базе перед первым запуском:

```bash
migrate -path internal/db/migrations -database "$POSTGRES_URL" up
```

### 4. SSL и Nginx

Telegram принимает webhook только по HTTPS. На сервере нужен сертификат Let's Encrypt (`certbot certonly`), compose монтирует `/etc/letsencrypt` в контейнер Nginx. Конфиг — `deploy/nginx/nginx.conf`, в нём указать свой домен.

### 5. Проверка после запуска

- `curl https://<домен>/healthz` — сервис жив.
- Написать боту `/start` — должен ответить и предложить регистрацию.
