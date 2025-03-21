package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/expki/go-vectorsearch/database"
	_ "github.com/expki/go-vectorsearch/env"
	"github.com/expki/go-vectorsearch/logger"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

type FetchCategoryNamesRequest struct {
	Owner string `json:"owner"`
}

type FetchCategoryNamesResponse struct {
	CategoryNames []string `json:"category_names"`
}

func (s *Server) FetchCategoryNamesHttp(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	txid := index.Add(1)
	logger.Sugar().Debugf("%d fetch categories started", txid)
	w.Header().Set("Content-Type", "application/json")

	// Ensure the request method is GET or POST
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
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
	var req FetchCategoryNamesRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		logger.Sugar().Debugf("%d request invalid: %v", txid, err)
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":"Invalid request"}`)
		return
	}

	// Handle the upload request
	categoryNames, err := s.FetchCategoryNames(r.Context(), req.Owner)
	if err == nil {
		// upload was successful
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		// upload request canceled
		logger.Sugar().Warnf("%d upload request canceled after %s", txid, time.Since(start).String())
		w.WriteHeader(499)
		io.WriteString(w, `{"error":"Client canceled delete request"}`)
		return
	} else {
		// upload failed
		logger.Sugar().Errorf("%d delete category request failed: %s", txid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"Delete request failed"}`)
		return
	}
	res := FetchCategoryNamesResponse{
		CategoryNames: categoryNames,
	}

	// Marshal response
	raw, err := json.Marshal(res)
	if err != nil {
		logger.Sugar().Errorf("%d marshal response: %v", txid, err)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"Marshal response exception"}`)
		return
	}

	// Set the response headers and write the JSON response
	w.WriteHeader(http.StatusOK)
	w.Write(raw)
	logger.Sugar().Infof("%d fetch categories request suceeded (%dms)", txid, time.Since(start).Milliseconds())
}

func (s *Server) FetchCategoryNames(ctx context.Context, owner string) (categoryNames []string, err error) {
	var ownerDetails database.Owner
	err = s.db.WithContext(ctx).Clauses(dbresolver.Read).Where("name = ?", owner).Select("id").Take(&ownerDetails).Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return nil, err
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		return []string{}, nil
	} else {
		return nil, errors.Join(errors.New("get owner exception"), err)
	}
	var categories []database.Category
	err = s.db.WithContext(ctx).Clauses(dbresolver.Read).Where("owner_id = ?", ownerDetails.ID).Select("name").Find(&categories).Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return nil, err
	} else {
		return nil, errors.Join(errors.New("get categories exception"), err)
	}
	categoryNames = make([]string, len(categories))
	for idx, category := range categories {
		categoryNames[idx] = category.Name
	}
	return categoryNames, nil
}
