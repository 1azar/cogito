package llm

import (
	"context"
	"testing"
)

func TestParamsContextRoundTrip(t *testing.T) {
	temp := 0.3
	maxTokens := 128

	ctx := WithParams(context.Background(), Params{
		Temperature: &temp,
		MaxTokens:   &maxTokens,
	})

	got := ParamsFromContext(ctx)
	if got.Temperature == nil || *got.Temperature != temp {
		t.Fatalf("unexpected temperature: %#v", got.Temperature)
	}
	if got.MaxTokens == nil || *got.MaxTokens != maxTokens {
		t.Fatalf("unexpected max tokens: %#v", got.MaxTokens)
	}
}

func TestWithParamsMergesExistingContext(t *testing.T) {
	temp := 0.1
	topP := 0.8
	repeatPenalty := 1.2

	ctx := WithParams(context.Background(), Params{Temperature: &temp})
	ctx = WithParams(ctx, Params{TopP: &topP, RepeatPenalty: &repeatPenalty})

	got := ParamsFromContext(ctx)
	if got.Temperature == nil || *got.Temperature != temp {
		t.Fatalf("temperature not preserved: %#v", got.Temperature)
	}
	if got.TopP == nil || *got.TopP != topP {
		t.Fatalf("top_p not set: %#v", got.TopP)
	}
	if got.RepeatPenalty == nil || *got.RepeatPenalty != repeatPenalty {
		t.Fatalf("repeat penalty not set: %#v", got.RepeatPenalty)
	}
}
