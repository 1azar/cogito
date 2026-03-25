package workflow

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	cogruntime "github.com/1azar/cogito/runtime"
)

func TestConsoleObserverWritesEvents(t *testing.T) {
	buf := &bytes.Buffer{}
	obs := NewConsoleObserver(buf)

	obs.OnEvent(context.Background(), Event{Type: EventWorkflowStarted, NodeID: "router"})
	obs.OnEvent(context.Background(), Event{Type: EventNodeStarted, NodeID: "router", Iteration: 0, NodeDescription: "Route"})
	obs.OnEvent(context.Background(), Event{Type: EventNodeFinished, NodeID: "router", Iteration: 0, Duration: 42 * time.Millisecond})

	out := buf.String()
	if !strings.Contains(out, "[workflow] start entry=router") {
		t.Fatalf("expected workflow start line, got: %q", out)
	}
	if !strings.Contains(out, "[workflow] node start id=router") {
		t.Fatalf("expected node start line, got: %q", out)
	}
	if !strings.Contains(out, "[workflow] node finish id=router") {
		t.Fatalf("expected node finish line, got: %q", out)
	}
}

func TestTUIObserverWritesEvents(t *testing.T) {
	buf := &bytes.Buffer{}
	obs := NewTUIObserver(buf)

	obs.OnEvent(context.Background(), Event{Type: EventWorkflowStarted, NodeID: "router"})
	obs.OnEvent(context.Background(), Event{Type: EventNodeStarted, NodeID: "router", Iteration: 1, StartedAt: time.Now()})
	time.Sleep(20 * time.Millisecond)
	obs.OnEvent(context.Background(), Event{Type: EventNodeFinished, NodeID: "router", Iteration: 1, Duration: 120 * time.Millisecond})
	obs.OnEvent(context.Background(), Event{Type: EventWorkflowFinished})

	out := buf.String()
	if !strings.Contains(out, "[workflow] starting entry=router") {
		t.Fatalf("expected workflow start line, got: %q", out)
	}
	if !strings.Contains(out, "[workflow] node done id=router") {
		t.Fatalf("expected node done line, got: %q", out)
	}
	if !strings.Contains(out, "[workflow] finished") {
		t.Fatalf("expected workflow finished line, got: %q", out)
	}
}

func TestWorkflowPublishesEventsToRuntimeBus(t *testing.T) {
	g := NewGraph[map[string]any]()
	g.AddNode(FunctionNode(
		"step",
		func(ctx context.Context, st State) (State, error) {
			return st, nil
		},
		"Simple step",
	)).
		AddEdge("step", EndNode).
		SetEntry("step")

	bus := cogruntime.NewEventBus()
	cfg := DefaultConfig()
	cfg.EventBus = bus
	g.SetConfig(cfg)

	types := make([]cogruntime.EventType, 0)
	unsubscribe := bus.Subscribe(func(ctx context.Context, event cogruntime.Event) {
		types = append(types, event.Type)
	})
	defer unsubscribe()

	if _, err := g.Run(context.Background(), map[string]any{}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if !slices.Contains(types, cogruntime.EventRunStarted) {
		t.Fatalf("expected run_started event, got %v", types)
	}
	if !slices.Contains(types, cogruntime.EventNodeStarted) {
		t.Fatalf("expected node_started event, got %v", types)
	}
	if !slices.Contains(types, cogruntime.EventRunFinished) {
		t.Fatalf("expected run_finished event, got %v", types)
	}
}
