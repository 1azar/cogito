package agent

import (
	"github.com/1azar/cogito/controller"
	"github.com/1azar/cogito/llm"
	"github.com/1azar/cogito/memory"
	cogruntime "github.com/1azar/cogito/runtime"
	"github.com/1azar/cogito/tool"
	"github.com/1azar/cogito/toolruntime"
)

func NewAgent[T any](l llm.LLM) *Agent[T] {
	return &Agent[T]{
		llm:        l,
		tools:      tool.NewRegistry(),
		executor:   toolruntime.NewExecutor(toolruntime.DefaultPolicy()),
		runManager: cogruntime.NewManager(),
		eventBus:   cogruntime.NewEventBus(),
	}
}

func (a *Agent[T]) WithMemory(m memory.Memory) *Agent[T] {
	a.memory = m
	return a
}

func (a *Agent[T]) WithController(c controller.Controller[T]) *Agent[T] {
	a.controller = c
	return a
}

func (a *Agent[T]) WithTools(tools ...tool.Tool) *Agent[T] {

	for _, t := range tools {

		if t == nil {
			panic("nil tool")
		}

		a.tools.Register(t)
	}

	return a
}

func (a *Agent[T]) WithToolExecutor(executor toolruntime.Executor) *Agent[T] {
	if executor == nil {
		panic("nil tool executor")
	}
	a.executor = executor
	return a
}

// WithPromptFunc sets a dynamic system prompt function that can generate prompts based on context and state
func (a *Agent[T]) WithPromptFunc(fn PromptFunc[T]) *Agent[T] {
	a.promptFunc = fn
	return a
}

func (a *Agent[T]) WithRunManager(m *cogruntime.Manager) *Agent[T] {
	a.runManager = m
	return a
}

func (a *Agent[T]) WithEventBus(bus cogruntime.EventBus) *Agent[T] {
	a.eventBus = bus
	return a
}

func (a *Agent[T]) WithSessionID(sessionID string) *Agent[T] {
	a.sessionID = sessionID
	return a
}
