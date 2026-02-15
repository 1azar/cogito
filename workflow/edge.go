package workflow

import (
	"fmt"
)

// Edge represents a transition between nodes
type Edge interface {
	// From returns the source node ID
	From() string
	// To determines the next node ID based on the given state
	To(state State) (string, error)
}

// DirectEdge is a fixed transition from one node to another
type DirectEdge struct {
	from string
	to   string
}

// NewDirectEdge creates a new DirectEdge
func NewDirectEdge(from, to string) *DirectEdge {
	if from == "" {
		panic("edge from node cannot be empty")
	}
	if to == "" {
		panic("edge to node cannot be empty")
	}
	return &DirectEdge{
		from: from,
		to:   to,
	}
}

// From returns the source node ID
func (e *DirectEdge) From() string {
	return e.from
}

// To returns the fixed destination node ID
func (e *DirectEdge) To(state State) (string, error) {
	return e.to, nil
}

// ConditionalEdge routes to different nodes based on state
type ConditionalEdge struct {
	from    string
	router  func(State) (string, error)
	targets map[string]string // decision -> nodeID
}

// NewConditionalEdge creates a new ConditionalEdge
func NewConditionalEdge(from string, router func(State) (string, error), targets map[string]string) *ConditionalEdge {
	if from == "" {
		panic("conditional edge from node cannot be empty")
	}
	if router == nil {
		panic("conditional edge router cannot be nil")
	}
	if len(targets) == 0 {
		panic("conditional edge targets cannot be empty")
	}
	return &ConditionalEdge{
		from:    from,
		router:  router,
		targets: targets,
	}
}

// From returns the source node ID
func (e *ConditionalEdge) From() string {
	return e.from
}

// To determines the next node by routing through the decision function
func (e *ConditionalEdge) To(state State) (string, error) {
	decision, err := e.router(state)
	if err != nil {
		return "", fmt.Errorf("router error in edge from %s: %w", e.from, err)
	}

	target, ok := e.targets[decision]
	if !ok {
		return "", fmt.Errorf("invalid decision '%s' from edge %s: no matching target", decision, e.from)
	}

	return target, nil
}
