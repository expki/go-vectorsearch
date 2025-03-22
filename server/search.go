package server

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/expki/go-vectorsearch/ai"
	"github.com/expki/go-vectorsearch/compute"
	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/database"
	_ "github.com/expki/go-vectorsearch/env"
	"github.com/expki/go-vectorsearch/logger"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

type SearchRequest struct {
	Owner     string `json:"owner"`
	Category  string `json:"category"`
	Prefix    string `json:"prefix,omitempty"`
	Text      string `json:"text"`
	Count     uint   `json:"count"`
	Offset    uint   `json:"offset,omitempty"`
	Centroids int    `json:"centroids,omitempty"`
}

type SearchResponse struct {
	Documents []DocumentSearch `json:"documents"`
}

type DocumentSearch struct {
	DocumentUpload
	DocumentID         uint64  `json:"document_id"`
	DocumentSimilarity float32 `json:"document_similarity"`
	CentroidSimilarity float32 `json:"centroid_similarity"`
}

func (s *Server) SearchHttp(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	txid := index.Add(1)
	logger.Sugar().Debugf("%d search request started", txid)
	w.Header().Set("Content-Type", "application/json")

	// Ensure the request method is POST or GET
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
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
	var req SearchRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		logger.Sugar().Debugf("%d request invalid: %v", txid, err)
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":"Invalid request"}`)
		return
	}

	// Handle the search request
	res, err := s.Search(r.Context(), req)
	if err == nil {
		// search was successful
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		// search request canceled
		logger.Sugar().Warnf("%d search request canceled after %s", txid, time.Since(start).String())
		w.WriteHeader(499)
		io.WriteString(w, `{"error":"Client canceled search request"}`)
		return
	} else {
		// search failed
		logger.Sugar().Errorf("%d search request failed: %s", txid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"Search request failed"}`)
		return
	}

	resBytes, err := json.Marshal(res)
	if err != nil {
		logger.Sugar().Errorf("%d database response marshal failed: %v", txid, err)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"Response failed"}`)
		return
	}

	// Set the response headers and write the JSON response
	w.WriteHeader(http.StatusOK)
	w.Write(resBytes)
	logger.Sugar().Infof("%d search request suceeded (%dms)", txid, time.Since(start).Milliseconds())
}

