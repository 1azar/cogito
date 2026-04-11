package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	cogruntime "github.com/1azar/cogito/runtime"
)

func main() {
	bus := cogruntime.NewEventBus()
	runID := cogruntime.NewRunID("demo")
	ctx := cogruntime.WithRunID(context.Background(), runID)

	var mu sync.Mutex
	secondaryCalls := 0

	unsubscribePrimary := bus.Subscribe(func(ctx context.Context, event cogruntime.Event) {
		id := event.RunID
		if id == "" {
			if fromCtx, ok := cogruntime.RunIDFromContext(ctx); ok {
				id = fromCtx
			}
		}

		fmt.Printf("[primary] type=%s run=%s step=%d msg=%q\n", event.Type, id, event.Step, event.Message)
	})
	defer unsubscribePrimary()

	unsubscribeSecondary := bus.Subscribe(func(ctx context.Context, event cogruntime.Event) {
		mu.Lock()
		secondaryCalls++
		mu.Unlock()
	})

	publish(bus, ctx, cogruntime.Event{Type: cogruntime.EventRunStarted, Message: "run is starting"})
	publish(bus, cogruntime.WithStep(ctx, 1), cogruntime.Event{Type: cogruntime.EventControllerStepStarted, Step: 1, Message: "step 1"})

	// Stop secondary subscriber and publish once more.
	unsubscribeSecondary()
	publish(bus, ctx, cogruntime.Event{Type: cogruntime.EventRunFinished, Message: "run is finished"})

	mu.Lock()
	count := secondaryCalls
	mu.Unlock()

	fmt.Printf("secondary subscriber calls: %d\n", count)
	fmt.Println("expected: 2 (it did not receive the final event after unsubscribe)")
}

func publish(bus cogruntime.EventBus, ctx context.Context, event cogruntime.Event) {
	event.Timestamp = time.Now()
	if event.RunID == "" {
		if runID, ok := cogruntime.RunIDFromContext(ctx); ok {
			event.RunID = runID
		}
	}
	bus.Publish(ctx, event)
}
