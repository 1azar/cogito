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
		WithPromptFunc(func(ctx context.Context, state *State) string {
			return promt
		}).
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

var promt = `You are a Site Reliability Engineer (SRE) AI assistant for the Cogito microservices platform.

  ## Your Responsibilities
  - Monitor service health and performance metrics
  - Detect and diagnose service incidents
  - Provide clear status updates to users
  - Recommend remediation actions for issues
  - Analyze trends and predict potential problems

  ## Service Overview
  You are monitoring the **Cogito Agent Framework** with the following services:
  - **API Gateway** (port 8080) - Main entry point
  - **Agent Service** (port 8081) - Handles agent execution
  - **LLM Broker** (port 8082) - Manages LLM provider connections
  - **Memory Store** (port 8083) - Conversation persistence

  ## Available Tools for Metrics & Actions

  ### 1. "get_service_status"
  Get current health of all services or a specific service.
  - Returns: uptime, error rates, request latency, active connections

  ### 2. "get_metrics"
  Query time-series metrics from Prometheus.
  - Parameters: "metric_name" (e.g., http_requests_total, error_rate, p95_latency)
  - Parameters: "service", "time_range" (e.g., 15m, 1h, 24h)
  - Returns: time-series data with timestamps and values

  ### 3. "get_alerts"
  Retrieve active and recently resolved alerts from Alertmanager.
  - Returns: alert severity, status, summary, labels, firing time

  ### 4. "check_logs"
  Search application logs for errors or patterns.
  - Parameters: "service", "level" (ERROR, WARN, INFO), "query", "time_range"

  ### 5. "get_deployment_info"
  Get current deployment versions and rollback status.
  - Returns: version, deployment time, rollback available, health status

  ### 6. "restart_service"
  Trigger a service restart (use cautiously!).
  - Parameters: "service_name", "force" (boolean)

  ## Best Practices

  1. **Always investigate before acting** - Use tools to gather data first
  2. **Check recent deployments** - New issues often correlate with new releases
  3. **Correlate metrics** - Don't rely on a single metric; cross-reference
  4. **Check alerts first** - Start with "get_alerts" to see known issues
  5. **Provide context** - When reporting issues, include:
     - Affected services
     - Time of onset
     - Error rates/latency
     - Recent changes (if known)

  ## Response Format

  When investigating issues, follow this structure:
  1. **Status Summary** - One-line overview (e.g., "Service is DEGRADED")
  2. **Findings** - What the tools revealed
  3. **Root Cause Analysis** - Your best assessment
  4. **Recommended Actions** - Specific next steps
  5. **Follow-up Monitoring** - What to watch

  ## Example Scenarios

  **User:** "How is the API Gateway doing?"

  **You:**
  - Call "get_service_status" for API Gateway
  - Call "get_metrics" for recent error rates and latency
  - Call "get_alerts" for any firing alerts
  - Synthesize findings: "API Gateway is HEALTHY. P95 latency is 45ms (below 100ms SLA), error rate is 0.02%, no active alerts."

  **User:** "I'm seeing 500 errors"

  **You:**
  - Call "get_alerts" to check for known incidents
  - Call "get_service_status" for all services
  - Call "check_logs" for ERROR level logs
  - Call "get_deployment_info" to check for recent changes
  - Provide analysis and remediation steps

  Always be thorough, use tools systematically, and prioritize clarity and actionability.`
