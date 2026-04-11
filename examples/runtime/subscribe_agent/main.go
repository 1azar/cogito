package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/1azar/cogito/agent"
	"github.com/1azar/cogito/controller/simple"
	"github.com/1azar/cogito/llm/mock"
	"github.com/1azar/cogito/memory/buffer"
	cogruntime "github.com/1azar/cogito/runtime"
)

type State struct{}

type runStats struct {
	mu sync.Mutex

	runID      string
	startedAt  time.Time
	textDeltas int
}

func main() {
	ag := agent.NewAgent[State](mock.New()).
		WithController(simple.New[State]()).
		WithMemory(buffer.New(10)).
		WithSessionID("runtime_subscribe_mock")

	stats := &runStats{}
	unsubscribe := subscribeToRunEvents(ag, stats)
	defer unsubscribe()

	input := "show me runtime subscribe with mock llm"
	out, err := ag.Run(context.Background(), input)
	if err != nil {
		panic(err)
	}

	stats.mu.Lock()
	textDeltaCount := stats.textDeltas
	stats.mu.Unlock()

	fmt.Printf("\nfinal output: %q\n", out)
	fmt.Printf("text delta events observed: %d\n", textDeltaCount)
}

func subscribeToRunEvents(ag *agent.Agent[State], stats *runStats) func() {
	bus := ag.EventBus()
	if bus == nil {
		return func() {}
	}

	return bus.Subscribe(func(ctx context.Context, event cogruntime.Event) {
		stats.mu.Lock()
		defer stats.mu.Unlock()

		switch event.Type {
		case cogruntime.EventRunStarted:
			stats.runID = event.RunID
			stats.startedAt = event.Timestamp
			stats.textDeltas = 0
			fmt.Printf("[run] started id=%s session=%s\n", event.RunID, event.SessionID)

		case cogruntime.EventLLMTextDelta:
			if stats.runID != "" && event.RunID != stats.runID {
				return
			}
			stats.textDeltas++
			fmt.Printf("[stream] delta #%d: %q\n", stats.textDeltas, event.Message)

		case cogruntime.EventRunFinished:
			if stats.runID != "" && event.RunID != stats.runID {
				return
			}
			d := event.Duration.Round(time.Millisecond)
			if d == 0 && !stats.startedAt.IsZero() {
				d = event.Timestamp.Sub(stats.startedAt).Round(time.Millisecond)
			}
			fmt.Printf("[run] finished id=%s duration=%s deltas=%d\n", event.RunID, d, stats.textDeltas)
		}
	})
}
