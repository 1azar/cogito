package structured

import (
	"context"
	"errors"
	"testing"

	"github.com/1azar/cogito/controller"
	"github.com/1azar/cogito/schema"
	"github.com/1azar/cogito/tool"
	"github.com/1azar/cogito/toolruntime"
)

type fakeAgent struct {
	responses []string
	idx       int
}

func (f *fakeAgent) CallLLM(ctx context.Context, input string) (string, error) {
	if f.idx >= len(f.responses) {
		return "", errors.New("no fake response configured")
	}
	out := f.responses[f.idx]
	f.idx++
	return out, nil
}

func (f *fakeAgent) CallLLMWithTools(ctx context.Context, input string) (*controller.Completion, error) {
	panic("not used in structured tests")
}

func (f *fakeAgent) State() *struct{} { return &struct{}{} }

func (f *fakeAgent) Tools() *tool.Registry { return tool.NewRegistry() }

func (f *fakeAgent) RunToolCalls(ctx context.Context, calls []schema.ToolCall) ([]toolruntime.Result, error) {
	panic("not used in structured tests")
}

type person struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestStructuredController_ParsesOnFirstAttempt(t *testing.T) {
	var out person
	c := New[struct{}](Config{
		Output:      &out,
		MaxAttempts: 1,
	})

	agent := &fakeAgent{responses: []string{`{"name":"Ana","age":7}`}}

	got, err := c.Run(context.Background(), agent, "extract person")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.Name != "Ana" || out.Age != 7 {
		t.Fatalf("unexpected parsed output: %+v", out)
	}

	if got != `{"name":"Ana","age":7}` {
		t.Fatalf("unexpected run output: %s", got)
	}
}

func TestStructuredController_RetryAndRepair(t *testing.T) {
	var out person
	c := New[struct{}](Config{
		Output:      &out,
		MaxAttempts: 2,
	})

	agent := &fakeAgent{responses: []string{
		`{"name":"Ana","age":"oops"}`,
		`{"name":"Ana","age":7}`,
	}}

	_, err := c.Run(context.Background(), agent, "extract person")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if agent.idx != 2 {
		t.Fatalf("expected 2 attempts, got %d", agent.idx)
	}

	if out.Name != "Ana" || out.Age != 7 {
		t.Fatalf("unexpected parsed output: %+v", out)
	}
}

func TestStructuredController_StrictUnknownFields(t *testing.T) {
	var out person
	c := New[struct{}](Config{
		Output:      &out,
		MaxAttempts: 1,
	})

	agent := &fakeAgent{responses: []string{`{"name":"Ana","age":7,"extra":1}`}}

	_, err := c.Run(context.Background(), agent, "extract person")
	if err == nil {
		t.Fatalf("expected strict decode error, got nil")
	}
}
