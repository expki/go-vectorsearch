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
	"strings"
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
	if request.Options == nil {
		request.Options = map[string]any{
			"num_ctx": ai.embed.cfg.NumCtx,
		}
	} else if _, ok := request.Options["num_ctx"]; !ok {
		request.Options["num_ctx"] = ai.embed.cfg.NumCtx
	}
	// Create request body
	body, err := json.Marshal(request)
	if err != nil {
		return response, errors.Join(errors.New("failed to marshal request body"), err)
	}
	// Request compression
	if ai.embed.compression {
		body = compress(body)
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
	if ai.embed.compression {
		req.Header.Set("Content-Encoding", "zstd")
	}
	req.Header.Set("Accept-Encoding", "zstd")
	if ai.embed.token != "" {
		req.Header.Set("Authorization", "Bearer "+ai.embed.token)
	}
	// Send request
	client, close := ai.clientManager.GetHttpClient(uri.Host)
	defer close()
	resp, err := client.Do(req)
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return response, err
	} else {
		return response, errors.Join(errors.New("failed to send request"), err)
	}
	// Read response
	body = nil
	if resp.Body != nil {
		body, err = io.ReadAll(resp.Body)
		if err != nil {
			resp.Body.Close()
			return response, errors.Join(errors.New("failed to read response body"), err)
		}
		resp.Body.Close()
		if strings.TrimSpace(strings.ToLower(resp.Header.Get("Content-Encoding"))) == "zstd" {
			body, err = decompress(body)
			if err != nil {
				return response, errors.Join(errors.New("failed to decompress response"), err)
			}
		}
	}
	if resp.StatusCode != http.StatusOK {
		return response, fmt.Errorf("%s (%d): %s", uri.String(), resp.StatusCode, string(body))
	}
	// Decode response
	err = json.Unmarshal(body, &response)
	if err != nil {
		return response, errors.Join(errors.New("failed to unmarshal response"), err)
	}
	return response, nil
}

type Embeddings []Embedding

func (e Embeddings) Value() [][]uint8 {
	value := make([][]uint8, len(e))
	for i, v := range e {
		value[i] = v
	}
	return value
}

type Embedding []uint8

func (e *Embedding) UnmarshalJSON(data []byte) error {
	var vector []float32
	err := json.Unmarshal(data, &vector)
	if err != nil {
		return err
	}
	*e = compute.QuantizeVectorFloat32(vector)
	return nil
}

func (e Embedding) Dims() int {
	return len(e) - 8
}

func (e Embedding) Value() []uint8 {
	return e
}
