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
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
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
) (*llm.Completion, error) {

	reqBody := chatRequest{
		Model:    c.cfg.Model,
		Messages: make([]chatMessage, 0, len(msgs)),
	}

	for _, m := range msgs {
		reqBody.Messages = append(reqBody.Messages, chatMessage{
			Role:    string(m.Role),
			Content: m.Content,
		})
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

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

	return &llm.Completion{
		Text: out.Choices[0].Message.Content,
	}, nil
}
