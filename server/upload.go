package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/expki/go-vectorsearch/ai"
	"github.com/expki/go-vectorsearch/compute"
	"github.com/expki/go-vectorsearch/database"
	_ "github.com/expki/go-vectorsearch/env"
	"github.com/expki/go-vectorsearch/logger"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

type UploadRequest struct {
	Owner     string           `json:"owner"`
	Category  string           `json:"category"`
	Documents []DocumentUpload `json:"documents"`
}

type DocumentUpload struct {
	Name       string `json:"name,omitempty"`
	ExternalID string `json:"external_id,omitempty"`
	Document   any    `json:"document"`
}

type UploadResponse struct {
	DocumentIDs []uint64 `json:"document_ids"`
}

func (s *Server) UploadHttp(w http.ResponseWriter, r *http.Request) {
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

	// Handle the upload request
	res, err := s.Upload(r.Context(), req)
	if err == nil {
		// upload was successful
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		// upload request canceled
		logger.Sugar().Warnf("%d upload request canceled after %s", txid, time.Since(start).String())
		w.WriteHeader(499)
		io.WriteString(w, `{"error":"Client canceled upload request"}`)
		return
	} else {
		// upload failed
		logger.Sugar().Errorf("%d upload request failed: %s", txid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"Upload request failed"}`)
		return
	}

	// Marshal the response to JSON
	resBytes, err := json.Marshal(res)
	if err != nil {
		logger.Sugar().Errorf("%d database response marshal failed: %v", txid, err)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"Creating response failed"}`)
		return
	}

	// Set the response headers and write the JSON response
	w.WriteHeader(http.StatusOK)
	w.Write(resBytes)
	logger.Sugar().Infof("%d upload request suceeded (%dms)", txid, time.Since(start).Milliseconds())
}

