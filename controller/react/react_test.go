package react

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/1azar/cogito/controller"
	"github.com/1azar/cogito/llm"
	cogruntime "github.com/1azar/cogito/runtime"
	"github.com/1azar/cogito/schema"
	"github.com/1azar/cogito/tool"
	"github.com/1azar/cogito/toolruntime"
)

type fakeAgent struct {
	completions []*controller.Completion
}

func (a *fakeAgent) CallLLM(ctx context.Context, input string) (string, error) {
	return "", nil
}

func (a *fakeAgent) CallLLMWithTools(ctx context.Context, input string) (*controller.Completion, error) {
	if len(a.completions) == 0 {
		return &controller.Completion{}, nil
	}
	out := a.completions[0]
	a.completions = a.completions[1:]
	return out, nil
}

func (a *fakeAgent) State() *struct{} {
	v := struct{}{}
	return &v
}

func (a *fakeAgent) Tools() *tool.Registry {
	return tool.NewRegistry()
}

func (a *fakeAgent) RunToolCalls(ctx context.Context, calls []schema.ToolCall) ([]toolruntime.Result, error) {
	results := make([]toolruntime.Result, len(calls))
	for i, call := range calls {
		results[i] = toolruntime.Result{ToolCallID: call.ID, Name: call.Name, Status: toolruntime.StatusSuccess}
	}
	return results, nil
}

func TestRunPublishesControllerAndLLMEvents(t *testing.T) {
	ctrl := New[struct{}](Config{MaxSteps: 3})
	ag := &fakeAgent{completions: []*controller.Completion{
		{
			ToolCalls: []schema.ToolCall{{ID: "call_1", Name: "calc", Arguments: json.RawMessage(`{"expression":"2+2"}`)}},
			Usage: llm.Usage{
				InputTokens:  10,
				OutputTokens: 5,
				TotalTokens:  15,
			},
		},
		{Text: "done"},
	}}

	bus := cogruntime.NewEventBus()
	events := make([]cogruntime.EventType, 0)
	unsubscribe := bus.Subscribe(func(ctx context.Context, event cogruntime.Event) {
		events = append(events, event.Type)
	})
	defer unsubscribe()

	ctx := cogruntime.WithEventBus(context.Background(), bus)
	ctx = cogruntime.WithRunID(ctx, "run_test")

	out, err := ctrl.Run(ctx, ag, "hello")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if out != "done" {
		t.Fatalf("unexpected output: %q", out)
	}

	for _, expected := range []cogruntime.EventType{
		cogruntime.EventControllerStepStarted,
		cogruntime.EventLLMCallStarted,
		cogruntime.EventLLMCallFinished,
		cogruntime.EventControllerStepFinished,
	} {
		if !slices.Contains(events, expected) {
			t.Fatalf("expected event %s, got %v", expected, events)
		}
	}
}
