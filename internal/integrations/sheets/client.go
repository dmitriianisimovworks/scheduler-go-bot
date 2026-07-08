package sheets

import "meeting-bot/internal/config"

type Client struct {
	config config.Config
}

func New(cfg config.Config) *Client {
	return &Client{config: cfg}
}
