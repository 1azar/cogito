package workflow

import (
	"context"
	"fmt"
	"time"

	cogruntime "github.com/1azar/cogito/runtime"
)

type observerContextKey struct{}

type eventBusContextKey struct{}

// Run executes the workflow graph starting from the entry node
func (g *Graph[T]) Run(ctx context.Context, initialState T) (T, error) {
	g.mu.RLock()
	observer := g.config.Observer
	eventBus := g.config.EventBus
	entry := g.entry
	g.mu.RUnlock()

	if observer == nil {
		if inheritedObserver, ok := observerFromContext(ctx); ok {
			observer = inheritedObserver
		}
	}

	if eventBus == nil {
		if inheritedEventBus, ok := eventBusFromContext(ctx); ok {
			eventBus = inheritedEventBus
		}
	}

	ctx = withObserver(ctx, observer)
	ctx = withEventBus(ctx, eventBus)

	runID, ok := cogruntime.RunIDFromContext(ctx)
	if !ok {
		runID = cogruntime.NewRunID("wf")
		ctx = cogruntime.WithRunID(ctx, runID)
	}

	// Validate the graph before execution
	if err := g.Validate(); err != nil {
		var zero T
		g.emitEvent(ctx, observer, eventBus, Event{Type: EventWorkflowFailed, NodeID: entry, Err: err})
		return zero, fmt.Errorf("graph validation failed: %w", err)
	}

	g.mu.RLock()
	maxIterations := g.config.MaxIterations
	errorHandler := g.config.ErrorHandler
	observer = g.config.Observer
	eventBus = g.config.EventBus
	entry = g.entry
	g.mu.RUnlock()

	if observer == nil {
		if inheritedObserver, ok := observerFromContext(ctx); ok {
			observer = inheritedObserver
		}
	}

	if eventBus == nil {
		if inheritedEventBus, ok := eventBusFromContext(ctx); ok {
			eventBus = inheritedEventBus
		}
	}

	ctx = withObserver(ctx, observer)
	ctx = withEventBus(ctx, eventBus)

	g.emitEvent(ctx, observer, eventBus, Event{Type: EventWorkflowStarted, NodeID: entry})

	state := initialState
	currentNode := entry

	for iteration := 0; iteration < maxIterations; iteration++ {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			g.emitEvent(ctx, observer, eventBus, Event{Type: EventWorkflowCanceled, NodeID: currentNode, Iteration: iteration, Err: ctx.Err()})
			return state, fmt.Errorf("execution cancelled: %w", ctx.Err())
		default:
		}

		// Check if we've reached the end
		if currentNode == EndNode {
			g.emitEvent(ctx, observer, eventBus, Event{Type: EventWorkflowFinished, Iteration: iteration})
			return state, nil
		}

		// Get the current node
		g.mu.RLock()
		node, exists := g.nodes[currentNode]
		g.mu.RUnlock()

		if !exists {
			err := fmt.Errorf("node '%s' not found", currentNode)
			g.emitEvent(ctx, observer, eventBus, Event{Type: EventWorkflowFailed, NodeID: currentNode, Iteration: iteration, Err: err})
			return state, err
		}

		startedAt := time.Now()
		desc := node.Description()
		g.emitEvent(ctx, observer, eventBus, Event{
			Type:            EventNodeStarted,
			NodeID:          currentNode,
			NodeDescription: desc,
			Iteration:       iteration,
			StartedAt:       startedAt,
		})

		// Execute the node
		newState, err := node.Execute(ctx, state)
		duration := time.Since(startedAt)
		if err != nil {
			g.emitEvent(ctx, observer, eventBus, Event{
				Type:            EventNodeFailed,
				NodeID:          currentNode,
				NodeDescription: desc,
				Iteration:       iteration,
				StartedAt:       startedAt,
				Duration:        duration,
				Err:             err,
			})

			if errorHandler != nil {
				// Call error handler
				handledState, handlerErr := errorHandler(currentNode, err, state)
				if handlerErr != nil {
					resultErr := fmt.Errorf("node '%s' error: %w (error handler also failed: %v)", currentNode, err, handlerErr)
					g.emitEvent(ctx, observer, eventBus, Event{Type: EventWorkflowFailed, NodeID: currentNode, Iteration: iteration, Err: resultErr})
					return state, resultErr
				}
				// Type assert the handled state back to T
				var ok bool
				state, ok = handledState.(T)
				if !ok {
					err := fmt.Errorf("error handler returned invalid state type")
					g.emitEvent(ctx, observer, eventBus, Event{Type: EventWorkflowFailed, NodeID: currentNode, Iteration: iteration, Err: err})
					return state, err
				}
			} else {
				resultErr := fmt.Errorf("node '%s' execution error: %w", currentNode, err)
				g.emitEvent(ctx, observer, eventBus, Event{Type: EventWorkflowFailed, NodeID: currentNode, Iteration: iteration, Err: resultErr})
				return state, resultErr
			}
		} else {
			g.emitEvent(ctx, observer, eventBus, Event{
				Type:            EventNodeFinished,
				NodeID:          currentNode,
				NodeDescription: desc,
				Iteration:       iteration,
				StartedAt:       startedAt,
				Duration:        duration,
			})

			// Type assert the new state back to T
			var ok bool
			state, ok = newState.(T)
			if !ok {
				err := fmt.Errorf("node execution returned invalid state type")
				g.emitEvent(ctx, observer, eventBus, Event{Type: EventWorkflowFailed, NodeID: currentNode, Iteration: iteration, Err: err})
				return state, err
			}
		}

		// Find the next node
		nextNode, err := g.findNextNode(currentNode, state)
		if err != nil {
			resultErr := fmt.Errorf("error finding next node from '%s': %w", currentNode, err)
			g.emitEvent(ctx, observer, eventBus, Event{Type: EventWorkflowFailed, NodeID: currentNode, Iteration: iteration, Err: resultErr})
			return state, resultErr
		}

		g.emitEvent(ctx, observer, eventBus, Event{Type: EventEdgeEvaluated, NodeID: currentNode, NextNode: nextNode, Iteration: iteration})

		currentNode = nextNode
	}

	err := fmt.Errorf("max iterations (%d) exceeded, current node: %s", maxIterations, currentNode)
	g.emitEvent(ctx, observer, eventBus, Event{Type: EventWorkflowFailed, NodeID: currentNode, Err: err})
	return state, err
}

