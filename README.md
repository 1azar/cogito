# Cogito

A modular, type-safe LLM agent framework for Go.

> **Status:** Early Development | **Go:** 1.24+

## Overview

Cogito is a Go framework for building LLM-powered agents, inspired by LangChain, LangGraph, and AutoGen. It emphasizes:

- **Type Safety** - Generics-based agents with compile-time state validation
- **Modularity** - Pluggable LLM providers, memory systems, and controllers
- **Simplicity** - Builder pattern for clean, readable agent configuration
- **Flexibility** - Swap any component (LLM, Memory, Controller) via interfaces

## Quick Start

```bash
go get github.com/1azar/cogito
```

```go
package main

import (
    "context"
    "fmt"
    "github.com/1azar/cogito/agent"
    "github.com/1azar/cogito/controller/simple"
    "github.com/1azar/cogito/llm/openai"
    "github.com/1azar/cogito/memory/buffer"
)

type State struct{}

func main() {
    // Create LLM client
    llm, _ := openai.New(openai.Config{
        APIKey: "your-api-key",
        Model:  "gpt-4o-mini",
    })

    // Build agent with memory and controller
    ag := agent.NewAgent[State](llm).
        WithController(simple.New[State]()).
        WithMemory(buffer.New(10))

    // Run the agent
    response, err := ag.Run(context.Background(), "Hello!")
    if err != nil {
        panic(err)
    }

    fmt.Println(response)
}
```

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                      Agent[T]                       │
│  ┌───────────┐  ┌───────────┐  ┌──────────────┐   │
│  │    LLM    │  │  Memory   │  │  Controller  │   │
│  │ Provider  │  │           │  │  (Strategy)  │   │
│  └───────────┘  └───────────┘  └──────────────┘   │
└─────────────────────────────────────────────────────┘
```

### Core Components

| Component | Interface | Purpose |
|-----------|-----------|---------|
| **LLM** | `Generate(ctx, messages) -> Completion` | Abstraction over LLM providers |
| **Memory** | `Add/Get/Clear` | Conversation history storage |
| **Controller** | `Run(ctx, agent, input) -> string` | Agent behavior strategy |
| **Agent** | Generic over state `T` | Orchestrates all components |

## Features

### Type-Safe State

```go
type AgentState struct {
    Query  string
    Result string
    Count  int
}

agent := agent.NewAgent[AgentState](llm)
state := agent.State()  // *AgentState
```

### Pluggable Controllers

Controllers define how the agent "thinks":

```go
import "github.com/1azar/cogito/controller/simple"

// Simple: direct LLM call
agent.WithController(simple.New[State]())

// ReAct: thought-action-observation loop (planned)
// agent.WithController(react.New[State]())
```

### Memory Implementations

```go
import "github.com/1azar/cogito/memory/buffer"

// Sliding window buffer (keeps last N messages)
agent.WithMemory(buffer.New(10))

// Vector stores with RAG support (planned)
```

### LLM Providers

```go
import "github.com/1azar/cogito/llm/openai"

llm, _ := openai.New(openai.Config{
    APIKey:  "your-api-key",
    BaseURL:  "https://api.openai.com/v1", // optional
    Model:   "gpt-4o-mini",
    Timeout: 60 * time.Second,
})

// Mock for testing
import "github.com/1azar/cogito/llm/mock"
llm := mock.New()
```

## Project Structure

```
cogito/
├── agent/          - Core Agent[T] with builder pattern
├── controller/     - Behavior strategies (simple, react, etc.)
├── llm/            - LLM providers (openai, mock)
├── memory/         - Memory implementations (buffer)
├── schema/         - Shared types (Message, Role)
└── example/        - Usage examples
```

## Development

```bash
# Run tests
go test ./...

# Run with race detector
go test -race ./...

# Run example
go run ./example/main.go
```

## Roadmap

### Phase 1: Core Foundation (Current)
- [x] Basic Agent with generics
- [x] LLM abstraction (OpenAI, Mock)
- [x] Memory interface (Buffer)
- [x] Simple controller
- [ ] Tool system (function calling)
  - [ ] FuncWithState
  - [ ] Validation (simple)
  - [ ] Middleware
  - [ ] Streaming
  - [ ] LLM interface should support tool abstraction (independent drom opeaiprotocol etc)!
- [ ] Additional controllers (ReAct, Plan-and-Solve)

### Phase 2: Memory & RAG
- [ ] Vector stores (Pinecone, Qdrant, pgvector)
- [ ] Embeddings (OpenAI, local)
- [ ] Semantic memory with RAG
- [ ] Document processing (PDF, HTML)

### Phase 3: Multi-Agent
- [ ] Workflow graph engine
- [ ] Agent coordination (Supervisor pattern)
- [ ] Shared memory between agents

### Phase 4: Production
- [ ] Middleware (retry, cache, rate-limit)
- [ ] Streaming API
- [ ] Observability (OpenTelemetry, Prometheus)
- [ ] Structured output

## Design Principles

1. **Interfaces over implementations** - Swap any component
2. **Generics for type safety** - Compile-time validation
3. **Zero boilerplate** - Reflection for tools, not users
4. **Go idioms** - Clean code, minimal dependencies

## License

MIT
