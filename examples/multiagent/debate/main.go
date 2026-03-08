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

// DebateState represents the state of a debate between two agents
type DebateState struct {
	Topic         string
	ProponentMsg  string
	OpponentMsg   string
	Round         int
	ProponentDone bool
	OpponentDone  bool
	Winner        string
}

func main() {
	fmt.Println("=== AI Debate Example ===")
	DebateExample()
}

// DebateExample demonstrates two AI agents debating a topic
func DebateExample() {
	llm := mock.New()

	// Agent 1: Proponent (argues FOR the topic)
	proponentAgent := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]()).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return `You are a proponent in a debate. Argue FOR the topic.
Be convincing but respectful. Use logical arguments and evidence.

After making 3 points, end your response with "I REST MY CASE"

Keep responses concise (2-3 sentences per point).`
		})

	// Agent 2: Opponent (argues AGAINST the topic)
	opponentAgent := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]()).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return `You are an opponent in a debate. Argue AGAINST the topic.
Counter the opponent's points with your own arguments.

After making 3 counterpoints, end your response with "I REST MY CASE"

Keep responses concise (2-3 sentences per point).`
		})

	// Judge agent (decides the winner)
	judgeAgent := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]()).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return `You are a debate judge. Evaluate the arguments presented.
Consider:
- Logical consistency
- Quality of evidence
- Persuasiveness

Respond with only: "proponent" or "opponent" based on who argued better.`
		})

	g := workflow.NewGraph[*DebateState]()

	// Proponent node
	g.AddNode("proponent", workflow.NewAgentNodeWithField(
		"proponent",
		proponentAgent,
		func(s *DebateState) string {
			if s.Round == 0 {
				return fmt.Sprintf("Topic: %s\n\nPresent your opening argument.", s.Topic)
			}
			return fmt.Sprintf("Opponent said: %s\n\nCounter their argument!", s.OpponentMsg)
		},
		func(s *DebateState, output string) {
			s.ProponentMsg = output
			s.ProponentDone = strings.Contains(output, "I REST MY CASE")
		},
		"Proponent argues FOR the topic",
	))

	// Opponent node
	g.AddNode("opponent", workflow.NewAgentNodeWithField(
		"opponent",
		opponentAgent,
		func(s *DebateState) string {
			return fmt.Sprintf("Proponent said: %s\n\nCounter their argument!", s.ProponentMsg)
		},
		func(s *DebateState, output string) {
			s.OpponentMsg = output
			s.OpponentDone = strings.Contains(output, "I REST MY CASE")
		},
		"Opponent argues AGAINST the topic",
	))

	// Judge node
	g.AddNode("judge", workflow.NewAgentNodeWithField(
		"judge",
		judgeAgent,
		func(s *DebateState) string {
			return fmt.Sprintf(`Debate on: %s

Proponent's final position: %s

Opponent's final position: %s

Who won? Respond with "proponent" or "opponent"`, s.Topic, s.ProponentMsg, s.OpponentMsg)
		},
		func(s *DebateState, output string) {
			winner := strings.ToLower(strings.TrimSpace(output))
			if strings.Contains(winner, "proponent") {
				s.Winner = "Proponent"
			} else if strings.Contains(winner, "opponent") {
				s.Winner = "Opponent"
			} else {
				s.Winner = "Tie"
			}
		},
		"Judge evaluates the debate",
	))

	// Routing: proponent -> opponent -> judge -> (end or continue)
	g.AddEdge("proponent", "opponent")
	g.AddEdge("opponent", "judge")

	g.AddConditionalEdge("judge", func(s *DebateState) (string, error) {
		// Check if both sides are done
		if s.ProponentDone && s.OpponentDone {
			return "end", nil
		}

		// Check if we've done too many rounds
		if s.Round >= 5 {
			return "end", nil
		}

		s.Round++
		return "proponent", nil
	}, map[string]string{
		"end":       workflow.EndNode,
		"proponent": "proponent",
	})

	g.SetEntry("proponent")

	// Run the debate
	ctx := context.Background()
	initialState := &DebateState{
		Topic: "AI should have legal rights",
		Round: 0,
	}

	fmt.Printf("Topic: %s\n\n", initialState.Topic)
	fmt.Println("--- Starting Debate ---")

	result, err := g.Run(ctx, initialState)
	if err != nil {
		log.Printf("Debate error: %v", err)
		return
	}

	fmt.Println("\n--- Debate Complete ---")
	fmt.Printf("Rounds: %d\n", result.Round)
	fmt.Printf("Proponent's final message: %s\n", truncate(result.ProponentMsg, 100))
	fmt.Printf("Opponent's final message: %s\n", truncate(result.OpponentMsg, 100))
	fmt.Printf("\n🏆 Winner: %s\n", result.Winner)
}

// truncate shortens a string for display
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
