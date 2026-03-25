package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/1azar/cogito/agent"
	"github.com/1azar/cogito/controller/react"
	"github.com/1azar/cogito/llm/openai"
	"github.com/1azar/cogito/memory/buffer"
	cogruntime "github.com/1azar/cogito/runtime"
	"github.com/1azar/cogito/tool"
	"github.com/1azar/cogito/tool/tools"
)

type State struct{}

type runResult struct {
	Output string
	Err    error
}

type transcriptEntry struct {
	Role    string
	Content string
	At      time.Time
}

type usageTotals struct {
	InputTokens         int64
	OutputTokens        int64
	TotalTokens         int64
	EstimatedCostMicros int64
}

func (u *usageTotals) Add(evt cogruntime.Event) {
	u.InputTokens += evt.InputTokens
	u.OutputTokens += evt.OutputTokens
	u.TotalTokens += evt.TotalTokens
	u.EstimatedCostMicros += evt.EstimatedCostMicros
}

type sessionState struct {
	mu sync.Mutex

	activeRunID     string
	hadStreamOutput bool
	streamLineOpen  bool

	sessionUsage usageTotals
	runUsage     map[string]usageTotals

	history []transcriptEntry
}

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
		WithSessionID("chat").
		WithMemory(buffer.New(50)).
		WithTools(
			weatherTool,
			tools.CalculatorTool,
			tools.CurrentTimeTool,
		)

	ctx := context.Background()
	state := &sessionState{runUsage: make(map[string]usageTotals)}
	subscribeToRuntimeEvents(ctx, ag, state)

	reader := bufio.NewReader(os.Stdin)
	inputCh := make(chan string)
	go readInput(reader, inputCh)

	runDone := make(chan runResult)
	runInFlight := false

	printWelcome()

	for {
		if !runInFlight {
			fmt.Print("\n> ")
		}

		select {
		case line := <-inputCh:
			input := strings.TrimSpace(line)
			if input == "" {
				continue
			}

			if strings.HasPrefix(input, "/") {
				if handleCommand(ctx, ag, state, input) {
					return
				}
				continue
			}

			if runInFlight {
				fmt.Println("Agent is running. Use /status or /interrupt.")
				continue
			}

			appendHistory(state, "user", input)
			markRunRequested(state)
			runInFlight = true
			go func(userInput string) {
				out, err := ag.Run(ctx, userInput)
				runDone <- runResult{Output: out, Err: err}
			}(input)

		case result := <-runDone:
			runInFlight = false
			if result.Err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", result.Err)
				continue
			}

			appendHistory(state, "assistant", result.Output)
			if !consumeRunStreamedOutput(state) {
				fmt.Println("\nAgent:", result.Output)
			}
		}
	}
}

func printWelcome() {
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    Cogito Chat Mode                         ║")
	fmt.Println("╠════════════════════════════════════════════════════════════╣")
	fmt.Println("║  Available commands:                                       ║")
	fmt.Println("║    /quit      - Exit the chat                              ║")
	fmt.Println("║    /clear     - Clear conversation history                 ║")
	fmt.Println("║    /status    - Show current run status                    ║")
	fmt.Println("║    /interrupt - Cancel active run                          ║")
	fmt.Println("║    /tokens    - Show token usage                           ║")
	fmt.Println("║    /history   - Show last chat messages                    ║")
	fmt.Println("║    /help      - Show this help message                     ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
}

func handleCommand(ctx context.Context, ag *agent.Agent[State], state *sessionState, cmd string) bool {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return false
	}

	switch parts[0] {
	case "/quit", "/q", "/exit":
		fmt.Println("\nGoodbye!")
		return true

	case "/clear", "/c":
		if err := ag.ClearMemory(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to clear memory: %v\n", err)
		} else {
			clearHistory(state)
			fmt.Println("Conversation history cleared.")
		}

	case "/status", "/s":
		printStatus(ag, state)

	case "/interrupt", "/i":
		interruptRun(ag, state)

	case "/tokens", "/t":
		printTokens(state)

	case "/history":
		printHistory(state)

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
	fmt.Println("  /quit, /q, /exit      - Exit the chat")
	fmt.Println("  /clear, /c            - Clear conversation history")
	fmt.Println("  /status, /s           - Show active run status")
	fmt.Println("  /interrupt, /i        - Interrupt active run")
	fmt.Println("  /tokens, /t           - Show token usage")
	fmt.Println("  /history              - Show recent messages")
	fmt.Println("  /help, /h, ?          - Show this help message")
}

func readInput(reader *bufio.Reader, out chan<- string) {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			continue
		}
		out <- line
	}
}

