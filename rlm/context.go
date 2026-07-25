package rlm

import "context"

type rootPromptKey struct{}

// WithRootPrompt explicitly separates the short task from the long serialized
// request context available to the REPL.
func WithRootPrompt(ctx context.Context, prompt string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, rootPromptKey{}, prompt)
}

func rootPromptFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	prompt, ok := ctx.Value(rootPromptKey{}).(string)
	return prompt, ok
}
