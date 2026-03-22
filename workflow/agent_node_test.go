package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/1azar/cogito/agent"
	"github.com/1azar/cogito/controller/simple"
	"github.com/1azar/cogito/llm/mock"
	"github.com/1azar/cogito/memory/buffer"
)

// TestAgentNodeBasic tests basic agent node functionality
func TestAgentNodeBasic(t *testing.T) {
	llm := mock.New()
	agt := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]())

	node := NewAgentNode[struct{}](
		"testAgent",
		agt,
		func(state State) (string, error) {
			return "hello", nil
		},
		func(state State, output string) (State, error) {
			s := state.(map[string]string)
			if s == nil {
				s = make(map[string]string)
			}
			s["result"] = output
			return s, nil
		},
		"Test agent node",
	)

	ctx := context.Background()
	result, err := node.Execute(ctx, make(map[string]string))

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	typedResult, ok := result.(map[string]string)
	if !ok {
		t.Fatal("result is not map[string]string")
	}

	if typedResult["result"] == "" {
		t.Error("expected result to be set")
	}

	if node.ID() != "testAgent" {
		t.Errorf("expected ID 'testAgent', got '%s'", node.ID())
	}

	if node.Description() != "Test agent node" {
		t.Errorf("expected description 'Test agent node', got '%s'", node.Description())
	}
}

// TestAgentNodeInWorkflow tests agent node within a workflow graph
func TestAgentNodeInWorkflow(t *testing.T) {
	llm := mock.New()
	agt := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]())

	g := NewGraph[map[string]string]()

	g.AddNode(NewAgentNodeWithKey(
		"agent",
		agt,
		"input",
		"output",
		"Process input",
	))

	g.AddEdge("agent", EndNode)
	g.SetEntry("agent")

	ctx := context.Background()
	initialState := map[string]string{"input": "test input"}
	result, err := g.Run(ctx, initialState)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if result["output"] == "" {
		t.Error("expected output to be set by agent")
	}
}

// TestAgentNodeWithField tests the NewAgentNodeWithField factory function
func TestAgentNodeWithField(t *testing.T) {
	type TestState struct {
		Input  string
		Output string
	}

	llm := mock.New()
	agt := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]())

	node := NewAgentNodeWithField(
		"test",
		agt,
		func(s *TestState) string { return s.Input },
		func(s *TestState, output string) { s.Output = output },
		"Test with field",
	)

	ctx := context.Background()
	state := &TestState{Input: "test"}
	result, err := node.Execute(ctx, state)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	typedResult, ok := result.(*TestState)
	if !ok {
		t.Fatal("result is not *TestState")
	}

	if typedResult.Output == "" {
		t.Error("expected Output to be set")
	}
}

// TestAgentNodeWithFieldInWorkflow tests NewAgentNodeWithField in a workflow
func TestAgentNodeWithFieldInWorkflow(t *testing.T) {
	type WorkflowState struct {
		Task   string
		Result string
		Done   bool
	}

	llm := mock.New()
	agt := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]())

	g := NewGraph[*WorkflowState]()

	g.AddNode(NewAgentNodeWithField(
		"agent",
		agt,
		func(s *WorkflowState) string { return s.Task },
		func(s *WorkflowState, output string) {
			s.Result = output
			s.Done = true
		},
		"Process task",
	))

	g.AddEdge("agent", EndNode)
	g.SetEntry("agent")

	ctx := context.Background()
	initialState := &WorkflowState{Task: "Do something"}
	result, err := g.Run(ctx, initialState)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !result.Done {
		t.Error("expected Done to be true")
	}

	if result.Result == "" {
		t.Error("expected Result to be set")
	}
}

// TestAgentNodeWithKeyValue tests the NewAgentNodeWithKeyValue factory function
func TestAgentNodeWithKeyValue(t *testing.T) {
	llm := mock.New()
	agt := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]())

	node := NewAgentNodeWithKeyValue(
		"test",
		agt,
		"input",
		"output",
		"Test with key-value",
	)

	ctx := context.Background()
	state := map[string]any{"input": "test input"}
	result, err := node.Execute(ctx, state)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	typedResult, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result is not map[string]any")
	}

	output, exists := typedResult["output"]
	if !exists {
		t.Fatal("expected 'output' key to exist")
	}

	outputStr, ok := output.(string)
	if !ok {
		t.Fatal("output is not a string")
	}

	if outputStr == "" {
		t.Error("expected output to be non-empty")
	}
}

