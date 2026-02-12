package react

import (
	"context"
	"errors"
	"fmt"

	"github.com/1azar/cogito/controller"
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
		// Call LLM with tools available
		completion, err := agent.CallLLMWithTools(ctx, input)
		if err != nil {
			return "", fmt.Errorf("step %d: LLM call failed: %w", step, err)
		}

		// If no tool calls were made, we have the final answer
		if len(completion.ToolCalls) == 0 {
			return completion.Text, nil
		}

		// Execute tool calls and add results to memory
		for _, toolCall := range completion.ToolCalls {
			// Execute the tool
			result, err := agent.ExecuteTool(ctx, toolCall.Name, string(toolCall.Arguments))
			if err != nil {
				return "", fmt.Errorf("step %d: tool %s failed: %w", step, toolCall.Name, err)
			}

			// Add tool result message to memory for next iteration
			agent.AddToolMessage(ctx, toolCall.ID, result)
		}

		// For subsequent iterations, use empty string since memory has the full context
		input = ""
	}

	return "", errors.New("max steps reached without final answer")
}

var _ controller.Controller[any] = (*Controller[any])(nil)
