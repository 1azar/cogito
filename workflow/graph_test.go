package workflow

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestState is a simple state type for testing
type TestState struct {
	Counter int
	Value   string
	Error   bool
}

// TestSimpleLinearGraph tests a basic linear workflow with three nodes
func TestSimpleLinearGraph(t *testing.T) {
	g := NewGraph[*TestState]()

	// Create nodes
	increment := NewSimpleNode(
		"increment",
		func(ctx context.Context, state State) (State, error) {
			s := state.(*TestState)
			s.Counter++
			return s, nil
		},
		"Increment counter",
	)

	setValue := NewSimpleNode(
		"setValue",
		func(ctx context.Context, state State) (State, error) {
			s := state.(*TestState)
			s.Value = "processed"
			return s, nil
		},
		"Set value",
	)

	doubleIt := NewSimpleNode(
		"doubleIt",
		func(ctx context.Context, state State) (State, error) {
			s := state.(*TestState)
			s.Counter *= 2
			return s, nil
		},
		"Double counter",
	)

	// Build graph: increment -> setValue -> doubleIt -> END
	g.AddNode("increment", increment).
		AddNode("setValue", setValue).
		AddNode("doubleIt", doubleIt).
		AddEdge("increment", "setValue").
		AddEdge("setValue", "doubleIt").
		AddEdge("doubleIt", EndNode).
		SetEntry("increment")

	// Run the graph
	ctx := context.Background()
	initialState := &TestState{Counter: 1}
	result, err := g.Run(ctx, initialState)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if result.Counter != 4 { // (1 + 1) * 2 = 4
		t.Errorf("expected Counter=4, got %d", result.Counter)
	}

	if result.Value != "processed" {
		t.Errorf("expected Value='processed', got '%s'", result.Value)
	}
}

// TestConditionalRouting tests conditional edges based on state
func TestConditionalRouting(t *testing.T) {
	g := NewGraph[*TestState]()

	// Create nodes
	decide := NewSimpleNode(
		"decide",
		func(ctx context.Context, state State) (State, error) {
			s := state.(*TestState)
			s.Counter++
			return s, nil
		},
		"Decide path",
	)

	lowPath := NewSimpleNode(
		"low",
		func(ctx context.Context, state State) (State, error) {
			s := state.(*TestState)
			s.Value = "low"
			return s, nil
		},
		"Low path",
	)

	highPath := NewSimpleNode(
		"high",
		func(ctx context.Context, state State) (State, error) {
			s := state.(*TestState)
			s.Value = "high"
			return s, nil
		},
		"High path",
	)

	// Build graph with conditional routing
	g.AddNode("decide", decide).
		AddNode("low", lowPath).
		AddNode("high", highPath).
		AddConditionalEdge("decide", func(s *TestState) (string, error) {
			if s.Counter < 5 {
				return "low", nil
			}
			return "high", nil
		}, map[string]string{
			"low":  "low",
			"high": "high",
		}).
		AddEdge("low", EndNode).
		AddEdge("high", EndNode).
		SetEntry("decide")

	// Test low path
	t.Run("LowPath", func(t *testing.T) {
		ctx := context.Background()
		initialState := &TestState{Counter: 1}
		result, err := g.Run(ctx, initialState)

		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if result.Value != "low" {
			t.Errorf("expected Value='low', got '%s'", result.Value)
		}

		if result.Counter != 2 {
			t.Errorf("expected Counter=2, got %d", result.Counter)
		}
	})

	// Test high path
	t.Run("HighPath", func(t *testing.T) {
		ctx := context.Background()
		initialState := &TestState{Counter: 10}
		result, err := g.Run(ctx, initialState)

		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if result.Value != "high" {
			t.Errorf("expected Value='high', got '%s'", result.Value)
		}

		if result.Counter != 11 {
			t.Errorf("expected Counter=11, got %d", result.Counter)
		}
	})
}

