package tool

import (
	"context"
	"encoding/json"
)

type Tool interface {
	Name() string
	Description() string
	Spec() Spec

	Call(ctx context.Context, input json.RawMessage) (any, error)
}
