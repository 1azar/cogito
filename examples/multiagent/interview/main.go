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

// InterviewState represents the state of a job interview
type InterviewState struct {
	Position      string
	Question      string
	Answer        string
	Evaluation    string
	QuestionCount int
	Hired         bool
}

func main() {
	fmt.Println("=== AI Interview Example ===")
	InterviewExample()
}

// InterviewExample demonstrates an interviewer agent interviewing a candidate agent
func InterviewExample() {
	llm := mock.New()

	// Interviewer agent - asks questions
	interviewerAgent := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]()).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return `You are a technical interviewer conducting a job interview.

Ask relevant, thoughtful questions that test:
- Technical knowledge
- Problem-solving skills
- Experience
- Cultural fit

Keep questions concise and specific.

After 5 questions, evaluate the candidate and respond with:
"HIRE" if they did well, or "REJECT" if not.

DO NOT include these words in your questions.`
		})

	// Candidate agent - answers questions
	candidateAgent := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]()).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return `You are a job candidate interviewing for a technical position.

Answer questions:
- Honestly and professionally
- Highlighting your skills and experience
- Providing specific examples when relevant
- Being concise but thorough

Show enthusiasm and ask for clarification if needed.`
		})

	g := workflow.NewGraph[*InterviewState]()

	// Interviewer asks a question
	g.AddNode("interviewer", workflow.NewAgentNodeWithField(
		"interviewer",
		interviewerAgent,
		func(s *InterviewState) string {
			if s.QuestionCount == 0 {
				return fmt.Sprintf("Position: %s\n\nWelcome! Let's start the interview.\nAsk your first question.", s.Position)
			}
			return fmt.Sprintf("Candidate answered: %s\n\nAsk your next question OR evaluate if this was question #5.",
				s.Answer)
		},
		func(s *InterviewState, output string) {
			s.Question = output
			s.QuestionCount++

			outputUpper := strings.ToUpper(output)
			if strings.Contains(outputUpper, "HIRE") {
				s.Hired = true
			} else if strings.Contains(outputUpper, "REJECT") {
				s.Hired = false
			}
		},
		"Interviewer asks questions",
	))

	// Candidate answers
	g.AddNode("candidate", workflow.NewAgentNodeWithField(
		"candidate",
		candidateAgent,
		func(s *InterviewState) string {
			return fmt.Sprintf("Question #%d: %s\n\nAnswer:", s.QuestionCount, s.Question)
		},
		func(s *InterviewState, output string) {
			s.Answer = output
		},
		"Candidate answers questions",
	))

	// Routing: interviewer -> candidate -> interviewer -> ... -> end
	g.AddEdge("interviewer", "candidate")

	g.AddConditionalEdge("candidate", func(s *InterviewState) (string, error) {
		// End if we've asked 5+ questions or got a hiring decision
		if s.QuestionCount >= 5 || strings.Contains(strings.ToUpper(s.Question), "HIRE") ||
			strings.Contains(strings.ToUpper(s.Question), "REJECT") {
			return "end", nil
		}
		return "interviewer", nil
	}, map[string]string{
		"interviewer": "interviewer",
		"end":         workflow.EndNode,
	})

	g.SetEntry("interviewer")

	// Run the interview
	ctx := context.Background()
	initialState := &InterviewState{
		Position: "Senior Go Developer",
	}

	fmt.Printf("Position: %s\n\n", initialState.Position)
	fmt.Println("--- Starting Interview ---")

	result, err := g.Run(ctx, initialState)
	if err != nil {
		log.Printf("Interview error: %v", err)
		return
	}

	fmt.Println("\n--- Interview Complete ---")
	fmt.Printf("Questions asked: %d\n", result.QuestionCount)
	fmt.Printf("Decision: %s\n", result.Question)

	if result.Hired {
		fmt.Println("✅ HIRED")
	} else if strings.Contains(strings.ToUpper(result.Question), "REJECT") {
		fmt.Println("❌ REJECTED")
	} else {
		fmt.Println("⏸️ PENDING (more questions needed)")
	}

	// Show last Q&A
	fmt.Println("\n--- Last Exchange ---")
	fmt.Printf("Q: %s\n", truncate(result.Question, 80))
	fmt.Printf("A: %s\n", truncate(result.Answer, 150))
}

// truncate shortens a string for display
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
