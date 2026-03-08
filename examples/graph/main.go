package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/1azar/cogito/agent"
	"github.com/1azar/cogito/controller/react"
	"github.com/1azar/cogito/llm/mock"
	"github.com/1azar/cogito/llm/openai"
	"github.com/1azar/cogito/memory/buffer"
	"github.com/1azar/cogito/workflow"
)

// SharedState represents the shared state between agents in the workflow
type SharedState struct {
	Task      string
	Query     string
	Decision  string
	Result    string
	Completed bool
}

func main() {
	// Example 1: Supervisor pattern
	//fmt.Println("=== Supervisor Pattern Example ===")
	//supervisorExample()

	// Example 2: Sequential multi-agent workflow
	fmt.Println("\n=== Sequential Multi-Agent Workflow Example ===")
	sequentialWorkflowExample()
}

// supervisorExample demonstrates a supervisor pattern where a manager agent
// delegates tasks to specialized worker agents based on the task type
func supervisorExample() {
	// Create LLM and memory
	llm := mock.New()
	mem := buffer.New(100)

	// Create manager agent (supervisor)
	managerAgent := agent.NewAgent[struct{}](llm).
		WithController(react.New[struct{}](react.Config{MaxSteps: 5})).
		WithMemory(mem).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return "You are a supervisor. Analyze the task and respond with either 'research', 'code', or 'done'."
		})

	// Create worker agents
	researcherAgent := agent.NewAgent[struct{}](llm).
		WithController(react.New[struct{}](react.Config{MaxSteps: 5})).
		WithMemory(buffer.New(100)).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return "You are a researcher. Provide research findings."
		})

	coderAgent := agent.NewAgent[struct{}](llm).
		WithController(react.New[struct{}](react.Config{MaxSteps: 5})).
		WithMemory(buffer.New(100)).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return "You are a coder. Write code to solve the task."
		})

	// Build workflow graph
	g := workflow.NewGraph[*SharedState]()

	// Add nodes
	g.AddNode("manager", workflow.NewAgentNodeWithKey(
		"manager",
		managerAgent,
		"task",     // input key
		"decision", // output key
		"Supervisor decides next action",
	))

	g.AddNode("researcher", workflow.NewAgentNodeWithKey(
		"researcher",
		researcherAgent,
		"query",
		"result",
		"Researcher investigates",
	))

	g.AddNode("coder", workflow.NewAgentNodeWithKey(
		"coder",
		coderAgent,
		"query",
		"result",
		"Coder implements solution",
	))

	// Set entry point
	g.SetEntry("manager")

	// Add conditional routing from manager
	g.AddConditionalEdge("manager", func(state *SharedState) (string, error) {
		decision := state.Decision
		if decision == "research" {
			return "researcher", nil
		}
		if decision == "code" {
			return "coder", nil
		}
		return "done", nil
	}, map[string]string{
		"researcher": "researcher",
		"coder":      "coder",
		"done":       workflow.EndNode,
	})

	// Worker agents loop back to manager
	g.AddEdge("researcher", "manager")
	g.AddEdge("coder", "manager")

	// Run workflow
	ctx := context.Background()
	initialState := &SharedState{
		Task:  "Build a web scraper",
		Query: "How should I scrape data from websites?",
	}

	result, err := g.Run(ctx, initialState)
	if err != nil {
		log.Printf("Workflow error: %v", err)
		return
	}

	fmt.Printf("Final state: %+v\n", result)
}

// sequentialWorkflowExample demonstrates a sequential multi-agent workflow
// where each agent processes the task in order: planner -> executor -> reviewer
func sequentialWorkflowExample() {
	// Create LLM
	//llm := mock.New()
	llm, err := openai.New(openai.Config{
		APIKey:  os.Getenv("OPENAI_API_KEY"),
		BaseURL: os.Getenv("OPENAI_BASE_URL"),
		Model:   os.Getenv("OPENAI_MODEL"),
		Timeout: 0,
	})
	if err != nil {
		panic(err)
	}

	// Create specialized agents
	plannerAgent := agent.NewAgent[struct{}](llm).
		WithController(react.New[struct{}](react.Config{MaxSteps: 5})).
		WithMemory(buffer.New(100)).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return "You are a planner. Break down the task into steps."
		})

	executorAgent := agent.NewAgent[struct{}](llm).
		WithController(react.New[struct{}](react.Config{MaxSteps: 5})).
		WithMemory(buffer.New(100)).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return "You are an executor. Implement the planned solution."
		})

	reviewerAgent := agent.NewAgent[struct{}](llm).
		WithController(react.New[struct{}](react.Config{MaxSteps: 5})).
		WithMemory(buffer.New(100)).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return "You are a reviewer. Review the implementation and provide feedback."
		})

	// Build workflow graph
	g := workflow.NewGraph[*SharedState]()

	// Add nodes in sequence
	g.AddNode("planner", workflow.NewAgentNodeWithField(
		"planner",
		plannerAgent,
		func(s *SharedState) string { return s.Task },
		func(s *SharedState, output string) { s.Result = output },
		"Plan the task",
	))

	g.AddNode("executor", workflow.NewAgentNodeWithField(
		"executor",
		executorAgent,
		func(s *SharedState) string { return s.Result }, // Use planner's output as input
		func(s *SharedState, output string) { s.Result = output },
		"Execute the plan",
	))

	g.AddNode("reviewer", workflow.NewAgentNodeWithField(
		"reviewer",
		reviewerAgent,
		func(s *SharedState) string { return s.Result }, // Use executor's output as input
		func(s *SharedState, output string) {
			s.Result = output
			s.Completed = true
		},
		"Review the execution",
	))

	// Create linear workflow: planner -> executor -> reviewer -> END
	g.AddEdge("planner", "executor")
	g.AddEdge("executor", "reviewer")
	g.AddEdge("reviewer", workflow.EndNode)
	g.SetEntry("planner")

	// Run workflow
	ctx := context.Background()
	initialState := &SharedState{
		Task: "Create a REST API for user management",
	}

	fmt.Printf("Initial task: %s\n", initialState.Task)
	fmt.Println("\n--- Starting workflow ---")

	result, err := g.Run(ctx, initialState)
	if err != nil {
		log.Printf("Workflow error: %v", err)
		return
	}

	fmt.Println("--- Workflow completed ---")
	fmt.Printf("Final result: %s\n", result.Result)
	fmt.Printf("Completed: %v\n", result.Completed)
}
