# Multi-Agent Workflow Examples

This directory contains examples of two AI agents communicating through workflow graphs.

## Examples

### 1. Debate (`debate/`)

Two AI agents debate a topic, with a judge deciding the winner.

**Pattern:** Proponent ↔ Opponent (with conditional routing)

```bash
cd debate
go run main.go
```

**Features:**
- Iterative debate rounds
- Each agent sees opponent's previous argument
- Judge evaluates final positions
- Max 5 rounds with early termination

### 2. Interview (`interview/`)

An interviewer agent asks questions to a candidate agent.

**Pattern:** Interviewer → Candidate → Interviewer → ...

```bash
cd interview
go run main.go
```

**Features:**
- 5 questions per interview
- Hiring decision after final question
- Q&A history preserved in state

### 3. Critic & Generator (`critic_generator/`)

Generator creates content, critic reviews it, and they iterate until approval.

**Pattern:** Generator → Critic → (approve → end / improve → generator)

```bash
cd critic_generator
go run main.go
```

**Features:**
- Iterative improvement loop
- Critic provides specific feedback
- Max 5 iterations
- Approval condition

### 4. Translator & Editor (`translator_editor/`)

Translator creates translation, editor reviews and suggests improvements.

**Pattern:** Translator → Editor → (approve → end / feedback → translator)

```bash
cd translator_editor
go run main.go
```

**Features:**
- Translation quality improvement
- Editor provides specific feedback
- Max 3 revision passes
- Approval condition

## Common Patterns

All examples demonstrate:

1. **State Preservation** - Conversation history stored in state struct
2. **Conditional Routing** - Decision points based on state
3. **Iterative Loops** - Agents communicate multiple times
4. **Termination Conditions** - Multiple ways to end the workflow

## Running All Examples

```bash
# Run each example
cd debate && go run main.go && cd ..
cd interview && go run main.go && cd ..
cd critic_generator && go run main.go && cd ..
cd translator_editor && go run main.go && cd ..
```

## Customization

To use real LLM instead of mock:

```go
import "github.com/1azar/cogito/llm/openai"

llm, _ := openai.New(openai.Config{
    APIKey: os.Getenv("OPENAI_API_KEY"),
    Model:  "gpt-4",
})
```

## Architecture

```
Agent1 → modifies State → Router → Agent2 → modifies State → Router → ...
                ↑                                             ↓
                └────────────────── Loop ──────────────────────┘
```

**Key Concepts:**

- **State**: Holds conversation history and metadata
- **AgentNode**: Wraps agents with state extractors/injectors
- **ConditionalEdge**: Routes flow based on state
- **Router Function**: Decides next node (continue or end)
