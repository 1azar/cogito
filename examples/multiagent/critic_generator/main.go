package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/1azar/cogito/agent"
	"github.com/1azar/cogito/controller/simple"
	"github.com/1azar/cogito/llm/mock"
	"github.com/1azar/cogito/workflow"
)

// IterationState represents the state of an iterative improvement process
type IterationState struct {
	Prompt       string
	CurrentDraft string
	Critique     string
	Iteration    int
	Approved     bool
}

func main() {
	fmt.Println("=== Critic-Generator Loop Example ===\n")
	CriticGeneratorLoop()
}

// CriticGeneratorLoop demonstrates a generator creating content and a critic reviewing it
// This continues in a loop until the critic approves or max iterations reached
func CriticGeneratorLoop() {
	llm := mock.New()

	// Generator agent - creates content
	generatorAgent := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]()).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return `You are a content generator. Create high-quality, compelling content.

Guidelines:
- Be clear and concise
- Use engaging language
- Focus on value propositions
- Keep it professional

Improve the content based on feedback provided.`
		})

	// Critic agent - reviews and critiques
	criticAgent := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]()).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return `You are a critical reviewer. Evaluate content rigorously.

Check for:
- Clarity and coherence
- Grammar and style
- Accuracy and completeness
- Persuasiveness and impact

Provide specific, actionable feedback.

If the content is excellent, respond with "APPROVE".
Otherwise, explain what needs improvement.`
		})

	g := workflow.NewGraph[*IterationState]()

	// Generator creates or improves content
	g.AddNode("generator", workflow.NewAgentNodeWithField(
		"generator",
		generatorAgent,
		func(s *IterationState) string {
			if s.Iteration == 0 {
				return fmt.Sprintf("Generate content for: %s", s.Prompt)
			}
			return fmt.Sprintf("Improve the content based on this critique:\n%s\n\nCurrent draft:\n%s",
				s.Critique, s.CurrentDraft)
		},
		func(s *IterationState, output string) {
			s.CurrentDraft = output
		},
		"Content generator",
	))

	// Critic reviews the content
	g.AddNode("critic", workflow.NewAgentNodeWithField(
		"critic",
		criticAgent,
		func(s *IterationState) string {
			return fmt.Sprintf("Review this content:\n\n%s", s.CurrentDraft)
		},
		func(s *IterationState, output string) {
			s.Critique = output
			s.Approved = strings.Contains(strings.ToUpper(output), "APPROVE")
		},
		"Content critic",
	))

	// Loop: generator -> critic -> (approve -> end / critique -> generator)
	g.AddEdge("generator", "critic")

	g.AddConditionalEdge("critic", func(s *IterationState) (string, error) {
		// End if approved or max iterations reached
		if s.Approved || s.Iteration >= 5 {
			return "end", nil
		}
		s.Iteration++
		return "generator", nil
	}, map[string]string{
		"generator": "generator",
		"end":       workflow.EndNode,
	})

	g.SetEntry("generator")

	// Run the iteration
	ctx := context.Background()
	initialState := &IterationState{
		Prompt:    "Write a product description for a workflow orchestration library",
		Iteration: 0,
	}

	fmt.Printf("Prompt: %s\n\n", initialState.Prompt)
	fmt.Println("--- Starting Iteration Process ---\n")

	result, err := g.Run(ctx, initialState)
	if err != nil {
		log.Printf("Iteration error: %v", err)
		return
	}

	fmt.Println("\n--- Process Complete ---")
	fmt.Printf("Iterations: %d\n", result.Iteration)
	fmt.Printf("Approved: %v\n", result.Approved)

	if result.Approved {
		fmt.Println("\n✅ Content Approved!")
	} else {
		fmt.Println("\n⚠️  Max iterations reached")
	}

	fmt.Println("\n--- Final Content ---")
	fmt.Println(result.CurrentDraft)

	if !result.Approved && result.Iteration >= 1 {
		fmt.Println("\n--- Final Critique ---")
		fmt.Println(result.Critique)
	}
}
