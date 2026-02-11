package llm

import (
	"context"

	"github.com/1azar/cogito/schema"
)

type Completion struct {
	Text string
}

type LLM interface {
	Generate(ctx context.Context, msgs []schema.Message) (*Completion, error)
}
