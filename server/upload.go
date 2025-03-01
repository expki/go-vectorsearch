package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/expki/govecdb/ai"
	"github.com/expki/govecdb/database"
	_ "github.com/expki/govecdb/env"
	"github.com/expki/govecdb/logger"
	"gorm.io/plugin/dbresolver"
)

type UploadRequest struct {
	Prefix      string           `json:"prefix,omitempty"`
	Documents   []map[string]any `json:"documents"`
	Information []string         `json:"-"`
}

type UploadResponse struct {
	DocumentIDs []uint `json:"document_ids"`
}

func (s *server) Upload(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	txid := index.Add(1)
	logger.Sugar().Debugf("%d upload request started", txid)
	w.Header().Set("Content-Type", "application/json")

	// Ensure the request method is POST
	if r.Method != http.MethodPost {
		logger.Sugar().Debugf("%d request method denied: %s", txid, r.Method)
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		io.WriteString(w, `{"error":"Invalid request method"}`)
		return
	}

	// Read the request body
	logger.Sugar().Debugf("%d reading request body", txid)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Sugar().Debugf("%d request body invalid: %v", txid, err)
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":"Invalid request body"}`)
		return
	}
	defer r.Body.Close()

	// Parse the JSON request body into the RequestBody struct
	logger.Sugar().Debugf("%d unmarshing request body", txid)
	var req UploadRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		logger.Sugar().Debugf("%d request invalid: %v", txid, err)
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":"Invalid request"}`)
		return
	}

	// Flatten each file and apply prefix
	logger.Sugar().Debugf("%d flattening files", txid)
	req.Information = make([]string, len(req.Documents))
	for idx, file := range req.Documents {
		info := FlattenMap(file)
		if req.Prefix != "" {
			info = fmt.Sprintf("%s %s", req.Prefix, info)
		}
		req.Information[idx] = fmt.Sprintf("search_document: %s", info)
	}

	// Get embeddings
	logger.Sugar().Debugf("%d request embeddings from ollama", txid)
	embedRes, err := s.ai.Embed(r.Context(), ai.EmbedRequest{
		Model: s.config.Ollama.Embed,
		Input: req.Information,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
			logger.Sugar().Debugf("%d request canceled after %dms while embedding", txid, time.Since(start).Milliseconds())
			w.WriteHeader(499)
			io.WriteString(w, `{"error":"Client canceled request during generating embedding"}`)
			return
		}
		logger.Sugar().Errorf("%d ai embed failed: %v", txid, err)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"Embedding failed"}`)
		return
	}

	// Save embeddings and documents
	logger.Sugar().Debugf("%d saving embeddings to database", txid)
	embeddings := make([]database.Embedding, len(embedRes.Embeddings))
	for idx, embedding := range embedRes.Embeddings.Underlying() {
		file, _ := json.Marshal(req.Documents[idx])
		document := database.Document{
			Prefix:   req.Prefix,
			Document: file,
		}
		embedding := database.Embedding{
			Vector:   embedding,
			Document: document,
		}
		embeddings[idx] = embedding
	}
	result := s.db.Clauses(dbresolver.Write).WithContext(r.Context()).Create(&embeddings)
	if result.Error != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
			logger.Sugar().Debugf("%d request canceled after %dms while saving", txid, time.Since(start).Milliseconds())
			w.WriteHeader(499)
			io.WriteString(w, `{"error":"Client canceled request during record embedding"}`)
			return
		}
		logger.Sugar().Errorf("%d database record failed: %v", txid, result.Error)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"Record failed"}`)
		return
	}

	// Create response
	res := UploadResponse{
		DocumentIDs: make([]uint, len(embeddings)),
	}
	for idx, embedding := range embeddings {
		res.DocumentIDs[idx] = embedding.Document.ID
	}
	resBytes, err := json.Marshal(res)
	if result.Error != nil {
		logger.Sugar().Errorf("%d database response marshal failed: %v", txid, err)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"Response failed"}`)
		return
	}

	// Set the response headers and write the JSON response
	w.WriteHeader(http.StatusOK)
	w.Write(resBytes)
	logger.Sugar().Infof("%d upload request suceeded (%dms)", txid, time.Since(start).Milliseconds())
}
