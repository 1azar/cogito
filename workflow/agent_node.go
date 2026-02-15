package workflow

import (
	"context"
	"fmt"

	"github.com/1azar/cogito/agent"
)

// AgentNode wraps an Agent[T] for use in a workflow graph
// It bridges the workflow State with the Agent's internal state type T
type AgentNode[T any] struct {
	id             string
	agent          *agent.Agent[T]
	stateExtractor func(State) (string, error)
	stateInjector  func(State, string) (State, error)
	description    string
}

// NewAgentNode creates a new AgentNode with full control over state mapping
// The stateExtractor function extracts the input string for the agent from the workflow state
// The stateInjector function injects the agent's output back into the workflow state
func NewAgentNode[T any](
	id string,
	agt *agent.Agent[T],
	extractor func(State) (string, error),
	injector func(State, string) (State, error),
	desc string,
) *AgentNode[T] {
	if id == "" {
		panic("agent node id cannot be empty")
	}
	if agt == nil {
		panic("agent cannot be nil")
	}
	if extractor == nil {
		panic("stateExtractor cannot be nil")
	}
	if injector == nil {
		panic("stateInjector cannot be nil")
	}

	return &AgentNode[T]{
		id:             id,
		agent:          agt,
		stateExtractor: extractor,
		stateInjector:  injector,
		description:    desc,
	}
}

// ID returns the node's unique identifier
func (n *AgentNode[T]) ID() string {
	return n.id
}

// Execute runs the agent with input extracted from the workflow state
// and injects the agent's output back into the state
func (n *AgentNode[T]) Execute(ctx context.Context, state State) (State, error) {
	// Extract input for the agent from the workflow state
	input, err := n.stateExtractor(state)
	if err != nil {
		return state, fmt.Errorf("error extracting input for agent '%s': %w", n.id, err)
	}

	// Run the agent
	output, err := n.agent.Run(ctx, input)
	if err != nil {
		return state, fmt.Errorf("agent '%s' execution error: %w", n.id, err)
	}

	// Inject the output back into the workflow state
	newState, err := n.stateInjector(state, output)
	if err != nil {
		return state, fmt.Errorf("error injecting output from agent '%s': %w", n.id, err)
	}

	return newState, nil
}

// Description returns a human-readable description of this node
func (n *AgentNode[T]) Description() string {
	if n.description == "" {
		return fmt.Sprintf("AgentNode: %s", n.id)
	}
	return n.description
}

// Agent returns the underlying agent for advanced use cases
func (n *AgentNode[T]) Agent() *agent.Agent[T] {
	return n.agent
}
