package ollama

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

	"github.com/expki/go-vectorsearch/ai/aicomms"
	"github.com/expki/go-vectorsearch/ai/httpclient"
	_ "github.com/expki/go-vectorsearch/env"
)

func (ai *Ollama) Embed(ctx context.Context, request aicomms.EmbedRequest) (response aicomms.EmbedResponse, err error) {
	if request.Options == nil {
		request.Options = map[string]any{
			"num_ctx": ai.embed.Cfg.NumCtx,
		}
	} else if _, ok := request.Options["num_ctx"]; !ok {
		request.Options["num_ctx"] = ai.embed.Cfg.NumCtx
	}
	// Create request body
	body, err := json.Marshal(request)
	if err != nil {
		return response, errors.Join(errors.New("failed to marshal request body"), err)
	}
	// Request compression
	if ai.embed.Cfg.RequestCompression {
		body = httpclient.Compress(body)
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
	if ai.embed.Cfg.RequestCompression {
		req.Header.Set("Content-Encoding", "zstd")
	}
	req.Header.Set("Accept-Encoding", "zstd")
	if ai.embed.Token != "" {
		req.Header.Set("Authorization", "Bearer "+ai.embed.Token)
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
			body, err = httpclient.Decompress(body)
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
