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
	"strings"

	"github.com/expki/go-vectorsearch/ai/aicomms"
	"github.com/expki/go-vectorsearch/ai/httpclient"
	_ "github.com/expki/go-vectorsearch/env"
)

func (ai *OpenAI) Generate(ctx context.Context, request aicomms.GenerateRequest) (response aicomms.GenerateResponse, err error) {
	if request.Options == nil {
		request.Options = map[string]any{
			"num_ctx": ai.generate.Cfg.NumCtx,
		}
	} else if _, ok := request.Options["num_ctx"]; !ok {
		request.Options["num_ctx"] = ai.generate.Cfg.NumCtx
	}
	// Create request body
	request.Stream = false
	body, err := json.Marshal(request)
	if err != nil {
		return response, errors.Join(errors.New("failed to marshal request body"), err)
	}
	// Create request
	uri, uriDone := ai.generate.Url()
	defer uriDone()
	uri.Path = "/v1/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri.String(), bytes.NewReader(body))
	if err != nil {
		return response, errors.Join(errors.New("failed to create request"), err)
	}
	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if ai.generate.Token != "" {
		req.Header.Set("Authorization", "Bearer "+ai.generate.Token)
	}
	// Send request
	client, close := ai.clientManager.GetHttpClient(uri.Host)
	defer close()
	resp, err := client.Do(req)
	if err != nil {
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
	err = json.Unmarshal(body, &response)
	if err != nil {
		return response, errors.Join(errors.New("failed to unmarshal response"), err)
	}
	return response, nil
}

func (ai *OpenAI) GenerateStream(ctx context.Context, request aicomms.GenerateRequest) (stream io.Reader) {
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
		uri.Path = "/v1/completions"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri.String(), bytes.NewReader(body))
		if err != nil {
			writer.CloseWithError(errors.Join(errors.New("failed to create request"), err))
			return
		}
		// Set headers
		req.Header.Set("Content-Type", "application/json")
		if ai.generate.Token != "" {
			req.Header.Set("Authorization", "Bearer "+ai.generate.Token)
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
			var res aicomms.GenerateStream
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
