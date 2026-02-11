package simple

import (
	"context"

	"github.com/1azar/cogito/controller"
)

type Simple[T any] struct{}

func New[T any]() *Simple[T] {
	return &Simple[T]{}
}

func (s *Simple[T]) Run(ctx context.Context, agent controller.AgentLike[T], input string) (string, error) {
	return agent.CallLLM(ctx, input)
}

var _ controller.Controller[any] = (*Simple[any])(nil)
