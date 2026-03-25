package runtime

import (
	"context"
	"sync"
)

type EventHandler func(ctx context.Context, event Event)

type EventBus interface {
	Publish(ctx context.Context, event Event)
	Subscribe(handler EventHandler) func()
}

type InMemoryEventBus struct {
	mu       sync.RWMutex
	nextID   uint64
	handlers map[uint64]EventHandler
}

func NewEventBus() *InMemoryEventBus {
	return &InMemoryEventBus{
		handlers: make(map[uint64]EventHandler),
	}
}

func (b *InMemoryEventBus) Publish(ctx context.Context, event Event) {
	b.mu.RLock()
	handlers := make([]EventHandler, 0, len(b.handlers))
	for _, h := range b.handlers {
		handlers = append(handlers, h)
	}
	b.mu.RUnlock()

	for _, h := range handlers {
		func(handler EventHandler) {
			defer func() { _ = recover() }()
			handler(ctx, event)
		}(h)
	}
}

func (b *InMemoryEventBus) Subscribe(handler EventHandler) func() {
	if handler == nil {
		return func() {}
	}

	b.mu.Lock()
	b.nextID++
	id := b.nextID
	b.handlers[id] = handler
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		delete(b.handlers, id)
		b.mu.Unlock()
	}
}

var _ EventBus = (*InMemoryEventBus)(nil)
