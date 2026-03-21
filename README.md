# Cogito

A modular, type-safe framework for building LLM agents and workflow orchestration in Go.

> **Status:** Early Development  
> **Go:** 1.24+

## Overview

Cogito focuses on three goals:

- **Composable architecture**: providers, controllers, memory, tools, and workflows are decoupled.
- **Type-safe orchestration**: agents are generic over state (`Agent[T]`), and workflows are typed (`Graph[T]`).
- **Pragmatic runtime**: ReAct + tool execution includes strict argument validation, retries, timeouts, and parallelism.

## Installation

```bash
go get github.com/1azar/cogito
```

## Quick Start

```go
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
	provider, err := openai.New(openai.Config{
		APIKey: os.Getenv("OPENAI_API_KEY"),
		Model:  "gpt-4o-mini",
	})
	if err != nil {
		panic(err)
	}

	ag := agent.NewAgent[State](provider).
		WithController(simple.New[State]()).
		WithMemory(buffer.New(10))

	out, err := ag.Run(context.Background(), "Hello!")
	if err != nil {
		panic(err)
	}

	fmt.Println(out)
}
```

## Architecture

### High-Level Component Model

```text
Application Layer
  ├─ Workflow Graph (optional)
  └─ Agent composition

Agent Core
  ├─ Controller (strategy: simple, react, ...)
  ├─ LLM interface (provider-agnostic request/response)
  ├─ Memory interface (conversation state)
  ├─ Tool registry (tool specs + implementations)
  └─ Tool runtime executor (validation/retry/timeout/parallel)

Provider Adapters
  ├─ openai
  ├─ ollama
  └─ mock
```

### Core Interfaces (Current)

| Component | Interface / Contract | Responsibility |
|---|---|---|
| `llm.LLM` | `Generate(ctx, llm.Request) (*llm.Response, error)` | Provider-agnostic model invocation |
| `memory.Memory` | `Add/Get/Clear` | Conversation persistence |
| `controller.Controller[T]` | `Run(ctx, agent, input) (string, error)` | Agent behavior loop |
| `tool.Tool` | `Name/Description/Spec/Call` | Typed tool declaration and execution |
| `toolruntime.Executor` | `Execute(ctx, calls, registry) ([]Result, error)` | Tool-call orchestration policy |
| `workflow.Graph[T]` | `Run(ctx, initialState) (T, error)` | Typed multi-node orchestration |

## Provider Abstraction (`llm`)

Provider abstraction is centered on explicit request/response types:

- `llm.Request`:
  - `Messages []schema.Message`
  - `Tools []tool.Spec`
  - `ToolChoice llm.ToolChoice`
  - `Params llm.Params` (`Temperature`, `TopP`, `RepeatPenalty`, `MaxTokens`, `Stop`)
- `llm.Response`:
  - `Text string`
  - `ToolCalls []schema.ToolCall`
  - `FinishReason string`
  - `Raw any` (provider-specific payload for diagnostics)

This keeps provider-specific wire formats inside adapters (e.g. `llm/openai`, `llm/ollama`) and out of business logic.

Runtime params can be overridden per call via context:

```go
temp := 0.2
topP := 0.9
ctx = llm.WithParams(ctx, llm.Params{
	Temperature: &temp,
	TopP:        &topP,
})
```

## Tool System

### Tool Declaration

Tools are declared as Go functions and wrapped via `tool.Func`:

```go
t, err := tool.Func("get_weather", "Get weather by city", func(ctx context.Context, in WeatherInput) (WeatherOutput, error) {
	return WeatherOutput{TempC: 21}, nil
})
```

### Schema Generation

`tool.Func` reflects input types into `tool.JSONSchema`:

- exported struct fields become properties
- `json:"field,omitempty"` marks non-required fields
- nested structs, arrays, primitive types supported

### Validation

Before invoking a tool, `toolruntime` validates arguments against the tool schema.

Current validator behavior:

- required fields are enforced
- unknown fields are rejected
- type mismatches are rejected
- validation errors are returned as structured executor errors

## Tool Runtime (`toolruntime`)

`toolruntime.DefaultExecutor` is the execution plane for tool calls.

### Capabilities

- strict argument validation
- configurable parallel execution (`MaxParallel`)
- per-tool timeout (`PerToolTimeout`)
- retries (`Retry.MaxAttempts`)
- middleware chain (`Middleware`) around tool invocation
- structured result envelope (`toolruntime.Result`)

