package workflow

import (
	"context"
	"fmt"
)

// Node represents a workflow node that can process state
type Node interface {
	// ID returns the unique identifier for this node
	ID() string
	// Execute runs the node's logic with the given state and returns the updated state
	Execute(ctx context.Context, state State) (State, error)
	// Description returns a human-readable description of this node
	Description() string
}

// SimpleNode wraps a function as a Node
type SimpleNode struct {
	id          string
	description string
	fn          func(ctx context.Context, state State) (State, error)
}

// NewSimpleNode creates a new SimpleNode from a function
func NewSimpleNode(id string, fn func(ctx context.Context, state State) (State, error), description string) *SimpleNode {
	if id == "" {
		panic("node id cannot be empty")
	}
	if fn == nil {
		panic("node function cannot be nil")
	}
	return &SimpleNode{
		id:          id,
		description: description,
		fn:          fn,
	}
}

// ID returns the node's unique identifier
func (n *SimpleNode) ID() string {
	return n.id
}

// Execute runs the node's function
func (n *SimpleNode) Execute(ctx context.Context, state State) (State, error) {
	return n.fn(ctx, state)
}

// Description returns the node's description
func (n *SimpleNode) Description() string {
	if n.description == "" {
		return fmt.Sprintf("Node: %s", n.id)
	}
	return n.description
}

// FunctionNode is an alias for NewSimpleNode for convenience
func FunctionNode(id string, fn func(ctx context.Context, state State) (State, error), description string) *SimpleNode {
	return NewSimpleNode(id, fn, description)
}
