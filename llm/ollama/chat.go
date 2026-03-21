package ollama

import (
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

	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

type contentToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (c *Client) Generate(ctx context.Context, req llm.Request) (*llm.Response, error) {
	reqBody := chatRequest{
		Model:    c.cfg.Model,
		Messages: make([]chatMessage, 0, len(req.Messages)),
		Tools:    make([]chatTool, 0, len(req.Tools)),
	}

	temperature := c.cfg.Temperature
	reqBody.Temperature = &temperature
	topP := c.cfg.TopP
	reqBody.TopP = &topP
	repeatPenalty := c.cfg.RepeatPenalty
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
		cm := chatMessage{
			Role:    string(m.Role),
			Content: m.Content,
		}
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

	return completion, nil
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
