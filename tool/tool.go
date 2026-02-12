package tool

import (
	"context"
	"encoding/json"
)

type Tool interface {
	Name() string
	Description() string
	Schema() Schema

	Call(ctx context.Context, input json.RawMessage) (any, error)
}
