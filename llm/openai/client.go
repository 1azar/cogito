package openai

import (
	"errors"
	"net/http"
	"time"
)

const defaultBaseURL = "https://api.openai.com/v1"

type Client struct {
	cfg    Config
	client *http.Client
}

func New(cfg Config) (*Client, error) {

	if cfg.APIKey == "" {
		return nil, errors.New("openai: api key required")
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}

	if cfg.Model == "" {
		cfg.Model = "gpt-4o-mini"
	}

	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}

	return &Client{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}, nil
}
