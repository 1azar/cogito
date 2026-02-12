package controller

import (
	"context"

	"github.com/1azar/cogito/schema"
	"github.com/1azar/cogito/tool"
)

type Completion struct {
	Text      string
	ToolCalls []schema.ToolCall
}

type AgentLike[T any] interface {
	CallLLM(ctx context.Context, input string) (string, error)
	CallLLMWithTools(ctx context.Context, input string) (*Completion, error)
	State() *T
	Tools() *tool.Registry
	ExecuteTool(ctx context.Context, name string, arguments string) (string, error)
	AddToolMessage(ctx context.Context, toolCallID string, content string)
}

type Controller[T any] interface {
	Run(ctx context.Context, agent AgentLike[T], input string) (string, error)
}
