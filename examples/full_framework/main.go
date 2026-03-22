package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/1azar/cogito/agent"
	"github.com/1azar/cogito/controller/react"
	"github.com/1azar/cogito/controller/simple"
	"github.com/1azar/cogito/llm"
	"github.com/1azar/cogito/memory/buffer"
	"github.com/1azar/cogito/schema"
	"github.com/1azar/cogito/tool"
	builtins "github.com/1azar/cogito/tool/tools"
	"github.com/1azar/cogito/toolruntime"
	"github.com/1azar/cogito/workflow"
)

type DemoState struct {
	Input       string
	Route       string
	RouteReason string
	Reply       string
}

type routeDecision struct {
	Route  string `json:"route"`
	Reason string `json:"reason"`
}

type unstableQuoteInput struct {
	Topic string `json:"topic"`
}

type unstableQuoteOutput struct {
	Quote   string `json:"quote"`
	Attempt int32  `json:"attempt"`
}

type routerLLM struct{}
type generalLLM struct{}
type specialistLLM struct{}

func main() {
	scenarios := []string{
		"Please calculate 2+2*5, tell current UTC time, and add one quote about orchestration.",
		"What is Cogito?",
	}

	ctx := context.Background()
	for i, input := range scenarios {
		fmt.Printf("=== Scenario %d ===\n", i+1)
		fmt.Printf("Input: %s\n", input)

		result, err := runScenario(ctx, input)
		if err != nil {
			fmt.Printf("Error: %v\n\n", err)
			continue
		}

		fmt.Printf("Route: %s (%s)\n", result.Route, result.RouteReason)
		fmt.Printf("Reply:\n%s\n\n", result.Reply)
	}
}

func runScenario(ctx context.Context, input string) (*DemoState, error) {
	unstableTool, err := newUnstableQuoteTool()
	if err != nil {
		return nil, fmt.Errorf("create unstable tool: %w", err)
	}

	routerAgent := agent.NewAgent[struct{}](&routerLLM{}).
		WithController(simple.New[struct{}]()).
		WithMemory(buffer.New(10)).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return "Route request to specialist for tool-heavy tasks, otherwise route to general."
		})

	generalAgent := agent.NewAgent[struct{}](&generalLLM{}).
		WithController(simple.New[struct{}]()).
		WithMemory(buffer.New(10)).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return "Provide a short general answer."
		})

	executor := toolruntime.NewExecutor(toolruntime.Policy{
		MaxParallel:     3,
		PerToolTimeout:  3 * time.Second,
		Retry:           toolruntime.RetryPolicy{MaxAttempts: 2},
		ContinueOnError: true,
	}, loggingMiddleware)

	specialistAgent := agent.NewAgent[struct{}](&specialistLLM{}).
		WithController(react.New[struct{}](react.Config{MaxSteps: 4})).
		WithMemory(buffer.New(30)).
		WithTools(
			builtins.CalculatorTool,
			builtins.CurrentTimeTool,
			unstableTool,
		).
		WithToolExecutor(executor).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return "Use tools when possible and summarize structured tool results."
		})

	g := workflow.NewGraph[*DemoState]()

	g.AddNode(workflow.NewAgentNodeWithField(
		"router",
		routerAgent,
		func(s *DemoState) string { return s.Input },
		func(s *DemoState, output string) {
			d, err := parseRouteDecision(output)
			if err != nil {
				s.Route = "general"
				s.RouteReason = "invalid router response"
				return
			}
			s.Route = d.Route
			s.RouteReason = d.Reason
		},
		"Router decides which agent should handle input",
	))

	g.AddNode(workflow.NewAgentNodeWithField(
		"specialist",
		specialistAgent,
		func(s *DemoState) string { return s.Input },
		func(s *DemoState, output string) { s.Reply = output },
		"ReAct specialist with tools",
	))

	g.AddNode(workflow.NewAgentNodeWithField(
		"general",
		generalAgent,
		func(s *DemoState) string { return s.Input },
		func(s *DemoState, output string) { s.Reply = output },
		"General fallback agent",
	))

	g.AddConditionalEdge("router", func(s *DemoState) (string, error) {
		switch s.Route {
		case "specialist":
			return "specialist", nil
		default:
			return "general", nil
		}
	}, map[string]string{
		"specialist": "specialist",
		"general":    "general",
	})

	g.AddEdge("specialist", workflow.EndNode)
	g.AddEdge("general", workflow.EndNode)
	g.SetEntry("router")

	initial := &DemoState{Input: input}
	out, err := g.Run(ctx, initial)
	if err != nil {
		return nil, err
	}

	return out, nil
}

