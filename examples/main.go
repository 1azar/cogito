package main

import (
	"context"
	"fmt"
	"os"

	"github.com/1azar/cogito/agent"
	"github.com/1azar/cogito/controller/simple"
	"github.com/1azar/cogito/llm/openai"
	"github.com/1azar/cogito/memory/buffer"
)

type State struct{}

func main() {
	// llm := mock.New()

	llm, err := openai.New(openai.Config{
		APIKey:  os.Getenv("OPENAI_API_KEY"),
		BaseURL: os.Getenv("OPENAI_BASE_URL"),
		Model:   os.Getenv("OPENAI_MODEL"),
		Timeout: 0,
	})
	if err != nil {
		panic(err)
	}

	ag := agent.NewAgent[State](llm).
		WithController(simple.New[State]()).
		WithMemory(buffer.New(5))

	out, err := ag.Run(context.Background(), "Hello! Tell me who you are")
	if err != nil {
		panic(err)
	}

	fmt.Println(out)
}
