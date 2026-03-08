package mock

import (
	"context"

	"github.com/1azar/cogito/llm"
)

type Mock struct{}

func New() *Mock {
	return &Mock{}
}

func (m Mock) Generate(ctx context.Context, req llm.Request) (*llm.Response, error) {
	msgs := req.Messages
	if len(msgs) < 1 {
		return nil, nil
	}

	last := msgs[len(msgs)-1]

	return &llm.Response{
		Text: `Answer to: "` + last.Content + `"`,
	}, nil
}
