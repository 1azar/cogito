package environment

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var ErrCodeTimeout = errors.New("REPL code execution timed out")

// Factory creates an isolated, persistent REPL session for one RLM Generate call.
type Factory interface {
	NewSession(ctx context.Context, config SessionConfig) (Session, error)
}

type SessionConfig struct {
	Context        json.RawMessage
	CodeTimeout    time.Duration
	MaxOutputBytes int
}

// Session executes code in a persistent namespace. Implementations must serialize
// Execute calls, while a CallHandler may execute a batch concurrently.
type Session interface {
	Execute(ctx context.Context, code string, handler CallHandler) (ExecutionResult, error)
	Close(ctx context.Context) error
}

type CallKind string

const (
	CallLLM CallKind = "llm"
	CallRLM CallKind = "rlm"
)

type CallRequest struct {
	Kind    CallKind `json:"kind"`
	Prompts []string `json:"prompts"`
	Batched bool     `json:"batched"`
}

type CallResult struct {
	Results []string `json:"results,omitempty"`
	Error   string   `json:"error,omitempty"`
}

type CallHandler interface {
	HandleCall(ctx context.Context, request CallRequest) CallResult
}

type CallHandlerFunc func(context.Context, CallRequest) CallResult

func (f CallHandlerFunc) HandleCall(ctx context.Context, request CallRequest) CallResult {
	return f(ctx, request)
}

type Answer struct {
	Text      string          `json:"text,omitempty"`
	ToolCalls json.RawMessage `json:"tool_calls,omitempty"`
}

type ExecutionResult struct {
	Stdout    string
	Stderr    string
	Answer    *Answer
	Truncated bool
	Duration  time.Duration
}
