package llm

import (
	"context"

	"github.com/1azar/cogito/schema"
)

type Completion struct {
	Text      string
	ToolCalls []schema.ToolCall
}

type LLM interface {
	Generate(ctx context.Context, msgs []schema.Message, tools []map[string]any) (*Completion, error)
}
