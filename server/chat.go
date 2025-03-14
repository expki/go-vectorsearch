package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/expki/go-vectorsearch/ai"
	"github.com/expki/go-vectorsearch/database"
	_ "github.com/expki/go-vectorsearch/env"
	"github.com/expki/go-vectorsearch/logger"
	"gorm.io/plugin/dbresolver"
)

type ChatRequest struct {
	Prefix      string   `json:"prefix,omitempty"`
	History     []string `json:"history,omitempty"`
	Text        string   `json:"text"`
	DocumentIDs []uint   `json:"document_ids,omitempty"`
	Documents   []any    `json:"documents,omitempty"`
}

func (s *server) ChatHttp(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	txid := index.Add(1)
	logger.Sugar().Debugf("%d chat request started", txid)
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Transfer-Encoding", "chunked")

	// Ensure the request method is POST or GET
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		logger.Sugar().Debugf("%d request method denied: %s", txid, r.Method)
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		io.WriteString(w, `Invalid request method`)
		return
	}

	// Read the request body
	logger.Sugar().Debugf("%d reading request body", txid)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Sugar().Debugf("%d request body invalid: %v", txid, err)
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `Invalid request body`)
		return
	}
	defer r.Body.Close()

	// Parse the JSON request body into the RequestBody struct
	logger.Sugar().Debugf("%d unmarshing request body", txid)
	var req ChatRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		logger.Sugar().Debugf("%d request invalid: %v", txid, err)
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `Invalid request`)
		return
	}

	// Handle the chat request
	resStream, errStream := s.Chat(r.Context(), req)

	// Chunk flusher
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Failed to stream response", http.StatusInternalServerError)
		return
	}

	// Write chat response in chunks
	for {
		select {
		case <-r.Context().Done():
			// finish reading streams
			go func() {
				for range resStream {
				}
				for range errStream {
				}
			}()
			return
		case res, hasMore := <-resStream:
			if res != nil {
				w.Write(res)
				flusher.Flush()
			}
			if !hasMore {
				logger.Sugar().Infof("%d chat request suceeded (%dms)", txid, time.Since(start).Milliseconds())
				return
			}
		case err := <-errStream:
			if err == nil {
				// we are done
			} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
				// chat request canceled
				logger.Sugar().Warnf("%d chat request canceled after %s", txid, time.Since(start).String())
			} else {
				// chat failed
				logger.Sugar().Errorf("%d chat request failed: %s", txid, err.Error())
			}
			return
		}
	}
}

// Chat handles a chat request and returns a stream of bytes for the response.
func (s *server) Chat(ctx context.Context, req ChatRequest) (resStream <-chan []byte, errStream <-chan error) {
	resChan := make(chan []byte, 1024)
	errChan := make(chan error, 1)

	go func() {
		defer func() {
			close(resChan)
			close(errChan)
		}()
		// Get documents
		var documents []database.Document
		if len(req.DocumentIDs) > 0 {
			result := s.db.Clauses(dbresolver.Read).WithContext(ctx).Find(&documents, req.DocumentIDs)
			if result.Error != nil {
				if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
					errChan <- result.Error
					return
				}
				errChan <- errors.Join(fmt.Errorf(`retrieving documents failed`), result.Error)
				return
			}
		}
		for _, doc := range documents {
			req.Documents = append(req.Documents, doc.Document.JSON())
		}

		// Create history chat
		messages := make([]ai.ChatMessage, len(req.History), len(req.History)+1)
		for idx, content := range req.History {
			var role string
			if idx%2 == 0 {
				role = "user"
			} else {
				role = "assistant"
			}
			messages[idx] = ai.ChatMessage{
				Role:    role,
				Content: content,
			}
		}

		// Add document context
		var query strings.Builder
		if len(req.Documents) > 0 {
			query.WriteString("I have ")
			query.WriteString(strconv.Itoa(len(req.Documents)))
			query.WriteString(" text document that I'd like to use as context for my question. Here's the relevant part")
			if len(req.Documents) > 1 {
				query.WriteRune('s')
			}
			query.WriteString(":\n\n")
			for _, doc := range req.Documents {
				query.WriteRune('"')
				query.WriteString(Flatten(doc))
				query.WriteRune('"')
				query.WriteRune('\n')
			}
			query.WriteRune('\n')
		}

		// Construct question
		query.WriteString("My question is: ")

		// Add query
		if req.Prefix != "" {
			req.Text = fmt.Sprintf(`%s. %s`, req.Prefix, req.Text)
		}
		query.WriteString(req.Text)

		// Construct message
		messages = append(messages, ai.ChatMessage{
			Role:    "user",
			Content: query.String(),
		})

		// Start chat
		chat := s.ai.ChatStream(ctx, ai.ChatRequest{
			Model:    s.config.Ollama.Chat,
			Messages: messages,
		})

		// Write chat response in chunks
		buf := make([]byte, 1)
		for {
			// Read one byte from the io.Reader
			n, err := chat.Read(buf)
			if err != nil {
				if err == io.EOF {
					break
				}
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
					errChan <- err
					return
				}
				errChan <- errors.Join(errors.New("chat failed"), err)
				return
			}

			// Write the byte to the response
			for idx := range buf[:n] {
				resChan <- buf[idx : idx+1]
			}
		}
	}()

	return resChan, errChan
}
