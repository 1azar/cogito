package toolruntime

import (
	"context"

	"github.com/1azar/cogito/schema"
	"github.com/1azar/cogito/tool"
)

type CallRequest struct {
	Call schema.ToolCall
	Tool tool.Tool
}

type CallResponse struct {
	Output any
	Err    error
}

type Handler func(ctx context.Context, req CallRequest) CallResponse

type Middleware func(next Handler) Handler
