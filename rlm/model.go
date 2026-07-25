package rlm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/1azar/cogito/llm"
	"github.com/1azar/cogito/rlm/environment"
	cogruntime "github.com/1azar/cogito/runtime"
	"github.com/1azar/cogito/schema"
)

var replBlockPattern = regexp.MustCompile("(?s)```(?:repl|python)\\s*\\n(.*?)```")

type Model struct {
	base   llm.LLM
	config Config
}

type Metadata struct {
	Iterations      int           `json:"iterations"`
	Calls           int64         `json:"calls"`
	MaxDepthReached int           `json:"max_depth_reached"`
	Duration        time.Duration `json:"duration"`
	Termination     string        `json:"termination"`
}

type serializedContext struct {
	Messages   []schema.Message `json:"messages"`
	Tools      any              `json:"tools,omitempty"`
	ToolChoice llm.ToolChoice   `json:"tool_choice,omitempty"`
}

func New(base llm.LLM, config Config) (*Model, error) {
	if base == nil {
		return nil, &Error{Kind: ErrorInvalidConfig, Err: errors.New("base LLM is nil")}
	}
	config = config.withDefaults(base)
	if config.Environment == nil {
		return nil, &Error{Kind: ErrorInvalidConfig, Err: errors.New("environment is nil")}
	}
	if config.MaxDepth < 0 || config.MaxIterations < 1 || config.MaxConcurrentSubcalls < 1 ||
		config.MaxConsecutiveErrors < 1 || config.MaxOutputBytes < 1 || config.RootPromptMaxBytes < 1 {
		return nil, &Error{Kind: ErrorInvalidConfig, Err: errors.New("limits must be positive (MaxDepth may be zero only when explicitly defaulted)")}
	}
	return &Model{base: base, config: config}, nil
}

func (m *Model) Generate(ctx context.Context, req llm.Request) (*llm.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, m.config.Timeout)
	defer cancel()

	state := &runState{
		model:   m,
		req:     req,
		started: time.Now(),
		sem:     make(chan struct{}, m.config.MaxConcurrentSubcalls),
	}
	serialized, err := json.Marshal(serializedContext{
		Messages: req.Messages, Tools: req.Tools, ToolChoice: req.ToolChoice,
	})
	if err != nil {
		return nil, &Error{Kind: ErrorProtocol, Err: fmt.Errorf("serialize context: %w", err)}
	}
	state.context = serialized

	prompt := deriveRootPrompt(ctx, req, m.config.RootPromptMaxBytes)
	emit(ctx, cogruntime.EventRLMRunStarted, 0, 0, "")
	resp, err := state.run(ctx, prompt, 0)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			err = &Error{Kind: ErrorCanceled, Err: ctx.Err()}
		} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			err = &Error{Kind: ErrorTimeout, Err: ctx.Err()}
		}
		emitRunFinished(ctx, time.Since(state.started), llm.Usage{}, err)
		return nil, err
	}
	resp.Usage = state.usageSnapshot()
	metadata, _ := resp.Raw.(Metadata)
	metadata.Calls = state.calls.Load()
	metadata.MaxDepthReached = int(state.maxDepth.Load())
	metadata.Duration = time.Since(state.started)
	resp.Raw = metadata
	emitRunFinished(ctx, metadata.Duration, resp.Usage, nil)
	return resp, nil
}

func (m *Model) GenerateStream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	resp, err := m.Generate(ctx, req)
	if err != nil {
		return nil, err
	}
	return llm.StreamFromResponse(resp), nil
}

type runState struct {
	model   *Model
	req     llm.Request
	context json.RawMessage
	started time.Time
	sem     chan struct{}

	calls    atomic.Int64
	tokens   atomic.Int64
	cost     atomic.Int64
	maxDepth atomic.Int64
	usageMu  sync.Mutex
	usage    llm.Usage
}

