package rlm

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/1azar/cogito/llm"
	"github.com/1azar/cogito/rlm/environment"
	cogruntime "github.com/1azar/cogito/runtime"
	"github.com/1azar/cogito/schema"
	"github.com/1azar/cogito/tool"
)

type routingLLM struct {
	mu       sync.Mutex
	requests []llm.Request
}

type llmFunc func(context.Context, llm.Request) (*llm.Response, error)

func (f llmFunc) Generate(ctx context.Context, req llm.Request) (*llm.Response, error) {
	return f(ctx, req)
}

func (f llmFunc) GenerateStream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	resp, err := f(ctx, req)
	if err != nil {
		return nil, err
	}
	return llm.StreamFromResponse(resp), nil
}

func (m *routingLLM) Generate(_ context.Context, req llm.Request) (*llm.Response, error) {
	m.mu.Lock()
	m.requests = append(m.requests, req)
	m.mu.Unlock()
	last := req.Messages[len(req.Messages)-1].Content
	switch {
	case last == "root":
		return response("```repl\nSUBCALL\n```"), nil
	case last == "child":
		return response("```repl\nCHILD_ANSWER\n```"), nil
	case strings.Contains(last, "REPL result"):
		return response("```repl\nFINAL_ANSWER\n```"), nil
	default:
		return response("```repl\nPRINT_CONTEXT\n```"), nil
	}
}

func (m *routingLLM) GenerateStream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	resp, err := m.Generate(ctx, req)
	if err != nil {
		return nil, err
	}
	return llm.StreamFromResponse(resp), nil
}

func response(text string) *llm.Response {
	return &llm.Response{
		Text: text,
		Usage: llm.Usage{
			InputTokens: 2, OutputTokens: 3, TotalTokens: 5,
			EstimatedCostMicros: 7, Provider: "fake",
		},
	}
}

type fakeFactory struct {
	execute func(context.Context, string, environment.CallHandler) (environment.ExecutionResult, error)
	mu      sync.Mutex
	configs []environment.SessionConfig
}

func (f *fakeFactory) NewSession(_ context.Context, config environment.SessionConfig) (environment.Session, error) {
	f.mu.Lock()
	f.configs = append(f.configs, config)
	f.mu.Unlock()
	return &fakeSession{execute: f.execute}, nil
}

type fakeSession struct {
	execute func(context.Context, string, environment.CallHandler) (environment.ExecutionResult, error)
}

func (s *fakeSession) Execute(ctx context.Context, code string, handler environment.CallHandler) (environment.ExecutionResult, error) {
	return s.execute(ctx, code, handler)
}
func (s *fakeSession) Close(context.Context) error { return nil }

