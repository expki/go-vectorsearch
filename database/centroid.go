package database

import (
	"context"
	"errors"
	"math"
	"os"
	"sync"
	"time"

	"github.com/expki/go-vectorsearch/compute"
	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/plugin/dbresolver"
)

func (d *Database) refreshCentroidJob(appCtx context.Context) {
	logger.Sugar().Debug("Starting centroid refresh job")

	var lock sync.Mutex
	run := func() {
		d.splitCentroidJob(appCtx)
		d.centerCentroidJob(appCtx)
		lock.Unlock()
	}

	lock.Lock()
	go run()
	ticker := time.NewTicker(config.CENTROID_REFRESH_INTERVAL)
	for {
		select {
		case <-appCtx.Done():
			logger.Sugar().Info("Shutting down centroid refresh job")
			ticker.Stop()
			return
		case <-ticker.C:
			if lock.TryLock() {
				logger.Sugar().Debug("Checking centroids")
				go run()
			}
		}
	}
}

func (d *Database) splitCentroidJob(appCtx context.Context) {
	logger.Sugar().Info("Splitting Centroids started")
	var centroids []Centroid
	tx := d.WithContext(appCtx).Begin().Clauses(dbresolver.Write)
	result := tx.Clauses(
		clause.Locking{
			Strength: "SHARE",
			Table:    clause.Table{Name: clause.CurrentTable},
			Options:  "SKIP LOCKED",
		},
	).Select("id", "last_updated", "category_id").FindInBatches(&centroids, config.BATCH_SIZE_DATABASE, func(tx *gorm.DB, batch int) error {
		for _, centroid := range centroids {
			// get document countDocuments
			var countDocuments int64
			err := tx.Model(&Document{}).Where("centroid_id = ?", centroid.ID).Count(&countDocuments).Error
			if err == nil {
			} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
				return nil
			} else {
				tx.Rollback()
				return errors.Join(errors.New("failed to count documents for centroid"), err)
			}

			// check if centroid should be split
			if countDocuments < config.MAX_CENTROID_SIZE {
				continue
			}

			// calculate new centroid count
			countCentroids := int(math.Ceil(float64(countDocuments) / float64(config.MAX_CENTROID_SIZE)))

			// fetch new centroid documents
			var newCentroidDocuments []Document
			err = tx.Where("centroid_id = ?", centroid.ID).Select("id", "vector").Order(gorm.Expr("RANDOM()")).Limit(countCentroids).Find(&newCentroidDocuments).Error
			if err == nil {
			} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
				tx.Rollback()
				return nil
			} else {
				return errors.Join(errors.New("failed to fetch documents for centroid"), err)
			}

			// Create new centroids
			newCentroids := make([]Centroid, countCentroids)
			newCentroids[0] = centroid
			for idx := range countCentroids {
				newCentroids[idx].Vector = newCentroidDocuments[idx].Vector
				newCentroids[idx].LastUpdated = time.Time{}
				newCentroids[idx].CategoryID = centroid.CategoryID
			}
			err = tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "id"}},
				DoUpdates: clause.AssignmentColumns([]string{"last_updated", "vector"}),
			}).Create(&newCentroids).Error
			if err == nil {
			} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
				tx.Rollback()
				return nil
			} else {
				return errors.Join(errors.New("failed to create initial centroid"), err)
			}

			// reassign
			err = reassignCentroidToCentroids(tx, centroid, newCentroids)
			if err != nil {
				return errors.Join(errors.New("failed to reassign centroid"), err)
			}
			logger.Sugar().Debugf("Splitted centroid %d into %d centroids", centroid.ID, len(newCentroids))
			return nil
		}
		return nil
	})
	if result.Error == nil {
		// centroids found and handled
	} else if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
		// centroids request canceled
		logger.Sugar().Debug("Split centroids cancelled")
		tx.Rollback()
		return
	} else {
		// centroids retrieve error
		logger.Sugar().Errorf("Failed to retrieve centroids: %s", result.Error.Error())
		tx.Rollback()
		return
	}
	tx.Commit()
	logger.Sugar().Info("Splitting Centroids completed")
}

