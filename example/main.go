package main

import (
	"context"
	"fmt"

	"github.com/1azar/cogito/agent"
	"github.com/1azar/cogito/controller/simple"
	"github.com/1azar/cogito/llm/mock"
	"github.com/1azar/cogito/memory/buffer"
)

type State struct{}

func main() {
	llm := mock.New()

	ag := agent.NewAgent[State](llm).
		WithController(simple.New[State]()).
		WithMemory(buffer.New(5))

	out, err := ag.Run(context.Background(), "Привет!")
	if err != nil {
		panic(err)
	}

	fmt.Println(out)
}