func (m *routerLLM) Generate(ctx context.Context, req llm.Request) (*llm.Response, error) {
	if len(req.Messages) == 0 {
		return nil, errors.New("router: empty messages")
	}
	question := req.Messages[len(req.Messages)-1].Content
	lq := strings.ToLower(question)

	route := "general"
	reason := "no tool-related intent detected"
	if strings.Contains(lq, "calculate") || strings.Contains(lq, "time") || strings.Contains(lq, "quote") {
		route = "specialist"
		reason = "detected tool-oriented request"
	}

	body, _ := json.Marshal(routeDecision{
		Route:  route,
		Reason: reason,
	})

	return &llm.Response{Text: string(body)}, nil
}

func (m *routerLLM) GenerateStream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	resp, err := m.Generate(ctx, req)
	if err != nil {
		return nil, err
	}

	return llm.StreamFromResponse(resp), nil
}

func (m *generalLLM) Generate(ctx context.Context, req llm.Request) (*llm.Response, error) {
	if len(req.Messages) == 0 {
		return &llm.Response{Text: "General fallback: no input provided."}, nil
	}
	last := req.Messages[len(req.Messages)-1]
	return &llm.Response{
		Text: "General fallback answer: Cogito is a modular Go framework for LLM agents and orchestration.",
		Raw:  last.Content,
	}, nil
}

func (m *generalLLM) GenerateStream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	resp, err := m.Generate(ctx, req)
	if err != nil {
		return nil, err
	}

	return llm.StreamFromResponse(resp), nil
}

func (m *specialistLLM) Generate(ctx context.Context, req llm.Request) (*llm.Response, error) {
	if len(req.Messages) == 0 {
		return &llm.Response{Text: ""}, nil
	}

	last := req.Messages[len(req.Messages)-1]
	if last.Role == schema.RoleTool {
		results := collectTrailingToolResults(req.Messages)
		if len(results) == 0 {
			return &llm.Response{Text: "No tool results found."}, nil
		}
		return &llm.Response{Text: renderToolSummary(results)}, nil
	}

	calls := []schema.ToolCall{
		{
			ID:        "call_calc_valid",
			Name:      "calculator",
			Arguments: json.RawMessage(`{"expression":"2+2*5"}`),
		},
		{
			ID:        "call_time",
			Name:      "current_time",
			Arguments: json.RawMessage(`{"timezone":"UTC"}`),
		},
		{
			ID:        "call_unstable_quote",
			Name:      "unstable_quote",
			Arguments: json.RawMessage(`{"topic":"workflow orchestration"}`),
		},
		{
			ID:        "call_calc_invalid",
			Name:      "calculator",
			Arguments: json.RawMessage(`{"expr":"invalid_field"}`),
		},
	}

	return &llm.Response{
		ToolCalls: calls,
	}, nil
}

func (m *specialistLLM) GenerateStream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	resp, err := m.Generate(ctx, req)
	if err != nil {
		return nil, err
	}

	return llm.StreamFromResponse(resp), nil
}

func parseRouteDecision(raw string) (routeDecision, error) {
	var d routeDecision
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &d); err != nil {
		return routeDecision{}, err
	}
	if d.Route != "specialist" && d.Route != "general" {
		return routeDecision{}, fmt.Errorf("unknown route: %s", d.Route)
	}
	if strings.TrimSpace(d.Reason) == "" {
		d.Reason = "router decision"
	}
	return d, nil
}

