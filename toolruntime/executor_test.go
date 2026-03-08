package toolruntime

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/1azar/cogito/schema"
	"github.com/1azar/cogito/tool"
)

type echoInput struct {
	Name string `json:"name"`
}

type echoOutput struct {
	Reply string `json:"reply"`
}

func TestDefaultExecutor_ExecuteStructuredErrors(t *testing.T) {
	echoTool, err := tool.Func("echo", "echoes name", func(ctx context.Context, in echoInput) (echoOutput, error) {
		return echoOutput{Reply: "hello " + in.Name}, nil
	})
	if err != nil {
		t.Fatalf("failed to create tool: %v", err)
	}

	reg := tool.NewRegistry()
	reg.Register(echoTool)

	executor := NewExecutor(Policy{
		MaxParallel:     2,
		PerToolTimeout:  time.Second,
		Retry:           RetryPolicy{MaxAttempts: 1},
		ContinueOnError: true,
	})

	results, execErr := executor.Execute(context.Background(), []schema.ToolCall{
		{ID: "c1", Name: "echo", Arguments: json.RawMessage(`{"name":"Bob"}`)},
		{ID: "c2", Name: "echo", Arguments: json.RawMessage(`{"unknown":"x"}`)},
		{ID: "c3", Name: "missing", Arguments: json.RawMessage(`{}`)},
	}, reg)
	if execErr != nil {
		t.Fatalf("unexpected executor error: %v", execErr)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	if results[0].Status != StatusSuccess {
		t.Fatalf("expected first result success, got %s", results[0].Status)
	}

	var out echoOutput
	if err := json.Unmarshal(results[0].Output, &out); err != nil {
		t.Fatalf("failed to decode tool output: %v", err)
	}
	if out.Reply != "hello Bob" {
		t.Fatalf("unexpected output: %q", out.Reply)
	}

	if results[1].Status != StatusError || results[1].Error == nil || results[1].Error.Code != "validation_error" {
		t.Fatalf("expected validation_error for second result, got %+v", results[1])
	}

	if results[2].Status != StatusError || results[2].Error == nil || results[2].Error.Code != "tool_not_found" {
		t.Fatalf("expected tool_not_found for third result, got %+v", results[2])
	}
}

func TestDefaultExecutor_ParallelExecution(t *testing.T) {
	var active int32
	var maxActive int32

	slowTool, err := tool.Func("slow", "slow tool", func(ctx context.Context, in echoInput) (echoOutput, error) {
		n := atomic.AddInt32(&active, 1)
		for {
			prev := atomic.LoadInt32(&maxActive)
			if n <= prev || atomic.CompareAndSwapInt32(&maxActive, prev, n) {
				break
			}
		}

		time.Sleep(80 * time.Millisecond)
		atomic.AddInt32(&active, -1)
		return echoOutput{Reply: in.Name}, nil
	})
	if err != nil {
		t.Fatalf("failed to create slow tool: %v", err)
	}

	reg := tool.NewRegistry()
	reg.Register(slowTool)

	executor := NewExecutor(Policy{
		MaxParallel:     4,
		PerToolTimeout:  time.Second,
		Retry:           RetryPolicy{MaxAttempts: 1},
		ContinueOnError: true,
	})

	calls := []schema.ToolCall{
		{ID: "1", Name: "slow", Arguments: json.RawMessage(`{"name":"a"}`)},
		{ID: "2", Name: "slow", Arguments: json.RawMessage(`{"name":"b"}`)},
		{ID: "3", Name: "slow", Arguments: json.RawMessage(`{"name":"c"}`)},
		{ID: "4", Name: "slow", Arguments: json.RawMessage(`{"name":"d"}`)},
	}

	started := time.Now()
	results, err := executor.Execute(context.Background(), calls, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(started)

	if len(results) != len(calls) {
		t.Fatalf("unexpected result count: %d", len(results))
	}
	if maxActive < 2 {
		t.Fatalf("expected concurrent execution, max active=%d", maxActive)
	}
	if elapsed >= 240*time.Millisecond {
		t.Fatalf("expected parallel execution to finish faster, elapsed=%s", elapsed)
	}
}
