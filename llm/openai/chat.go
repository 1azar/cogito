package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/1azar/cogito/llm"
	"github.com/1azar/cogito/schema"
	"github.com/1azar/cogito/tool"
)

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Tools       []chatTool    `json:"tools,omitempty"`
	ToolChoice  any           `json:"tool_choice,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	Stop        []string      `json:"stop,omitempty"`
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
	ID       string           `json:"id"`
	Type     string           `json:"type"`
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

func (c *Client) Generate(ctx context.Context, req llm.Request) (*llm.Response, error) {

	reqBody := chatRequest{
		Model:    c.cfg.Model,
		Messages: make([]chatMessage, 0, len(req.Messages)),
		Tools:    make([]chatTool, 0, len(req.Tools)),
	}

	if req.Params.Temperature != nil {
		reqBody.Temperature = req.Params.Temperature
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

	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf(
			"openai http %d: %s",
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
			"openai error: %s",
			out.Error.Message,
		)
	}

	if len(out.Choices) == 0 {
		return nil, errors.New("openai: empty response")
	}

	completion := &llm.Response{
		Text: out.Choices[0].Message.Content,
		Raw:  out,
	}
	completion.FinishReason = out.Choices[0].FinishReason

	if len(out.Choices[0].Message.ToolCalls) > 0 {
		completion.ToolCalls = make([]schema.ToolCall, len(out.Choices[0].Message.ToolCalls))
		for i, tc := range out.Choices[0].Message.ToolCalls {
			completion.ToolCalls[i] = schema.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: json.RawMessage(tc.Function.Arguments),
			}
		}
	}

	if out.Usage != nil {
		completion.Usage = llm.Usage{
			InputTokens:  out.Usage.PromptTokens,
			OutputTokens: out.Usage.CompletionTokens,
			TotalTokens:  out.Usage.TotalTokens,
			Provider:     "openai",
		}
	}

	return completion, nil
}

func (c *Client) GenerateStream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	resp, err := c.Generate(ctx, req)
	if err != nil {
		return nil, err
	}

	return llm.StreamFromResponse(resp), nil
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
