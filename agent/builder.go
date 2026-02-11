package agent

import (
	"github.com/1azar/cogito/controller"
	"github.com/1azar/cogito/llm"
	"github.com/1azar/cogito/memory"
)

func NewAgent[T any](l llm.LLM) *Agent[T] {
	return &Agent[T]{
		llm: l,
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
