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
		paragraph := Flatten(file.Document)
		sections := Split(paragraph, s.config.Embed.GetNumCtx())
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
	logger.Sugar().Debug("retrieve owner")
	var owner database.Owner
	result := s.db.WithContext(ctx).Clauses(dbresolver.Read).Where("name = ?", req.Owner).Take(&owner)
	if result.Error == nil {
		// owner found
	} else if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
		// owner request canceled
		return res, result.Error
	} else if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		// owner create
		owner = database.Owner{
			Name: req.Owner,
		}
		result = s.db.WithContext(ctx).Clauses(dbresolver.Write).Create(&owner)
		if result.Error != nil {
			return res, errors.Join(errors.New("failed to create owner"), result.Error)
		}
	} else {
		// owner retrieve error
		return res, errors.Join(errors.New("failed to get owner"), result.Error)
	}

	// Get Category
	logger.Sugar().Debug("retrieve category")
	var category database.Category
	result = s.db.WithContext(ctx).Clauses(dbresolver.Read).Where("name = ? AND owner_id = ?", req.Category, owner.ID).Take(&category)
	if result.Error == nil {
		// category found
	} else if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
		// category request canceled
		return res, result.Error
	} else if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		// category create
		category = database.Category{
			Name:    req.Category,
			OwnerID: owner.ID,
			Owner:   &owner,
		}
		result = s.db.WithContext(ctx).Clauses(dbresolver.Write).Create(&category)
		if result.Error != nil {
			return res, errors.Join(errors.New("failed to create category"), result.Error)
		}
	} else {
		// category retrieve error
		return res, errors.Join(errors.New("failed to get category"), result.Error)
	}

	// Get Centroids
	logger.Sugar().Debug("retrieve centroids")
	var centroids []database.Centroid
	result = s.db.WithContext(ctx).Clauses(dbresolver.Read).Where("category_id = ?", category.ID).Find(&centroids)
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
		// centroid create
		centroids = append(centroids, database.Centroid{
			Vector:     matrixEmbeddings[0],
			CategoryID: category.ID,
			Category:   &category,
		})
		result = s.db.WithContext(ctx).Clauses(dbresolver.Write).Create(&centroids)
		if result.Error != nil {
			return res, errors.Join(errors.New("failed to create initial centroid"), result.Error)
		}
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
			newDocumentEmbeddings = append(newDocumentEmbeddings, &database.Embedding{
				Vector:     vector,
				CentroidID: centroid.ID,
				Centroid:   &centroid,
				Document:   document,
			})
		}

		// save
		newEmbeddings = append(newEmbeddings, newDocumentEmbeddings...)
		newDocuments[idx] = document
	}

	// Save Documents
	logger.Sugar().Debug("saving embeddings")
	err = s.db.WithContext(ctx).Clauses(dbresolver.Write).Session(&gorm.Session{FullSaveAssociations: true}).Create(&newEmbeddings).Error
	if err == nil {
		// documents created
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		// documents request canceled
		return res, err
	} else {
		// documents save error
		return res, errors.Join(errors.New("failed to save documents"), err)
	}

	// Create response
	logger.Sugar().Debug("creating response")
	res.DocumentIDs = make([]uint64, len(newDocuments))
	for idx, document := range newDocuments {
		res.DocumentIDs[idx] = document.ID
	}

	return res, nil
}
