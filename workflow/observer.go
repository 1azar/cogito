package workflow

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type EventType string

const (
	EventWorkflowStarted  EventType = "workflow_started"
	EventWorkflowFinished EventType = "workflow_finished"
	EventWorkflowFailed   EventType = "workflow_failed"
	EventWorkflowCanceled EventType = "workflow_canceled"
	EventNodeStarted      EventType = "node_started"
	EventNodeFinished     EventType = "node_finished"
	EventNodeFailed       EventType = "node_failed"
	EventEdgeEvaluated    EventType = "edge_evaluated"
)

type Event struct {
	Type            EventType
	NodeID          string
	NodeDescription string
	Iteration       int
	StartedAt       time.Time
	Duration        time.Duration
	NextNode        string
	Err             error
}

type Observer interface {
	OnEvent(ctx context.Context, event Event)
}

type ObserverFunc func(ctx context.Context, event Event)

func (f ObserverFunc) OnEvent(ctx context.Context, event Event) {
	f(ctx, event)
}

type ConsoleObserver struct {
	w io.Writer

	mu sync.Mutex
}

func NewConsoleObserver(w io.Writer) *ConsoleObserver {
	if w == nil {
		w = os.Stdout
	}
	return &ConsoleObserver{w: w}
}

func (o *ConsoleObserver) OnEvent(ctx context.Context, event Event) {
	o.mu.Lock()
	defer o.mu.Unlock()

	switch event.Type {
	case EventWorkflowStarted:
		fmt.Fprintf(o.w, "[workflow] start entry=%s\n", event.NodeID)
	case EventWorkflowFinished:
		fmt.Fprintf(o.w, "[workflow] finish\n")
	case EventWorkflowCanceled:
		fmt.Fprintf(o.w, "[workflow] canceled err=%v\n", event.Err)
	case EventWorkflowFailed:
		fmt.Fprintf(o.w, "[workflow] failed node=%s err=%v\n", event.NodeID, event.Err)
	case EventNodeStarted:
		fmt.Fprintf(o.w, "[workflow] node start id=%s iter=%d desc=%q\n", event.NodeID, event.Iteration, event.NodeDescription)
	case EventNodeFinished:
		fmt.Fprintf(o.w, "[workflow] node finish id=%s iter=%d duration=%s\n", event.NodeID, event.Iteration, event.Duration.Round(time.Millisecond))
	case EventNodeFailed:
		fmt.Fprintf(o.w, "[workflow] node failed id=%s iter=%d duration=%s err=%v\n", event.NodeID, event.Iteration, event.Duration.Round(time.Millisecond), event.Err)
	case EventEdgeEvaluated:
		fmt.Fprintf(o.w, "[workflow] edge from=%s to=%s iter=%d\n", event.NodeID, event.NextNode, event.Iteration)
	default:
		fmt.Fprintf(o.w, "[workflow] event=%s node=%s iter=%d\n", event.Type, event.NodeID, event.Iteration)
	}
}
