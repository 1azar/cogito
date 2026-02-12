package buffer

import (
	"context"
	"sync"

	"github.com/1azar/cogito/memory"
	"github.com/1azar/cogito/schema"
)

type Buffer struct {
	mu    sync.Mutex
	limit int
	data  []schema.Message
}

func New(limit int) *Buffer {
	return &Buffer{
		limit: limit,
		data:  make([]schema.Message, 0, limit),
	}
}

func (b *Buffer) Add(_ context.Context, msg schema.Message) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.data = append(b.data, msg)

	if len(b.data) > b.limit {
		b.data = b.data[1:]
	}

	return nil
}

func (b *Buffer) Get(_ context.Context) ([]schema.Message, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := make([]schema.Message, len(b.data))
	copy(out, b.data)

	return out, nil
}

func (b *Buffer) Clear() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = make([]schema.Message, 0, b.limit)
	return nil
}

var _ memory.Memory = (*Buffer)(nil)