// Search for a previously uploaded embedding vector in the database and return similar documents.
func (s *Server) Search(ctx context.Context, req SearchRequest) (res SearchResponse, err error) {
	req.Count = max(1, min(req.Count, 20))
	req.Offset = max(0, req.Offset)
	if req.Centroids == 0 {
		req.Centroids = 1
	} else if req.Centroids < 0 {
		req.Centroids = math.MaxInt
	}

	// Get embedding
	if req.Prefix != "" {
		req.Text = fmt.Sprintf(`%s. %s`, req.Prefix, req.Text)
	}
	embedRes, err := s.ai.Embed(ctx, ai.EmbedRequest{
		Model: s.config.Embed,
		Input: []string{fmt.Sprintf("search_query: %s", req.Text)},
	})
	if err == nil {
		// success
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		// request canceled
		return res, err
	} else {
		// exception encountered
		return res, errors.Join(errors.New("failed to embed search query"), err)
	}
	if len(embedRes.Embeddings) < 1 {
		return res, errors.New("embedding returned empty response")
	}
	target := compute.NewTensor(embedRes.Embeddings.Underlying()[0])

	// Get Owner
	owner := database.Owner{Name: req.Owner}
	result := s.db.Clauses(dbresolver.Read).WithContext(ctx).Where("name = ?", req.Owner).Take(&owner)
	if result.Error == nil {
		// owner found
	} else if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
		// owner request canceled
		return res, result.Error
	} else if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		// owner not found
		return res, nil
	} else {
		// owner retrieve error
		return res, errors.Join(errors.New("failed to get owner"), result.Error)
	}

	// Get Category
	category := database.Category{Name: req.Category, OwnerID: owner.ID, Owner: owner}
	result = s.db.Clauses(dbresolver.Read).WithContext(ctx).Where("name = ? AND owner_id = ?", req.Category, owner.ID).Select("id").Take(&category)
	if result.Error == nil {
		// category found
	} else if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
		// category request canceled
		return res, result.Error
	} else if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		// category not found
		return res, nil
	} else {
		// category retrieve error
		return res, errors.Join(errors.New("failed to get category"), result.Error)
	}

	// Get Centroids
	var centroids []database.Centroid
	result = s.db.Clauses(dbresolver.Read).WithContext(ctx).Where("category_id = ?", category.ID).Select("id", "vector").Find(&centroids)
	if result.Error == nil {
		// centroids found
	} else if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
		// centroids request canceled
		return res, result.Error
	} else {
		// centroids retrieve error
		return res, errors.Join(errors.New("failed to get centroids"), result.Error)
	}
	if len(centroids) == 0 {
		return res, nil
	}

	// Find closest centroids to embedding
	type centroidSimilarity struct {
		centroid   database.Centroid
		similarity float32
	}
	closestCentroids := make([]centroidSimilarity, len(centroids))
	// Convert centroids to matrix format for cosine similarity calculation
	matrixCentroids := make([][]uint8, len(centroids))
	for idx, centroid := range centroids {
		matrixCentroids[idx] = centroid.Vector
	}
	for idx, similarity := range target.Clone().MatrixCosineSimilarity(compute.NewMatrix(matrixCentroids)) {
		closestCentroids[idx] = centroidSimilarity{
			centroid:   centroids[idx],
			similarity: similarity,
		}
	}
	slices.SortFunc(closestCentroids, func(a, b centroidSimilarity) int {
		return cmp.Compare(b.similarity, a.similarity)
	})
	closestCentroids = closestCentroids[:min(req.Centroids, len(closestCentroids))]

	// create new cosine similarity graph
	cosineSimilarity, closeGraph := compute.VectorMatrixCosineSimilarity()
	defer closeGraph()

	// For each centroid, find the closest documents to the embedding
	type documentSimilarity struct {
		centroidSimilarity *centroidSimilarity

		document   database.Document
		similarity float32
	}
	closestDocuments := make([]documentSimilarity, 0, req.Count+req.Offset+config.BATCH_SIZE_DATABASE)
	for _, centroid := range closestCentroids[:req.Centroids] {
		// Find centroid documents in batches
		var documents []database.Document
		result = s.db.Clauses(dbresolver.Read).WithContext(ctx).Where("centroid_id = ?", centroid.centroid.ID).Select("vector", "id").FindInBatches(&documents, config.BATCH_SIZE_DATABASE, func(tx *gorm.DB, n int) error {
			// Find nearest documents to the embedding
			matrixDocuments := make([][]uint8, len(documents))
			for idx, document := range documents {
				matrixDocuments[idx] = document.Vector
			}
			for idx, similarity := range cosineSimilarity(target.Clone(), compute.NewMatrix(matrixDocuments)) {
				closestDocuments = append(closestDocuments, documentSimilarity{
					centroidSimilarity: &centroid,
					document:           documents[idx],
					similarity:         similarity,
				})
			}
			slices.SortFunc(closestDocuments, func(a, b documentSimilarity) int {
				return cmp.Compare(b.similarity, a.similarity)
			})
			closestDocuments = closestDocuments[:min(req.Count+req.Offset, uint(len(closestDocuments)))]
			return nil
		})
		if result.Error == nil {
			// success
		} else if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
			// request canceled
			return res, result.Error
		} else {
			// exception encountered
			return res, errors.Join(errors.New("database document batch retrieval failed"), result.Error)
		}
	}

	// Fetch closest documents data
	ids := make([]uint64, len(closestDocuments))
	for idx, item := range closestDocuments {
		ids[idx] = item.document.ID
	}
	var documents []database.Document
	result = s.db.Clauses(dbresolver.Read).WithContext(ctx).Select("id", "external_id", "document").Find(&documents, ids)
	if result.Error == nil {
		// success
	} else if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
		// request canceled
		return res, result.Error
	} else {
		// exception encountered
		return res, errors.Join(errors.New("database document retrieval failed"), result.Error)
	}
	for _, document := range documents {
		for idx, item := range closestDocuments {
			if item.document.ID == document.ID {
				closestDocuments[idx].document.Document = document.Document
				break
			}
		}
	}

	// Create response
	res.Documents = make([]DocumentSearch, req.Count)
	skipCount := 0
	addCount := 0
	for idx, item := range closestDocuments {
		if uint(idx) < req.Offset {
			skipCount++
			continue
		}
		addCount++
		res.Documents[idx-skipCount] = DocumentSearch{
			DocumentUpload: DocumentUpload{
				ExternalID: item.document.ExternalID,
				Document:   item.document.Document.JSON(),
			},
			DocumentID:         item.document.ID,
			DocumentSimilarity: item.similarity,
			CentroidSimilarity: item.centroidSimilarity.similarity,
		}
	}
	res.Documents = res.Documents[:addCount]

	return res, nil
}
