package workflow

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	cogruntime "github.com/1azar/cogito/runtime"
)

type subgraphState struct {
	Counter int
}

func TestSubgraphNodeSameState(t *testing.T) {
	child := NewGraph[*subgraphState]()
	child.AddNode(FunctionNode(
		"child_inc",
		func(ctx context.Context, st State) (State, error) {
			s := st.(*subgraphState)
			s.Counter += 2
			return s, nil
		},
		"Child increment",
	)).AddEdge("child_inc", EndNode).SetEntry("child_inc")

	parent := NewGraph[*subgraphState]()
	parent.AddNode(FunctionNode(
		"parent_inc",
		func(ctx context.Context, st State) (State, error) {
			s := st.(*subgraphState)
			s.Counter++
			return s, nil
		},
		"Parent increment",
	)).
		AddNode(NewSubgraphNode("pipeline", child, "Child pipeline")).
		AddEdge("parent_inc", "pipeline").
		AddEdge("pipeline", EndNode).
		SetEntry("parent_inc")

	result, err := parent.Run(context.Background(), &subgraphState{Counter: 1})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if result.Counter != 4 {
		t.Fatalf("expected counter=4, got %d", result.Counter)
	}
}

func TestSubgraphNodeWithMapper(t *testing.T) {
	type parentState struct {
		Input  int
		Output int
	}
	type childState struct {
		Value int
	}

	child := NewGraph[*childState]()
	child.AddNode(FunctionNode(
		"double",
		func(ctx context.Context, st State) (State, error) {
			s := st.(*childState)
			s.Value *= 2
			return s, nil
		},
		"Double value",
	)).AddEdge("double", EndNode).SetEntry("double")

	parent := NewGraph[*parentState]()
	parent.AddNode(NewSubgraphNodeWithMapper(
		"mapper_subgraph",
		child,
		func(p *parentState) (*childState, error) {
			return &childState{Value: p.Input}, nil
		},
		func(p *parentState, c *childState) (*parentState, error) {
			p.Output = c.Value
			return p, nil
		},
		"Mapped subgraph",
	)).AddEdge("mapper_subgraph", EndNode).SetEntry("mapper_subgraph")

	result, err := parent.Run(context.Background(), &parentState{Input: 5})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if result.Output != 10 {
		t.Fatalf("expected output=10, got %d", result.Output)
	}
}

func TestSubgraphCombinesMultipleGraphsIntoOne(t *testing.T) {
	makeChild := func(nodeID, desc string, transform func(*subgraphState)) *Graph[*subgraphState] {
		child := NewGraph[*subgraphState]()
		child.AddNode(FunctionNode(
			nodeID,
			func(ctx context.Context, st State) (State, error) {
				s := st.(*subgraphState)
				transform(s)
				return s, nil
			},
			desc,
		)).AddEdge(nodeID, EndNode).SetEntry(nodeID)
		return child
	}

	addTwo := makeChild("add_two", "Add 2", func(s *subgraphState) {
		s.Counter += 2
	})
	multiplyByThree := makeChild("mul_three", "Multiply by 3", func(s *subgraphState) {
		s.Counter *= 3
	})
	subtractFive := makeChild("sub_five", "Subtract 5", func(s *subgraphState) {
		s.Counter -= 5
	})

	parent := NewGraph[*subgraphState]()
	parent.AddNode(NewSubgraphNode("stage_add", addTwo, "Stage 1: +2")).
		AddNode(NewSubgraphNode("stage_mul", multiplyByThree, "Stage 2: *3")).
		AddNode(NewSubgraphNode("stage_sub", subtractFive, "Stage 3: -5")).
		AddEdge("stage_add", "stage_mul").
		AddEdge("stage_mul", "stage_sub").
		AddEdge("stage_sub", EndNode).
		SetEntry("stage_add")

	result, err := parent.Run(context.Background(), &subgraphState{Counter: 4})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// ((4 + 2) * 3) - 5 = 13
	if result.Counter != 13 {
		t.Fatalf("expected counter=13, got %d", result.Counter)
	}
}