### Result Envelope

Tool results written to memory use a structured JSON payload:

```json
{
  "tool_call_id": "call_1",
  "name": "calculator",
  "status": "success",
  "output": {"result": 12},
  "meta": {"duration_ms": 3, "attempts": 1}
}
```

Error case:

```json
{
  "tool_call_id": "call_2",
  "name": "calculator",
  "status": "error",
  "error": {"code": "validation_error", "message": "$.expression is required"},
  "meta": {"duration_ms": 0, "attempts": 1}
}
```

## Controllers

### `simple`

Single LLM call, no iterative tool loop.

### `react`

ReAct loop (`Thought -> Action -> Observation`) with bounded steps:

1. Call LLM with messages + tool specs.
2. If `ToolCalls` is empty, return final text.
3. Execute tool calls via `toolruntime.Executor`.
4. Append structured tool observations to memory.
5. Repeat until final answer or `MaxSteps` reached.

### `structured`

Strict structured output controller with self-repair loop:

1. Injects a schema-aware prompt for target output type.
2. Extracts JSON even if model returns extra text.
3. Decodes into user-provided Go pointer (`Config.Output`).
4. Retries with repair prompt when parsing/validation fails.

## Agent Lifecycle

An `Agent[T]` wires the runtime pieces together:

1. build with `agent.NewAgent[T](provider)`
2. attach behavior with `.WithController(...)`
3. attach memory with `.WithMemory(...)`
4. register tools with `.WithTools(...)`
5. optionally override executor via `.WithToolExecutor(...)`
6. optionally add dynamic system prompt via `.WithPromptFunc(...)`
7. run via `.Run(ctx, input)`

## Workflow Orchestration (`workflow`)

`workflow.Graph[T]` lets you combine multiple nodes (agents or functions) into typed orchestration flows.

- `AddNode(id, node)`
- `AddEdge(from, to)` for direct transitions
- `AddConditionalEdge(from, router, targets)` for branching
- `SetEntry(id)` to define start node
- `SetConfig(config)` to tune execution (max iterations, error handler, observer)
- `Run(ctx, state)` to execute until `workflow.EndNode`

For opt-in workflow observability, attach an observer in config:

```go
cfg := workflow.DefaultConfig()
cfg.Observer = workflow.NewTUIObserver(os.Stdout) // or NewConsoleObserver

g.SetConfig(cfg)
```

When `Observer` is nil (default), no workflow events are emitted.

Try the dedicated observability example:

```bash
# plain event logs
COGITO_WORKFLOW_OBS=console go run ./examples/observability

# animated terminal output
COGITO_WORKFLOW_OBS=tui go run ./examples/observability
```

For agent nodes, use factory helpers:

- `workflow.NewAgentNodeWithField`
- `workflow.NewAgentNodeWithKey`
- `workflow.NewAgentNodeWithKeyValue`

## Detailed Example (Full Stack)

The most complete example is:

- `examples/full_framework/main.go`
- `examples/dynamic_params/main.go` (context-based per-call LLM params)

It demonstrates in one flow:

- typed workflow routing (router -> specialist/general)
- dynamic prompts
- memory-backed conversations
- ReAct with multiple tool calls
- strict validation error handling
- retry of transient tool failures
- parallel execution policy
- custom tool runtime middleware logging

Run it:

```bash
go run ./examples/full_framework
```

## Project Structure

```text
cogito/
├── agent/          # Agent[T] core and builder
├── controller/     # Behavior strategies (simple, react)
├── llm/            # Provider abstraction + adapters (openai, ollama, mock)
├── memory/         # Memory interfaces and implementations
├── schema/         # Shared message and tool-call schemas
├── tool/           # Tool declaration, registry, schema reflection
├── toolruntime/    # Tool execution runtime (policy/middleware/validation)
├── workflow/       # Typed workflow graph orchestration
└── examples/       # Runnable examples
```

## Development

```bash
# Run all tests
go test ./...

# Run with race detector
go test -race ./...

# Run basic example
go run ./examples/main.go

# Run full architecture example
go run ./examples/full_framework
```

## Current Limitations

- First-party provider adapters currently included: `openai`, `ollama`, `mock`.
- Streaming API is not yet implemented.
- JSON schema validation covers the framework's supported subset, not the full JSON Schema spec.

## License

MIT
