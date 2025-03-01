package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	_ "github.com/expki/govecdb/env"
)

type ChatRequest struct {
	Model     string            `json:"model"`
	Messages  []ChatMessage     `json:"messages"`
	Tools     []json.RawMessage `json:"tools,omitempty"`
	Format    string            `json:"format,omitempty"`
	Options   json.RawMessage   `json:"options,omitempty"`
	Stream    bool              `json:"stream"`
	KeepAlive *time.Duration    `json:"keep_alive,omitempty"`
}

type ChatResponse struct {
	Model              string      `json:"model"`
	CreatedAt          time.Time   `json:"created_at"`
	Message            ChatMessage `json:"message"`
	Done               bool        `json:"done"`
	Context            []int       `json:"context"`
	TotalDuration      int64       `json:"total_duration"`
	LoadDuration       int64       `json:"load_duration"`
	PromptEvalCount    int         `json:"prompt_eval_count"`
	PromptEvalDuration int64       `json:"prompt_eval_duration"`
	EvalCount          int         `json:"eval_count"`
	EvalDuration       int64       `json:"eval_duration"`
}

type ChatMessage struct {
	Role      string            `json:"role"`
	Content   string            `json:"content"`
	Images    []string          `json:"images,omitempty"`
	ToolCalls []json.RawMessage `json:"tool_calls,omitempty"`
}

func (ai *Ollama) Chat(ctx context.Context, request ChatRequest) (response ChatResponse, err error) {
	// Create request body
	body, err := json.Marshal(request)
	if err != nil {
		return response, fmt.Errorf("failed to marshal request body: %v", err)
	}
	// Create request
	uri := ai.uri
	uri.Path = "/api/chat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri.String(), bytes.NewReader(body))
	if err != nil {
		return response, fmt.Errorf("failed to create request: %v", err)
	}
	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ai.token)
	// Send request
	resp, err := ai.client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
			return response, err
		}
		return response, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return response, fmt.Errorf("response returned bad status code: %d", resp.StatusCode)
	}
	// Read response
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return response, fmt.Errorf("failed to read response body: %v", err)
	}
	err = json.Unmarshal(buf, &response)
	if err != nil {
		return response, fmt.Errorf("failed to unmarshal response: %v", err)
	}
	return response, nil
}
