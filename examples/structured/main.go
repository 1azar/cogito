package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/1azar/cogito/agent"
	"github.com/1azar/cogito/controller/structured"
	"github.com/1azar/cogito/llm"
	"github.com/1azar/cogito/llm/ollama"
)

type State struct{}

type SupportTicket struct {
	Category string   `json:"category"`
	Priority int      `json:"priority"`
	Tags     []string `json:"tags,omitempty"`
	Summary  string   `json:"summary"`
}

func main() {
	//	model := &scriptedLLM{}
	model, err := ollama.New(ollama.Config{
		Model: "gemma2:2b",
	})

	var out SupportTicket
	ag := agent.NewAgent[State](model).
		WithController(structured.New[State](structured.Config{
			Output:      &out,
			MaxAttempts: 3,
		}))

	raw, err := ag.Run(context.Background(), `Classify request: "Payments are failing for EU users after latest deploy"`)
	if err != nil {
		panic(err)
	}

	fmt.Println("Raw canonical JSON:")
	fmt.Println(raw)

	pretty, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println("\nParsed struct:")
	fmt.Println(string(pretty))
}

// scriptedLLM is a deterministic example model:
// 1st response has invalid type (priority as string) to trigger repair.
// 2nd response has extra prose + fenced JSON to show JSON extraction.
type scriptedLLM struct {
	attempt int
}

func (m *scriptedLLM) Generate(ctx context.Context, req llm.Request) (*llm.Response, error) {
	m.attempt++

	if m.attempt == 1 {
		return &llm.Response{
			Text: `{"category":"billing","priority":"high","summary":"EU payments fail after deploy"}`,
		}, nil
	}

	return &llm.Response{
		Text: "Fixed output:\n```json\n{\"category\":\"billing\",\"priority\":1,\"tags\":[\"payments\",\"eu\",\"incident\"],\"summary\":\"EU payments are failing after deploy\"}\n```",
	}, nil
}

func (m *scriptedLLM) GenerateStream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	resp, err := m.Generate(ctx, req)
	if err != nil {
		return nil, err
	}

	return llm.StreamFromResponse(resp), nil
}
