package workflow

import (
	"fmt"

	"github.com/1azar/cogito/agent"
)

// NewAgentNodeWithField creates an AgentNode using field accessor functions
// This is a convenience function for the common case where the workflow state
// is a struct with input/output fields
//
// Example:
//
//	type MyState struct {
//	    UserInput string
//	    AgentResponse string
//	}
//
//	node := NewAgentNodeWithField(
//	    "agent1",
//	    myAgent,
//	    func(s *MyState) string { return s.UserInput },
//	    func(s *MyState, output string) { s.AgentResponse = output },
//	    "Process user input",
//	)
func NewAgentNodeWithField[T any, S any](
	id string,
	agt *agent.Agent[T],
	inputField func(*S) string,
	outputField func(*S, string),
	desc string,
) Node {
	return NewAgentNode[T](
		id,
		agt,
		func(state State) (string, error) {
			typedState, ok := state.(*S)
			if !ok {
				var zero *S
				return "", &TypeError{Expected: zero, Got: state}
			}
			return inputField(typedState), nil
		},
		func(state State, output string) (State, error) {
			typedState, ok := state.(*S)
			if !ok {
				var zero *S
				return nil, &TypeError{Expected: zero, Got: state}
			}
			outputField(typedState, output)
			return typedState, nil
		},
		desc,
	)
}

// NewAgentNodeWithKey creates an AgentNode that reads from and writes to
// map[string]string state using specific keys
//
// Example:
//
//	node := NewAgentNodeWithKey(
//	    "agent1",
//	    myAgent,
//	    "input",
//	    "output",
//	    "Process input",
//	)
func NewAgentNodeWithKey[T any](
	id string,
	agt *agent.Agent[T],
	inputKey string,
	outputKey string,
	desc string,
) Node {
	return NewAgentNode[T](
		id,
		agt,
		func(state State) (string, error) {
			typedState, ok := state.(map[string]string)
			if !ok {
				return "", &TypeError{Expected: map[string]string{}, Got: state}
			}
			input, exists := typedState[inputKey]
			if !exists {
				return "", fmt.Errorf("input key '%s' not found in state", inputKey)
			}
			return input, nil
		},
		func(state State, output string) (State, error) {
			typedState, ok := state.(map[string]string)
			if !ok {
				return nil, &TypeError{Expected: map[string]string{}, Got: state}
			}
			if typedState == nil {
				typedState = make(map[string]string)
			}
			typedState[outputKey] = output
			return typedState, nil
		},
		desc,
	)
}

// NewAgentNodeWithKeyValue creates an AgentNode that reads from and writes to
// map[string]any state using specific keys (for more complex types)
func NewAgentNodeWithKeyValue[T any](
	id string,
	agt *agent.Agent[T],
	inputKey string,
	outputKey string,
	desc string,
) Node {
	return NewAgentNode[T](
		id,
		agt,
		func(state State) (string, error) {
			typedState, ok := state.(map[string]any)
			if !ok {
				return "", &TypeError{Expected: map[string]any{}, Got: state}
			}
			input, exists := typedState[inputKey]
			if !exists {
				return "", fmt.Errorf("input key '%s' not found in state", inputKey)
			}
			inputStr, ok := input.(string)
			if !ok {
				return "", fmt.Errorf("input key '%s' is not a string", inputKey)
			}
			return inputStr, nil
		},
		func(state State, output string) (State, error) {
			typedState, ok := state.(map[string]any)
			if !ok {
				return state, &TypeError{Expected: map[string]any{}, Got: state}
			}
			if typedState == nil {
				typedState = make(map[string]any)
			}
			typedState[outputKey] = output
			return typedState, nil
		},
		desc,
	)
}

// NewAgentNodeWithExtractorOnly creates an AgentNode that only extracts input
// but doesn't modify the state (useful for agents that don't need to write back)
func NewAgentNodeWithExtractorOnly[T any](
	id string,
	agt *agent.Agent[T],
	extractor func(State) (string, error),
	desc string,
) Node {
	return NewAgentNode[T](
		id,
		agt,
		extractor,
		func(state State, output string) (State, error) {
			// Return state unchanged
			return state, nil
		},
		desc,
	)
}

// NewAgentNodeConst creates an AgentNode that uses a constant string as input
// and injects the output using the provided injector function
func NewAgentNodeConst[T any](
	id string,
	agt *agent.Agent[T],
	constInput string,
	injector func(State, string) (State, error),
	desc string,
) Node {
	return NewAgentNode[T](
		id,
		agt,
		func(state State) (string, error) {
			return constInput, nil
		},
		injector,
		desc,
	)
}
