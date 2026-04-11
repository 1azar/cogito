package workflow

import (
	"context"
	"fmt"
)

// SubgraphNode wraps a child graph so it can be used as a node in a parent graph.
// P is the parent state type, C is the child state type.
type SubgraphNode[P State, C State] struct {
	id          string
	graph       *Graph[C]
	extract     func(State) (C, error)
	inject      func(State, C) (State, error)
	description string
}

// NewSubgraphNode creates a subgraph node for the common case where parent and child
// graphs use the same state type.
func NewSubgraphNode[T State](id string, graph *Graph[T], desc string) *SubgraphNode[T, T] {
	if id == "" {
		panic("subgraph node id cannot be empty")
	}
	if graph == nil {
		panic("subgraph graph cannot be nil")
	}

	return &SubgraphNode[T, T]{
		id:    id,
		graph: graph,
		extract: func(state State) (T, error) {
			typedState, ok := state.(T)
			if !ok {
				var zero T
				return zero, &TypeError{Expected: zero, Got: state}
			}
			return typedState, nil
		},
		inject: func(state State, childState T) (State, error) {
			return childState, nil
		},
		description: desc,
	}
}

// NewSubgraphNodeWithMapper creates a subgraph node for parent/child graphs that use
// different state types.
func NewSubgraphNodeWithMapper[P State, C State](
	id string,
	graph *Graph[C],
	extract func(P) (C, error),
	inject func(P, C) (P, error),
	desc string,
) *SubgraphNode[P, C] {
	if id == "" {
		panic("subgraph node id cannot be empty")
	}
	if graph == nil {
		panic("subgraph graph cannot be nil")
	}
	if extract == nil {
		panic("subgraph extract cannot be nil")
	}
	if inject == nil {
		panic("subgraph inject cannot be nil")
	}

	return &SubgraphNode[P, C]{
		id:    id,
		graph: graph,
		extract: func(state State) (C, error) {
			typedState, ok := state.(P)
			if !ok {
				var zeroParent P
				var zeroChild C
				return zeroChild, &TypeError{Expected: zeroParent, Got: state}
			}
			return extract(typedState)
		},
		inject: func(state State, childState C) (State, error) {
			typedState, ok := state.(P)
			if !ok {
				var zeroParent P
				return nil, &TypeError{Expected: zeroParent, Got: state}
			}
			newParentState, err := inject(typedState, childState)
			if err != nil {
				return nil, err
			}
			return newParentState, nil
		},
		description: desc,
	}
}

// ID returns the node's unique identifier.
func (n *SubgraphNode[P, C]) ID() string {
	return n.id
}

// Execute runs the child graph and injects its result back into parent state.
func (n *SubgraphNode[P, C]) Execute(ctx context.Context, state State) (State, error) {
	childState, err := n.extract(state)
	if err != nil {
		return state, fmt.Errorf("error extracting state for subgraph '%s': %w", n.id, err)
	}

	newChildState, err := n.graph.Run(ctx, childState)
	if err != nil {
		return state, fmt.Errorf("subgraph '%s' execution error: %w", n.id, err)
	}

	newState, err := n.inject(state, newChildState)
	if err != nil {
		return state, fmt.Errorf("error injecting state from subgraph '%s': %w", n.id, err)
	}

	return newState, nil
}

// Description returns a human-readable description of this node.
func (n *SubgraphNode[P, C]) Description() string {
	if n.description == "" {
		return fmt.Sprintf("SubgraphNode: %s", n.id)
	}
	return n.description
}
