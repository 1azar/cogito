package controller

import (
	"context"

	"github.com/1azar/cogito/llm"
	"github.com/1azar/cogito/schema"
	"github.com/1azar/cogito/tool"
	"github.com/1azar/cogito/toolruntime"
)

type Completion struct {
	Text      string
	ToolCalls []schema.ToolCall
	Usage     llm.Usage
}

type AgentLike[T any] interface {
	CallLLM(ctx context.Context, input string) (string, error)
	CallLLMWithTools(ctx context.Context, input string) (*Completion, error)
	State() *T
	Tools() *tool.Registry
	RunToolCalls(ctx context.Context, calls []schema.ToolCall) ([]toolruntime.Result, error)
}

type Controller[T any] interface {
	Run(ctx context.Context, agent AgentLike[T], input string) (string, error)
}