func reassignCentroidToCentroids(tx *gorm.DB, prevCentroid Centroid, newCentroids []Centroid) error {
	// create new centroid matrix
	newCentroidMatrix := make([][]uint8, len(newCentroids))
	for idx, centroid := range newCentroids {
		newCentroidMatrix[idx] = centroid.Vector
	}
	matrixCentroids := compute.NewMatrix(newCentroidMatrix)

	// fetch documents for previous centroid
	var documents []Document
	result := tx.Select("id", "centroid_id", "vector").FindInBatches(&documents, config.BATCH_SIZE_DATABASE, func(tx *gorm.DB, batch int) (err error) {
		// find closest centroids
		documentMatrix := make([][]uint8, len(documents))
		for idx, document := range documents {
			documentMatrix[idx] = document.Vector
		}
		_, centroidIdList := matrixCentroids.Clone().CosineSimilarity(compute.NewMatrix(documentMatrix))

		// split documents by centroid id
		centroidToDocumentMap := make(map[uint64][]uint64, 0)
		for _, centroid := range newCentroids {
			centroidToDocumentMap[centroid.CategoryID] = make([]uint64, 0, 1000)
		}
		for idx, document := range documents {
			key := newCentroids[centroidIdList[idx]].ID
			centroidToDocumentMap[key] = append(centroidToDocumentMap[key], document.ID)
		}

		// update document centroid id
		for centroidId, documentIds := range centroidToDocumentMap {
			if centroidId == prevCentroid.ID {
				continue
			}
			err = tx.Model(&Document{}).Where("id IN ?", documentIds).Update("centroid_id", uint64(centroidId)).Error
			if err != nil {
				return errors.Join(errors.New("failed to update document centroid id"), err)
			}
		}
		return nil
	})
	if result.Error == nil {
		// centroids found and handled
	} else if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
		// centroids request canceled
		return result.Error
	} else {
		// centroids retrieve error
		return errors.Join(errors.New("failed to update documents"), result.Error)
	}
	return nil
}

func (d *Database) centerCentroidJob(appCtx context.Context) {
	logger.Sugar().Info("Centering Centroids started")
	var centroids []Centroid
	tx := d.WithContext(appCtx).Begin().Clauses(dbresolver.Write)
	result := tx.Clauses(
		clause.Locking{
			Strength: "SHARE",
			Table:    clause.Table{Name: clause.CurrentTable},
			Options:  "SKIP LOCKED",
		},
	).Select("id", "last_updated").FindInBatches(&centroids, config.BATCH_SIZE_DATABASE, func(tx *gorm.DB, batch int) error {
		for _, centroid := range centroids {
			// Check if centroid requries update
			var latestDocument Document
			err := tx.Where("centroid_id = ?", centroid.ID).Order("last_updated DESC").Take(&latestDocument).Error
			if err == nil {
			} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
				return err
			} else {
				return errors.Join(errors.New("failed to find latest document"), err)
			}
			if latestDocument.LastUpdated.Before(centroid.LastUpdated) {
				continue
			}

			// get centroid documents
			var documents []Document
			err = tx.Where("centroid_id = ?", centroid.ID).Select("id", "vector").Find(&documents).Error
			if err == nil || errors.Is(err, gorm.ErrRecordNotFound) {
			} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
				return err
			} else {
				return errors.Join(errors.New("failed to find documents"), err)
			}
			if len(documents) == 0 {
				continue
			}

			// center centroid
			centerVector := make([]float32, len(documents[0].Vector))
			for _, document := range documents {
				for i, v := range compute.DequantizeVector(document.Vector.Underlying(), -1, 1) {
					centerVector[i] += v
				}
			}
			for i := range centerVector {
				centerVector[i] /= float32(len(documents))
			}

			// update centroid vector
			centroid.LastUpdated = time.Now()
			centroid.Vector = compute.QuantizeVector(centerVector, -1, 1)
			err = tx.Model(&centroid).Updates(centroid).Error
			if err == nil {
			} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
				return err
			} else {
				return errors.Join(errors.New("failed to update centroid"), err)
			}
			logger.Sugar().Debugf("Updated centroid %d with new vector", centroid.ID)
		}
		return nil
	})
	if result.Error == nil {
		// centroids found and handled
	} else if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
		// centroids request canceled
		logger.Sugar().Debug("Split centroids cancelled")
		tx.Rollback()
		return
	} else {
		// centroids retrieve error
		logger.Sugar().Errorf("Failed to retrieve centroids: %s", result.Error.Error())
		tx.Rollback()
		return
	}
	tx.Commit()
	logger.Sugar().Info("Centering Centroids completed")
}
