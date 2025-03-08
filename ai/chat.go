package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	_ "github.com/expki/go-vectorsearch/env"
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
	request.Stream = false
	body, err := json.Marshal(request)
	if err != nil {
		return response, errors.Join(errors.New("failed to marshal request body"), err)
	}
	// Create request
	uri, uriDone := ai.Url()
	defer uriDone()
	uri.Path = "/api/chat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri.String(), bytes.NewReader(body))
	if err != nil {
		return response, errors.Join(errors.New("failed to create request"), err)
	}
	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if ai.token != "" {
		req.Header.Set("Authorization", "Bearer "+ai.token)
	}
	// Send request
	resp, err := ai.client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
			return response, err
		}
		return response, errors.Join(errors.New("failed to send request"), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return response, fmt.Errorf("response returned bad status code: %d", resp.StatusCode)
	}
	// Read response
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return response, errors.Join(errors.New("failed to read response body"), err)
	}
	err = json.Unmarshal(buf, &response)
	if err != nil {
		return response, errors.Join(errors.New("failed to unmarshal response"), err)
	}
	return response, nil
}

type chatStream struct {
	Model     string      `json:"model"`
	CreatedAt time.Time   `json:"created_at"`
	Message   ChatMessage `json:"message"`
	Done      bool        `json:"done"`
}

func (ai *Ollama) ChatStream(ctx context.Context, request ChatRequest) (stream io.Reader) {
	request.Stream = true
	stream, writer := io.Pipe()

	go func() {
		defer writer.Close()
		// Create request body
		body, err := json.Marshal(request)
		if err != nil {
			writer.CloseWithError(errors.Join(errors.New("failed to marshal request body"), err))
			return
		}
		// Create request
		uri, uriDone := ai.Url()
		defer uriDone()
		uri.Path = "/api/chat"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri.String(), bytes.NewReader(body))
		if err != nil {
			writer.CloseWithError(errors.Join(errors.New("failed to create request"), err))
			return
		}
		// Set headers
		req.Header.Set("Content-Type", "application/json")
		if ai.token != "" {
			req.Header.Set("Authorization", "Bearer "+ai.token)
		}
		// Send request
		resp, err := ai.client.Do(req)
		if err != nil {
			writer.CloseWithError(errors.Join(errors.New("failed to send request"), err))
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			writer.CloseWithError(fmt.Errorf("response returned bad status code: %d", resp.StatusCode))
			return
		}
		// Read response
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err == io.EOF {
				break
			}
			if err != nil {
				writer.CloseWithError(err)
				return
			}
			var res chatStream
			err = json.Unmarshal([]byte(line), &res)
			if err != nil {
				writer.CloseWithError(err)
				return
			}
			writer.Write([]byte(res.Message.Content))
			if res.Done {
				break
			}
		}
	}()

	return stream
}
