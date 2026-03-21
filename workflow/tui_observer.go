package workflow

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type TUIObserver struct {
	w io.Writer

	mu         sync.Mutex
	outputMu   sync.Mutex
	spinActive bool
	spinStop   chan struct{}
	spinDone   chan struct{}
}

func NewTUIObserver(w io.Writer) *TUIObserver {
	if w == nil {
		w = os.Stdout
	}

	return &TUIObserver{w: w}
}

func (o *TUIObserver) OnEvent(ctx context.Context, event Event) {
	switch event.Type {
	case EventWorkflowStarted:
		o.printLine("[workflow] starting entry=%s", event.NodeID)
	case EventNodeStarted:
		o.startSpinner(event)
	case EventNodeFinished:
		o.stopSpinner()
		o.printLine(
			"[workflow] node done id=%s iter=%d duration=%s",
			event.NodeID,
			event.Iteration,
			event.Duration.Round(time.Millisecond),
		)
	case EventNodeFailed:
		o.stopSpinner()
		o.printLine(
			"[workflow] node failed id=%s iter=%d duration=%s err=%v",
			event.NodeID,
			event.Iteration,
			event.Duration.Round(time.Millisecond),
			event.Err,
		)
	case EventEdgeEvaluated:
		o.printLine("[workflow] route from=%s to=%s iter=%d", event.NodeID, event.NextNode, event.Iteration)
	case EventWorkflowFinished:
		o.stopSpinner()
		o.printLine("[workflow] finished")
	case EventWorkflowFailed:
		o.stopSpinner()
		o.printLine("[workflow] failed node=%s err=%v", event.NodeID, event.Err)
	case EventWorkflowCanceled:
		o.stopSpinner()
		o.printLine("[workflow] canceled err=%v", event.Err)
	default:
		o.printLine("[workflow] event=%s node=%s iter=%d", event.Type, event.NodeID, event.Iteration)
	}
}

func (o *TUIObserver) startSpinner(event Event) {
	o.stopSpinner()

	stop := make(chan struct{})
	done := make(chan struct{})

	o.mu.Lock()
	o.spinActive = true
	o.spinStop = stop
	o.spinDone = done
	o.mu.Unlock()

	nodeID := event.NodeID
	iter := event.Iteration
	startedAt := event.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now()
	}

	go func() {
		defer close(done)

		frames := []byte{'|', '/', '-', '\\'}
		idx := 0
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				elapsed := time.Since(startedAt).Round(100 * time.Millisecond)
				o.printInline(
					"[workflow] %c running id=%s iter=%d elapsed=%s",
					frames[idx%len(frames)],
					nodeID,
					iter,
					elapsed,
				)
				idx++
			case <-stop:
				return
			}
		}
	}()
}

func (o *TUIObserver) stopSpinner() {
	o.mu.Lock()
	if !o.spinActive {
		o.mu.Unlock()
		return
	}

	stop := o.spinStop
	done := o.spinDone
	o.spinActive = false
	o.spinStop = nil
	o.spinDone = nil
	o.mu.Unlock()

	close(stop)
	<-done

	o.clearInline()
}

func (o *TUIObserver) printLine(format string, args ...any) {
	o.outputMu.Lock()
	defer o.outputMu.Unlock()
	fmt.Fprintf(o.w, "\r\033[K"+format+"\n", args...)
}

func (o *TUIObserver) printInline(format string, args ...any) {
	o.outputMu.Lock()
	defer o.outputMu.Unlock()
	fmt.Fprintf(o.w, "\r\033[K"+format, args...)
}

func (o *TUIObserver) clearInline() {
	o.outputMu.Lock()
	defer o.outputMu.Unlock()
	fmt.Fprint(o.w, "\r\033[K")
}
