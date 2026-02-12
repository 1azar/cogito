package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/1azar/cogito/controller"
	"github.com/1azar/cogito/llm"
	"github.com/1azar/cogito/memory"
	"github.com/1azar/cogito/schema"
	"github.com/1azar/cogito/tool"
)

type Agent[T any] struct {
	llm        llm.LLM
	memory     memory.Memory
	controller controller.Controller[T]
	tools      *tool.Registry

	state T
}

func (a *Agent[T]) Run(ctx context.Context, input string) (string, error) {
	if a.controller == nil {
		return "", errors.New("controller is nil")
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

	// Collect tool schemas (empty for simple LLM calls, but the interface expects it)
	var tools []map[string]any

	resp, err := a.llm.Generate(ctx, msgs, tools)
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

func (a *Agent[T]) ExecuteTool(ctx context.Context, name string, arguments string) (string, error) {
	t, ok := a.tools.Get(name)
	if !ok {
		return "", fmt.Errorf("tool %s not found", name)
	}

	result, err := t.Call(ctx, json.RawMessage(arguments))
	if err != nil {
		return "", err
	}

	output, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal tool result: %w", err)
	}

	return string(output), nil
}

// AddToolMessage adds a tool response message to memory
func (a *Agent[T]) AddToolMessage(ctx context.Context, toolCallID string, content string) {
	if a.memory != nil {
		_ = a.memory.Add(ctx, schema.Message{
			Role:       schema.RoleTool,
			ToolCallID: toolCallID,
			Content:    content,
		})
	}
}

func (a *Agent[T]) CallLLMWithTools(ctx context.Context, input string) (*controller.Completion, error) {
	var msgs []schema.Message

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
		_ = a.memory.Add(ctx, schema.Message{
			Role:    schema.RoleUser,
			Content: input,
		})
	}

	// Collect tool schemas
	var tools []map[string]any
	schemas := a.tools.Schemas()
	for _, tschema := range schemas {
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tschema.Name,
				"description": tschema.Description,
				"parameters":  tschema.Parameters,
			},
		})
	}

	resp, err := a.llm.Generate(ctx, msgs, tools)
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
