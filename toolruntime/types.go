package toolruntime

import (
	"encoding/json"
	"time"
)

type Status string

const (
	StatusSuccess Status = "success"
	StatusError   Status = "error"
)

type ExecError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
}

type Meta struct {
	DurationMillis int64 `json:"duration_ms"`
	Attempts       int   `json:"attempts"`
	TimedOut       bool  `json:"timed_out,omitempty"`
}

type Result struct {
	ToolCallID string          `json:"tool_call_id"`
	Name       string          `json:"name"`
	Status     Status          `json:"status"`
	Output     json.RawMessage `json:"output,omitempty"`
	Error      *ExecError      `json:"error,omitempty"`
	Meta       Meta            `json:"meta"`
}

type RetryPolicy struct {
	MaxAttempts int
}

type Policy struct {
	MaxParallel     int
	PerToolTimeout  time.Duration
	Retry           RetryPolicy
	ContinueOnError bool
}

func DefaultPolicy() Policy {
	return Policy{
		MaxParallel:     4,
		PerToolTimeout:  15 * time.Second,
		Retry:           RetryPolicy{MaxAttempts: 1},
		ContinueOnError: true,
	}
}
