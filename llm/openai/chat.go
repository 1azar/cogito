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
)

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Tools       []chatTool    `json:"tools,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

type chatTool struct {
	Type     string         `json:"type"`
	Function map[string]any `json:"function"`
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
	} `json:"choices"`

	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (c *Client) Generate(
	ctx context.Context,
	msgs []schema.Message,
	tools []map[string]any,
) (*llm.Completion, error) {

	reqBody := chatRequest{
		Model:    c.cfg.Model,
		Messages: make([]chatMessage, 0, len(msgs)),
		Tools:    make([]chatTool, 0, len(tools)),
	}

	for _, m := range msgs {
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

	for _, t := range tools {
		reqBody.Tools = append(reqBody.Tools, chatTool{
			Type:     t["type"].(string),
			Function: t["function"].(map[string]any),
		})
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	// Debug: print request body
	fmt.Printf("[DEBUG] OpenAI Request: %s\n", string(data))

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.cfg.BaseURL+"/chat/completions",
		bytes.NewReader(data),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
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

	completion := &llm.Completion{
		Text: out.Choices[0].Message.Content,
	}

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

	return completion, nil
}
