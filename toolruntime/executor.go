package toolruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	cogruntime "github.com/1azar/cogito/runtime"
	"github.com/1azar/cogito/schema"
	"github.com/1azar/cogito/tool"
)

type Executor interface {
	Execute(ctx context.Context, calls []schema.ToolCall, reg *tool.Registry) ([]Result, error)
}

type DefaultExecutor struct {
	policy     Policy
	middleware []Middleware
}

func NewExecutor(policy Policy, middleware ...Middleware) *DefaultExecutor {
	def := DefaultPolicy()
	p := policy

	if policy == (Policy{}) {
		p = def
	}

	if p.MaxParallel <= 0 {
		p.MaxParallel = def.MaxParallel
	}
	if p.PerToolTimeout <= 0 {
		p.PerToolTimeout = def.PerToolTimeout
	}
	if p.Retry.MaxAttempts <= 0 {
		p.Retry.MaxAttempts = def.Retry.MaxAttempts
	}

	return &DefaultExecutor{
		policy:     p,
		middleware: middleware,
	}
}

func (e *DefaultExecutor) Execute(ctx context.Context, calls []schema.ToolCall, reg *tool.Registry) ([]Result, error) {
	results := make([]Result, len(calls))
	if len(calls) == 0 {
		return results, nil
	}

	workers := e.policy.MaxParallel
	if workers < 1 {
		workers = 1
	}
	if workers > len(calls) {
		workers = len(calls)
	}

	jobs := make(chan int, len(calls))
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				results[idx] = e.executeCall(ctx, calls[idx], reg)
			}
		}()
	}

	for i := range calls {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	if e.policy.ContinueOnError {
		return results, nil
	}

	for _, result := range results {
		if result.Status == StatusError {
			return results, fmt.Errorf(
				"tool %s failed (call_id=%s): %s",
				result.Name,
				result.ToolCallID,
				result.Error.Message,
			)
		}
	}

	return results, nil
}

func (e *DefaultExecutor) executeCall(ctx context.Context, call schema.ToolCall, reg *tool.Registry) Result {
	started := time.Now()
	publishToolEvent(ctx, cogruntime.Event{
		Timestamp:  started,
		Type:       cogruntime.EventToolCallStarted,
		Component:  "toolruntime.executor",
		ToolName:   call.Name,
		ToolCallID: call.ID,
	})

	result := Result{
		ToolCallID: call.ID,
		Name:       call.Name,
		Status:     StatusError,
	}

	t, ok := reg.Get(call.Name)
	if !ok {
		result.Error = &ExecError{
			Code:    "tool_not_found",
			Message: fmt.Sprintf("tool %s not found", call.Name),
		}
		result.Meta.DurationMillis = time.Since(started).Milliseconds()
		result.Meta.Attempts = 1
		publishToolFinishEvent(ctx, result, started)
		return result
	}

	spec := t.Spec()
	if err := ValidateArguments(spec.Parameters, call.Arguments); err != nil {
		result.Error = &ExecError{
			Code:    "validation_error",
			Message: err.Error(),
		}
		result.Meta.DurationMillis = time.Since(started).Milliseconds()
		result.Meta.Attempts = 1
		publishToolFinishEvent(ctx, result, started)
		return result
	}

	handler := e.makeHandler()
	attempts := e.policy.Retry.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	var timedOut bool
	for attempt := 1; attempt <= attempts; attempt++ {
		result.Meta.Attempts = attempt

		callCtx := ctx
		cancel := func() {}
		if e.policy.PerToolTimeout > 0 {
			callCtx, cancel = context.WithTimeout(ctx, e.policy.PerToolTimeout)
		}

		resp := handler(callCtx, CallRequest{
			Call: call,
			Tool: t,
		})
		cancel()

		if resp.Err == nil {
			out, err := json.Marshal(resp.Output)
			if err != nil {
				lastErr = fmt.Errorf("failed to marshal tool result: %w", err)
				break
			}

			result.Status = StatusSuccess
			result.Output = out
			result.Error = nil
			result.Meta.TimedOut = false
			result.Meta.DurationMillis = time.Since(started).Milliseconds()
			publishToolFinishEvent(ctx, result, started)
			return result
		}

		lastErr = resp.Err
		if errors.Is(resp.Err, context.DeadlineExceeded) || errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			timedOut = true
		}
	}

	result.Meta.DurationMillis = time.Since(started).Milliseconds()
	result.Meta.TimedOut = timedOut
	result.Error = classifyExecError(lastErr, timedOut)
	publishToolFinishEvent(ctx, result, started)
	return result
}

func (e *DefaultExecutor) makeHandler() Handler {
	base := func(ctx context.Context, req CallRequest) (resp CallResponse) {
		defer func() {
			if recovered := recover(); recovered != nil {
				resp.Err = fmt.Errorf("tool panicked: %v", recovered)
			}
		}()

		out, err := req.Tool.Call(ctx, req.Call.Arguments)
		resp.Output = out
		resp.Err = err
		return resp
	}

	wrapped := base
	for i := len(e.middleware) - 1; i >= 0; i-- {
		wrapped = e.middleware[i](wrapped)
	}

	return wrapped
}

func classifyExecError(err error, timedOut bool) *ExecError {
	if err == nil {
		return &ExecError{
			Code:    "execution_error",
			Message: "unknown tool execution error",
		}
	}

	if timedOut || errors.Is(err, context.DeadlineExceeded) {
		return &ExecError{
			Code:      "timeout",
			Message:   err.Error(),
			Retryable: true,
		}
	}

	return &ExecError{
		Code:    "execution_error",
		Message: err.Error(),
	}
}

var _ Executor = (*DefaultExecutor)(nil)

func publishToolEvent(ctx context.Context, event cogruntime.Event) {
	bus, ok := cogruntime.EventBusFromContext(ctx)
	if !ok {
		return
	}
	if event.RunID == "" {
		runID, _ := cogruntime.RunIDFromContext(ctx)
		event.RunID = runID
	}
	if event.Step == 0 {
		if step, ok := cogruntime.StepFromContext(ctx); ok {
			event.Step = step
		}
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	bus.Publish(ctx, event)
}

func publishToolFinishEvent(ctx context.Context, result Result, started time.Time) {
	var err error
	if result.Error != nil {
		err = errors.New(result.Error.Message)
	}

	publishToolEvent(ctx, cogruntime.Event{
		Timestamp:  time.Now(),
		Type:       cogruntime.EventToolCallFinished,
		Component:  "toolruntime.executor",
		ToolName:   result.Name,
		ToolCallID: result.ToolCallID,
		Attempts:   result.Meta.Attempts,
		TimedOut:   result.Meta.TimedOut,
		Duration:   time.Since(started),
		Err:        err,
	})
}
