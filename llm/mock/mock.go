package mock

import (
	"context"

	"github.com/1azar/cogito/llm"
	"github.com/1azar/cogito/schema"
)

type Mock struct{}

func New() *Mock {
	return &Mock{}
}

func (m Mock) Generate(ctx context.Context, msgs []schema.Message) (*llm.Completion, error) {
	if len(msgs) < 1 {
		return nil, nil
	}

	last := msgs[len(msgs)-1]

	return &llm.Completion{
		Text: `Answer to: "` + last.Content + `"`,
	}, nil
}
