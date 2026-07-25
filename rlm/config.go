package rlm

import (
	"time"

	"github.com/1azar/cogito/llm"
	"github.com/1azar/cogito/rlm/environment"
)

type Config struct {
	Environment environment.Factory
	SubLLM      llm.LLM

	MaxDepth              int
	MaxIterations         int
	MaxConcurrentSubcalls int
	MaxTotalCalls         int64
	MaxTotalTokens        int64
	MaxCostMicros         int64
	Timeout               time.Duration
	CodeTimeout           time.Duration
	MaxConsecutiveErrors  int
	MaxOutputBytes        int
	RootPromptMaxBytes    int
}

func (c Config) withDefaults(base llm.LLM) Config {
	if c.SubLLM == nil {
		c.SubLLM = base
	}
	if c.MaxDepth == 0 {
		c.MaxDepth = 1
	}
	if c.MaxIterations == 0 {
		c.MaxIterations = 30
	}
	if c.MaxConcurrentSubcalls == 0 {
		c.MaxConcurrentSubcalls = 4
	}
	if c.Timeout == 0 {
		c.Timeout = 10 * time.Minute
	}
	if c.CodeTimeout == 0 {
		c.CodeTimeout = 30 * time.Second
	}
	if c.MaxConsecutiveErrors == 0 {
		c.MaxConsecutiveErrors = 3
	}
	if c.MaxOutputBytes == 0 {
		c.MaxOutputBytes = 20 << 10
	}
	if c.RootPromptMaxBytes == 0 {
		c.RootPromptMaxBytes = 8 << 10
	}
	return c
}
