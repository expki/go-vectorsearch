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
	"time"

	"github.com/cespare/xxhash"
	"github.com/expki/go-vectorsearch/ai"
	"github.com/expki/go-vectorsearch/database"
	_ "github.com/expki/go-vectorsearch/env"
	"github.com/expki/go-vectorsearch/logger"
	"gorm.io/gorm/clause"
	"gorm.io/plugin/dbresolver"
)

type UploadRequest struct {
	Prefix    string `json:"prefix,omitempty"`
	Documents []any  `json:"documents"`
}

type UploadResponse struct {
	DocumentIDs []uint64 `json:"document_ids"`
}

func (s *server) UploadHttp(w http.ResponseWriter, r *http.Request) {
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
	flattenedFiles := make([]string, len(req.Documents))
	information := make([]string, len(req.Documents))
	for idx, file := range req.Documents {
		info := Flatten(file)
		flattenedFiles[idx] = info
		if req.Prefix != "" {
			info = fmt.Sprintf(`%s. %s`, req.Prefix, info)
		}
		information[idx] = fmt.Sprintf("search_document: %s", info)
	}

	// Get embeddings
	logger.Sugar().Debugf("%d request embeddings from ollama", txid)
	embedRes, err := s.ai.Embed(r.Context(), ai.EmbedRequest{
		Model: s.config.Ollama.Embed,
		Input: information,
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
	logger.Sugar().Debugf("%d saving documents to database", txid)
	documents := make([]database.Document, len(embedRes.Embeddings))
	for idx, embedding := range embedRes.Embeddings.Underlying() {
		file, _ := json.Marshal(req.Documents[idx])
		embedding := database.Document{
			Vector:   embedding,
			Prefix:   req.Prefix,
			Document: file,
			Hash:     strconv.FormatUint(xxhash.Sum64([]byte(flattenedFiles[idx])), 36),
		}
		documents[idx] = embedding
	}
	result := s.db.Clauses(
		dbresolver.Write,
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "hash"}},
			DoUpdates: clause.AssignmentColumns([]string{"updated_at", "prefix", "vector"}),
		},
	).WithContext(r.Context()).Create(&documents)
	if result.Error != nil {
		if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
			logger.Sugar().Debugf("%d request canceled after %dms while saving", txid, time.Since(start).Milliseconds())
			w.WriteHeader(499)
			io.WriteString(w, `{"error":"Client canceled request during record document"}`)
			return
		}
		logger.Sugar().Errorf("%d database record failed: %v", txid, result.Error)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"Record failed"}`)
		return
	}

	// Create response
	res := UploadResponse{
		DocumentIDs: make([]uint64, len(documents)),
	}
	for idx, embedding := range documents {
		res.DocumentIDs[idx] = embedding.ID
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

// Upload calculates the embedding for the uploaded document then saves the document and embedding in the database.
func (s *server) Upload(ctx context.Context, req UploadRequest) (res UploadResponse, err error) {
	// Flatten each file and apply prefix
	flattenedFiles := make([]string, len(req.Documents))
	information := make([]string, len(req.Documents))
	for idx, file := range req.Documents {
		info := Flatten(file)
		flattenedFiles[idx] = info
		if req.Prefix != "" {
			info = fmt.Sprintf(`%s. %s`, req.Prefix, info)
		}
		information[idx] = fmt.Sprintf("search_document: %s", info)
	}

	// Get embeddings
	embedRes, err := s.ai.Embed(ctx, ai.EmbedRequest{
		Model: s.config.Ollama.Embed,
		Input: information,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
			return res, err
		}
		return res, errors.Join(errors.New("failed to embed documents"), err)
	}

	// Save embeddings and documents
	documents := make([]database.Document, len(embedRes.Embeddings))
	for idx, embedding := range embedRes.Embeddings.Underlying() {
		file, _ := json.Marshal(req.Documents[idx])
		embedding := database.Document{
			Vector:   embedding,
			Prefix:   req.Prefix,
			Document: file,
			Hash:     strconv.FormatUint(xxhash.Sum64([]byte(flattenedFiles[idx])), 36),
		}
		documents[idx] = embedding
	}
	result := s.db.Clauses(
		dbresolver.Write,
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "hash"}},
			DoUpdates: clause.AssignmentColumns([]string{"updated_at", "prefix", "vector"}),
		},
	).WithContext(ctx).Create(&documents)
	if result.Error != nil {
		if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
			return res, result.Error
		}
		return res, errors.Join(errors.New("failed to save document"), result.Error)
	}

	// Create response
	res.DocumentIDs = make([]uint64, len(documents))
	for idx, embedding := range documents {
		res.DocumentIDs[idx] = embedding.ID
	}

	return res, nil
}
