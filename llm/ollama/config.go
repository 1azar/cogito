package ollama

import "time"

type Config struct {
	APIKey  string
	BaseURL string

	Model string

	Temperature   float64
	TopP          float64
	RepeatPenalty float64

	Timeout time.Duration
}
