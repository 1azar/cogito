package workflow

import (
	"context"
	"fmt"
)

// Run executes the workflow graph starting from the entry node
func (g *Graph[T]) Run(ctx context.Context, initialState T) (T, error) {
	// Validate the graph before execution
	if err := g.Validate(); err != nil {
		var zero T
		return zero, fmt.Errorf("graph validation failed: %w", err)
	}

	g.mu.RLock()
	maxIterations := g.config.MaxIterations
	errorHandler := g.config.ErrorHandler
	g.mu.RUnlock()

	state := initialState
	currentNode := g.entry

	for iteration := 0; iteration < maxIterations; iteration++ {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return state, fmt.Errorf("execution cancelled: %w", ctx.Err())
		default:
		}

		// Check if we've reached the end
		if currentNode == EndNode {
			return state, nil
		}

		// Get the current node
		g.mu.RLock()
		node, exists := g.nodes[currentNode]
		g.mu.RUnlock()

		if !exists {
			return state, fmt.Errorf("node '%s' not found", currentNode)
		}

		// Execute the node
		newState, err := node.Execute(ctx, state)
		if err != nil {
			if errorHandler != nil {
				// Call error handler
				handledState, handlerErr := errorHandler(currentNode, err, state)
				if handlerErr != nil {
					return state, fmt.Errorf("node '%s' error: %w (error handler also failed: %v)", currentNode, err, handlerErr)
				}
				// Type assert the handled state back to T
				var ok bool
				state, ok = handledState.(T)
				if !ok {
					return state, fmt.Errorf("error handler returned invalid state type")
				}
			} else {
				return state, fmt.Errorf("node '%s' execution error: %w", currentNode, err)
			}
		} else {
			// Type assert the new state back to T
			var ok bool
			state, ok = newState.(T)
			if !ok {
				return state, fmt.Errorf("node execution returned invalid state type")
			}
		}

		// Find the next node
		nextNode, err := g.findNextNode(currentNode, state)
		if err != nil {
			return state, fmt.Errorf("error finding next node from '%s': %w", currentNode, err)
		}

		currentNode = nextNode
	}

	return state, fmt.Errorf("max iterations (%d) exceeded, current node: %s", maxIterations, currentNode)
}

// findNextNode finds the next node based on edges from the current node
func (g *Graph[T]) findNextNode(fromNode string, state State) (string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Find the edge from the current node
	for _, edge := range g.edges {
		if edge.From() == fromNode {
			return edge.To(state)
		}
	}

	// No outgoing edge means we should end
	return EndNode, nil
}
