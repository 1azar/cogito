package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/1azar/cogito/agent"
	"github.com/1azar/cogito/controller/simple"
	"github.com/1azar/cogito/llm/mock"
	"github.com/1azar/cogito/workflow"
)

// CollaborationState represents the state of a translator-editor collaboration
type CollaborationState struct {
	OriginalText string
	Translation  string
	Review       string
	PassCount    int
	Finalized    bool
}

func main() {
	fmt.Println("=== Translator-Editor Collaboration Example ===")
	TranslatorEditorCollab()
}

// TranslatorEditorCollab demonstrates a translator and editor working together
// to produce a high-quality translation through iterative review
func TranslatorEditorCollab() {
	llm := mock.New()

	// Translator agent - performs translation
	translatorAgent := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]()).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return `You are a professional translator. Translate text accurately while:
- Preserving the original meaning and tone
- Using natural, idiomatic language
- Maintaining cultural appropriateness
- Ensuring grammatical correctness

Focus on quality over literal word-for-word translation.`
		})

	// Editor agent - reviews and refines translation
	editorAgent := agent.NewAgent[struct{}](llm).
		WithController(simple.New[struct{}]()).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return `You are a translation editor. Review the translation for:
- Accuracy and completeness
- Natural flow and readability
- Appropriate tone and style
- Grammatical correctness

Provide specific feedback if improvements needed.
If the translation is excellent, respond with "APPROVED".`
		})

	g := workflow.NewGraph[*CollaborationState]()

	// Translator creates or revises translation
	g.AddNode(workflow.NewAgentNodeWithField(
		"translator",
		translatorAgent,
		func(s *CollaborationState) string {
			if s.PassCount == 0 {
				return fmt.Sprintf("Translate this text to Spanish:\n\n%s", s.OriginalText)
			}
			return fmt.Sprintf("Revise your translation based on this feedback:\n%s\n\nPrevious translation:\n%s",
				s.Review, s.Translation)
		},
		func(s *CollaborationState, output string) {
			s.Translation = output
		},
		"Translator",
	))

	// Editor reviews the translation
	g.AddNode(workflow.NewAgentNodeWithField(
		"editor",
		editorAgent,
		func(s *CollaborationState) string {
			return fmt.Sprintf("Review this translation:\n\nOriginal: %s\n\nTranslation: %s",
				s.OriginalText, s.Translation)
		},
		func(s *CollaborationState, output string) {
			s.Review = output
			s.Finalized = strings.Contains(strings.ToUpper(output), "APPROVED")
		},
		"Editor",
	))

	// Loop: translator -> editor -> (approve -> end / feedback -> translator)
	g.AddEdge("translator", "editor")

	g.AddConditionalEdge("editor", func(s *CollaborationState) (string, error) {
		// End if approved or max revisions reached
		if s.Finalized || s.PassCount >= 3 {
			return "end", nil
		}
		s.PassCount++
		return "translator", nil
	}, map[string]string{
		"translator": "translator",
		"end":        workflow.EndNode,
	})

	g.SetEntry("translator")

	// Run the collaboration
	ctx := context.Background()
	initialState := &CollaborationState{
		OriginalText: "Hello, world! Welcome to the future of AI-powered workflows.",
		PassCount:    0,
	}

	fmt.Printf("Original text: %s\n\n", initialState.OriginalText)
	fmt.Println("--- Starting Translation Process ---")

	result, err := g.Run(ctx, initialState)
	if err != nil {
		log.Printf("Collaboration error: %v", err)
		return
	}

	fmt.Println("\n--- Translation Complete ---")
	fmt.Printf("Review passes: %d\n", result.PassCount)
	fmt.Printf("Finalized: %v\n", result.Finalized)

	if result.Finalized {
		fmt.Println("\n✅ Translation Approved!")
	} else {
		fmt.Println("\n⚠️  Max revisions reached")
	}

	fmt.Println("\n--- Original Text ---")
	fmt.Println(result.OriginalText)

	fmt.Println("\n--- Final Translation ---")
	fmt.Println(result.Translation)

	if result.PassCount > 0 && !result.Finalized {
		fmt.Println("\n--- Final Review ---")
		fmt.Println(result.Review)
	}
}
