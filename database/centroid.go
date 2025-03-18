package database

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/logger"
	"github.com/schollz/progressbar/v3"
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
	// Retrieve list of categories
	var categories []Category
	err := d.DB.WithContext(appCtx).Clauses(dbresolver.Read).Select("id").Find(&categories).Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		logger.Sugar().Info("Refresh centroids cancelled")
		return
	} else {
		logger.Sugar().Errorw("Failed to retrieve list of categories", "error", err)
		return
	}

	// Process each item one by one with row-level locking
	bar := progressbar.Default(int64(len(categories)), "Processing categories")
	for _, category := range categories {
		// Lock category
		tx := d.DB.WithContext(appCtx).Clauses(dbresolver.Write) // Start a transaction
		if d.provider == config.DatabaseProvider_PostgreSQL {
			tx = tx.Begin()
			tx = tx.Clauses(clause.Locking{
				Strength: "SHARE",
				Table:    clause.Table{Name: clause.CurrentTable},
				Options:  "NOWAIT",
			})
		}
		err = tx.First(&category, category.ID).Error
		if err == nil {
		} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
			logger.Sugar().Info("Refresh centroids cancelled")
			if d.provider == config.DatabaseProvider_PostgreSQL {
				tx.Rollback()
			}
			return
		} else if d.provider == config.DatabaseProvider_PostgreSQL && strings.Contains(strings.ToLower(err.Error()), "lock") {
			tx.Rollback()
			bar.Add(1)
			continue // skip if another instance is already processing the category
		} else {
			logger.Sugar().Errorw("Failed to retrieve list of categories", "error", err)
			if d.provider == config.DatabaseProvider_PostgreSQL {
				tx.Rollback()
			}
			return
		}

		// Process category
		err = d.KMeansCentroidAssignment(appCtx, category.ID)
		if err == nil {
		} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
			logger.Sugar().Info("Refresh centroids cancelled")
			if d.provider == config.DatabaseProvider_PostgreSQL {
				tx.Rollback()
			}
			return
		} else {
			logger.Sugar().Errorw("Failed to process category", "error", err)
			if d.provider == config.DatabaseProvider_PostgreSQL {
				tx.Rollback()
			}
			return
		}

		// Unlock category
		if d.provider == config.DatabaseProvider_PostgreSQL {
			tx.Commit()
		}
		bar.Add(1)
	}
}
