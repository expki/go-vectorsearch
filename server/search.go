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
	logger.Sugar().Debug("search request received")

	// Get embedding
	if req.Prefix != "" {
		req.Text = fmt.Sprintf(`%s. %s`, req.Prefix, req.Text)
	}
	logger.Sugar().Debug("embedding search query")
	embedRes, err := s.ai.Embed(ctx, ai.EmbedRequest{
		Model: s.config.AI.Embed.Model,
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
	target := compute.NewVector(embedRes.Embeddings[0])

	// Get Owner
	logger.Sugar().Debugf("retrieving owner: %s", req.Owner)
	owner, err := s.cache.FetchOwner(req.Owner, func() (owner database.Owner, err error) {
		logger.Sugar().Debug("retrieve owner from database")
		return owner, s.db.WithContext(ctx).Clauses(dbresolver.Read).Where("name = ?", req.Owner).Take(&owner).Error
	})
	if err == nil {
		// owner found
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		// owner request canceled
		return res, err
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		// owner not found
		return res, nil
	} else {
		// owner retrieve error
		return res, errors.Join(errors.New("failed to get owner"), err)
	}

	// Get Category
	logger.Sugar().Debugf("retrieving category: %s", req.Category)
	category, err := s.cache.FetchCategory(req.Category, owner.ID, func() (category database.Category, err error) {
		logger.Sugar().Debug("retrieve category from database")
		return category, s.db.WithContext(ctx).Clauses(dbresolver.Read).Where("name = ? AND owner_id = ?", req.Category, owner.ID).Take(&category).Error
	})
	if err == nil {
		// category found
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		// category request canceled
		return res, err
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		// category not found
		return res, nil
	} else {
		// category retrieve error
		return res, errors.Join(errors.New("failed to get category"), err)
	}

	// Get Centroids
	logger.Sugar().Debug("retrieving centroids")
	centroids, err := s.cache.FetchCentroids(category.ID, func() (centroids []database.Centroid, err error) {
		logger.Sugar().Debug("retrieve centroids from database")
		return centroids, s.db.WithContext(ctx).Clauses(dbresolver.Read).Where("category_id = ?", category.ID).Find(&centroids).Error
	})
	if err == nil {
		// centroids found
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		// centroids request canceled
		return res, err
	} else {
		// centroids retrieve error
		return res, errors.Join(errors.New("failed to get centroids"), err)
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
	logger.Sugar().Debugf("calculate nearest centroids: %d", min(req.Centroids, len(closestCentroids)))
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
	closestCentroidIdList := make([]uint64, len(closestCentroids))
	for idx, centroid := range closestCentroids {
		closestCentroidIdList[idx] = centroid.centroid.ID
	}

	// create new cosine similarity graph
	cosineSimilarity, closeGraph := compute.VectorMatrixCosineSimilarity()
	defer closeGraph()

	// For each centroid, find the closest documents to the embedding
	type documentSimilarity struct {
		documentID uint64
		document   database.Document
		similarity float32
	}
	closestDocuments := make([]documentSimilarity, 0, req.Count+req.Offset+config.BATCH_SIZE_DATABASE)
	var embeddings []database.Embedding
	err = s.db.WithContext(ctx).Clauses(dbresolver.Read).
		Where("centroid_id IN ?", closestCentroidIdList).
		FindInBatches(&embeddings, config.BATCH_SIZE_DATABASE, func(tx *gorm.DB, n int) error {
			// find nearest embedding to the query
			matrixEmbeddings := make([][]uint8, len(embeddings))
			for idx, embedding := range embeddings {
				matrixEmbeddings[idx] = embedding.Vector
			}
			for idx, similarity := range cosineSimilarity(target.Clone(), compute.NewMatrix(matrixEmbeddings)) {
				closestDocuments = append(closestDocuments, documentSimilarity{
					documentID: embeddings[idx].DocumentID,
					similarity: similarity,
				})
			}
			// sort by nearest
			slices.SortFunc(closestDocuments, func(a, b documentSimilarity) int {
				return cmp.Compare(b.similarity, a.similarity)
			})
			// remove duplicates
			seen := make(map[uint64]struct{}, len(closestDocuments))
			unqiueClosestDocuents := make([]documentSimilarity, 0, len(closestDocuments))
			for _, document := range closestDocuments {
				if _, ok := seen[document.documentID]; !ok {
					unqiueClosestDocuents = append(unqiueClosestDocuents, document)
					seen[document.documentID] = struct{}{}
				}
			}
			closestDocuments = unqiueClosestDocuents
			// truncate list
			closestDocuments = closestDocuments[:min(req.Count+req.Offset, uint(len(closestDocuments)))]
			return nil
		}).
		Error
	if err == nil {
		// success
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		// request canceled
		return res, err
	} else {
		// exception encountered
		return res, errors.Join(errors.New("database document embedding batch retrieval failed"), err)
	}

	// Fetch closest documents data
	ids := make([]uint64, len(closestDocuments))
	for idx, item := range closestDocuments {
		ids[idx] = item.documentID
	}
	var documents []database.Document
	logger.Sugar().Debug("fetching nearest documents")
	err = s.db.WithContext(ctx).Clauses(dbresolver.Read).Find(&documents, ids).Error
	if err == nil {
		// success
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		// request canceled
		return res, err
	} else {
		// exception encountered
		return res, errors.Join(errors.New("database document retrieval failed"), err)
	}
	for _, document := range documents {
		for idx, item := range closestDocuments {
			if item.documentID == document.ID {
				closestDocuments[idx].document = document
				break
			}
		}
	}

	// Create response
	logger.Sugar().Debug("creating response")
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
				Name:       item.document.Name,
				ExternalID: item.document.ExternalID,
				Document:   item.document.Document.JSON(),
			},
			DocumentID:         item.documentID,
			DocumentSimilarity: item.similarity,
		}
	}
	res.Documents = res.Documents[:addCount]

	return res, nil
}
