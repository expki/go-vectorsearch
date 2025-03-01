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

type GenerateRequest struct {
	// Standard params
	Model  string `json:"model"`
	Prompt string `json:"prompt,omitempty"`
	Suffix string `json:"suffix,omitempty"`
	Images string `json:"images,omitempty"`
	// Advanced params
	Format    string         `json:"format,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
	System    string         `json:"system,omitempty"`
	Template  string         `json:"template,omitempty"`
	Stream    bool           `json:"stream"`
	Raw       bool           `json:"raw"`
	KeepAlive *time.Duration `json:"keep_alive,omitempty"`
}

type GenerateResponse struct {
	Model              string    `json:"model"`
	CreatedAt          time.Time `json:"created_at"`
	Response           string    `json:"response"`
	Done               bool      `json:"done"`
	Context            []int     `json:"context"`
	TotalDuration      int64     `json:"total_duration"`
	LoadDuration       int64     `json:"load_duration"`
	PromptEvalCount    int       `json:"prompt_eval_count"`
	PromptEvalDuration int64     `json:"prompt_eval_duration"`
	EvalCount          int       `json:"eval_count"`
	EvalDuration       int64     `json:"eval_duration"`
}

func (ai *Ollama) Generate(ctx context.Context, request GenerateRequest) (response GenerateResponse, err error) {
	// Create request body
	body, err := json.Marshal(request)
	if err != nil {
		return response, fmt.Errorf("failed to marshal request body: %v", err)
	}
	// Create request
	uri := ai.uri
	uri.Path = "/api/generate"
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
		return response, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return response, fmt.Errorf("response returned bad status code: %d", resp.StatusCode)
	}
	// Read response
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
			return response, err
		}
		return response, fmt.Errorf("failed to read response body: %v", err)
	}
	err = json.Unmarshal(buf, &response)
	if err != nil {
		return response, fmt.Errorf("failed to unmarshal response: %v", err)
	}
	return response, nil
}
