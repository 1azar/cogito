package main

import (
	"context"
	"fmt"
	"log"

	"github.com/1azar/cogito/workflow"
)

type PipelineState struct {
	Value   int
	History []string
}

func main() {
	// Child graphs that will be merged into one parent pipeline.
	addTen := newChildGraph("add_ten", "Add 10", func(s *PipelineState) {
		s.Value += 10
		s.History = append(s.History, "add_ten")
	})

	double := newChildGraph("double", "Multiply by 2", func(s *PipelineState) {
		s.Value *= 2
		s.History = append(s.History, "double")
	})

	subtractFour := newChildGraph("subtract_four", "Subtract 4", func(s *PipelineState) {
		s.Value -= 4
		s.History = append(s.History, "subtract_four")
	})

	// Parent graph combines multiple child graphs into a single flow.
	parent := workflow.NewGraph[*PipelineState]()
	parent.AddNode(workflow.NewSubgraphNode("stage_1", addTen, "Stage 1")).
		AddNode(workflow.NewSubgraphNode("stage_2", double, "Stage 2")).
		AddNode(workflow.NewSubgraphNode("stage_3", subtractFour, "Stage 3")).
		AddEdge("stage_1", "stage_2").
		AddEdge("stage_2", "stage_3").
		AddEdge("stage_3", workflow.EndNode).
		SetEntry("stage_1")

	initial := &PipelineState{Value: 7}
	result, err := parent.Run(context.Background(), initial)
	if err != nil {
		log.Fatalf("run failed: %v", err)
	}

	fmt.Printf("Initial value: %d\n", 7)
	fmt.Printf("Final value: %d\n", result.Value)
	fmt.Printf("Execution order: %v\n", result.History)
}

func newChildGraph(nodeID, desc string, transform func(*PipelineState)) *workflow.Graph[*PipelineState] {
	child := workflow.NewGraph[*PipelineState]()
	child.AddNode(workflow.FunctionNode(
		nodeID,
		func(ctx context.Context, st workflow.State) (workflow.State, error) {
			s := st.(*PipelineState)
			transform(s)
			return s, nil
		},
		desc,
	)).AddEdge(nodeID, workflow.EndNode).SetEntry(nodeID)

	return child
}
