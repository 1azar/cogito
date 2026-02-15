package workflow

import (
	"fmt"
	"log"
)

// Validate checks the graph structure for validity
func (g *Graph[T]) Validate() error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Check entry node is set
	if g.entry == "" {
		return fmt.Errorf("entry node not set")
	}

	// Check entry node exists
	if _, exists := g.nodes[g.entry]; !exists {
		return fmt.Errorf("entry node '%s' does not exist", g.entry)
	}

	// Build a set of all valid node IDs
	validNodes := make(map[string]bool)
	for id := range g.nodes {
		validNodes[id] = true
	}
	validNodes[EndNode] = true // EndNode is always valid

	// Track which nodes are targets of edges (for unreachable node detection)
	targetNodes := make(map[string]bool)

	// Validate each edge
	for i, edge := range g.edges {
		from := edge.From()

		// Check source node exists
		if _, exists := g.nodes[from]; !exists {
			return fmt.Errorf("edge %d: source node '%s' does not exist", i, from)
		}

		// Test the edge with nil state to validate target
		// For conditional edges, this may fail if the router requires specific state
		// We do a best-effort check here
		to, err := edge.To(nil)
		if err != nil {
			// If the edge can't determine target with nil state, that's okay for conditional edges
			// We'll do additional validation for conditional edges
			if condEdge, ok := edge.(*ConditionalEdge); ok {
				// For conditional edges, verify all targets exist
				for decision, target := range condEdge.targets {
					if _, exists := validNodes[target]; !exists {
						return fmt.Errorf("edge %d (%s): target node '%s' for decision '%s' does not exist", i, from, target, decision)
					}
					targetNodes[target] = true
				}
				continue
			}
			return fmt.Errorf("edge %d (%s): %w", i, from, err)
		}

		// Check target node exists
		if _, exists := validNodes[to]; !exists {
			return fmt.Errorf("edge %d (%s): target node '%s' does not exist", i, from, to)
		}

		targetNodes[to] = true
	}

	// Check for unreachable nodes (excluding entry node)
	for id := range g.nodes {
		if id == g.entry {
			continue
		}
		if !targetNodes[id] {
			log.Printf("warning: node '%s' appears unreachable (no edges target it)", id)
		}
	}

	// Ensure at least one node exists
	if len(g.nodes) == 0 {
		return fmt.Errorf("graph has no nodes")
	}

	return nil
}
