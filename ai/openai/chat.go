package openai

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
	"strings"
	"time"

	"github.com/expki/go-vectorsearch/ai/aicomms"
	"github.com/expki/go-vectorsearch/ai/httpclient"
	_ "github.com/expki/go-vectorsearch/env"
)

func (ai *OpenAI) Chat(ctx context.Context, request aicomms.ChatRequest) (response aicomms.ChatResponse, err error) {
	if request.Options == nil {
		request.Options = map[string]any{
			"num_ctx": ai.chat.Cfg.NumCtx,
		}
	} else if _, ok := request.Options["num_ctx"]; !ok {
		request.Options["num_ctx"] = ai.chat.Cfg.NumCtx
	}
	// Create request body
	request.Stream = false
	body, err := json.Marshal(request)
	if err != nil {
		return response, errors.Join(errors.New("failed to marshal request body"), err)
	}
	// Create request
	uri, uriDone := ai.chat.Url()
	defer uriDone()
	uri.Path = "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri.String(), bytes.NewReader(body))
	if err != nil {
		return response, errors.Join(errors.New("failed to create request"), err)
	}
	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if ai.chat.Token != "" {
		req.Header.Set("Authorization", "Bearer "+ai.chat.Token)
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

type chatStream struct {
	Model     string              `json:"model"`
	CreatedAt time.Time           `json:"created_at"`
	Message   aicomms.ChatMessage `json:"message"`
	Done      bool                `json:"done"`
}

func (ai *OpenAI) ChatStream(ctx context.Context, request aicomms.ChatRequest) (stream io.ReadCloser) {
	if request.Options == nil {
		request.Options = map[string]any{
			"num_ctx": ai.chat.Cfg.NumCtx,
		}
	} else if _, ok := request.Options["num_ctx"]; !ok {
		request.Options["num_ctx"] = ai.chat.Cfg.NumCtx
	}
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
		uri, uriDone := ai.chat.Url()
		defer uriDone()
		uri.Path = "/v1/chat/completions"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri.String(), bytes.NewReader(body))
		if err != nil {
			writer.CloseWithError(errors.Join(errors.New("failed to create request"), err))
			return
		}
		// Set headers
		req.Header.Set("Content-Type", "application/json")
		if ai.chat.Token != "" {
			req.Header.Set("Authorization", "Bearer "+ai.chat.Token)
		}
		// Send request
		client, close := ai.clientManager.GetHttpClient(uri.Host)
		defer close()
		resp, err := client.Do(req)
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
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			var res chatStream
			err = json.Unmarshal([]byte(scanner.Text()), &res)
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
