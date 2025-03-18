package database

import (
	"context"
	"errors"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/expki/go-vectorsearch/compute"
	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/logger"
	"github.com/schollz/progressbar/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/plugin/dbresolver"
)

func (d *Database) refreshCentroidJob(appCtx context.Context) {
	logger.Sugar().Debug("Starting centroid refresh job")

	var lock sync.Mutex

	ticker := time.NewTicker(config.CENTROID_REFRESH_INTERVAL)
	for {
		select {
		case <-appCtx.Done():
			logger.Sugar().Info("Shutting down centroid refresh job")
			ticker.Stop()
			return
		case <-ticker.C:
			if lock.TryLock() {
				d.refreshCentroids(appCtx)
				lock.Unlock()
			}
		}
	}
}

func (d *Database) refreshCentroids(appCtx context.Context) {
	logger.Sugar().Info("Starting centroids refresh job")
	order := clause.OrderByColumn{
		Column: clause.Column{Name: "last_updated"},
	}
	var centroids []Centroid
	err := d.WithContext(appCtx).Clauses(dbresolver.Read).Order(order).Find(&centroids).Error
	if err != nil {
		logger.Sugar().Errorw("Failed to fetch centroids", "error", err)
		return
	}
	bar := progressbar.Default(int64(len(centroids)), "Refreshing centroids")
	for _, centroid := range centroids {
		d.refreshCentroid(appCtx, centroid)
		bar.Add(1)
	}

	// check if it still needs to be refreshed
	if d.provider == config.DatabaseProvider_PostgreSQL {
		for {
			var maxCount int
			err := d.DB.WithContext(appCtx).Clauses(dbresolver.Write).Raw(`
	SELECT COUNT(d.id) AS dc_count
	FROM centroids AS c
	LEFT JOIN documents AS d
	ON d.centroid_id = c.id
	GROUP BY c.id, c.last_updated
	ORDER BY dc_count DESC
	LIMIT 1;`).Scan(&maxCount).Error
			if err == nil {
			} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
				break
			} else {
				logger.Sugar().Errorw("Failed re-refresh centroid", "error", err)
				break
			}
			if maxCount <= config.MAX_CENTROID_SIZE {
				break
			}
			bar.AddMax(len(centroids))
			for _, centroid := range centroids {
				d.refreshCentroid(appCtx, centroid)
				bar.Add(1)
			}
		}
	}

	bar.Close()
	logger.Sugar().Info("Finished centroids refresh job")
}

func (d *Database) refreshCentroid(appCtx context.Context, centroid Centroid) {
	// Lock centroid for writing
	tx := d.WithContext(appCtx).Clauses(dbresolver.Write)
	if d.provider == config.DatabaseProvider_PostgreSQL {
		tx = tx.Begin()
		tx = tx.Clauses(clause.Locking{
			Strength: "SHARE",
			Table:    clause.Table{Name: clause.CurrentTable},
			Options:  "NOWAIT",
		})
	}
	err := tx.Take(&centroid, centroid.ID).Error
	if err == nil {
		logger.Sugar().Debug("Starting centroid refresh job", "centroid_id", centroid.ID)
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		tx.Rollback()
		return
	} else if strings.Contains(err.Error(), "lock") {
		// ignore if other services is already refreshing
		tx.Rollback()
		return
	} else {
		tx.Rollback()
		logger.Sugar().Errorw("Failed to lock centroid", "error", err)
		return
	}

	// split centroid
	centroid, newCentroids := d.splitCentroid(appCtx, centroid)

	// re-balance centroid across splitted centroids
	if len(newCentroids) > 0 {
		d.reBalanceCentroids(appCtx, centroid, newCentroids)
	}

	// Unlock centroid
	logger.Sugar().Debug("Centroid refreshed", "centroid_id", centroid.ID)
	if d.provider == config.DatabaseProvider_PostgreSQL {
		tx.Commit()
	}

	// re-center original and splitted centroids
	for _, newCentroid := range newCentroids {
		err = d.reCenterCentroid(appCtx, newCentroid)
		if err == nil {
		} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
			tx.Rollback()
			return
		} else {
			tx.Rollback()
			logger.Sugar().Errorw("Failed to re-center centroid", "error", err)
			return
		}
	}
	err = d.reCenterCentroid(appCtx, centroid)
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		tx.Rollback()
		return
	} else {
		tx.Rollback()
		logger.Sugar().Errorw("Failed to re-center centroid", "error", err)
		return
	}
}