func TestModelExploresContextAndAnswers(t *testing.T) {
	modelLLM := &routingLLM{}
	factory := &fakeFactory{execute: func(_ context.Context, code string, _ environment.CallHandler) (environment.ExecutionResult, error) {
		switch {
		case strings.Contains(code, "PRINT_CONTEXT"):
			return environment.ExecutionResult{Stdout: "found relevant section"}, nil
		case strings.Contains(code, "FINAL_ANSWER"):
			return environment.ExecutionResult{Answer: &environment.Answer{Text: "final"}}, nil
		default:
			return environment.ExecutionResult{}, errors.New("unexpected code")
		}
	}}
	model := mustModel(t, modelLLM, factory, Config{})
	resp, err := model.Generate(context.Background(), llm.Request{
		Messages: []schema.Message{
			{Role: schema.RoleSystem, Content: strings.Repeat("long context ", 100)},
			{Role: schema.RoleUser, Content: "question"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "final" {
		t.Fatalf("text = %q", resp.Text)
	}
	if resp.Usage.TotalTokens != 10 {
		t.Fatalf("total tokens = %d, want 10", resp.Usage.TotalTokens)
	}
	meta, ok := resp.Raw.(Metadata)
	if !ok || meta.Iterations != 2 || meta.Calls != 2 || meta.Termination != "answer" {
		t.Fatalf("metadata = %#v", resp.Raw)
	}
	if len(factory.configs) != 1 || !json.Valid(factory.configs[0].Context) {
		t.Fatalf("serialized context was not provided: %#v", factory.configs)
	}
}

func TestModelRecursiveSubcallSharesUsage(t *testing.T) {
	modelLLM := &routingLLM{}
	factory := &fakeFactory{execute: func(ctx context.Context, code string, handler environment.CallHandler) (environment.ExecutionResult, error) {
		switch {
		case strings.Contains(code, "SUBCALL"):
			result := handler.HandleCall(ctx, environment.CallRequest{Kind: environment.CallRLM, Prompts: []string{"child"}})
			if result.Error != "" {
				return environment.ExecutionResult{}, errors.New(result.Error)
			}
			return environment.ExecutionResult{Answer: &environment.Answer{Text: "parent:" + result.Results[0]}}, nil
		case strings.Contains(code, "CHILD_ANSWER"):
			return environment.ExecutionResult{Answer: &environment.Answer{Text: "child result"}}, nil
		default:
			return environment.ExecutionResult{}, errors.New("unexpected code")
		}
	}}
	model := mustModel(t, modelLLM, factory, Config{MaxDepth: 2})
	resp, err := model.Generate(context.Background(), llm.Request{
		Messages: []schema.Message{{Role: schema.RoleUser, Content: "root"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "parent:child result" {
		t.Fatalf("text = %q", resp.Text)
	}
	meta := resp.Raw.(Metadata)
	if meta.MaxDepthReached != 1 || meta.Calls != 2 || resp.Usage.TotalTokens != 10 {
		t.Fatalf("metadata=%+v usage=%+v", meta, resp.Usage)
	}
}

func TestModelReturnsValidatedToolCalls(t *testing.T) {
	modelLLM := &routingLLM{}
	factory := &fakeFactory{execute: func(_ context.Context, _ string, _ environment.CallHandler) (environment.ExecutionResult, error) {
		return environment.ExecutionResult{Answer: &environment.Answer{
			ToolCalls: json.RawMessage(`[{"name":"calculator","arguments":{"expression":"2+2"}}]`),
		}}, nil
	}}
	model := mustModel(t, modelLLM, factory, Config{})
	resp, err := model.Generate(WithRootPrompt(context.Background(), "explicit"), llm.Request{
		Messages: []schema.Message{{Role: schema.RoleUser, Content: "long input"}},
		Tools:    []tool.Spec{{Name: "calculator"}},
		ToolChoice: llm.ToolChoice{
			Mode: llm.ToolChoiceNamed, Name: "calculator",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID == "" || resp.FinishReason != "tool_calls" {
		t.Fatalf("response = %#v", resp)
	}
	modelLLM.mu.Lock()
	firstPrompt := modelLLM.requests[0].Messages[1].Content
	modelLLM.mu.Unlock()
	if firstPrompt != "explicit" {
		t.Fatalf("root prompt = %q", firstPrompt)
	}
}

func TestModelEnforcesSharedCallBudget(t *testing.T) {
	modelLLM := &routingLLM{}
	factory := &fakeFactory{execute: func(ctx context.Context, _ string, handler environment.CallHandler) (environment.ExecutionResult, error) {
		result := handler.HandleCall(ctx, environment.CallRequest{
			Kind: environment.CallLLM, Batched: true, Prompts: []string{"a", "b", "c"},
		})
		if result.Error != "" {
			return environment.ExecutionResult{}, errors.New(result.Error)
		}
		return environment.ExecutionResult{Answer: &environment.Answer{Text: strings.Join(result.Results, ",")}}, nil
	}}
	model := mustModel(t, modelLLM, factory, Config{MaxTotalCalls: 2})
	_, err := model.Generate(context.Background(), llm.Request{
		Messages: []schema.Message{{Role: schema.RoleUser, Content: "question"}},
	})
	if err == nil || !IsKind(err, ErrorBudget) || !strings.Contains(err.Error(), "calls") {
		t.Fatalf("error = %v", err)
	}
}

func TestModelRejectsUnregisteredTool(t *testing.T) {
	modelLLM := &routingLLM{}
	factory := &fakeFactory{execute: func(_ context.Context, _ string, _ environment.CallHandler) (environment.ExecutionResult, error) {
		return environment.ExecutionResult{Answer: &environment.Answer{
			ToolCalls: json.RawMessage(`[{"name":"danger","arguments":{}}]`),
		}}, nil
	}}
	model := mustModel(t, modelLLM, factory, Config{})
	_, err := model.Generate(context.Background(), llm.Request{
		Messages: []schema.Message{{Role: schema.RoleUser, Content: "question"}},
	})
	if err == nil || !IsKind(err, ErrorToolCall) {
		t.Fatalf("error = %v", err)
	}
}

func TestModelStopsAfterRepeatedMissingCode(t *testing.T) {
	base := llmFunc(func(context.Context, llm.Request) (*llm.Response, error) {
		return response("plain text"), nil
	})
	factory := &fakeFactory{execute: func(context.Context, string, environment.CallHandler) (environment.ExecutionResult, error) {
		t.Fatal("REPL should not execute")
		return environment.ExecutionResult{}, nil
	}}
	model := mustModel(t, base, factory, Config{MaxConsecutiveErrors: 2})
	_, err := model.Generate(context.Background(), llm.Request{
		Messages: []schema.Message{{Role: schema.RoleUser, Content: "question"}},
	})
	if err == nil || !IsKind(err, ErrorProtocol) {
		t.Fatalf("error = %v", err)
	}
}

func TestModelReturnsTypedCancellation(t *testing.T) {
	base := llmFunc(func(context.Context, llm.Request) (*llm.Response, error) {
		return response("```repl\nWAIT\n```"), nil
	})
	factory := &fakeFactory{execute: func(ctx context.Context, _ string, _ environment.CallHandler) (environment.ExecutionResult, error) {
		<-ctx.Done()
		return environment.ExecutionResult{}, ctx.Err()
	}}
	model := mustModel(t, base, factory, Config{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := model.Generate(ctx, llm.Request{
		Messages: []schema.Message{{Role: schema.RoleUser, Content: "question"}},
	})
	if err == nil || !IsKind(err, ErrorCanceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestRunFinishedEventContainsAggregatedUsage(t *testing.T) {
	modelLLM := &routingLLM{}
	factory := &fakeFactory{execute: func(_ context.Context, code string, _ environment.CallHandler) (environment.ExecutionResult, error) {
		if strings.Contains(code, "PRINT_CONTEXT") {
			return environment.ExecutionResult{Stdout: "result"}, nil
		}
		return environment.ExecutionResult{Answer: &environment.Answer{Text: "done"}}, nil
	}}
	model := mustModel(t, modelLLM, factory, Config{})
	bus := cogruntime.NewEventBus()
	var finished cogruntime.Event
	bus.Subscribe(func(_ context.Context, event cogruntime.Event) {
		if event.Type == cogruntime.EventRLMRunFinished {
			finished = event
		}
	})
	ctx := cogruntime.WithEventBus(context.Background(), bus)
	_, err := model.Generate(ctx, llm.Request{
		Messages: []schema.Message{{Role: schema.RoleUser, Content: "question"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if finished.TotalTokens != 10 || finished.InputTokens != 4 || finished.OutputTokens != 6 {
		t.Fatalf("finished event usage = %+v", finished)
	}
}

func mustModel(t *testing.T, base llm.LLM, factory environment.Factory, config Config) *Model {
	t.Helper()
	config.Environment = factory
	model, err := New(base, config)
	if err != nil {
		t.Fatal(err)
	}
	return model
}
