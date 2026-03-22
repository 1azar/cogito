package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/1azar/cogito/llm"
	"github.com/1azar/cogito/schema"
	"github.com/1azar/cogito/tool"
)

type chatRequest struct {
	Model         string        `json:"model"`
	Messages      []chatMessage `json:"messages"`
	Tools         []chatTool    `json:"tools,omitempty"`
	ToolChoice    any           `json:"tool_choice,omitempty"`
	Stream        bool          `json:"stream,omitempty"`
	Temperature   *float64      `json:"temperature,omitempty"`
	TopP          *float64      `json:"top_p,omitempty"`
	RepeatPenalty *float64      `json:"repeat_penalty,omitempty"`
	MaxTokens     *int          `json:"max_tokens,omitempty"`
	Stop          []string      `json:"stop,omitempty"`
}

type chatTool struct {
	Type     string           `json:"type"`
	Function chatToolFunction `json:"function"`
}

type chatToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  tool.JSONSchema `json:"parameters"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
}

type chatToolCall struct {
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function chatFunctionCall `json:"function"`
}

type chatFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content   string         `json:"content"`
			ToolCalls []chatToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
		TotalTokens      int64 `json:"total_tokens"`
	} `json:"usage,omitempty"`

	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

type chatStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content   string              `json:"content"`
			ToolCalls []chatToolCallDelta `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
		TotalTokens      int64 `json:"total_tokens"`
	} `json:"usage,omitempty"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

type chatToolCallDelta struct {
	Index    int                   `json:"index"`
	ID       string                `json:"id"`
	Type     string                `json:"type"`
	Function chatFunctionCallDelta `json:"function"`
}

type chatFunctionCallDelta struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type contentToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (c *Client) Generate(ctx context.Context, req llm.Request) (*llm.Response, error) {
	reqBody := buildChatRequest(c.cfg, req, false)

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.cfg.BaseURL+"/chat/completions",
		bytes.NewReader(data),
	)
	if err != nil {
		return nil, err
	}

	if c.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf(
			"ollama http %d: %s",
			resp.StatusCode,
			body,
		)
	}

	var out chatResponse

	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}

	if out.Error != nil {
		return nil, fmt.Errorf(
			"ollama error: %s",
			out.Error.Message,
		)
	}

	if len(out.Choices) == 0 {
		return nil, errors.New("ollama: empty response")
	}

	completion := &llm.Response{
		Text: out.Choices[0].Message.Content,
		Raw:  out,
	}
	completion.FinishReason = out.Choices[0].FinishReason

	if len(out.Choices[0].Message.ToolCalls) > 0 {
		completion.ToolCalls = toSchemaToolCalls(out.Choices[0].Message.ToolCalls)
		return completion, nil
	}

	contentToolCalls := parseContentToolCalls(out.Choices[0].Message.Content)
	if len(contentToolCalls) > 0 {
		completion.ToolCalls = contentToolCalls
	}

	if out.Usage != nil {
		completion.Usage = llm.Usage{
			InputTokens:  out.Usage.PromptTokens,
			OutputTokens: out.Usage.CompletionTokens,
			TotalTokens:  out.Usage.TotalTokens,
			Provider:     "ollama",
		}
	}

	return completion, nil
}

func (c *Client) GenerateStream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	reqBody := buildChatRequest(c.cfg, req, true)

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.cfg.BaseURL+"/chat/completions",
		bytes.NewReader(data),
	)
	if err != nil {
		return nil, err
	}

	if c.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama http %d: %s", resp.StatusCode, body)
	}

	ch := make(chan llm.StreamEvent, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		final := &llm.Response{Raw: map[string]any{"stream": true}}
		var textBuilder strings.Builder
		toolCalls := make(map[int]*schema.ToolCall)

		scanner := bufio.NewScanner(resp.Body)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 10*1024*1024)

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, ":") {
				continue
			}
			if !strings.HasPrefix(line, "data:") {
				continue
			}

			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "[DONE]" {
				break
			}

			var evt chatStreamResponse
			if err := json.Unmarshal([]byte(payload), &evt); err != nil {
				ch <- llm.StreamEvent{Type: llm.StreamEventError, Err: err}
				return
			}
			if evt.Error != nil {
				ch <- llm.StreamEvent{Type: llm.StreamEventError, Err: fmt.Errorf("ollama error: %s", evt.Error.Message)}
				return
			}

			if evt.Usage != nil {
				usage := llm.Usage{
					InputTokens:  evt.Usage.PromptTokens,
					OutputTokens: evt.Usage.CompletionTokens,
					TotalTokens:  evt.Usage.TotalTokens,
					Provider:     "ollama",
				}
				final.Usage = usage
				ch <- llm.StreamEvent{Type: llm.StreamEventUsageDelta, UsageDelta: &usage}
			}

			if len(evt.Choices) == 0 {
				continue
			}
			choice := evt.Choices[0]
			if choice.FinishReason != "" {
				final.FinishReason = choice.FinishReason
			}

			if choice.Delta.Content != "" {
				textBuilder.WriteString(choice.Delta.Content)
				ch <- llm.StreamEvent{Type: llm.StreamEventTextDelta, TextDelta: choice.Delta.Content}
			}

			for _, tc := range choice.Delta.ToolCalls {
				toolCall, ok := toolCalls[tc.Index]
				if !ok {
					toolCall = &schema.ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: json.RawMessage(`{}`)}
					toolCalls[tc.Index] = toolCall
				}
				if tc.ID != "" {
					toolCall.ID = tc.ID
				}
				if tc.Function.Name != "" {
					toolCall.Name = tc.Function.Name
				}

				prevArgs := string(toolCall.Arguments)
				if prevArgs == "" || prevArgs == "{}" {
					prevArgs = ""
				}
				if tc.Function.Arguments != "" {
					toolCall.Arguments = json.RawMessage(prevArgs + tc.Function.Arguments)
				}

				delta := *toolCall
				ch <- llm.StreamEvent{Type: llm.StreamEventToolCallDelta, ToolCallDelta: &delta}
			}
		}

		if err := scanner.Err(); err != nil {
			ch <- llm.StreamEvent{Type: llm.StreamEventError, Err: err}
			return
		}

		final.Text = textBuilder.String()
		if len(toolCalls) > 0 {
			ordered := make([]schema.ToolCall, 0, len(toolCalls))
			for i := 0; i < len(toolCalls); i++ {
				if tc, ok := toolCalls[i]; ok {
					if len(bytes.TrimSpace(tc.Arguments)) == 0 {
						tc.Arguments = json.RawMessage(`{}`)
					}
					ordered = append(ordered, *tc)
				}
			}
			final.ToolCalls = ordered
		}

		ch <- llm.StreamEvent{Type: llm.StreamEventDone, Response: final}
	}()

	return ch, nil
}

func buildChatRequest(cfg Config, req llm.Request, stream bool) chatRequest {
	reqBody := chatRequest{
		Model:    cfg.Model,
		Messages: make([]chatMessage, 0, len(req.Messages)),
		Tools:    make([]chatTool, 0, len(req.Tools)),
		Stream:   stream,
	}

	temperature := cfg.Temperature
	reqBody.Temperature = &temperature
	topP := cfg.TopP
	reqBody.TopP = &topP
	repeatPenalty := cfg.RepeatPenalty
	reqBody.RepeatPenalty = &repeatPenalty

	if req.Params.Temperature != nil {
		reqBody.Temperature = req.Params.Temperature
	}
	if req.Params.TopP != nil {
		reqBody.TopP = req.Params.TopP
	}
	if req.Params.RepeatPenalty != nil {
		reqBody.RepeatPenalty = req.Params.RepeatPenalty
	}
	if req.Params.MaxTokens != nil {
		reqBody.MaxTokens = req.Params.MaxTokens
	}
	if len(req.Params.Stop) > 0 {
		reqBody.Stop = req.Params.Stop
	}

	if !req.ToolChoice.IsZero() {
		reqBody.ToolChoice = mapToolChoice(req.ToolChoice)
	}

	for _, m := range req.Messages {
		cm := chatMessage{Role: string(m.Role), Content: m.Content}
		if m.ToolCallID != "" {
			cm.ToolCallID = m.ToolCallID
		}
		if len(m.ToolCalls) > 0 {
			cm.ToolCalls = make([]chatToolCall, len(m.ToolCalls))
			for i, tc := range m.ToolCalls {
				cm.ToolCalls[i] = chatToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: chatFunctionCall{
						Name:      tc.Name,
						Arguments: string(tc.Arguments),
					},
				}
			}
		}
		reqBody.Messages = append(reqBody.Messages, cm)
	}

	for _, t := range req.Tools {
		reqBody.Tools = append(reqBody.Tools, chatTool{
			Type: "function",
			Function: chatToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}

	return reqBody
}

func toSchemaToolCalls(calls []chatToolCall) []schema.ToolCall {
	out := make([]schema.ToolCall, len(calls))
	for i, tc := range calls {
		id := tc.ID
		if id == "" {
			id = fmt.Sprintf("call_%d", i+1)
		}
		out[i] = schema.ToolCall{
			ID:        id,
			Name:      tc.Function.Name,
			Arguments: json.RawMessage(tc.Function.Arguments),
		}
	}
	return out
}

func parseContentToolCalls(content string) []schema.ToolCall {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil
	}

	trimmed = stripCodeFence(trimmed)

	var single contentToolCall
	if err := json.Unmarshal([]byte(trimmed), &single); err == nil {
		if single.Name != "" {
			return []schema.ToolCall{normalizeContentToolCall(single, 1)}
		}
	}

	var many []contentToolCall
	if err := json.Unmarshal([]byte(trimmed), &many); err == nil {
		calls := make([]schema.ToolCall, 0, len(many))
		for i, call := range many {
			if call.Name == "" {
				continue
			}
			calls = append(calls, normalizeContentToolCall(call, i+1))
		}
		return calls
	}

	return nil
}

func normalizeContentToolCall(call contentToolCall, idx int) schema.ToolCall {
	id := call.ID
	if id == "" {
		id = fmt.Sprintf("call_%d", idx)
	}

	args := json.RawMessage(call.Arguments)
	if len(bytes.TrimSpace(args)) == 0 {
		args = json.RawMessage(`{}`)
	}

	return schema.ToolCall{
		ID:        id,
		Name:      call.Name,
		Arguments: args,
	}
}

func stripCodeFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}

	lines := strings.Split(s, "\n")
	if len(lines) < 3 {
		return s
	}

	start := 1
	end := len(lines)
	if strings.TrimSpace(lines[len(lines)-1]) == "```" {
		end = len(lines) - 1
	}

	if start >= end {
		return s
	}

	return strings.Join(lines[start:end], "\n")
}

func mapToolChoice(choice llm.ToolChoice) any {
	switch choice.Mode {
	case llm.ToolChoiceAuto:
		return "auto"
	case llm.ToolChoiceNone:
		return "none"
	case llm.ToolChoiceRequired:
		return "required"
	case llm.ToolChoiceNamed:
		if choice.Name == "" {
			return "auto"
		}
		return map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": choice.Name,
			},
		}
	default:
		return "auto"
	}
}
