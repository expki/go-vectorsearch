package server

import (
	"bufio"
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
	DocumentIDs []uint64 `json:"document_ids,omitempty"`
	Documents   []any    `json:"documents,omitempty"`
}

func (s *Server) ChatHttp(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	txid := index.Add(1)
	logger.Sugar().Debugf("%d chat request started", txid)
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Transfer-Encoding", "chunked")

	// Ensure the ResponseWriter supports streaming
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

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
	resStream, err := s.Chat(r.Context(), req)
	if err != nil {
		http.Error(w, "Failed to initialize chat", http.StatusInternalServerError)
		return
	}
	defer resStream.Close()
	charReader := bufio.NewReader(resStream)

	// Stream the response
	for {
		char, _, err := charReader.ReadRune()
		if err != nil {
			if err == io.EOF {
				break
			}
			http.Error(w, "Error reading stream", http.StatusInternalServerError)
			return
		}

		_, writeErr := io.WriteString(w, string(char))
		if writeErr != nil {
			return
		}

		flusher.Flush()
	}

	logger.Sugar().Infof("%d chat request suceeded (%dms)", txid, time.Since(start).Milliseconds())
}

// Chat handles a chat request and returns a stream of bytes for the response.
func (s *Server) Chat(ctx context.Context, req ChatRequest) (resStream io.ReadCloser, err error) {
	// Get documents
	var documents []database.Document
	if len(req.DocumentIDs) > 0 {
		err = s.db.Clauses(dbresolver.Read).WithContext(ctx).Select("document").Find(&documents, req.DocumentIDs).Error
		if err == nil {
		} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
			return nil, err
		} else {
			return nil, errors.Join(fmt.Errorf(`retrieving documents failed`), err)
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
			query.WriteString(`"""`)
			query.WriteString(Flatten(doc))
			query.WriteString(`"""`)
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
		Model:    s.config.Chat,
		Messages: messages,
	})

	return chat, err
}
