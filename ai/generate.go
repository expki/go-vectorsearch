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
	GenerateStream
	Context            []int `json:"context"`
	TotalDuration      int64 `json:"total_duration"`
	LoadDuration       int64 `json:"load_duration"`
	PromptEvalCount    int   `json:"prompt_eval_count"`
	PromptEvalDuration int64 `json:"prompt_eval_duration"`
	EvalCount          int   `json:"eval_count"`
	EvalDuration       int64 `json:"eval_duration"`
}

func (ai *ai) Generate(ctx context.Context, request GenerateRequest) (response GenerateResponse, err error) {
	// Create request body
	request.Stream = false
	body, err := json.Marshal(request)
	if err != nil {
		return response, errors.Join(errors.New("failed to marshal request body"), err)
	}
	// Create request
	uri, uriDone := ai.generate.Url()
	defer uriDone()
	uri.Path = "/api/generate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri.String(), bytes.NewReader(body))
	if err != nil {
		return response, errors.Join(errors.New("failed to create request"), err)
	}
	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if ai.generate.token != "" {
		req.Header.Set("Authorization", "Bearer "+ai.generate.token)
	}
	// Send request
	resp, err := ai.client.Do(req)
	if err != nil {
		return response, errors.Join(errors.New("failed to send request"), err)
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
		return response, errors.Join(errors.New("failed to read response body"), err)
	}
	err = json.Unmarshal(buf, &response)
	if err != nil {
		return response, errors.Join(errors.New("failed to unmarshal response"), err)
	}
	return response, nil
}

type GenerateStream struct {
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	Response  string    `json:"response"`
	Done      bool      `json:"done"`
}

func (ai *ai) GenerateStream(ctx context.Context, request GenerateRequest) (stream io.Reader) {
	request.Stream = true
	stream, writer := io.Pipe()

	go func() {
		defer writer.Close()
		// Create request body

		body, err := json.Marshal(request)
		if err != nil {
			writer.CloseWithError(errors.Join(errors.New("failed to marshal request body"), err))
		}
		// Create request
		uri, uriDone := ai.generate.Url()
		defer uriDone()
		uri.Path = "/api/generate"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri.String(), bytes.NewReader(body))
		if err != nil {
			writer.CloseWithError(errors.Join(errors.New("failed to create request"), err))
			return
		}
		// Set headers
		req.Header.Set("Content-Type", "application/json")
		if ai.generate.token != "" {
			req.Header.Set("Authorization", "Bearer "+ai.generate.token)
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
			var res GenerateStream
			err = json.Unmarshal([]byte(line), &res)
			if err != nil {
				writer.CloseWithError(err)
				return
			}
			writer.Write([]byte(res.Response))
			if res.Done {
				break
			}
		}
	}()

	return stream
}