// TestMaxIterations tests the MaxIterations limit
func TestMaxIterations(t *testing.T) {
	g := NewGraph[*TestState]()

	// Create a node that doesn't progress
	loopNode := NewSimpleNode(
		"loop",
		func(ctx context.Context, state State) (State, error) {
			return state, nil
		},
		"Loop node",
	)

	g.AddNode("loop", loopNode).
		AddEdge("loop", "loop"). // Self-loop
		SetEntry("loop")

	g.SetConfig(GraphConfig{
		MaxIterations: 5,
		ErrorHandler:  nil,
	})

	ctx := context.Background()
	initialState := &TestState{}
	_, err := g.Run(ctx, initialState)

	if err == nil {
		t.Fatal("expected max iterations error, got nil")
	}

	if err.Error() != "max iterations (5) exceeded, current node: loop" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestInvalidGraph tests graph validation errors
func TestInvalidGraph(t *testing.T) {
	tests := []struct {
		name       string
		buildGraph func() *Graph[*TestState]
		wantErr    string
	}{
		{
			name: "Missing entry node",
			buildGraph: func() *Graph[*TestState] {
				g := NewGraph[*TestState]()
				node := NewSimpleNode("node1", func(ctx context.Context, state State) (State, error) {
					return state, nil
				}, "Test node")
				g.AddNode("node1", node).AddEdge("node1", EndNode)
				return g
			},
			wantErr: "graph validation failed: entry node not set",
		},
		{
			name: "Entry node does not exist",
			buildGraph: func() *Graph[*TestState] {
				g := NewGraph[*TestState]()
				node := NewSimpleNode("node1", func(ctx context.Context, state State) (State, error) {
					return state, nil
				}, "Test node")
				g.AddNode("node1", node).SetEntry("nonexistent")
				return g
			},
			wantErr: "graph validation failed: entry node 'nonexistent' does not exist",
		},
		{
			name: "Edge from non-existent node",
			buildGraph: func() *Graph[*TestState] {
				g := NewGraph[*TestState]()
				g.SetEntry("node1")
				return g
			},
			wantErr: "graph validation failed: entry node 'node1' does not exist",
		},
		{
			name: "Edge to non-existent node",
			buildGraph: func() *Graph[*TestState] {
				g := NewGraph[*TestState]()
				node := NewSimpleNode("node1", func(ctx context.Context, state State) (State, error) {
					return state, nil
				}, "Test node")
				g.AddNode("node1", node).
					AddEdge("node1", "nonexistent").
					SetEntry("node1")
				return g
			},
			wantErr: "graph validation failed: edge 0 (node1): target node 'nonexistent' does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := tt.buildGraph()
			ctx := context.Background()
			_, err := g.Run(ctx, &TestState{})

			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if err.Error() != tt.wantErr {
				t.Errorf("expected error '%s', got '%s'", tt.wantErr, err.Error())
			}
		})
	}
}

// TestContextCancellation tests that context cancellation stops execution
func TestContextCancellation(t *testing.T) {
	g := NewGraph[*TestState]()

	cancelCalled := false

	slowNode := NewSimpleNode(
		"slow",
		func(ctx context.Context, state State) (State, error) {
			cancelCalled = true
			// Wait for cancellation
			<-ctx.Done()
			return state, ctx.Err()
		},
		"Slow node",
	)

	g.AddNode("slow", slowNode).
		AddEdge("slow", "slow").
		SetEntry("slow")

	ctx, cancel := context.WithCancel(context.Background())

	// Start execution in background
	errChan := make(chan error, 1)
	go func() {
		_, err := g.Run(ctx, &TestState{})
		errChan <- err
	}()

	// Give the node time to start
	time.Sleep(10 * time.Millisecond)

	// Cancel the context
	cancel()

	// Wait for error
	err := <-errChan

	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}

	if !cancelCalled {
		t.Error("node was not called before cancellation")
	}
}

