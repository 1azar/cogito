package ollama

import (
	"errors"
	"net/http"
	"time"
)

const defaultBaseURL = "http://localhost:11434/v1"

type Client struct {
	cfg    Config
	client *http.Client
}

func New(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}

	if cfg.TopP == 0 {
		cfg.TopP = 0.9
	}

	if cfg.RepeatPenalty == 0 {
		cfg.RepeatPenalty = 1.1
	}

	if cfg.Model == "" {
		return nil, errors.New("ollama: model required")
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
