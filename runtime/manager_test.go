package runtime

import (
	"context"
	"errors"
	"testing"
)

func TestManagerStartAndComplete(t *testing.T) {
	m := NewManager()
	ctx, run := m.Start(context.Background(), "session-1", "hello")

	if run == nil {
		t.Fatal("expected run, got nil")
	}

	runID, ok := RunIDFromContext(ctx)
	if !ok || runID == "" {
		t.Fatal("expected run id in context")
	}
	if runID != run.ID() {
		t.Fatalf("run id mismatch: ctx=%s run=%s", runID, run.ID())
	}

	if err := m.Complete(run.ID()); err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	snap := run.Snapshot()
	if snap.Status != RunCompleted {
		t.Fatalf("expected status %s, got %s", RunCompleted, snap.Status)
	}
}

func TestManagerCancel(t *testing.T) {
	m := NewManager()
	_, run := m.Start(context.Background(), "session-2", "hello")

	if err := m.Cancel(run.ID(), context.Canceled); err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}

	snap := run.Snapshot()
	if snap.Status != RunCanceled {
		t.Fatalf("expected status %s, got %s", RunCanceled, snap.Status)
	}
}

func TestManagerFail(t *testing.T) {
	m := NewManager()
	_, run := m.Start(context.Background(), "session-3", "hello")

	expected := errors.New("boom")
	if err := m.Fail(run.ID(), expected); err != nil {
		t.Fatalf("Fail returned error: %v", err)
	}

	snap := run.Snapshot()
	if snap.Status != RunFailed {
		t.Fatalf("expected status %s, got %s", RunFailed, snap.Status)
	}
	if !errors.Is(snap.Err, expected) {
		t.Fatalf("expected fail error %v, got %v", expected, snap.Err)
	}
}
