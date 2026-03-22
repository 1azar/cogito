package react

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/1azar/cogito/controller"
	cogruntime "github.com/1azar/cogito/runtime"
)

type Config struct {
	MaxSteps int
}

type Controller[T any] struct {
	maxSteps int
}

func New[T any](cfg Config) *Controller[T] {
	if cfg.MaxSteps <= 0 {
		cfg.MaxSteps = 5
	}

	return &Controller[T]{
		maxSteps: cfg.MaxSteps,
	}
}

// Run implements the ReAct loop:
// Thought -> Action (tool call) -> Observation -> Thought -> ... -> Final Answer
func (c *Controller[T]) Run(ctx context.Context, agent controller.AgentLike[T], input string) (string, error) {
	for step := 0; step < c.maxSteps; step++ {
		stepCtx := cogruntime.WithStep(ctx, step)
		publishEvent(stepCtx, cogruntime.Event{
			Timestamp: time.Now(),
			Type:      cogruntime.EventControllerStepStarted,
			Component: "controller.react",
			Step:      step,
		})

		// Call LLM with tools available
		llmStartedAt := time.Now()
		publishEvent(stepCtx, cogruntime.Event{
			Timestamp: llmStartedAt,
			Type:      cogruntime.EventLLMCallStarted,
			Component: "controller.react",
			Step:      step,
		})

		completion, err := agent.CallLLMWithTools(stepCtx, input)
		if err != nil {
			publishEvent(stepCtx, cogruntime.Event{
				Timestamp: time.Now(),
				Type:      cogruntime.EventLLMCallFinished,
				Component: "controller.react",
				Step:      step,
				Duration:  time.Since(llmStartedAt),
				Err:       err,
			})
			publishEvent(stepCtx, cogruntime.Event{
				Timestamp: time.Now(),
				Type:      cogruntime.EventControllerStepFinished,
				Component: "controller.react",
				Step:      step,
				Err:       err,
			})
			return "", fmt.Errorf("step %d: LLM call failed: %w", step, err)
		}

		publishEvent(stepCtx, cogruntime.Event{
			Timestamp:           time.Now(),
			Type:                cogruntime.EventLLMCallFinished,
			Component:           "controller.react",
			Step:                step,
			Duration:            time.Since(llmStartedAt),
			InputTokens:         completion.Usage.InputTokens,
			OutputTokens:        completion.Usage.OutputTokens,
			TotalTokens:         completion.Usage.TotalTokens,
			EstimatedCostMicros: completion.Usage.EstimatedCostMicros,
		})

		if completion.Text != "" {
			publishEvent(stepCtx, cogruntime.Event{
				Timestamp: time.Now(),
				Type:      cogruntime.EventLLMTextDelta,
				Component: "controller.react",
				Step:      step,
				Message:   completion.Text,
			})
		}

		// If no tool calls were made, we have the final answer
		if len(completion.ToolCalls) == 0 {
			publishEvent(stepCtx, cogruntime.Event{
				Timestamp: time.Now(),
				Type:      cogruntime.EventControllerStepFinished,
				Component: "controller.react",
				Step:      step,
			})
			return completion.Text, nil
		}

		// Execute tool calls via runtime (validation, retry, timeout, parallelism)
		_, err = agent.RunToolCalls(stepCtx, completion.ToolCalls)
		if err != nil {
			publishEvent(stepCtx, cogruntime.Event{
				Timestamp: time.Now(),
				Type:      cogruntime.EventControllerStepFinished,
				Component: "controller.react",
				Step:      step,
				Err:       err,
			})
			return "", fmt.Errorf("step %d: tool execution failed: %w", step, err)
		}

		publishEvent(stepCtx, cogruntime.Event{
			Timestamp: time.Now(),
			Type:      cogruntime.EventControllerStepFinished,
			Component: "controller.react",
			Step:      step,
		})

		// For subsequent iterations, use empty string since memory has the full context
		input = ""
	}

	return "", errors.New("max steps reached without final answer")
}

var _ controller.Controller[any] = (*Controller[any])(nil)

func publishEvent(ctx context.Context, event cogruntime.Event) {
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
