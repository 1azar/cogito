package llm

import (
	"context"
	"errors"

	"github.com/1azar/cogito/schema"
	"github.com/1azar/cogito/tool"
)

type Response struct {
	Text      string
	ToolCalls []schema.ToolCall
	Raw       any
	Usage     Usage

	FinishReason string
}

type Usage struct {
	InputTokens         int64
	OutputTokens        int64
	TotalTokens         int64
	Provider            string
	EstimatedCostMicros int64
}

func (u Usage) IsZero() bool {
	return u.InputTokens == 0 && u.OutputTokens == 0 && u.TotalTokens == 0 && u.EstimatedCostMicros == 0 && u.Provider == ""
}

type StreamEventType string

const (
	StreamEventTextDelta     StreamEventType = "text_delta"
	StreamEventToolCallDelta StreamEventType = "tool_call_delta"
	StreamEventUsageDelta    StreamEventType = "usage_delta"
	StreamEventDone          StreamEventType = "done"
	StreamEventError         StreamEventType = "error"
)

type StreamEvent struct {
	Type          StreamEventType
	TextDelta     string
	ToolCallDelta *schema.ToolCall
	UsageDelta    *Usage
	Response      *Response
	Err           error
}

type Stream <-chan StreamEvent

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
	GenerateStream(ctx context.Context, req Request) (Stream, error)
}

func StreamFromResponse(resp *Response) Stream {
	ch := make(chan StreamEvent, 3)

	if resp == nil {
		ch <- StreamEvent{
			Type: StreamEventError,
			Err:  errors.New("nil response"),
		}
		close(ch)
		return ch
	}

	if resp.Text != "" {
		ch <- StreamEvent{
			Type:      StreamEventTextDelta,
			TextDelta: resp.Text,
		}
	}

	if !resp.Usage.IsZero() {
		usage := resp.Usage
		ch <- StreamEvent{
			Type:       StreamEventUsageDelta,
			UsageDelta: &usage,
		}
	}

	ch <- StreamEvent{
		Type:     StreamEventDone,
		Response: resp,
	}

	close(ch)
	return ch
}