// TestGraphBuilder tests the builder pattern and chaining
func TestGraphBuilder(t *testing.T) {
	g := NewGraph[*TestState]()

	node1 := NewSimpleNode("node1", func(ctx context.Context, state State) (State, error) {
		return state, nil
	}, "Node 1")

	node2 := NewSimpleNode("node2", func(ctx context.Context, state State) (State, error) {
		return state, nil
	}, "Node 2")

	// Test chaining
	result := g.
		AddNode("node1", node1).
		AddNode("node2", node2).
		AddEdge("node1", "node2").
		AddEdge("node2", EndNode).
		SetEntry("node1").
		SetConfig(GraphConfig{
			MaxIterations: 50,
		})

	if result == nil {
		t.Fatal("builder returned nil")
	}

	// Verify configuration
	if result.GetEntry() != "node1" {
		t.Errorf("expected entry 'node1', got '%s'", result.GetEntry())
	}

	nodes := result.GetNodes()
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}

	edges := result.GetEdges()
	if len(edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(edges))
	}
}

// TestErrorHandler tests the error handler functionality
func TestErrorHandler(t *testing.T) {
	g := NewGraph[*TestState]()

	expectedErr := errors.New("node error")

	errorNode := NewSimpleNode(
		"errorNode",
		func(ctx context.Context, state State) (State, error) {
			return state, expectedErr
		},
		"Error node",
	)

	handlerCalled := false
	var handledNodeID string
	var handledErr error

	errorHandler := func(nodeID string, err error, state State) (State, error) {
		handlerCalled = true
		handledNodeID = nodeID
		handledErr = err
		s := state.(*TestState)
		s.Counter = 999
		return s, nil
	}

	g.AddNode("errorNode", errorNode).
		AddEdge("errorNode", EndNode).
		SetEntry("errorNode").
		SetConfig(GraphConfig{
			MaxIterations: 10,
			ErrorHandler:  errorHandler,
		})

	ctx := context.Background()
	result, err := g.Run(ctx, &TestState{})

	if err != nil {
		t.Fatalf("expected no error after handler, got: %v", err)
	}

	if !handlerCalled {
		t.Error("error handler was not called")
	}

	if handledNodeID != "errorNode" {
		t.Errorf("expected nodeID 'errorNode', got '%s'", handledNodeID)
	}

	if handledErr != expectedErr {
		t.Errorf("expected original error, got: %v", handledErr)
	}

	if result.Counter != 999 {
		t.Errorf("expected Counter=999 from handler, got %d", result.Counter)
	}
}

