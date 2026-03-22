package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/1azar/cogito/agent"
	"github.com/1azar/cogito/controller/simple"
	"github.com/1azar/cogito/llm/ollama"
	"github.com/1azar/cogito/memory/buffer"
	cogruntime "github.com/1azar/cogito/runtime"
)

type State struct{}

type cliState struct {
	mu sync.Mutex

	runActive  bool
	streamOpen bool
	streamed   bool
}

func main() {
	provider, err := ollama.New(ollama.Config{
		BaseURL: os.Getenv("OLLAMA_BASE_URL"),
		Model:   "gemma3:4b",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create Ollama client: %v\n", err)
		os.Exit(1)
	}

	ag := agent.NewAgent[State](provider).
		WithController(simple.New[State]()).
		WithMemory(buffer.New(30)).
		WithSessionID("ollama_cli")

	state := &cliState{}
	subscribe(ag, state)

	reader := bufio.NewReader(os.Stdin)
	ctx := context.Background()

	fmt.Println("Ollama CLI (gemma3:4b)")
	fmt.Println("Commands: /quit, /clear")

	for {
		fmt.Print("\n> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "Read error: %v\n", err)
			continue
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		switch input {
		case "/quit", "/q", "/exit":
			fmt.Println("Goodbye!")
			return
		case "/clear", "/c":
			if err := ag.ClearMemory(); err != nil {
				fmt.Printf("Failed to clear memory: %v\n", err)
			} else {
				fmt.Println("Memory cleared.")
			}
			continue
		}

		state.mu.Lock()
		state.runActive = true
		state.streamOpen = false
		state.streamed = false
		state.mu.Unlock()

		out, err := ag.Run(ctx, input)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		state.mu.Lock()
		streamed := state.streamed
		state.runActive = false
		state.streamOpen = false
		state.streamed = false
		state.mu.Unlock()

		if !streamed {
			fmt.Printf("Agent: %s\n", out)
		}
	}
}

func subscribe(ag *agent.Agent[State], st *cliState) {
	bus := ag.EventBus()
	if bus == nil {
		return
	}

	bus.Subscribe(func(ctx context.Context, event cogruntime.Event) {
		st.mu.Lock()
		defer st.mu.Unlock()

		if !st.runActive {
			return
		}

		switch event.Type {
		case cogruntime.EventLLMTextDelta:
			if !st.streamOpen {
				fmt.Print("Agent: ")
				st.streamOpen = true
			}
			fmt.Print(event.Message)
			st.streamed = true
		case cogruntime.EventRunFinished, cogruntime.EventRunFailed, cogruntime.EventRunCanceled:
			if st.streamOpen {
				fmt.Println()
				st.streamOpen = false
			}
		}
	})
}
