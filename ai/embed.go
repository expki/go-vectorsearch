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

	"github.com/expki/go-vectorsearch/compute"
	"github.com/expki/go-vectorsearch/config"
	_ "github.com/expki/go-vectorsearch/env"
)

type EmbedRequest struct {
	// Standard params
	Model string                       `json:"model"`
	Input config.SingleOrSlice[string] `json:"input"`
	// Advanced params
	Truncate  *bool          `json:"truncate,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
	KeepAlive *time.Duration `json:"keep_alive,omitempty"`
}

type EmbedResponse struct {
	Model           string     `json:"model"`
	Embeddings      Embeddings `json:"embeddings"`
	Done            bool       `json:"done"`
	TotalDuration   int64      `json:"total_duration"`
	LoadDuration    int64      `json:"load_duration"`
	PromptEvalCount int        `json:"prompt_eval_count"`
}

func (ai *ai) Embed(ctx context.Context, request EmbedRequest) (response EmbedResponse, err error) {
	// Create request body
	body, err := json.Marshal(request)
	if err != nil {
		return response, errors.Join(errors.New("failed to marshal request body"), err)
	}
	// Create request
	uri, uriDone := ai.embed.Url()
	defer uriDone()
	uri.Path = "/api/embed"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri.String(), bytes.NewReader(body))
	if err != nil {
		return response, errors.Join(errors.New("failed to create request"), err)
	}
	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if ai.embed.token != "" {
		req.Header.Set("Authorization", "Bearer "+ai.embed.token)
	}
	// Send request
	client, close := ai.clientManager.GetHttpClient(uri.Host)
	defer close()
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
			return response, err
		}
		return response, errors.Join(errors.New("failed to send request"), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return response, fmt.Errorf("response returned bad status code %s: %d", uri.String(), resp.StatusCode)
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

type Embeddings [][]EmbeddingValue

func (e Embeddings) Underlying() [][]uint8 {
	out := make([][]uint8, len(e))
	for i, embedding := range e {
		out[i] = make([]uint8, len(embedding))
		for j, v := range embedding {
			out[i][j] = uint8(v)
		}
	}
	return out
}

type EmbeddingValue uint8

func (e *EmbeddingValue) UnmarshalJSON(data []byte) error {
	var value float32
	err := json.Unmarshal(data, &value)
	if err != nil {
		return err
	}
	*e = EmbeddingValue(compute.Quantize(value, -1, 1))
	return nil
}
