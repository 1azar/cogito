package runtime

import (
	"context"
	"testing"
)

func TestEventBusSubscribePublish(t *testing.T) {
	bus := NewEventBus()
	called := false

	unsubscribe := bus.Subscribe(func(ctx context.Context, event Event) {
		called = true
	})

	bus.Publish(context.Background(), Event{Type: EventRunStarted})
	if !called {
		t.Fatal("expected subscriber to be called")
	}

	called = false
	unsubscribe()
	bus.Publish(context.Background(), Event{Type: EventRunFinished})
	if called {
		t.Fatal("expected unsubscribed handler not to be called")
	}
}