// TestAgentNodeWithExtractorOnly tests the NewAgentNodeWithExtractorOnly factory function
func TestAgentNodeWithExtractorOnly(t *testing.T) {
	llm := mock.New()
	agt := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]())

	node := NewAgentNodeWithExtractorOnly(
		"test",
		agt,
		func(state State) (string, error) {
			s := state.(map[string]string)
			return s["input"], nil
		},
		"Extractor only",
	)

	ctx := context.Background()
	state := map[string]string{"input": "test", "output": "unchanged"}
	result, err := node.Execute(ctx, state)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	typedResult, ok := result.(map[string]string)
	if !ok {
		t.Fatal("result is not map[string]string")
	}

	// Output should remain unchanged since the node doesn't modify state
	if typedResult["output"] != "unchanged" {
		t.Errorf("expected 'output' to remain 'unchanged', got '%s'", typedResult["output"])
	}
}

// TestAgentNodeConst tests the NewAgentNodeConst factory function
func TestAgentNodeConst(t *testing.T) {
	llm := mock.New()
	agt := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]())

	node := NewAgentNodeConst(
		"test",
		agt,
		"constant input",
		func(state State, output string) (State, error) {
			s := state.(map[string]string)
			if s == nil {
				s = make(map[string]string)
			}
			s["result"] = output
			return s, nil
		},
		"Constant input node",
	)

	ctx := context.Background()
	result, err := node.Execute(ctx, make(map[string]string))

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	typedResult, ok := result.(map[string]string)
	if !ok {
		t.Fatal("result is not map[string]string")
	}

	if typedResult["result"] == "" {
		t.Error("expected result to be set")
	}
}

// TestAgentNodeWithMemory tests that agent memory works correctly in workflow
func TestAgentNodeWithMemory(t *testing.T) {
	llm := mock.New()
	mem := buffer.New(100)

	agt := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]()).
		WithMemory(mem).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return "You are a helpful assistant."
		})

	node := NewAgentNode[struct{}](
		"test",
		agt,
		func(state State) (string, error) {
			return "test input", nil
		},
		func(state State, output string) (State, error) {
			s := state.(map[string]string)
			if s == nil {
				s = make(map[string]string)
			}
			s["result"] = output
			return s, nil
		},
		"Agent with memory",
	)

	ctx := context.Background()
	result, err := node.Execute(ctx, make(map[string]string))

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	typedResult, ok := result.(map[string]string)
	if !ok {
		t.Fatal("result is not map[string]string")
	}

	if typedResult["result"] == "" {
		t.Error("expected result to be set")
	}

	// Verify memory has entries
	history, _ := mem.Get(ctx)
	if len(history) == 0 {
		t.Error("expected memory to have entries")
	}
}

// TestAgentNodeExtractorError tests error handling when extractor fails
func TestAgentNodeExtractorError(t *testing.T) {
	llm := mock.New()
	agt := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]())

	node := NewAgentNode[struct{}](
		"test",
		agt,
		func(state State) (string, error) {
			return "", errors.New("extractor error")
		},
		func(state State, output string) (State, error) {
			return state, nil
		},
		"Test extractor error",
	)

	ctx := context.Background()
	_, err := node.Execute(ctx, make(map[string]string))

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.Error() != "error extracting input for agent 'test': extractor error" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestAgentNodeInjectorError tests error handling when injector fails
func TestAgentNodeInjectorError(t *testing.T) {
	llm := mock.New()
	agt := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]())

	node := NewAgentNode[struct{}](
		"test",
		agt,
		func(state State) (string, error) {
			return "input", nil
		},
		func(state State, output string) (State, error) {
			return nil, errors.New("injector error")
		},
		"Test injector error",
	)

	ctx := context.Background()
	_, err := node.Execute(ctx, make(map[string]string))

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.Error() != "error injecting output from agent 'test': injector error" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestAgentNodeTypeMismatch tests error handling when state type is wrong
func TestAgentNodeTypeMismatch(t *testing.T) {
	llm := mock.New()
	agt := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]())

	node := NewAgentNodeWithField(
		"test",
		agt,
		func(s *string) string { return *s },
		func(s *string, output string) { *s = output },
		"Type mismatch test",
	)

	ctx := context.Background()
	_, err := node.Execute(ctx, "not a pointer")

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Should be a TypeError
	var typeErr *TypeError
	if !errors.As(err, &typeErr) {
		t.Errorf("expected TypeError, got: %T", err)
	}
}

