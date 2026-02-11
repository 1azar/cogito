package openai

import "time"

type Config struct {
	APIKey  string
	BaseURL string

	Model string

	Timeout time.Duration
}
