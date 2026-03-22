package agent

import (
	"context"
	"testing"

	"github.com/1azar/cogito/llm"
	"github.com/1azar/cogito/memory/buffer"
	"github.com/1azar/cogito/schema"
)

type recordingLLM struct {
	responses []*llm.Response
	requests  []llm.Request
}

func (r *recordingLLM) Generate(_ context.Context, req llm.Request) (*llm.Response, error) {
	r.requests = append(r.requests, req)

	if len(r.responses) == 0 {
		return &llm.Response{}, nil
	}

	resp := r.responses[0]
	r.responses = r.responses[1:]
	return resp, nil
}

func TestCallLLMStoresUserAndAssistantInMemory(t *testing.T) {
	ctx := context.Background()
	m := buffer.New(10)
	fake := &recordingLLM{
		responses: []*llm.Response{{Text: "done"}},
	}

	ag := NewAgent[struct{}](fake).WithMemory(m)

	out, err := ag.CallLLM(ctx, "hello")
	if err != nil {
		t.Fatalf("CallLLM returned error: %v", err)
	}
	if out != "done" {
		t.Fatalf("unexpected output: %q", out)
	}

	history, err := m.Get(ctx)
	if err != nil {
		t.Fatalf("memory.Get returned error: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("unexpected history length: got %d want %d", len(history), 2)
	}
	if history[0].Role != schema.RoleUser || history[0].Content != "hello" {
		t.Fatalf("unexpected first message: %+v", history[0])
	}
	if history[1].Role != schema.RoleAssistant || history[1].Content != "done" {
		t.Fatalf("unexpected second message: %+v", history[1])
	}
}

func TestCallLLMDoesNotStoreEmptyUserInput(t *testing.T) {
	ctx := context.Background()
	m := buffer.New(10)
	fake := &recordingLLM{
		responses: []*llm.Response{{Text: "ok"}},
	}

	ag := NewAgent[struct{}](fake).WithMemory(m)

	_, err := ag.CallLLM(ctx, "")
	if err != nil {
		t.Fatalf("CallLLM returned error: %v", err)
	}

	history, err := m.Get(ctx)
	if err != nil {
		t.Fatalf("memory.Get returned error: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("unexpected history length: got %d want %d", len(history), 1)
	}
	if history[0].Role != schema.RoleAssistant || history[0].Content != "ok" {
		t.Fatalf("unexpected stored message: %+v", history[0])
	}

	if len(fake.requests) != 1 {
		t.Fatalf("unexpected requests length: got %d want %d", len(fake.requests), 1)
	}
	req := fake.requests[0]
	if len(req.Messages) != 1 {
		t.Fatalf("unexpected request messages length: got %d want %d", len(req.Messages), 1)
	}
	if req.Messages[0].Role != schema.RoleUser || req.Messages[0].Content != "" {
		t.Fatalf("unexpected request message: %+v", req.Messages[0])
	}
}

func TestCallLLMIncludesMemoryHistoryInRequest(t *testing.T) {
	ctx := context.Background()
	m := buffer.New(10)

	if err := m.Add(ctx, schema.Message{Role: schema.RoleUser, Content: "prev"}); err != nil {
		t.Fatalf("seed memory failed: %v", err)
	}

	fake := &recordingLLM{
		responses: []*llm.Response{{Text: "answer"}},
	}

	ag := NewAgent[struct{}](fake).WithMemory(m)

	_, err := ag.CallLLM(ctx, "next")
	if err != nil {
		t.Fatalf("CallLLM returned error: %v", err)
	}

	if len(fake.requests) != 1 {
		t.Fatalf("unexpected requests length: got %d want %d", len(fake.requests), 1)
	}
	req := fake.requests[0]
	if len(req.Messages) != 2 {
		t.Fatalf("unexpected request messages length: got %d want %d", len(req.Messages), 2)
	}
	if req.Messages[0].Role != schema.RoleUser || req.Messages[0].Content != "prev" {
		t.Fatalf("unexpected first request message: %+v", req.Messages[0])
	}
	if req.Messages[1].Role != schema.RoleUser || req.Messages[1].Content != "next" {
		t.Fatalf("unexpected second request message: %+v", req.Messages[1])
	}

	history, err := m.Get(ctx)
	if err != nil {
		t.Fatalf("memory.Get returned error: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("unexpected history length: got %d want %d", len(history), 3)
	}
	if history[1].Role != schema.RoleUser || history[1].Content != "next" {
		t.Fatalf("unexpected history user message: %+v", history[1])
	}
	if history[2].Role != schema.RoleAssistant || history[2].Content != "answer" {
		t.Fatalf("unexpected history assistant message: %+v", history[2])
	}
}
