package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/1azar/cogito/controller"
	"github.com/1azar/cogito/llm"
	"github.com/1azar/cogito/memory"
	cogruntime "github.com/1azar/cogito/runtime"
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
	runManager *cogruntime.Manager
	eventBus   cogruntime.EventBus
	sessionID  string

	state T
}

// PromptFunc is a function that dynamically generates a system prompt based on context and state
type PromptFunc[T any] func(ctx context.Context, state *T) string

func (a *Agent[T]) Run(ctx context.Context, input string) (string, error) {
	if a.controller == nil {
		return "", errors.New("controller is nil")
	}

	if a.runManager == nil {
		return a.controller.Run(ctx, a, input)
	}

	runCtx, run := a.runManager.Start(ctx, a.sessionID, input)
	a.publishEvent(runCtx, cogruntime.Event{
		Timestamp: time.Now(),
		Type:      cogruntime.EventRunStarted,
		RunID:     run.ID(),
		SessionID: a.sessionID,
		Component: "agent",
	})

	out, err := a.controller.Run(runCtx, a, input)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(runCtx.Err(), context.Canceled) || errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			_ = a.runManager.Cancel(run.ID(), err)
			a.publishEvent(runCtx, cogruntime.Event{
				Timestamp: time.Now(),
				Type:      cogruntime.EventRunCanceled,
				RunID:     run.ID(),
				SessionID: a.sessionID,
				Component: "agent",
				Err:       err,
			})
			return "", err
		}

		_ = a.runManager.Fail(run.ID(), err)
		a.publishEvent(runCtx, cogruntime.Event{
			Timestamp: time.Now(),
			Type:      cogruntime.EventRunFailed,
			RunID:     run.ID(),
			SessionID: a.sessionID,
			Component: "agent",
			Err:       err,
		})
		return "", err
	}

	_ = a.runManager.Complete(run.ID())
	snapshot := run.Snapshot()
	a.publishEvent(runCtx, cogruntime.Event{
		Timestamp: time.Now(),
		Type:      cogruntime.EventRunFinished,
		RunID:     run.ID(),
		SessionID: a.sessionID,
		Component: "agent",
		Duration:  snapshot.Duration,
	})

	return out, nil

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

	msgs = appendUserMessage(msgs, input, true)
	a.addUserToMemory(ctx, input)

	resp, err := a.llm.Generate(ctx, llm.Request{
		Messages: msgs,
		Params:   llm.ParamsFromContext(ctx),
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

func (a *Agent[T]) RunManager() *cogruntime.Manager {
	return a.runManager
}

func (a *Agent[T]) EventBus() cogruntime.EventBus {
	return a.eventBus
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

	msgs = appendUserMessage(msgs, input, false)
	a.addUserToMemory(ctx, input)

	resp, err := a.llm.Generate(ctx, llm.Request{
		Messages: msgs,
		Tools:    a.tools.Specs(),
		ToolChoice: llm.ToolChoice{
			Mode: llm.ToolChoiceAuto,
		},
		Params: llm.ParamsFromContext(ctx),
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

func appendUserMessage(msgs []schema.Message, input string, allowEmpty bool) []schema.Message {
	if !allowEmpty && input == "" {
		return msgs
	}

	return append(msgs, schema.Message{
		Role:    schema.RoleUser,
		Content: input,
	})
}

func (a *Agent[T]) addUserToMemory(ctx context.Context, input string) {
	if a.memory != nil && input != "" {
		_ = a.memory.Add(ctx, schema.Message{
			Role:    schema.RoleUser,
			Content: input,
		})
	}
}

func mustMarshalToolResult(result toolruntime.Result) string {
	payload, err := json.Marshal(result)
	if err != nil {
		return `{"status":"error","error":{"code":"marshal_error","message":"failed to serialize tool result"}}`
	}
	return string(payload)
}

func (a *Agent[T]) publishEvent(ctx context.Context, event cogruntime.Event) {
	if a.eventBus == nil {
		return
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	a.eventBus.Publish(ctx, event)
}
