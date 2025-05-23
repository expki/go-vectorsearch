package server

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/database"
	"github.com/expki/go-vectorsearch/dnc"
	"github.com/expki/go-vectorsearch/logger"
	"gorm.io/gorm/clause"
	"gorm.io/plugin/dbresolver"
)

func (d *Server) RefreshCentroids(appCtx context.Context) {
	// Retrieve list of categories
	var categories []database.Category
	err := d.db.WithContext(appCtx).Clauses(dbresolver.Read).Select("id").Find(&categories).Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		logger.Sugar().Info("Refresh centroids cancelled")
		return
	} else {
		logger.Sugar().Errorw("Failed to retrieve list of categories", "error", err)
		return
	}

	// Process each item one by one with row-level locking
	for _, category := range categories {
		// Lock category
		tx := d.db.WithContext(appCtx).Clauses(dbresolver.Write) // Start a transaction
		if d.db.Provider == config.DatabaseProvider_PostgreSQL {
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
			if d.db.Provider == config.DatabaseProvider_PostgreSQL {
				tx.Rollback()
			}
			return
		} else if d.db.Provider == config.DatabaseProvider_PostgreSQL && strings.Contains(strings.ToLower(err.Error()), "lock") {
			tx.Rollback()
			continue // skip if another instance is already processing the category
		} else {
			logger.Sugar().Errorw("Failed to retrieve list of categories", "error", err)
			if d.db.Provider == config.DatabaseProvider_PostgreSQL {
				tx.Rollback()
			}
			return
		}

		// Process category
		err = dnc.KMeansDivideAndConquer(appCtx, d.db, category.ID, d.config.Database.Cache)
		if err == nil {
		} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
			logger.Sugar().Info("Refresh centroids cancelled")
			if d.db.Provider == config.DatabaseProvider_PostgreSQL {
				tx.Rollback()
			}
			return
		} else {
			logger.Sugar().Errorw("Failed to process category", "error", err)
			if d.db.Provider == config.DatabaseProvider_PostgreSQL {
				tx.Rollback()
			}
			return
		}

		// Unlock category
		if d.db.Provider == config.DatabaseProvider_PostgreSQL {
			tx.Commit()
		}
	}
}