// TestErrorHandlerError tests that errors from error handler are propagated
func TestErrorHandlerError(t *testing.T) {
	g := NewGraph[*TestState]()

	errorNode := NewSimpleNode(
		"errorNode",
		func(ctx context.Context, state State) (State, error) {
			return state, errors.New("node error")
		},
		"Error node",
	)

	errorHandler := func(nodeID string, err error, state State) (State, error) {
		return state, errors.New("handler error")
	}

	g.AddNode("errorNode", errorNode).
		AddEdge("errorNode", EndNode).
		SetEntry("errorNode").
		SetConfig(GraphConfig{
			MaxIterations: 10,
			ErrorHandler:  errorHandler,
		})

	ctx := context.Background()
	_, err := g.Run(ctx, &TestState{})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.Error() != "node 'errorNode' error: node error (error handler also failed: handler error)" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestNodeDescription tests that node descriptions work correctly
func TestNodeDescription(t *testing.T) {
	node := NewSimpleNode("test", func(ctx context.Context, state State) (State, error) {
		return state, nil
	}, "Test description")

	if node.Description() != "Test description" {
		t.Errorf("expected 'Test description', got '%s'", node.Description())
	}

	// Test empty description fallback
	emptyNode := NewSimpleNode("empty", func(ctx context.Context, state State) (State, error) {
		return state, nil
	}, "")

	if emptyNode.Description() != "Node: empty" {
		t.Errorf("expected 'Node: empty', got '%s'", emptyNode.Description())
	}
}

// TestConditionalEdgeWithInvalidDecision tests conditional edge with invalid decision
func TestConditionalEdgeWithInvalidDecision(t *testing.T) {
	g := NewGraph[*TestState]()

	node1 := NewSimpleNode("node1", func(ctx context.Context, state State) (State, error) {
		return state, nil
	}, "Node 1")

	g.AddNode("node1", node1).
		AddConditionalEdge("node1", func(s *TestState) (string, error) {
			return "invalid", nil
		}, map[string]string{
			"valid": "node1",
		}).
		SetEntry("node1")

	ctx := context.Background()
	_, err := g.Run(ctx, &TestState{})

	if err == nil {
		t.Fatal("expected error for invalid decision, got nil")
	}

	if err.Error() != "error finding next node from 'node1': invalid decision 'invalid' from edge node1: no matching target" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestConditionalEdgeWithRouterError tests conditional edge when router returns error
func TestConditionalEdgeWithRouterError(t *testing.T) {
	g := NewGraph[*TestState]()

	node1 := NewSimpleNode("node1", func(ctx context.Context, state State) (State, error) {
		return state, nil
	}, "Node 1")

	g.AddNode("node1", node1).
		AddConditionalEdge("node1", func(s *TestState) (string, error) {
			return "", errors.New("router error")
		}, map[string]string{
			"valid": "node1",
		}).
		SetEntry("node1")

	ctx := context.Background()
	_, err := g.Run(ctx, &TestState{})

	if err == nil {
		t.Fatal("expected router error, got nil")
	}

	if err.Error() != "error finding next node from 'node1': router error in edge from node1: router error" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestNoEdgesToEndNode tests that a node with no edges automatically goes to END
func TestNoEdgesToEndNode(t *testing.T) {
	g := NewGraph[*TestState]()

	node1 := NewSimpleNode("node1", func(ctx context.Context, state State) (State, error) {
		s := state.(*TestState)
		s.Counter = 42
		return s, nil
	}, "Node 1")

	g.AddNode("node1", node1).
		SetEntry("node1")

	ctx := context.Background()
	result, err := g.Run(ctx, &TestState{})

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if result.Counter != 42 {
		t.Errorf("expected Counter=42, got %d", result.Counter)
	}
}

// TestPanicOnNilNode tests that adding nil node panics
func TestPanicOnNilNode(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil node, got none")
		}
	}()

	g := NewGraph[*TestState]()
	g.AddNode("test", nil)
}

// TestPanicOnNodeIDMismatch tests that node ID mismatch panics
func TestPanicOnNodeIDMismatch(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on node ID mismatch, got none")
		}
	}()

	g := NewGraph[*TestState]()
	node := NewSimpleNode("actualID", func(ctx context.Context, state State) (State, error) {
		return state, nil
	}, "Test")
	g.AddNode("differentID", node)
}

// TestPanicOnDuplicateNode tests that adding duplicate node panics
func TestPanicOnDuplicateNode(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate node, got none")
		}
	}()

	g := NewGraph[*TestState]()
	node := NewSimpleNode("test", func(ctx context.Context, state State) (State, error) {
		return state, nil
	}, "Test")
	g.AddNode("test", node).AddNode("test", node)
}

// TestFunctionNode tests the FunctionNode convenience function
func TestFunctionNode(t *testing.T) {
	node := FunctionNode("test", func(ctx context.Context, state State) (State, error) {
		return state, nil
	}, "Convenience test")

	if node.ID() != "test" {
		t.Errorf("expected ID 'test', got '%s'", node.ID())
	}

	if node.Description() != "Convenience test" {
		t.Errorf("expected description 'Convenience test', got '%s'", node.Description())
	}
}
