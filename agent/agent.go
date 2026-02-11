package agent

import (
	"context"
	"errors"

	"github.com/1azar/cogito/controller"
	"github.com/1azar/cogito/llm"
	"github.com/1azar/cogito/memory"
	"github.com/1azar/cogito/schema"
)

type Agent[T any] struct {
	llm        llm.LLM
	memory     memory.Memory
	controller controller.Controller[T]

	state T
}

func (a *Agent[T]) Run(ctx context.Context, input string) (string, error) {
	if a.controller == nil {
		return "", errors.New("controller is nil")
	}

	if a.memory != nil {
		_ = a.memory.Add(ctx, schema.Message{
			Role:    schema.RoleUser,
			Content: input,
		})
	}

	return a.controller.Run(ctx, a, input)
}

func (a *Agent[T]) CallLLM(ctx context.Context, input string) (string, error) {
	var msgs []schema.Message

	if a.memory != nil {
		history, _ := a.memory.Get(ctx)
		msgs = append(msgs, history...)
	}

	msgs = append(msgs, schema.Message{
		Role:    schema.RoleUser,
		Content: input,
	})

	resp, err := a.llm.Generate(ctx, msgs)
	if err != nil {
		return "", err
	}

	if a.memory != nil {
		_ = a.memory.Add(ctx, schema.Message{
			Role:    schema.RoleAssistant,
			Content: resp.Text,
		})
	}

	return resp.Text, nil
}

func (a *Agent[T]) State() *T {
	return &a.state
}
