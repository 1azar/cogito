package workflow

// State represents any workflow state - can be any type
type State interface{}

const (
	// StartNode is the implicit entry point for workflows
	StartNode = "__start__"
	// EndNode marks the terminal state of a workflow
	EndNode = "__end__"
)

// GraphConfig holds workflow configuration
type GraphConfig struct {
	// MaxIterations is the maximum number of node executions before returning an error
	MaxIterations int
	// ErrorHandler is called when a node returns an error.
	// If it returns a non-nil error, execution stops.
	// If it returns a modified state and nil error, execution continues.
	ErrorHandler func(nodeID string, err error, state State) (State, error)
}

// DefaultConfig returns a GraphConfig with sensible defaults
func DefaultConfig() GraphConfig {
	return GraphConfig{
		MaxIterations: 100,
		ErrorHandler:  nil,
	}
}
