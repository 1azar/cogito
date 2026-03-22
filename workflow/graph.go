package workflow

import (
	"fmt"
	"sync"
)

// Graph is a typed state graph for orchestrating workflows
// T is the State type, which can be any type satisfying the State interface
type Graph[T State] struct {
	nodes  map[string]Node
	edges  []Edge
	entry  string
	config GraphConfig
	mu     sync.RWMutex
}

// NewGraph creates a new Graph with the specified state type
func NewGraph[T State]() *Graph[T] {
	return &Graph[T]{
		nodes:  make(map[string]Node),
		edges:  make([]Edge, 0),
		config: DefaultConfig(),
	}
}

// AddNode adds a node to the graph
func (g *Graph[T]) AddNode(node Node) *Graph[T] {
	g.mu.Lock()
	defer g.mu.Unlock()

	if node == nil {
		panic("cannot add nil node")
	}
	id := node.ID()
	if id == "" {
		panic("node id cannot be empty")
	}
	if _, exists := g.nodes[id]; exists {
		panic("node with id '" + id + "' already exists")
	}

	g.nodes[id] = node
	return g
}

// AddEdge adds a direct edge from one node to another
func (g *Graph[T]) AddEdge(from, to string) *Graph[T] {
	g.mu.Lock()
	defer g.mu.Unlock()

	edge := NewDirectEdge(from, to)
	g.edges = append(g.edges, edge)
	return g
}

// AddConditionalEdge adds a conditional edge that routes based on state
// The router function returns a decision key, which is mapped to a target node ID
func (g *Graph[T]) AddConditionalEdge(from string, router func(T) (string, error), targets map[string]string) *Graph[T] {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Wrap the typed router to work with State interface
	wrappedRouter := func(state State) (string, error) {
		typedState, ok := state.(T)
		if !ok {
			return "", &TypeError{Expected: typedState, Got: state}
		}
		return router(typedState)
	}

	edge := NewConditionalEdge(from, wrappedRouter, targets)
	g.edges = append(g.edges, edge)
	return g
}

// SetEntry sets the entry node for the graph
func (g *Graph[T]) SetEntry(id string) *Graph[T] {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.entry = id
	return g
}

// SetConfig sets the graph configuration
func (g *Graph[T]) SetConfig(config GraphConfig) *Graph[T] {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.config = config
	return g
}

// GetNodes returns a copy of the nodes map (for testing/validation)
func (g *Graph[T]) GetNodes() map[string]Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	nodes := make(map[string]Node, len(g.nodes))
	for k, v := range g.nodes {
		nodes[k] = v
	}
	return nodes
}

// GetEdges returns a copy of the edges slice (for testing/validation)
func (g *Graph[T]) GetEdges() []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	edges := make([]Edge, len(g.edges))
	copy(edges, g.edges)
	return edges
}

// GetEntry returns the entry node ID
func (g *Graph[T]) GetEntry() string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return g.entry
}

// TypeError is returned when a state type assertion fails
type TypeError struct {
	Expected any
	Got      any
}

func (e *TypeError) Error() string {
	return fmt.Sprintf("type error: expected %T, got %T", e.Expected, e.Got)
}