func TestSubgraphNodeErrorPropagation(t *testing.T) {
	child := NewGraph[*subgraphState]()
	child.AddNode(FunctionNode(
		"fails",
		func(ctx context.Context, st State) (State, error) {
			return st, errors.New("boom")
		},
		"Failing child",
	)).AddEdge("fails", EndNode).SetEntry("fails")

	parent := NewGraph[*subgraphState]()
	parent.AddNode(NewSubgraphNode("child", child, "Child graph")).
		AddEdge("child", EndNode).
		SetEntry("child")

	_, err := parent.Run(context.Background(), &subgraphState{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "subgraph 'child' execution error") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSubgraphInheritsObserverAndEventBus(t *testing.T) {
	child := NewGraph[*subgraphState]()
	child.AddNode(FunctionNode(
		"inner",
		func(ctx context.Context, st State) (State, error) {
			s := st.(*subgraphState)
			s.Counter++
			return s, nil
		},
		"Inner node",
	)).AddEdge("inner", EndNode).SetEntry("inner")

	var (
		observerMu sync.Mutex
		obsEvents  []Event
		busMu      sync.Mutex
		busEvents  []cogruntime.Event
	)

	bus := cogruntime.NewEventBus()
	bus.Subscribe(func(ctx context.Context, event cogruntime.Event) {
		busMu.Lock()
		defer busMu.Unlock()
		busEvents = append(busEvents, event)
	})

	parentCfg := DefaultConfig()
	parentCfg.Observer = ObserverFunc(func(ctx context.Context, event Event) {
		observerMu.Lock()
		defer observerMu.Unlock()
		obsEvents = append(obsEvents, event)
	})
	parentCfg.EventBus = bus

	parent := NewGraph[*subgraphState]()
	parent.AddNode(NewSubgraphNode("sub", child, "Nested graph")).
		AddEdge("sub", EndNode).
		SetEntry("sub").
		SetConfig(parentCfg)

	_, err := parent.Run(context.Background(), &subgraphState{})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	observerMu.Lock()
	hasInnerObserverEvent := false
	for _, event := range obsEvents {
		if event.NodeID == "inner" {
			hasInnerObserverEvent = true
			break
		}
	}
	observerMu.Unlock()
	if !hasInnerObserverEvent {
		t.Fatal("expected parent observer to receive child node events")
	}

	busMu.Lock()
	defer busMu.Unlock()
	if len(busEvents) == 0 {
		t.Fatal("expected event bus events")
	}

	hasInnerBusEvent := false
	runID := busEvents[0].RunID
	if runID == "" {
		t.Fatal("expected non-empty run id")
	}
	for _, event := range busEvents {
		if event.RunID != runID {
			t.Fatalf("expected single run id, got %q and %q", runID, event.RunID)
		}
		if event.NodeID == "inner" {
			hasInnerBusEvent = true
		}
	}
	if !hasInnerBusEvent {
		t.Fatal("expected inherited event bus to receive child node events")
	}
}

func TestSubgraphChildObserverOverridesParentObserver(t *testing.T) {
	child := NewGraph[*subgraphState]()
	child.AddNode(FunctionNode(
		"inner",
		func(ctx context.Context, st State) (State, error) {
			return st, nil
		},
		"Inner node",
	)).AddEdge("inner", EndNode).SetEntry("inner")

	var (
		parentMu     sync.Mutex
		parentEvents []Event
		childMu      sync.Mutex
		childEvents  []Event
	)

	childCfg := DefaultConfig()
	childCfg.Observer = ObserverFunc(func(ctx context.Context, event Event) {
		childMu.Lock()
		defer childMu.Unlock()
		childEvents = append(childEvents, event)
	})
	child.SetConfig(childCfg)

	parentCfg := DefaultConfig()
	parentCfg.Observer = ObserverFunc(func(ctx context.Context, event Event) {
		parentMu.Lock()
		defer parentMu.Unlock()
		parentEvents = append(parentEvents, event)
	})

	parent := NewGraph[*subgraphState]()
	parent.AddNode(NewSubgraphNode("sub", child, "Nested graph")).
		AddEdge("sub", EndNode).
		SetEntry("sub").
		SetConfig(parentCfg)

	_, err := parent.Run(context.Background(), &subgraphState{})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	parentMu.Lock()
	parentSawInner := false
	for _, event := range parentEvents {
		if event.NodeID == "inner" {
			parentSawInner = true
			break
		}
	}
	parentMu.Unlock()
	if parentSawInner {
		t.Fatal("expected parent observer not to receive child node events when child observer is explicit")
	}

	childMu.Lock()
	defer childMu.Unlock()
	childSawInner := false
	for _, event := range childEvents {
		if event.NodeID == "inner" {
			childSawInner = true
			break
		}
	}
	if !childSawInner {
		t.Fatal("expected child observer to receive child node events")
	}
}
