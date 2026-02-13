package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/1azar/cogito/agent"
	"github.com/1azar/cogito/controller/react"
	"github.com/1azar/cogito/llm/openai"
	"github.com/1azar/cogito/memory/buffer"
	"github.com/1azar/cogito/tool"
	"github.com/1azar/cogito/tool/tools"
)

type State struct{}

func main() {
	llm, err := openai.New(openai.Config{
		APIKey:  os.Getenv("OPENAI_API_KEY"),
		BaseURL: os.Getenv("OPENAI_BASE_URL"),
		Model:   os.Getenv("OPENAI_MODEL"),
		Timeout: 0,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create LLM: %v\n", err)
		os.Exit(1)
	}

	weatherTool, err := tool.Func("get_best_girl_name", "return a name of the best girl in the planet", BestGilName)
	if err != nil {
		panic(err)
	}

	ag := agent.NewAgent[State](llm).
		WithController(react.New[State](react.Config{MaxSteps: 5})).
		WithMemory(buffer.New(50)).
		WithTools(
			weatherTool,
			tools.CalculatorTool,
			tools.CurrentTimeTool,
		)

	ctx := context.Background()
	reader := bufio.NewReader(os.Stdin)

	printWelcome()

	for {
		fmt.Print("\n> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			continue
		}

		input = strings.TrimSpace(input)

		// Handle empty input
		if input == "" {
			continue
		}

		// Handle commands
		if strings.HasPrefix(input, "/") {
			if handleCommand(ctx, ag, input) {
				// User wants to quit
				break
			}
			continue
		}

		// Run agent
		response, err := ag.Run(ctx, input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}

		fmt.Println("\nAgent:", response)
	}

	fmt.Println("\nGoodbye!")
}

func printWelcome() {
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    Cogito Chat Mode                         ║")
	fmt.Println("╠════════════════════════════════════════════════════════════╣")
	fmt.Println("║  Available commands:                                       ║")
	fmt.Println("║    /quit    - Exit the chat                                ║")
	fmt.Println("║    /clear   - Clear conversation history                   ║")
	fmt.Println("║    /help    - Show this help message                       ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
}

func handleCommand(ctx context.Context, ag *agent.Agent[State], cmd string) bool {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return false
	}

	switch parts[0] {
	case "/quit", "/q", "/exit":
		return true

	case "/clear", "/c":
		if err := ag.ClearMemory(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to clear memory: %v\n", err)
		} else {
			fmt.Println("Conversation history cleared.")
		}

	case "/help", "/h", "?":
		printHelp()

	default:
		fmt.Printf("Unknown command: %s\n", parts[0])
		fmt.Println("Type /help for available commands.")
	}

	return false
}

func printHelp() {
	fmt.Println("\nAvailable commands:")
	fmt.Println("  /quit, /q, /exit    - Exit the chat")
	fmt.Println("  /clear, /c          - Clear conversation history")
	fmt.Println("  /help, /h, ?        - Show this help message")
}

// GetWeather returns the current weather for a city
//type WeatherInput struct {
//	City string `json:"city"`
//}
//
//type WeatherOutput struct {
//	Temp int `json:"temp"`
//}
//
//func GetWeather(ctx context.Context, in WeatherInput) (WeatherOutput, error) {
//	return WeatherOutput{Temp: 9}, nil
//}

type BestGirlInput struct {
	//City string `json:"city"`
}

type BestGirlOutput struct {
	Name string `json:"name"`
}

func BestGilName(ctx context.Context, in BestGirlInput) (BestGirlOutput, error) {
	return BestGirlOutput{Name: "SABINA"}, nil
}
