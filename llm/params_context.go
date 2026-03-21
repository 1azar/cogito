package llm

import "context"

type paramsContextKey struct{}

func WithParams(ctx context.Context, params Params) context.Context {
	base := ParamsFromContext(ctx)
	merged := base.Merge(params)
	return context.WithValue(ctx, paramsContextKey{}, merged)
}

func ParamsFromContext(ctx context.Context) Params {
	if ctx == nil {
		return Params{}
	}

	params, ok := ctx.Value(paramsContextKey{}).(Params)
	if !ok {
		return Params{}
	}

	return params
}

func (p Params) Merge(override Params) Params {
	out := p

	if override.Temperature != nil {
		out.Temperature = override.Temperature
	}
	if override.TopP != nil {
		out.TopP = override.TopP
	}
	if override.RepeatPenalty != nil {
		out.RepeatPenalty = override.RepeatPenalty
	}
	if override.MaxTokens != nil {
		out.MaxTokens = override.MaxTokens
	}
	if len(override.Stop) > 0 {
		out.Stop = override.Stop
	}

	return out
}