func subscribeToRuntimeEvents(ctx context.Context, ag *agent.Agent[State], state *sessionState) {
	bus := ag.EventBus()
	if bus == nil {
		return
	}

	bus.Subscribe(func(ctx context.Context, event cogruntime.Event) {
		state.mu.Lock()
		defer state.mu.Unlock()

		switch event.Type {
		case cogruntime.EventRunStarted:
			state.activeRunID = event.RunID
			fmt.Printf("\n[run] started id=%s\n", event.RunID)
		case cogruntime.EventRunFinished:
			if state.activeRunID == event.RunID {
				state.activeRunID = ""
			}
			if state.streamLineOpen {
				fmt.Println()
				state.streamLineOpen = false
			}
			fmt.Printf("\n[run] finished id=%s duration=%s\n", event.RunID, event.Duration.Round(time.Millisecond))
		case cogruntime.EventRunFailed:
			if state.activeRunID == event.RunID {
				state.activeRunID = ""
			}
			if state.streamLineOpen {
				fmt.Println()
				state.streamLineOpen = false
			}
			fmt.Printf("\n[run] failed id=%s err=%v\n", event.RunID, event.Err)
		case cogruntime.EventRunCanceled:
			if state.activeRunID == event.RunID {
				state.activeRunID = ""
			}
			if state.streamLineOpen {
				fmt.Println()
				state.streamLineOpen = false
			}
			fmt.Printf("\n[run] canceled id=%s err=%v\n", event.RunID, event.Err)
		case cogruntime.EventControllerStepStarted:
			fmt.Printf("\n[step] #%d started\n", event.Step)
		case cogruntime.EventToolCallStarted:
			fmt.Printf("\n[tool] start %s (%s)\n", event.ToolName, event.ToolCallID)
		case cogruntime.EventToolCallFinished:
			if event.Err != nil {
				fmt.Printf("\n[tool] fail %s (%s) attempts=%d err=%v\n", event.ToolName, event.ToolCallID, event.Attempts, event.Err)
			} else {
				fmt.Printf("\n[tool] done %s (%s) attempts=%d\n", event.ToolName, event.ToolCallID, event.Attempts)
			}
		case cogruntime.EventLLMCallFinished:
			totals := state.runUsage[event.RunID]
			totals.Add(event)
			state.runUsage[event.RunID] = totals
			state.sessionUsage.Add(event)
		case cogruntime.EventLLMTextDelta:
			if !state.streamLineOpen {
				fmt.Print("\nAgent: ")
				state.streamLineOpen = true
			}
			if event.Message != "" {
				fmt.Print(event.Message)
				state.hadStreamOutput = true
			}
		}
	})
}

func printStatus(ag *agent.Agent[State], state *sessionState) {
	mgr := ag.RunManager()
	if mgr == nil {
		fmt.Println("run manager is not configured")
		return
	}

	state.mu.Lock()
	runID := state.activeRunID
	state.mu.Unlock()

	if runID == "" {
		all := mgr.List()
		if len(all) == 0 {
			fmt.Println("No runs yet.")
			return
		}
		last := all[len(all)-1]
		fmt.Printf("Last run: id=%s status=%s duration=%s\n", last.ID, last.Status, last.Duration.Round(time.Millisecond))
		if last.Err != nil {
			fmt.Printf("Error: %v\n", last.Err)
		}
		return
	}

	run, ok := mgr.Get(runID)
	if !ok {
		fmt.Printf("Active run %s not found\n", runID)
		return
	}

	snap := run.Snapshot()
	fmt.Printf("Active run: id=%s status=%s duration=%s\n", snap.ID, snap.Status, snap.Duration.Round(time.Millisecond))
	if snap.Err != nil {
		fmt.Printf("Error: %v\n", snap.Err)
	}
}

func interruptRun(ag *agent.Agent[State], state *sessionState) {
	mgr := ag.RunManager()
	if mgr == nil {
		fmt.Println("run manager is not configured")
		return
	}

	state.mu.Lock()
	runID := state.activeRunID
	state.mu.Unlock()

	if runID == "" {
		fmt.Println("No active run to interrupt.")
		return
	}

	if err := mgr.Cancel(runID, context.Canceled); err != nil {
		fmt.Printf("Failed to interrupt run %s: %v\n", runID, err)
		return
	}

	fmt.Printf("Interrupt signal sent to run %s\n", runID)
}

func printTokens(state *sessionState) {
	state.mu.Lock()
	defer state.mu.Unlock()

	fmt.Printf("Session tokens: in=%d out=%d total=%d\n", state.sessionUsage.InputTokens, state.sessionUsage.OutputTokens, state.sessionUsage.TotalTokens)

	if state.activeRunID == "" {
		return
	}

	runUsage := state.runUsage[state.activeRunID]
	fmt.Printf("Active run tokens: in=%d out=%d total=%d\n", runUsage.InputTokens, runUsage.OutputTokens, runUsage.TotalTokens)
}

func printHistory(state *sessionState) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if len(state.history) == 0 {
		fmt.Println("History is empty.")
		return
	}

	start := max(0, len(state.history)-10)
	for _, item := range state.history[start:] {
		fmt.Printf("[%s] %s: %s\n", item.At.Format("15:04:05"), item.Role, item.Content)
	}
}

func appendHistory(state *sessionState, role string, content string) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.history = append(state.history, transcriptEntry{
		Role:    role,
		Content: content,
		At:      time.Now(),
	})

	if len(state.history) > 200 {
		state.history = slices.Clone(state.history[len(state.history)-200:])
	}
}

func clearHistory(state *sessionState) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.history = nil
}

func markRunRequested(state *sessionState) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.hadStreamOutput = false
	state.streamLineOpen = false
}

func consumeRunStreamedOutput(state *sessionState) bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	had := state.hadStreamOutput
	state.hadStreamOutput = false
	state.streamLineOpen = false
	return had
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