func (g *Graph[T]) emitEvent(ctx context.Context, observer Observer, eventBus cogruntime.EventBus, event Event) {
	if observer == nil {
		// keep going to event bus path
	} else {
		defer func() {
			_ = recover()
		}()

		observer.OnEvent(ctx, event)
	}

	if eventBus == nil {
		return
	}

	runID, _ := cogruntime.RunIDFromContext(ctx)
	ts := event.StartedAt
	if ts.IsZero() {
		ts = time.Now()
	}
	eventBus.Publish(ctx, cogruntime.Event{
		Timestamp: ts,
		Type:      mapWorkflowEventType(event.Type),
		RunID:     runID,
		Component: "workflow",
		NodeID:    event.NodeID,
		Step:      event.Iteration,
		Duration:  event.Duration,
		Err:       event.Err,
		Message:   event.NodeDescription,
	})
}

func mapWorkflowEventType(eventType EventType) cogruntime.EventType {
	switch eventType {
	case EventNodeStarted:
		return cogruntime.EventNodeStarted
	case EventNodeFinished:
		return cogruntime.EventNodeFinished
	case EventNodeFailed:
		return cogruntime.EventNodeFailed
	case EventEdgeEvaluated:
		return cogruntime.EventEdgeEvaluated
	case EventWorkflowStarted:
		return cogruntime.EventRunStarted
	case EventWorkflowFinished:
		return cogruntime.EventRunFinished
	case EventWorkflowCanceled:
		return cogruntime.EventRunCanceled
	case EventWorkflowFailed:
		return cogruntime.EventRunFailed
	default:
		return cogruntime.EventRunFailed
	}
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

func withObserver(ctx context.Context, observer Observer) context.Context {
	return context.WithValue(ctx, observerContextKey{}, observer)
}

func observerFromContext(ctx context.Context) (Observer, bool) {
	observer, ok := ctx.Value(observerContextKey{}).(Observer)
	if !ok || observer == nil {
		return nil, false
	}
	return observer, true
}

func withEventBus(ctx context.Context, eventBus cogruntime.EventBus) context.Context {
	return context.WithValue(ctx, eventBusContextKey{}, eventBus)
}

func eventBusFromContext(ctx context.Context) (cogruntime.EventBus, bool) {
	eventBus, ok := ctx.Value(eventBusContextKey{}).(cogruntime.EventBus)
	if !ok || eventBus == nil {
		return nil, false
	}
	return eventBus, true
}
