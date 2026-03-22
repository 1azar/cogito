package workflow

import (
	"context"
	"fmt"
)

// Example_manualVerification demonstrates a simple linear workflow
func Example_manualVerification() {
	// Define a simple state type
	type CounterState struct {
		Counter int
	}

	// Create a new graph
	g := NewGraph[*CounterState]()

	// Add an increment node
	g.AddNode(FunctionNode(
		"increment",
		func(ctx context.Context, st State) (State, error) {
			s := st.(*CounterState)
			s.Counter++
			return s, nil
		},
		"Increment counter",
	))

	// Add edge to end and set entry
	g.AddEdge("increment", EndNode)
	g.SetEntry("increment")

	// Run the workflow
	ctx := context.Background()
	result, err := g.Run(ctx, &CounterState{Counter: 0})

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Final counter: %d\n", result.Counter)
	// Output: Final counter: 1
}

// Example_conditionalRouting demonstrates conditional routing based on state
func Example_conditionalRouting() {
	// Define a state type
	type OrderState struct {
		Total     float64
		Discount  float64
		Processed bool
		Approved  bool
	}

	g := NewGraph[*OrderState]()

	// Calculate discount
	calcDiscount := FunctionNode(
		"calcDiscount",
		func(ctx context.Context, st State) (State, error) {
			state := st.(*OrderState)
			if state.Total >= 100 {
				state.Discount = state.Total * 0.1
			} else {
				state.Discount = 0
			}
			return state, nil
		},
		"Calculate discount",
	)

	// Approve order
	approve := FunctionNode(
		"approve",
		func(ctx context.Context, st State) (State, error) {
			state := st.(*OrderState)
			state.Approved = true
			state.Processed = true
			return state, nil
		},
		"Approve order",
	)

	// Reject order
	reject := FunctionNode(
		"reject",
		func(ctx context.Context, st State) (State, error) {
			state := st.(*OrderState)
			state.Approved = false
			state.Processed = true
			return state, nil
		},
		"Reject order",
	)

	g.AddNode(calcDiscount).
		AddNode(approve).
		AddNode(reject).
		AddConditionalEdge("calcDiscount", func(s *OrderState) (string, error) {
			if s.Total >= 50 {
				return "approve", nil
			}
			return "reject", nil
		}, map[string]string{
			"approve": "approve",
			"reject":  "reject",
		}).
		AddEdge("approve", EndNode).
		AddEdge("reject", EndNode).
		SetEntry("calcDiscount")

	// Test with a large order
	ctx := context.Background()
	result, _ := g.Run(ctx, &OrderState{Total: 150})
	fmt.Printf("Order $150: Approved=%v, Discount=$%.2f\n", result.Approved, result.Discount)

	// Test with a small order
	result2, _ := g.Run(ctx, &OrderState{Total: 25})
	fmt.Printf("Order $25: Approved=%v, Discount=$%.2f\n", result2.Approved, result2.Discount)

	// Output:
	// Order $150: Approved=true, Discount=$15.00
	// Order $25: Approved=false, Discount=$0.00
}