func newUnstableQuoteTool() (tool.Tool, error) {
	var attempts int32

	return tool.Func(
		"unstable_quote",
		"Returns a short quote about a topic. Simulates transient failure on first attempt.",
		func(ctx context.Context, in unstableQuoteInput) (unstableQuoteOutput, error) {
			attempt := atomic.AddInt32(&attempts, 1)
			if attempt == 1 {
				return unstableQuoteOutput{}, errors.New("transient quote backend failure")
			}

			return unstableQuoteOutput{
				Quote:   "Orchestration is where small reliable steps become a system.",
				Attempt: attempt,
			}, nil
		},
	)
}

func loggingMiddleware(next toolruntime.Handler) toolruntime.Handler {
	return func(ctx context.Context, req toolruntime.CallRequest) toolruntime.CallResponse {
		started := time.Now()
		fmt.Printf(
			"[middleware] start tool=%s call_id=%s args=%s\n",
			req.Call.Name,
			req.Call.ID,
			string(req.Call.Arguments),
		)

		resp := next(ctx, req)
		if resp.Err != nil {
			fmt.Printf(
				"[middleware] end tool=%s call_id=%s err=%v duration=%s\n",
				req.Call.Name,
				req.Call.ID,
				resp.Err,
				time.Since(started).Round(time.Millisecond),
			)
			return resp
		}

		fmt.Printf(
			"[middleware] end tool=%s call_id=%s ok duration=%s\n",
			req.Call.Name,
			req.Call.ID,
			time.Since(started).Round(time.Millisecond),
		)

		return resp
	}
}

func collectTrailingToolResults(msgs []schema.Message) []toolruntime.Result {
	out := make([]toolruntime.Result, 0)
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != schema.RoleTool {
			break
		}
		var result toolruntime.Result
		if err := json.Unmarshal([]byte(msgs[i].Content), &result); err != nil {
			continue
		}
		out = append(out, result)
	}

	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}

	return out
}

func renderToolSummary(results []toolruntime.Result) string {
	lines := make([]string, 0, len(results)+1)
	lines = append(lines, "Specialist summary:")

	for _, result := range results {
		if result.Status == toolruntime.StatusError {
			lines = append(lines, fmt.Sprintf(
				"- %s (%s): ERROR code=%s message=%s attempts=%d",
				result.Name,
				result.ToolCallID,
				result.Error.Code,
				result.Error.Message,
				result.Meta.Attempts,
			))
			continue
		}

		switch result.Name {
		case "calculator":
			var out builtins.CalcOutput
			if err := json.Unmarshal(result.Output, &out); err == nil {
				lines = append(lines, fmt.Sprintf(
					"- calculator (%s): result=%.2f attempts=%d",
					result.ToolCallID,
					out.Result,
					result.Meta.Attempts,
				))
				continue
			}
		case "current_time":
			var out builtins.CurrentTimeOutput
			if err := json.Unmarshal(result.Output, &out); err == nil {
				lines = append(lines, fmt.Sprintf(
					"- current_time (%s): datetime=%s attempts=%d",
					result.ToolCallID,
					out.Datetime,
					result.Meta.Attempts,
				))
				continue
			}
		case "unstable_quote":
			var out unstableQuoteOutput
			if err := json.Unmarshal(result.Output, &out); err == nil {
				lines = append(lines, fmt.Sprintf(
					"- unstable_quote (%s): quote=%q attempts=%d",
					result.ToolCallID,
					out.Quote,
					result.Meta.Attempts,
				))
				continue
			}
		}

		lines = append(lines, fmt.Sprintf(
			"- %s (%s): output=%s attempts=%d",
			result.Name,
			result.ToolCallID,
			string(result.Output),
			result.Meta.Attempts,
		))
	}

	return strings.Join(lines, "\n")
}