// run RLM main cycle
func (s *runState) run(ctx context.Context, rootPrompt string, depth int) (*llm.Response, error) {
	// stats about how deep does the recursion go
	updateMax(&s.maxDepth, int64(depth))
	// start python session
	session, err := s.model.config.Environment.NewSession(ctx, environment.SessionConfig{
		Context:        s.context,
		CodeTimeout:    s.model.config.CodeTimeout,
		MaxOutputBytes: s.model.config.MaxOutputBytes,
	})
	if err != nil {
		return nil, &Error{Kind: ErrorSandbox, Err: err}
	}
	defer session.Close(context.Background())

	// compose conversation initial state
	conversation := []schema.Message{
		{Role: schema.RoleSystem, Content: systemPrompt(len(s.context), depth, s.model.config.MaxDepth)},
		{Role: schema.RoleUser, Content: rootPrompt},
	}
	// start the cycle (LLM → Python code → execute → observation)
	consecutiveErrors := 0
	for iteration := 1; iteration <= s.model.config.MaxIterations; iteration++ {
		emit(ctx, cogruntime.EventRLMIterationStarted, iteration, 0, "")
		// call llm, llm must response with code
		modelResp, err := s.callModel(ctx, s.modelForDepth(depth), llm.Request{Messages: conversation, Params: s.req.Params})
		if err != nil {
			return nil, err
		}
		conversation = append(conversation, schema.Message{Role: schema.RoleAssistant, Content: modelResp.Text})
		// try to extract code
		blocks := extractREPLBlocks(modelResp.Text)
		if len(blocks) == 0 {
			consecutiveErrors++
			emit(ctx, cogruntime.EventRLMIterationFinished, iteration, 0, "missing_repl_block")
			conversation = append(conversation, schema.Message{
				Role: schema.RoleUser, Content: "No `repl` code block was found. Inspect `context` using the REPL and call answer(...).",
			})
			if consecutiveErrors >= s.model.config.MaxConsecutiveErrors {
				return nil, &Error{Kind: ErrorProtocol, Err: errors.New("model repeatedly omitted a repl code block")}
			}
			continue
		}

		for _, code := range blocks {
			started := time.Now()
			var handlerErr error
			var handlerErrMu sync.Mutex
			result, execErr := session.Execute(ctx, code, environment.CallHandlerFunc(
				func(callCtx context.Context, request environment.CallRequest) environment.CallResult {
					callResult, callErr := s.handleCall(callCtx, request, depth)
					if callErr != nil {
						handlerErrMu.Lock()
						if handlerErr == nil {
							handlerErr = callErr
						}
						handlerErrMu.Unlock()
					}
					return callResult
				},
			))
			emit(ctx, cogruntime.EventRLMCodeExecuted, iteration, time.Since(started), executionMessage(result, execErr))
			handlerErrMu.Lock()
			fatalCallErr := handlerErr
			handlerErrMu.Unlock()
			if fatalCallErr != nil {
				return nil, fatalCallErr
			}
			if execErr != nil {
				if errors.Is(execErr, environment.ErrCodeTimeout) {
					return nil, &Error{Kind: ErrorTimeout, Limit: "code_timeout", Err: execErr}
				}
				consecutiveErrors++
				conversation = append(conversation, schema.Message{Role: schema.RoleUser, Content: "REPL error:\n" + execErr.Error()})
				if consecutiveErrors >= s.model.config.MaxConsecutiveErrors {
					return nil, &Error{Kind: ErrorSandbox, Err: execErr}
				}
				continue
			}
			consecutiveErrors = 0
			if result.Answer != nil {
				resp, validationErr := answerResponse(result.Answer, s.req)
				if validationErr != nil {
					return nil, validationErr
				}
				emit(ctx, cogruntime.EventRLMIterationFinished, iteration, time.Since(started), "answer")
				resp.Raw = Metadata{Iterations: iteration, Termination: "answer"}
				return resp, nil
			}
			output := formatExecution(result)
			conversation = append(conversation, schema.Message{Role: schema.RoleUser, Content: output})
		}
		emit(ctx, cogruntime.EventRLMIterationFinished, iteration, 0, "")
	}

	finalReq := llm.Request{
		Messages: append(conversation, schema.Message{
			Role:    schema.RoleUser,
			Content: "Iteration limit reached. Return the best final answer now as normal text; do not emit more REPL code.",
		}),
		Tools: s.req.Tools, ToolChoice: s.req.ToolChoice, Params: s.req.Params,
	}
	resp, err := s.callModel(ctx, s.modelForDepth(depth), finalReq)
	if err != nil {
		return nil, err
	}
	if err := validateToolCalls(resp.ToolCalls, s.req); err != nil {
		return nil, err
	}
	resp.Raw = Metadata{Iterations: s.model.config.MaxIterations, Termination: "max_iterations"}
	emit(ctx, cogruntime.EventRLMLimitReached, s.model.config.MaxIterations, 0, "max_iterations")
	return resp, nil
}

