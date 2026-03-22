package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/1azar/cogito/workflow"
)

type DemoState struct {
	Input    string
	Words    []string
	Category string
	Result   string
}

func main() {
	input := "Observability helps debug complex agent pipelines quickly"
	if len(os.Args) > 1 {
		input = strings.Join(os.Args[1:], " ")
	}

	g := workflow.NewGraph[*DemoState]()
	configureObservability(g)

	g.AddNode(workflow.FunctionNode(
		"tokenize",
		func(ctx context.Context, st workflow.State) (workflow.State, error) {
			s := st.(*DemoState)
			time.Sleep(550 * time.Millisecond)
			s.Words = strings.Fields(s.Input)
			if len(s.Words) > 6 {
				s.Category = "long"
			} else {
				s.Category = "short"
			}
			return s, nil
		},
		"Tokenize input and classify length",
	))

	g.AddNode(workflow.FunctionNode(
		"short",
		func(ctx context.Context, st workflow.State) (workflow.State, error) {
			s := st.(*DemoState)
			time.Sleep(350 * time.Millisecond)
			s.Result = fmt.Sprintf("short text: %d words", len(s.Words))
			return s, nil
		},
		"Handle short requests",
	))

	g.AddNode(workflow.FunctionNode(
		"long",
		func(ctx context.Context, st workflow.State) (workflow.State, error) {
			s := st.(*DemoState)
			time.Sleep(900 * time.Millisecond)
			s.Result = fmt.Sprintf("long text: %d words; first=%q", len(s.Words), s.Words[0])
			return s, nil
		},
		"Handle long requests",
	))

	g.AddConditionalEdge("tokenize", func(s *DemoState) (string, error) {
		if s.Category == "long" {
			return "long", nil
		}
		return "short", nil
	}, map[string]string{
		"short": "short",
		"long":  "long",
	})

	g.AddEdge("short", workflow.EndNode)
	g.AddEdge("long", workflow.EndNode)
	g.SetEntry("tokenize")

	out, err := g.Run(context.Background(), &DemoState{Input: input})
	if err != nil {
		fmt.Printf("workflow error: %v\n", err)
		return
	}

	fmt.Printf("\nResult: %s\n", out.Result)
	fmt.Printf("Words: %d\n", len(out.Words))
	fmt.Printf("\nTip: set COGITO_WORKFLOW_OBS=console or COGITO_WORKFLOW_OBS=tui\n")
}

func configureObservability(g *workflow.Graph[*DemoState]) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("COGITO_WORKFLOW_OBS")))
	//mode = "console"
	mode = "tui"
	if mode == "" || mode == "off" {
		return
	}

	cfg := workflow.DefaultConfig()
	switch mode {
	case "console":
		cfg.Observer = workflow.NewConsoleObserver(os.Stdout)
	case "tui":
		cfg.Observer = workflow.NewTUIObserver(os.Stdout)
	default:
		fmt.Printf("unknown COGITO_WORKFLOW_OBS mode %q, expected off|console|tui\n", mode)
		return
	}

	g.SetConfig(cfg)
}
