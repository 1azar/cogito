package workflow

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
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