func (s *runState) handleCall(ctx context.Context, request environment.CallRequest, depth int) (environment.CallResult, error) {
	if len(request.Prompts) == 0 {
		err := &Error{Kind: ErrorProtocol, Err: errors.New("empty prompt list")}
		return environment.CallResult{Error: err.Error()}, err
	}
	results := make([]string, len(request.Prompts))
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex
	for i, prompt := range request.Prompts {
		wg.Add(1)
		go func(i int, prompt string) {
			defer wg.Done()
			emit(ctx, cogruntime.EventRLMSubcallStarted, 0, 0, string(request.Kind))
			var resp *llm.Response
			var err error
			if request.Kind == environment.CallRLM && depth < s.model.config.MaxDepth {
				resp, err = s.run(ctx, prompt, depth+1)
			} else {
				resp, err = s.callModel(ctx, s.model.config.SubLLM, llm.Request{
					Messages: []schema.Message{
						{Role: schema.RoleSystem, Content: "Answer the subproblem concisely. The parent RLM will use your result."},
						{Role: schema.RoleUser, Content: prompt},
					},
					Params: s.req.Params,
				})
			}
			if err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
				emit(ctx, cogruntime.EventRLMSubcallFinished, 0, 0, errorMessage(err))
				return
			}
			results[i] = resp.Text
			emit(ctx, cogruntime.EventRLMSubcallFinished, 0, 0, "")
		}(i, prompt)
	}
	wg.Wait()
	if firstErr != nil {
		return environment.CallResult{Error: firstErr.Error()}, firstErr
	}
	return environment.CallResult{Results: results}, nil
}

func (s *runState) callModel(ctx context.Context, model llm.LLM, req llm.Request) (*llm.Response, error) {
	if err := s.reserveCall(); err != nil {
		emit(ctx, cogruntime.EventRLMLimitReached, 0, 0, err.Error())
		return nil, err
	}
	select {
	case s.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-s.sem }()

	resp, err := model.Generate(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, &Error{Kind: ErrorProtocol, Err: errors.New("LLM returned nil response")}
	}
	if err := s.addUsage(resp.Usage); err != nil {
		emit(ctx, cogruntime.EventRLMLimitReached, 0, 0, err.Error())
		return nil, err
	}
	return resp, nil
}

func (s *runState) reserveCall() error {
	call := s.calls.Add(1)
	if limit := s.model.config.MaxTotalCalls; limit > 0 && call > limit {
		return &Error{Kind: ErrorBudget, Limit: "calls", Err: fmt.Errorf("%d > %d", call, limit)}
	}
	return nil
}

func (s *runState) addUsage(usage llm.Usage) error {
	total := usage.TotalTokens
	if total == 0 {
		total = usage.InputTokens + usage.OutputTokens
	}
	tokens := s.tokens.Add(total)
	cost := s.cost.Add(usage.EstimatedCostMicros)
	s.usageMu.Lock()
	s.usage.InputTokens += usage.InputTokens
	s.usage.OutputTokens += usage.OutputTokens
	s.usage.TotalTokens += total
	s.usage.EstimatedCostMicros += usage.EstimatedCostMicros
	if s.usage.Provider == "" {
		s.usage.Provider = usage.Provider
	} else if usage.Provider != "" && s.usage.Provider != usage.Provider {
		s.usage.Provider = "multiple"
	}
	s.usageMu.Unlock()
	if limit := s.model.config.MaxTotalTokens; limit > 0 && tokens > limit {
		return &Error{Kind: ErrorBudget, Limit: "tokens", Err: fmt.Errorf("%d > %d", tokens, limit)}
	}
	if limit := s.model.config.MaxCostMicros; limit > 0 && cost > limit {
		return &Error{Kind: ErrorBudget, Limit: "cost", Err: fmt.Errorf("%d > %d", cost, limit)}
	}
	return nil
}

func (s *runState) usageSnapshot() llm.Usage {
	s.usageMu.Lock()
	defer s.usageMu.Unlock()
	return s.usage
}

func (s *runState) modelForDepth(depth int) llm.LLM {
	if depth > 0 {
		return s.model.config.SubLLM
	}
	return s.model.base
}

func deriveRootPrompt(ctx context.Context, req llm.Request, maxBytes int) string {
	if prompt, ok := rootPromptFromContext(ctx); ok {
		return truncateBytes(prompt, maxBytes)
	}
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == schema.RoleUser && req.Messages[i].Content != "" {
			return truncateBytes(req.Messages[i].Content, maxBytes)
		}
	}
	return "Analyze the serialized context and provide the most useful answer."
}

func systemPrompt(contextBytes, depth, maxDepth int) string {
	return fmt.Sprintf(`You are a Recursive Language Model controller. A JSON value named context (%d bytes) is available in a persistent Python REPL.
Use one or more fenced blocks labeled repl. Variables persist between iterations.
Available functions:
- llm_query(prompt) and llm_query_batched(prompts)
- rlm_query(prompt) and rlm_query_batched(prompts)
- answer(text) or answer({"text": "...", "tool_calls": [{"id":"...", "name":"...", "arguments":{...}}]})
Inspect and transform context programmatically. Never place the final answer outside answer(...).
Current recursion depth: %d; configured maximum depth: %d.`, contextBytes, depth, maxDepth)
}

