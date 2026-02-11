package controller

import (
	"context"
)

type AgentLike[T any] interface {
	CallLLM(ctx context.Context, input string) (string, error)
	State() *T
}

type Controller[T any] interface {
	Run(ctx context.Context, agent AgentLike[T], input string) (string, error)
}
