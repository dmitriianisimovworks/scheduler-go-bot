.PHONY: run build compose-up compose-down

run:
	go run ./cmd/bot

build:
	go build ./cmd/bot

compose-up:
	docker compose up --build -d

compose-down:
	docker compose down