func extractREPLBlocks(text string) []string {
	matches := replBlockPattern.FindAllStringSubmatch(text, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
			out = append(out, match[1])
		}
	}
	return out
}

func formatExecution(result environment.ExecutionResult) string {
	var b strings.Builder
	b.WriteString("REPL result")
	if result.Truncated {
		b.WriteString(" (truncated)")
	}
	b.WriteString(":\nstdout:\n")
	b.WriteString(result.Stdout)
	if result.Stderr != "" {
		b.WriteString("\nstderr:\n")
		b.WriteString(result.Stderr)
	}
	return b.String()
}

func answerResponse(answer *environment.Answer, req llm.Request) (*llm.Response, error) {
	resp := &llm.Response{Text: answer.Text, FinishReason: "stop"}
	if len(answer.ToolCalls) > 0 {
		if err := json.Unmarshal(answer.ToolCalls, &resp.ToolCalls); err != nil {
			return nil, &Error{Kind: ErrorToolCall, Err: fmt.Errorf("decode tool calls: %w", err)}
		}
		resp.FinishReason = "tool_calls"
	}
	if err := validateToolCalls(resp.ToolCalls, req); err != nil {
		return nil, err
	}
	return resp, nil
}

func validateToolCalls(calls []schema.ToolCall, req llm.Request) error {
	if len(calls) == 0 {
		if req.ToolChoice.Mode == llm.ToolChoiceRequired || req.ToolChoice.Mode == llm.ToolChoiceNamed {
			return &Error{Kind: ErrorToolCall, Err: errors.New("tool choice requires a tool call")}
		}
		return nil
	}
	if req.ToolChoice.Mode == llm.ToolChoiceNone {
		return &Error{Kind: ErrorToolCall, Err: errors.New("tool calls are disabled")}
	}
	allowed := make(map[string]struct{}, len(req.Tools))
	for _, spec := range req.Tools {
		allowed[spec.Name] = struct{}{}
	}
	for i := range calls {
		call := &calls[i]
		if _, ok := allowed[call.Name]; !ok {
			return &Error{Kind: ErrorToolCall, Err: fmt.Errorf("unregistered tool %q", call.Name)}
		}
		if req.ToolChoice.Mode == llm.ToolChoiceNamed && call.Name != req.ToolChoice.Name {
			return &Error{Kind: ErrorToolCall, Err: fmt.Errorf("tool %q does not match required tool %q", call.Name, req.ToolChoice.Name)}
		}
		if call.ID == "" {
			call.ID = fmt.Sprintf("rlm_call_%d", i+1)
		}
		if !json.Valid(call.Arguments) {
			return &Error{Kind: ErrorToolCall, Err: fmt.Errorf("invalid arguments for tool %q", call.Name)}
		}
	}
	return nil
}

func truncateBytes(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	for limit > 0 && value[limit]&0xc0 == 0x80 {
		limit--
	}
	return value[:limit]
}

func updateMax(value *atomic.Int64, candidate int64) {
	for current := value.Load(); candidate > current; current = value.Load() {
		if value.CompareAndSwap(current, candidate) {
			return
		}
	}
}

func executionMessage(result environment.ExecutionResult, err error) string {
	if err != nil {
		return err.Error()
	}
	if result.Truncated {
		return "output_truncated"
	}
	return ""
}

func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func emit(ctx context.Context, eventType cogruntime.EventType, iteration int, duration time.Duration, message string) {
	bus, ok := cogruntime.EventBusFromContext(ctx)
	if !ok {
		return
	}
	runID, _ := cogruntime.RunIDFromContext(ctx)
	step, _ := cogruntime.StepFromContext(ctx)
	bus.Publish(ctx, cogruntime.Event{
		Timestamp: time.Now(), Type: eventType, RunID: runID, Component: "rlm",
		Step: step, Attempts: iteration, Duration: duration, Message: message,
	})
}

func emitRunFinished(ctx context.Context, duration time.Duration, usage llm.Usage, err error) {
	bus, ok := cogruntime.EventBusFromContext(ctx)
	if !ok {
		return
	}
	runID, _ := cogruntime.RunIDFromContext(ctx)
	step, _ := cogruntime.StepFromContext(ctx)
	bus.Publish(ctx, cogruntime.Event{
		Timestamp:           time.Now(),
		Type:                cogruntime.EventRLMRunFinished,
		RunID:               runID,
		Component:           "rlm",
		Step:                step,
		Duration:            duration,
		Err:                 err,
		InputTokens:         usage.InputTokens,
		OutputTokens:        usage.OutputTokens,
		TotalTokens:         usage.TotalTokens,
		EstimatedCostMicros: usage.EstimatedCostMicros,
	})
}

var _ llm.LLM = (*Model)(nil)