// TestPanicOnNilAgentInNewAgentNode tests that nil agent panics
func TestPanicOnNilAgentInNewAgentNode(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil agent, got none")
		}
	}()

	NewAgentNode[struct{}](
		"test",
		nil,
		func(state State) (string, error) { return "", nil },
		func(state State, output string) (State, error) { return state, nil },
		"Test",
	)
}

// TestPanicOnNilExtractorInNewAgentNode tests that nil extractor panics
func TestPanicOnNilExtractorInNewAgentNode(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil extractor, got none")
		}
	}()

	llm := mock.New()
	agt := agent.NewAgent[struct{}](llm)

	NewAgentNode[struct{}](
		"test",
		agt,
		nil,
		func(state State, output string) (State, error) { return state, nil },
		"Test",
	)
}

// TestPanicOnNilInjectorInNewAgentNode tests that nil injector panics
func TestPanicOnNilInjectorInNewAgentNode(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil injector, got none")
		}
	}()

	llm := mock.New()
	agt := agent.NewAgent[struct{}](llm)

	NewAgentNode[struct{}](
		"test",
		agt,
		func(state State) (string, error) { return "", nil },
		nil,
		"Test",
	)
}

// TestAgentNodeEmptyDescription tests default description when empty string is provided
func TestAgentNodeEmptyDescription(t *testing.T) {
	llm := mock.New()
	agt := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]())

	node := NewAgentNode[struct{}](
		"testAgent",
		agt,
		func(state State) (string, error) { return "", nil },
		func(state State, output string) (State, error) { return state, nil },
		"", // Empty description
	)

	expectedDesc := "AgentNode: testAgent"
	if node.Description() != expectedDesc {
		t.Errorf("expected description '%s', got '%s'", expectedDesc, node.Description())
	}
}

// TestMultiAgentSequentialWorkflow tests a sequential workflow with multiple agents
func TestMultiAgentSequentialWorkflow(t *testing.T) {
	type WorkflowState struct {
		Task   string
		Plan   string
		Code   string
		Review string
		Done   bool
	}

	llm := mock.New()

	plannerAgent := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]())
	coderAgent := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]())
	reviewerAgent := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]())

	g := NewGraph[*WorkflowState]()

	g.AddNode(NewAgentNodeWithField(
		"planner",
		plannerAgent,
		func(s *WorkflowState) string { return s.Task },
		func(s *WorkflowState, output string) { s.Plan = output },
		"Planner",
	))

	g.AddNode(NewAgentNodeWithField(
		"coder",
		coderAgent,
		func(s *WorkflowState) string { return s.Plan },
		func(s *WorkflowState, output string) { s.Code = output },
		"Coder",
	))

	g.AddNode(NewAgentNodeWithField(
		"reviewer",
		reviewerAgent,
		func(s *WorkflowState) string { return s.Code },
		func(s *WorkflowState, output string) {
			s.Review = output
			s.Done = true
		},
		"Reviewer",
	))

	g.AddEdge("planner", "coder")
	g.AddEdge("coder", "reviewer")
	g.AddEdge("reviewer", EndNode)
	g.SetEntry("planner")

	ctx := context.Background()
	initialState := &WorkflowState{Task: "Build a web app"}
	result, err := g.Run(ctx, initialState)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !result.Done {
		t.Error("expected Done to be true")
	}

	if result.Plan == "" || result.Code == "" || result.Review == "" {
		t.Error("expected all stages to complete")
	}
}
