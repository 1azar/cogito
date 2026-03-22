package runtime

import "context"

type contextKey string

const (
	runIDContextKey contextKey = "cogito_run_id"
)

func WithRunID(ctx context.Context, runID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, runIDContextKey, runID)
}

func RunIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	v := ctx.Value(runIDContextKey)
	runID, ok := v.(string)
	if !ok || runID == "" {
		return "", false
	}
	return runID, true
}
