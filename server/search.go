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
	"github.com/schollz/progressbar/v3"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

type SearchRequest struct {
	Prefix      string `json:"prefix,omitempty"`
	Text        string `json:"text"`
	Count       uint   `json:"count"`
	Offset      uint   `json:"offset,omitempty"`
	NoDocuments bool   `json:"no_documents,omitempty"`
	Centroids   int    `json:"centroids,omitempty"`
}

type SearchResponse struct {
	Documents []DocumentSearchInfo `json:"documents"`
}

type DocumentSearchInfo struct {
	DocumentID uint64  `json:"document_id"`
	Similarity float32 `json:"similarity"`
	Document   any     `json:"document,omitempty"`
}

func (s *server) SearchHttp(w http.ResponseWriter, r *http.Request) {
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
	if req.Count == 0 {
		req.Count = 1
	} else if req.Count > 20 {
		req.Count = 20
	}
	if req.Centroids == 0 {
		req.Centroids = 1
	} else if req.Centroids < 0 {
		req.Centroids = math.MaxInt
	}

	// Get embeddings
	if req.Prefix != "" {
		req.Text = fmt.Sprintf(`%s. %s`, req.Prefix, req.Text)
	}
	logger.Sugar().Debugf("%d request embeddings from ollama", txid)
	embedRes, err := s.ai.Embed(r.Context(), ai.EmbedRequest{
		Model: s.config.Ollama.Embed,
		Input: []string{fmt.Sprintf("search_query: %s", req.Text)},
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
			logger.Sugar().Debugf("%d request canceled after %dms while embedding text", txid, time.Since(start).Milliseconds())
			w.WriteHeader(499)
			io.WriteString(w, `{"error":"Client canceled request during embedding text"}`)
			return
		}
		logger.Sugar().Errorf("%d ai embed failed: %v", txid, err)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"Embedding failed"}`)
		return
	}
	if len(embedRes.Embeddings) < 1 {
		logger.Sugar().Errorf("%d ai embed response is empty", txid)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"Embedding returned empty response"}`)
		return
	}
	target := compute.NewTensor(embedRes.Embeddings.Underlying()[0])

	// Scan embeddings from cache
	type item struct {
		DocumentID uint64
		Similarity float32
	}
	mostSimilar := make([]item, req.Count+req.Offset+config.CACHE_TARGET_INDEX_SIZE)
	cacheTotal, centroidReaderList, closeReaders := s.db.Cache.CentroidReaders(r.Context(), embedRes.Embeddings.Underlying()[0], req.Centroids)
	barCache := progressbar.Default(int64(cacheTotal), "Searching cache...")
	// read each matched centroid
	for _, centroidReader := range centroidReaderList {
		// read centroid vectors in batches
		for {
			// read matrix batch
			idList, matrix := database.ReadCentroidBatch(centroidReader, config.CACHE_TARGET_INDEX_SIZE)
			if len(idList) == 0 {
				break
			}

			// compute cosine similarity between target and matrix batch
			similarityList := target.CosineSimilarity(compute.NewMatrix(matrix))

			// append document similarity to output list
			for idx, id := range idList {
				mostSimilar = append(mostSimilar, item{
					DocumentID: id,
					Similarity: similarityList[idx],
				})
			}

			// sort list by most similar
			slices.SortFunc(mostSimilar, func(a, b item) int {
				return cmp.Compare(a.Similarity, b.Similarity)
			})

			// truncate list to requested count + offset
			mostSimilar = mostSimilar[:req.Count+req.Offset]
			barCache.Add(len(idList))

			// stop if target size could not be read
			if len(idList) < config.CACHE_TARGET_INDEX_SIZE {
				break
			}
		}
	}
	closeReaders()
	barCache.Finish()

	// Scan embeddings from database
	var total int64
	result := s.db.Clauses(dbresolver.Read).WithContext(r.Context()).Model(&database.Document{}).Where("updated_at > ?", s.db.Cache.LastUpdated()).Count(&total)
	if result.Error != nil {
		if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
			logger.Sugar().Debugf("%d request canceled after %dms while counting embeddings", txid, time.Since(start).Milliseconds())
			w.WriteHeader(499)
			io.WriteString(w, `{"error":"Client canceled request during counting records"}`)
			return
		}
		logger.Sugar().Errorf("%d database vector count retrieval failed: %v", txid, result.Error)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"Counting records failed"}`)
		return
	}
	barDatabase := progressbar.Default(total, "Searching database...")
	var batch []database.Document
	result = s.db.Clauses(dbresolver.Read).WithContext(r.Context()).Select("vector", "id").Where("updated_at > ?", s.db.Cache.LastUpdated()).FindInBatches(&batch, config.BATCH_SIZE_DATABASE, func(tx *gorm.DB, n int) error {
		matrix := make([][]uint8, len(batch))
		for idx, result := range batch {
			matrix[idx] = result.Vector.Underlying()
		}
		for idx, similarity := range target.CosineSimilarity(compute.NewMatrix(matrix)) {
			barDatabase.Add(1)
			mostSimilar = append(mostSimilar, item{
				DocumentID: batch[idx].ID,
				Similarity: similarity,
			})
		}
		slices.SortFunc(mostSimilar, func(a, b item) int {
			return cmp.Compare(a.Similarity, b.Similarity)
		})
		mostSimilar = mostSimilar[:req.Count+req.Offset]
		return nil
	})
	barDatabase.Finish()
	if result.Error != nil && err != gorm.ErrRecordNotFound {
		if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
			logger.Sugar().Debugf("%d request canceled after %dms while scanning embeddings", txid, time.Since(start).Milliseconds())
			w.WriteHeader(499)
			io.WriteString(w, `{"error":"Client canceled request during scanning records"}`)
			return
		}
		logger.Sugar().Errorf("%d database vector retrieval failed: %v", txid, result.Error)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"Scanning records failed"}`)
		return
	}
	slices.Reverse(mostSimilar)
	mostSimilar = mostSimilar[req.Offset : req.Count+req.Offset]

	// Fetch most similar documents
	var documents []database.Document
	ids := make([]uint64, 0, len(mostSimilar))
	for _, item := range mostSimilar {
		if item.DocumentID != 0 {
			ids = append(ids, item.DocumentID)
		}
	}
	result = s.db.Clauses(dbresolver.Read).WithContext(r.Context()).Find(&documents, ids)
	if result.Error != nil {
		if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
			logger.Sugar().Debugf("%d request canceled after %dms while fetching documents", txid, time.Since(start).Milliseconds())
			w.WriteHeader(499)
			io.WriteString(w, `{"error":"Client canceled request during fetching documents"}`)
			return
		}
		logger.Sugar().Errorf("%d database document retrieval failed: %v", txid, result.Error)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"Retrieving documents failed"}`)
		return
	}

	// Create response
	res := SearchResponse{
		Documents: make([]DocumentSearchInfo, req.Count),
	}
	for idx, item := range mostSimilar {
		res.Documents[idx].DocumentID = item.DocumentID
		res.Documents[idx].Similarity = item.Similarity
		if !req.NoDocuments {
			for _, doc := range documents {
				if doc.ID == item.DocumentID {
					res.Documents[idx].Document = doc.Document.JSON()
					break
				}
			}
		}
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
func (s *server) Search(ctx context.Context, req SearchRequest) (res SearchResponse, err error) {
	if req.Count == 0 {
		req.Count = 1
	} else if req.Count > 20 {
		req.Count = 20
	}
	if req.Centroids == 0 {
		req.Centroids = 1
	} else if req.Centroids < 0 {
		req.Centroids = math.MaxInt
	}

	// Get embeddings
	if req.Prefix != "" {
		req.Text = fmt.Sprintf(`%s. %s`, req.Prefix, req.Text)
	}
	embedRes, err := s.ai.Embed(ctx, ai.EmbedRequest{
		Model: s.config.Ollama.Embed,
		Input: []string{fmt.Sprintf("search_query: %s", req.Text)},
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
			return res, err
		}
		return res, errors.Join(errors.New("failed to embed search query"), err)
	}
	if len(embedRes.Embeddings) < 1 {
		return res, errors.New("embedding returned empty response")
	}
	target := compute.NewTensor(embedRes.Embeddings.Underlying()[0])

	// Scan embeddings from cache
	type item struct {
		DocumentID uint64
		Similarity float32
	}
	mostSimilar := make([]item, 0, req.Count+req.Offset+config.CACHE_TARGET_INDEX_SIZE)
	cacheTotal, centroidReaderList, closeReaders := s.db.Cache.CentroidReaders(ctx, embedRes.Embeddings.Underlying()[0], req.Centroids)
	barCache := progressbar.Default(int64(cacheTotal), "Searching cache...")
	// read each matched centroid
	for _, centroidReader := range centroidReaderList {
		// read centroid vectors in batches
		for {
			// read matrix batch
			idList, matrix := database.ReadCentroidBatch(centroidReader, config.CACHE_TARGET_INDEX_SIZE)
			if len(idList) == 0 {
				break
			}

			// compute cosine similarity between target and matrix batch
			similarityList := target.CosineSimilarity(compute.NewMatrix(matrix))

			// append document similarity to output list
			for idx, id := range idList {
				mostSimilar = append(mostSimilar, item{
					DocumentID: id,
					Similarity: similarityList[idx],
				})
			}

			// sort list by most similar
			slices.SortFunc(mostSimilar, func(a, b item) int {
				return cmp.Compare(a.Similarity, b.Similarity)
			})

			// truncate list to requested count + offset
			mostSimilar = mostSimilar[:req.Count+req.Offset]
			barCache.Add(len(idList))

			// stop if target size could not be read
			if len(idList) < config.CACHE_TARGET_INDEX_SIZE {
				break
			}
		}
	}
	closeReaders()
	barCache.Finish()

	// Scan embeddings from database
	var total int64
	result := s.db.Clauses(dbresolver.Read).WithContext(ctx).Model(&database.Document{}).Where("updated_at > ?", s.db.Cache.LastUpdated()).Count(&total)
	if result.Error != nil {
		if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
			return res, result.Error
		}
		return res, errors.Join(errors.New("counting total records failed"), result.Error)
	}
	barDatabase := progressbar.Default(total, "Searching database...")
	var batch []database.Document
	result = s.db.Clauses(dbresolver.Read).WithContext(ctx).Select("vector", "id").Where("updated_at > ?", s.db.Cache.LastUpdated()).FindInBatches(&batch, config.BATCH_SIZE_DATABASE, func(tx *gorm.DB, n int) error {
		matrix := make([][]uint8, len(batch))
		for idx, result := range batch {
			matrix[idx] = result.Vector.Underlying()
		}
		for idx, similarity := range target.CosineSimilarity(compute.NewMatrix(matrix)) {
			barDatabase.Add(1)
			mostSimilar = append(mostSimilar, item{
				DocumentID: batch[idx].ID,
				Similarity: similarity,
			})
		}
		slices.SortFunc(mostSimilar, func(a, b item) int {
			return cmp.Compare(a.Similarity, b.Similarity)
		})
		mostSimilar = mostSimilar[:req.Count+req.Offset]
		return nil
	})
	barDatabase.Finish()
	if result.Error != nil && err != gorm.ErrRecordNotFound {
		if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
			return res, result.Error
		}
		return res, errors.Join(errors.New("database vector retrieval failed"), result.Error)
	}
	slices.Reverse(mostSimilar)
	mostSimilar = mostSimilar[req.Offset : req.Count+req.Offset]

	// Fetch most similar documents
	var documents []database.Document
	ids := make([]uint64, 0, len(mostSimilar))
	for _, item := range mostSimilar {
		if item.DocumentID != 0 {
			ids = append(ids, item.DocumentID)
		}
	}
	result = s.db.Clauses(dbresolver.Read).WithContext(ctx).Find(&documents, ids)
	if result.Error != nil {
		if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
			return res, result.Error
		}
		return res, errors.Join(errors.New("database document retrieval failed"), result.Error)
	}

	// Create response
	res.Documents = make([]DocumentSearchInfo, req.Count)
	for idx, item := range mostSimilar {
		res.Documents[idx].DocumentID = item.DocumentID
		res.Documents[idx].Similarity = item.Similarity
		if !req.NoDocuments {
			for _, doc := range documents {
				if doc.ID == item.DocumentID {
					res.Documents[idx].Document = doc.Document.JSON()
					break
				}
			}
		}
	}

	return res, nil
}
