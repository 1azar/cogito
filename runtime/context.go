package runtime

import "context"

type contextKey string

const (
	runIDContextKey    contextKey = "cogito_run_id"
	eventBusContextKey contextKey = "cogito_event_bus"
	stepContextKey     contextKey = "cogito_step"
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

func WithEventBus(ctx context.Context, bus EventBus) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, eventBusContextKey, bus)
}

func EventBusFromContext(ctx context.Context) (EventBus, bool) {
	if ctx == nil {
		return nil, false
	}
	v := ctx.Value(eventBusContextKey)
	bus, ok := v.(EventBus)
	if !ok || bus == nil {
		return nil, false
	}
	return bus, true
}

func WithStep(ctx context.Context, step int) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, stepContextKey, step)
}

func StepFromContext(ctx context.Context) (int, bool) {
	if ctx == nil {
		return 0, false
	}
	v := ctx.Value(stepContextKey)
	step, ok := v.(int)
	if !ok {
		return 0, false
	}
	return step, true
}
