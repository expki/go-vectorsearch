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

func (s *Server) DeleteOwnerHttp(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	txid := index.Add(1)
	logger.Sugar().Debugf("%d delete owner started", txid)
	w.Header().Set("Content-Type", "application/json")

	// Ensure the request method is POST or DELETE
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
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
	type deleteOwner struct {
		Owner string `json:"owner"`
	}
	var req deleteOwner
	err = json.Unmarshal(body, &req)
	if err != nil {
		logger.Sugar().Debugf("%d request invalid: %v", txid, err)
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":"Invalid request"}`)
		return
	}

	// Handle the upload request
	err = s.DeleteOwner(r.Context(), req.Owner)
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
		logger.Sugar().Errorf("%d delete owner request failed: %s", txid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"Delete request failed"}`)
		return
	}

	// Set the response headers and write the JSON response
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, `{}`)
	logger.Sugar().Infof("%d delete owner request suceeded (%dms)", txid, time.Since(start).Milliseconds())
}

func (s *Server) DeleteCategoryHttp(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	txid := index.Add(1)
	logger.Sugar().Debugf("%d delete category started", txid)
	w.Header().Set("Content-Type", "application/json")

	// Ensure the request method is POST
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
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
	type deleteCategory struct {
		Owner    string `json:"owner"`
		Category string `json:"category"`
	}
	var req deleteCategory
	err = json.Unmarshal(body, &req)
	if err != nil {
		logger.Sugar().Debugf("%d request invalid: %v", txid, err)
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":"Invalid request"}`)
		return
	}

	// Handle the upload request
	err = s.DeleteCategory(r.Context(), req.Owner, req.Category)
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

	// Set the response headers and write the JSON response
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, `{}`)
	logger.Sugar().Infof("%d delete category request suceeded (%dms)", txid, time.Since(start).Milliseconds())
}

func (s *Server) DeleteDocumentHttp(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	txid := index.Add(1)
	logger.Sugar().Debugf("%d delete document started", txid)
	w.Header().Set("Content-Type", "application/json")

	// Ensure the request method is POST
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
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
	type deleteDocument struct {
		Owner      string `json:"owner"`
		Category   string `json:"category"`
		DocumentID uint64 `json:"document_id"`
	}
	var req deleteDocument
	err = json.Unmarshal(body, &req)
	if err != nil {
		logger.Sugar().Debugf("%d request invalid: %v", txid, err)
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":"Invalid request"}`)
		return
	}

	// Handle the upload request
	err = s.DeleteDocument(r.Context(), req.Owner, req.Category, req.DocumentID)
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
		logger.Sugar().Errorf("%d delete document request failed: %s", txid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"Delete request failed"}`)
		return
	}

	// Set the response headers and write the JSON response
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, `{}`)
	logger.Sugar().Infof("%d delete document request suceeded (%dms)", txid, time.Since(start).Milliseconds())
}

func (s *Server) DeleteOwner(ctx context.Context, owner string) (err error) {
	err = s.db.WithContext(ctx).Clauses(dbresolver.Write).Where("name = ?", owner).Delete(&database.Owner{}).Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else {
		return errors.Join(errors.New("delete owner exception"), err)
	}
	return nil
}

func (s *Server) DeleteCategory(ctx context.Context, ownerName, categoryName string) (err error) {
	ownerDetails, err := s.cache.FetchOwner(ownerName, func() (owner database.Owner, err error) {
		logger.Sugar().Debug("retrieve owner from database")
		return owner, s.db.WithContext(ctx).Clauses(dbresolver.Read).Where("name = ?", ownerName).Take(&owner).Error
	})
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else {
		return errors.Join(errors.New("get owner exception"), err)
	}
	err = s.db.WithContext(ctx).Clauses(dbresolver.Write).Where("owner_id = ? AND name = ?", ownerDetails.ID, categoryName).Delete(&database.Category{}).Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else {
		return errors.Join(errors.New("delete category exception"), err)
	}
	return nil
}

func (s *Server) DeleteDocument(ctx context.Context, ownerName, categoryName string, documentID uint64) (err error) {
	ownerDetails, err := s.cache.FetchOwner(ownerName, func() (owner database.Owner, err error) {
		logger.Sugar().Debug("retrieve owner from database")
		return owner, s.db.WithContext(ctx).Clauses(dbresolver.Read).Where("name = ?", ownerName).Take(&owner).Error
	})
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else {
		return errors.Join(errors.New("get owner exception"), err)
	}
	categoryDetails, err := s.cache.FetchCategory(categoryName, ownerDetails.ID, func() (category database.Category, err error) {
		logger.Sugar().Debug("retrieve category from database")
		return category, s.db.WithContext(ctx).Clauses(dbresolver.Read).Where("name = ? AND owner_id = ?", categoryName, ownerDetails.ID).Take(&category).Error
	})
	err = s.db.WithContext(ctx).Clauses(dbresolver.Read).Where("owner_id = ? AND name = ?", ownerDetails.ID, categoryName).Select("id").Take(&categoryDetails).Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else {
		return errors.Join(errors.New("get category exception"), err)
	}
	err = s.db.WithContext(ctx).Clauses(dbresolver.Write).Where("category_id = ?", categoryDetails.ID).Delete(&database.Document{}, documentID).Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else {
		return errors.Join(errors.New("delete document exception"), err)
	}
	return nil
}
