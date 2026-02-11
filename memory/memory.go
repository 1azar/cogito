package memory

import (
	"context"

	"github.com/1azar/cogito/schema"
)

type Memory interface {
	Add(ctx context.Context, msg schema.Message) error
	Get(ctx context.Context) ([]schema.Message, error)
	Clear() error
}
