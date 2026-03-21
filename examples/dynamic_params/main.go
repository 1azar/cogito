package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/1azar/cogito/agent"
	"github.com/1azar/cogito/controller"
	"github.com/1azar/cogito/llm"
	"github.com/1azar/cogito/memory/buffer"
	"github.com/1azar/cogito/schema"
	"github.com/1azar/cogito/tool/tools"
)

type State struct{}

type dynamicController[T any] struct{}

func (c *dynamicController[T]) Run(ctx context.Context, ag controller.AgentLike[T], input string) (string, error) {
	step0Ctx := llm.WithParams(ctx, llm.Params{
		Temperature: f64(0.1),
		TopP:        f64(0.9),
	})

	first, err := ag.CallLLMWithTools(step0Ctx, input)
	if err != nil {
		return "", err
	}

	if len(first.ToolCalls) == 0 {
		return first.Text, nil
	}

	if _, err := ag.RunToolCalls(step0Ctx, first.ToolCalls); err != nil {
		return "", err
	}

	step1Ctx := llm.WithParams(ctx, llm.Params{
		Temperature: f64(0.8),
		TopP:        f64(0.95),
	})

	second, err := ag.CallLLMWithTools(step1Ctx, "")
	if err != nil {
		return "", err
	}

	return second.Text, nil
}

type demoLLM struct{}

func (m *demoLLM) Generate(_ context.Context, req llm.Request) (*llm.Response, error) {
	if len(req.Messages) == 0 {
		return &llm.Response{Text: ""}, nil
	}

	last := req.Messages[len(req.Messages)-1]
	if last.Role == schema.RoleTool {
		return &llm.Response{
			Text: fmt.Sprintf(
				"Final answer with dynamic params: temperature=%.2f, top_p=%.2f",
				readFloat(req.Params.Temperature),
				readFloat(req.Params.TopP),
			),
		}, nil
	}

	args, _ := json.Marshal(map[string]string{"expression": "2+2"})
	return &llm.Response{
		Text: "Need a tool call first",
		ToolCalls: []schema.ToolCall{
			{
				ID:        "call_calc_1",
				Name:      tools.CalculatorTool.Name(),
				Arguments: args,
			},
		},
	}, nil
}

func main() {
	ag := agent.NewAgent[State](&demoLLM{}).
		WithController(&dynamicController[State]{}).
		WithMemory(buffer.New(10)).
		WithTools(tools.CalculatorTool)

	out, err := ag.Run(context.Background(), "What is 2+2?")
	if err != nil {
		panic(err)
	}

	fmt.Println(out)
}

func readFloat(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func f64(v float64) *float64 {
	return &v
}
