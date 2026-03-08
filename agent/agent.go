package agent

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/1azar/cogito/controller"
	"github.com/1azar/cogito/llm"
	"github.com/1azar/cogito/memory"
	"github.com/1azar/cogito/schema"
	"github.com/1azar/cogito/tool"
	"github.com/1azar/cogito/toolruntime"
)

type Agent[T any] struct {
	llm        llm.LLM
	memory     memory.Memory
	controller controller.Controller[T]
	tools      *tool.Registry
	executor   toolruntime.Executor
	promptFunc PromptFunc[T]

	state T
}

// PromptFunc is a function that dynamically generates a system prompt based on context and state
type PromptFunc[T any] func(ctx context.Context, state *T) string

func (a *Agent[T]) Run(ctx context.Context, input string) (string, error) {
	if a.controller == nil {
		return "", errors.New("controller is nil")
	}

	return a.controller.Run(ctx, a, input)
}

func (a *Agent[T]) CallLLM(ctx context.Context, input string) (string, error) {
	var msgs []schema.Message

	// Add system prompt if configured
	if a.promptFunc != nil {
		systemPrompt := a.promptFunc(ctx, &a.state)
		if systemPrompt != "" {
			msgs = append(msgs, schema.Message{
				Role:    schema.RoleSystem,
				Content: systemPrompt,
			})
		}
	}

	if a.memory != nil {
		history, _ := a.memory.Get(ctx)
		msgs = append(msgs, history...)
	}

	msgs = append(msgs, schema.Message{
		Role:    schema.RoleUser,
		Content: input,
	})

	resp, err := a.llm.Generate(ctx, llm.Request{
		Messages: msgs,
		Params:   llm.Params{},
	})
	if err != nil {
		return "", err
	}

	if a.memory != nil {
		msg := schema.Message{
			Role:    schema.RoleAssistant,
			Content: resp.Text,
		}
		if len(resp.ToolCalls) > 0 {
			msg.ToolCalls = resp.ToolCalls
		}
		_ = a.memory.Add(ctx, msg)
	}

	return resp.Text, nil
}

func (a *Agent[T]) State() *T {
	return &a.state
}

func (a *Agent[T]) Tools() *tool.Registry {
	return a.tools
}

func (a *Agent[T]) RunToolCalls(ctx context.Context, calls []schema.ToolCall) ([]toolruntime.Result, error) {
	if a.executor == nil {
		return nil, errors.New("tool executor is nil")
	}

	results, err := a.executor.Execute(ctx, calls, a.tools)
	if a.memory != nil {
		for _, result := range results {
			payload := mustMarshalToolResult(result)
			_ = a.memory.Add(ctx, schema.Message{
				Role:       schema.RoleTool,
				ToolCallID: result.ToolCallID,
				Content:    payload,
			})
		}
	}

	return results, err
}

func (a *Agent[T]) CallLLMWithTools(ctx context.Context, input string) (*controller.Completion, error) {
	var msgs []schema.Message

	// Add system prompt if configured
	if a.promptFunc != nil {
		systemPrompt := a.promptFunc(ctx, &a.state)
		if systemPrompt != "" {
			msgs = append(msgs, schema.Message{
				Role:    schema.RoleSystem,
				Content: systemPrompt,
			})
		}
	}

	if a.memory != nil {
		history, _ := a.memory.Get(ctx)
		msgs = append(msgs, history...)
	}

	// Only add user message if input is not empty (first iteration)
	if input != "" {
		msgs = append(msgs, schema.Message{
			Role:    schema.RoleUser,
			Content: input,
		})
		// Also add to memory
		if a.memory != nil {
			_ = a.memory.Add(ctx, schema.Message{
				Role:    schema.RoleUser,
				Content: input,
			})
		}
	}

	resp, err := a.llm.Generate(ctx, llm.Request{
		Messages: msgs,
		Tools:    a.tools.Specs(),
		ToolChoice: llm.ToolChoice{
			Mode: llm.ToolChoiceAuto,
		},
		Params: llm.Params{},
	})
	if err != nil {
		return nil, err
	}

	if a.memory != nil {
		msg := schema.Message{
			Role:    schema.RoleAssistant,
			Content: resp.Text,
		}
		if len(resp.ToolCalls) > 0 {
			msg.ToolCalls = resp.ToolCalls
		}
		_ = a.memory.Add(ctx, msg)
	}

	return &controller.Completion{
		Text:      resp.Text,
		ToolCalls: resp.ToolCalls,
	}, nil
}

// ClearMemory clears the agent's conversation history
func (a *Agent[T]) ClearMemory() error {
	if a.memory != nil {
		return a.memory.Clear()
	}
	return nil
}

func mustMarshalToolResult(result toolruntime.Result) string {
	payload, err := json.Marshal(result)
	if err != nil {
		return `{"status":"error","error":{"code":"marshal_error","message":"failed to serialize tool result"}}`
	}
	return string(payload)
}
