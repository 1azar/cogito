package llm

import (
	"context"

	"github.com/1azar/cogito/schema"
	"github.com/1azar/cogito/tool"
)

type Response struct {
	Text      string
	ToolCalls []schema.ToolCall
	Raw       any

	FinishReason string
}

type Request struct {
	Messages   []schema.Message
	Tools      []tool.Spec
	ToolChoice ToolChoice
	Params     Params
}

type Params struct {
	Temperature   *float64
	TopP          *float64
	RepeatPenalty *float64
	MaxTokens     *int
	Stop          []string
}

type ToolChoiceMode string

const (
	ToolChoiceAuto     ToolChoiceMode = "auto"
	ToolChoiceNone     ToolChoiceMode = "none"
	ToolChoiceRequired ToolChoiceMode = "required"
	ToolChoiceNamed    ToolChoiceMode = "named"
)

type ToolChoice struct {
	Mode ToolChoiceMode
	Name string
}

func (tc ToolChoice) IsZero() bool {
	return tc.Mode == ""
}

type LLM interface {
	Generate(ctx context.Context, req Request) (*Response, error)
}
