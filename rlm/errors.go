package rlm

import (
	"errors"
	"fmt"
)

type ErrorKind string

const (
	ErrorInvalidConfig ErrorKind = "invalid_config"
	ErrorCanceled      ErrorKind = "canceled"
	ErrorTimeout       ErrorKind = "timeout"
	ErrorBudget        ErrorKind = "budget_exceeded"
	ErrorSandbox       ErrorKind = "sandbox"
	ErrorProtocol      ErrorKind = "protocol"
	ErrorToolCall      ErrorKind = "invalid_tool_call"
)

type Error struct {
	Kind  ErrorKind
	Limit string
	Err   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Limit != "" {
		return fmt.Sprintf("rlm: %s (%s): %v", e.Kind, e.Limit, e.Err)
	}
	return fmt.Sprintf("rlm: %s: %v", e.Kind, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

func IsKind(err error, kind ErrorKind) bool {
	var target *Error
	return errors.As(err, &target) && target.Kind == kind
}