func (d *Database) splitCentroid(appCtx context.Context, centroid Centroid) (original Centroid, newCentroids []Centroid) {
	// get centroid document count
	var countDocuments int64
	err := d.DB.WithContext(appCtx).Clauses(dbresolver.Read).Model(&Document{}).Where("centroid_id = ?", centroid.ID).Count(&countDocuments).Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return
	} else {
		logger.Sugar().Errorw("Failed to count documents for centroid", "error", err)
		return
	}

	// check if centroid needs to be split
	if countDocuments <= config.MAX_CENTROID_SIZE {
		return
	}

	// calculate new centroid count
	newCentroidCount := int(math.Ceil(float64(countDocuments)/float64(config.MAX_CENTROID_SIZE))) - 1

	// fetch new centroid documents
	var newCentroidDocuments []Document
	err = d.DB.WithContext(appCtx).Clauses(dbresolver.Read).Where("centroid_id = ?", centroid.ID).Select("id", "vector").Order("RANDOM()").Limit(newCentroidCount).Find(&newCentroidDocuments).Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return
	} else {
		logger.Sugar().Errorw("Failed to fetch new centroid documents", "error", err)
		return
	}

	// create new centroids
	newCentroids = make([]Centroid, newCentroidCount)
	for idx := range newCentroidCount {
		newCentroids[idx].Vector = newCentroidDocuments[idx].Vector
		newCentroids[idx].LastUpdated = time.Time{}
		newCentroids[idx].CategoryID = centroid.CategoryID
		newCentroids[idx].Category = centroid.Category
	}
	err = d.DB.WithContext(appCtx).Clauses(dbresolver.Write).Create(&newCentroids).Error
	if err == nil {
		centroid.LastUpdated = time.Time{}
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return
	} else {
		logger.Sugar().Errorw("Failed to create new centroids", "error", err)
		return
	}

	return centroid, newCentroids
}

func (d *Database) reBalanceCentroids(appCtx context.Context, centroid Centroid, newCentroids []Centroid) {
	// create new centroid matrix
	newCentroidMatrix := make([][]uint8, len(newCentroids)+1)
	newCentroidMatrix[0] = centroid.Vector
	for idx, centroid := range newCentroids {
		newCentroidMatrix[idx+1] = centroid.Vector
	}
	matrixCentroids := compute.NewMatrix(newCentroidMatrix)

	// fetch documents for original centroid
	var documents []Document
	err := d.DB.WithContext(appCtx).Clauses(dbresolver.Write).Where("centroid_id = ?", centroid.ID).Select("id", "vector").FindInBatches(&documents, config.BATCH_SIZE_DATABASE, func(tx *gorm.DB, batch int) (err error) {
		// find closest centroids
		documentMatrix := make([][]uint8, len(documents))
		for idx, document := range documents {
			documentMatrix[idx] = document.Vector
		}
		_, centroidIdList := matrixCentroids.Clone().CosineSimilarity(compute.NewMatrix(documentMatrix))

		// group documents by nearest centroid
		centroidDocumentsMap := make(map[uint64][]uint64, len(newCentroids))
		for _, centroid := range newCentroids {
			centroidDocumentsMap[centroid.ID] = make([]uint64, 0, len(documents))
		}
		for idx, centroidIdx := range centroidIdList {
			if centroidIdx == 0 {
				continue // already in correct centroid
			}
			document := documents[idx]
			newCentroidIdx := centroidIdx - 1
			newCentroid := newCentroids[newCentroidIdx]
			centroidDocumentsMap[newCentroid.ID] = append(centroidDocumentsMap[newCentroid.ID], document.ID)
		}

		// update document centroid id
		for centroidId, documentIdList := range centroidDocumentsMap {
			err = tx.Model(&Document{}).Where("id IN ?", documentIdList).Update("centroid_id", centroidId).Error
			if err != nil {
				return errors.Join(errors.New("failed to update document centroid id"), err)
			}
		}
		return nil
	}).Error
	if err == nil {
		// centroids found and handled
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		// centroids request canceled
		return
	} else {
		// centroids retrieve error
		logger.Sugar().Errorw("failed to retrieve centroid documents", "error", err)
		return
	}
}

func (d *Database) reCenterCentroid(appCtx context.Context, centroid Centroid) error {
	// Check if centroid requries update
	var latestDocument Document
	err := d.DB.WithContext(appCtx).Clauses(dbresolver.Read).Where("centroid_id = ?", centroid.ID).Order("last_updated DESC").Take(&latestDocument).Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	}
	if latestDocument.LastUpdated.Before(centroid.LastUpdated) {
		return nil
	}

	// get centroid documents
	var documents []Document
	err = d.DB.WithContext(appCtx).Clauses(dbresolver.Read).Where("centroid_id = ?", centroid.ID).Select("id", "vector").Limit(config.MAX_CENTROID_SIZE).Find(&documents).Error
	if err == nil || errors.Is(err, gorm.ErrRecordNotFound) {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else {
		return errors.Join(errors.New("failed to find documents"), err)
	}
	if len(documents) == 0 {
		return nil
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
	centroid.LastUpdated = time.Now().UTC()
	centroid.Vector = compute.QuantizeVector(centerVector, -1, 1)
	err = d.DB.WithContext(appCtx).Clauses(dbresolver.Write).Save(&centroid).Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else {
		return errors.Join(errors.New("failed to update centroid"), err)
	}
	return nil
}
