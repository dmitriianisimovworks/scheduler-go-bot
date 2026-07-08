package main

import (
	"log"

	"meeting-bot/internal/app"
)

func main() {
	a, err := app.New()
	if err != nil {
		log.Fatalf("bootstrap app: %v", err)
	}

	if err := a.Run(); err != nil {
		log.Fatalf("run app: %v", err)
	}
}