// Upload calculates the embedding for the uploaded document then saves the document and embedding in the database.
func (s *Server) Upload(ctx context.Context, req UploadRequest) (res UploadResponse, err error) {
	if len(req.Documents) == 0 {
		return res, errors.New("no documents provided")
	}

	// Generate embeddings
	logger.Sugar().Debug("preparing documents")
	embeddingCountPerDocumentList := make([]int, len(req.Documents))
	embeddingInputList := make([]string, 0, len(req.Documents))
	for idx, file := range req.Documents {
		prefix := ""
		if file.Name != "" {
			prefix = strings.TrimSuffix(strings.TrimSpace(file.Name), ".") + ". "
		}
		document := Flatten(file.Document)
		sections := Split(prefix, document, s.config.Embed.GetNumCtx())
		for idx, section := range sections {
			sections[idx] = fmt.Sprintf("search_document: %s", section)
		}
		embeddingCountPerDocumentList[idx] = len(sections)
		embeddingInputList = append(embeddingInputList, sections...)
	}

	// Get embeddings
	logger.Sugar().Debug("generating embeddings")
	embedRes, err := s.ai.Embed(ctx, ai.EmbedRequest{
		Model: s.config.Embed.Model,
		Input: embeddingInputList,
	})
	if err == nil {
		// success
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		// request canceled
		return res, err
	} else {
		// exception encountered
		return res, errors.Join(errors.New("failed to embed documents"), err)
	}
	matrixEmbeddings := embedRes.Embeddings.Underlying()
	if len(matrixEmbeddings) != len(embeddingInputList) {
		return res, errors.New("invalid response embeddings count")
	}

	// Get Owner
	logger.Sugar().Debug("retrieve owner from cache")
	owner, err := s.cache.FetchOwner(req.Owner, func() (owner database.Owner, err error) {
		logger.Sugar().Debug("retrieve owner from database")
		err = s.db.WithContext(ctx).Clauses(dbresolver.Read).Where("name = ?", req.Owner).Take(&owner).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// owner create
			owner = database.Owner{
				Name: req.Owner,
			}
			err = s.db.WithContext(ctx).Clauses(dbresolver.Write).Create(&owner).Error
			if err != nil {
				err = errors.Join(errors.New("failed to create owner"), err)
			}
		}
		return owner, err
	})
	if err == nil {
		// owner found
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		// owner request canceled
		return res, err
	} else {
		// owner retrieve error
		return res, errors.Join(errors.New("failed to get owner"), err)
	}

	// Get Category
	logger.Sugar().Debug("retrieve category from cache")
	category, err := s.cache.FetchCategory(req.Category, owner.ID, func() (category database.Category, err error) {
		logger.Sugar().Debug("retrieve category from database")
		err = s.db.WithContext(ctx).Clauses(dbresolver.Read).Where("name = ? AND owner_id = ?", req.Category, owner.ID).Take(&category).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// category create
			category = database.Category{
				Name:    req.Category,
				OwnerID: owner.ID,
				Owner:   &owner,
			}
			err = s.db.WithContext(ctx).Clauses(dbresolver.Write).Create(&category).Error
			if err != nil {
				err = errors.Join(errors.New("failed to create category"), err)
			}
		}
		return category, err
	})
	if err == nil {
		// category found
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		// category request canceled
		return res, err
	} else {
		// category retrieve error
		return res, errors.Join(errors.New("failed to get category"), err)
	}

	// Get Centroids
	logger.Sugar().Debug("retrieve centroids from cache")
	centroids, err := s.cache.FetchCentroids(category.ID, func() (centroids []database.Centroid, err error) {
		logger.Sugar().Debug("retrieve centroids from database")
		err = s.db.WithContext(ctx).Clauses(dbresolver.Read).Where("category_id = ?", category.ID).Find(&centroids).Error
		if err == nil && len(centroids) == 0 {
			// centroid create
			centroids = append(centroids, database.Centroid{
				Vector:     matrixEmbeddings[0],
				CategoryID: category.ID,
				Category:   &category,
			})
			err = s.db.WithContext(ctx).Clauses(dbresolver.Write).Create(&centroids).Error
			if err != nil {
				err = errors.Join(errors.New("failed to create initial centroid"), err)
			}
		}
		return centroids, err
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

	// Assign Embeddings to Centroids
	logger.Sugar().Debug("calculating nearest centroid")
	matrixCentroids := make([][]uint8, len(centroids))
	for idx, centroid := range centroids {
		matrixCentroids[idx] = centroid.Vector
	}
	_, centroidIdxList := compute.NewMatrix(matrixCentroids).Clone().MatrixCosineSimilarity(compute.NewMatrix(matrixEmbeddings).Clone())

	// Create documents
	logger.Sugar().Debug("creating documents")
	newDocuments := make([]*database.Document, len(req.Documents))
	newEmbeddings := make([]*database.Embedding, 0, len(req.Documents))
	for idx, documentReq := range req.Documents {
		// create document
		file, _ := json.Marshal(req.Documents[idx].Document)
		document := &database.Document{
			Name:        documentReq.Name,
			ExternalID:  documentReq.ExternalID,
			LastUpdated: time.Now(),
			Document:    file,
			CategoryID:  category.ID,
			Category:    &category,
		}

		// create embeddings
		newDocumentEmbeddings := make([]*database.Embedding, 0, embeddingCountPerDocumentList[idx])
		for range embeddingCountPerDocumentList[idx] {
			vector := matrixEmbeddings[0]
			matrixEmbeddings = matrixEmbeddings[1:]
			centroidIdx := centroidIdxList[0]
			centroidIdxList = centroidIdxList[1:]
			centroid := centroids[centroidIdx]
			embedding := &database.Embedding{
				Vector:     vector,
				CentroidID: centroid.ID,
				Centroid:   &centroid,
				Document:   document,
			}
			newEmbeddings = append(newEmbeddings, embedding)
			newDocumentEmbeddings = append(newDocumentEmbeddings, embedding)
		}

		// save
		document.Embeddings = newDocumentEmbeddings
		newDocuments[idx] = document
	}

	// Save Documents
	logger.Sugar().Debug("saving documents")
	err = s.db.WithContext(ctx).Clauses(dbresolver.Write).Omit("Category", "Embeddings").Create(&newDocuments).Error
	if err == nil {
		// documents created
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		// documents request canceled
		return res, err
	} else {
		// documents save error
		return res, errors.Join(errors.New("failed to save documents"), err)
	}
	for _, embedding := range newEmbeddings {
		embedding.DocumentID = embedding.Document.ID
	}

	// Save Embeddings
	logger.Sugar().Debug("saving embeddings")
	err = s.db.WithContext(ctx).Clauses(dbresolver.Write).Omit("Centroid", "Document").Create(&newEmbeddings).Error
	if err == nil {
		// embeddings created
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		// embeddings request canceled
		return res, err
	} else {
		// embeddings save error
		return res, errors.Join(errors.New("failed to save embeddings"), err)
	}

	// Create response
	logger.Sugar().Debug("creating response")
	res.DocumentIDs = make([]uint64, len(newDocuments))
	for idx, document := range newDocuments {
		res.DocumentIDs[idx] = document.ID
	}

	return res, nil
}
