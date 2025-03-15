package database

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/expki/go-vectorsearch/compute"
	"github.com/expki/go-vectorsearch/config"
	_ "github.com/expki/go-vectorsearch/env"
	"github.com/expki/go-vectorsearch/logger"
	"github.com/schollz/progressbar/v3"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

type Cache struct {
	path       string
	vectorSize int

	lock      sync.RWMutex
	centroids []*centroid
	ivf       compute.IVFFlat
}

func (c *Cache) LastUpdated() time.Time {
	if c == nil {
		return time.Time{}
	}
	c.lock.RLock()
	defer c.lock.RUnlock()

	var newest time.Time
	for _, centroid := range c.centroids {
		if centroid.lastUpdated().After(newest) {
			newest = centroid.lastUpdated()
		}
	}

	return newest
}

func (c *Cache) Count() uint64 {
	if c == nil {
		return 0
	}
	c.lock.RLock()
	defer c.lock.RUnlock()

	var total uint64
	for _, centroid := range c.centroids {
		total += centroid.count()
	}

	return total
}

func (c *Cache) CentroidReaders(ctx context.Context, target []uint8, centroidCount int) (total uint64, readers []func() (id uint64, vector []uint8), done func()) {
	if c == nil {
		return 0, nil, func() {}
	}
	c.lock.RLock()
	topk := min(centroidCount, len(c.centroids))
	readers = make([]func() (uint64, []uint8), topk)
	readerClosers := make([]func(), topk)
	matchedCentroidIndexes, _ := c.ivf.NearestCentroids(target, topk)
	for idx, centroidIdx := range matchedCentroidIndexes {
		centroid := c.centroids[centroidIdx]
		total += centroid.count()
		readers[idx], readerClosers[idx] = centroid.createReader(c.vectorSize)
	}
	return total, readers, func() {
		for _, closer := range readerClosers {
			closer()
		}
		c.lock.RUnlock()
	}
}

func (c *Cache) Close() {
	if c == nil {
		return
	}
	c.lock.Lock()
	for _, centroid := range c.centroids {
		centroid.lock.Lock()
		centroid.file.Close()
		centroid.lock.Unlock()
	}
	c.centroids = nil
	c.lock.Unlock()
}

func (db *Database) RefreshCache(ctx context.Context) (err error) {
	// get total documents
	var total int64
	result := db.Clauses(dbresolver.Read).WithContext(ctx).Model(&Document{}).Count(&total)
	if result.Error == nil {
		// records found
	} else if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
		// context was canceled or deadline exceeded
		return result.Error
	} else {
		// request failed
		logger.Sugar().Errorf("database sample vectors count failed: %v", result.Error)
		return result.Error
	}
	if total == 0 {
		return
	}

	// Lock cache map
	db.CacheLock.Lock()
	defer db.CacheLock.Unlock()

	// close current cache files
	for _, cache := range db.Cache {
		cache.Close()
	}
	db.Cache = nil

	// get vector size
	doc := Document{}
	result = db.Clauses(dbresolver.Read).WithContext(ctx).Select("vector").Take(&doc)
	if result.Error == nil {
		// records found
	} else if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
		// context was canceled or deadline exceeded
		return result.Error
	} else {
		// request failed
		logger.Sugar().Errorf("database sample document failed: %v", result.Error)
		return result.Error
	}
	vectorSize := len(doc.Vector)

	// Get all categories
	var categories []Category
	result = db.Clauses(dbresolver.Read).WithContext(ctx).Select("id").Find(&categories)
	if result.Error == nil {
		// records found
	} else if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
		// context was canceled or deadline exceeded
		return result.Error
	} else {
		// request failed
		logger.Sugar().Errorf("database sample vectors retrieval failed: %v", result.Error)
		return result.Error
	}

	// create new cache for each category
	bar := progressbar.Default(total, "Building IVF Flat Index Cache...")
	db.Cache = make(map[uint64]*Cache, len(categories))
	for _, category := range categories {
		// create category cache dir
		fullpath := filepath.Join(db.cfg.Cache, strconv.Itoa(int(category.ID)))
		if _, err = os.Stat(fullpath); os.IsNotExist(err) {
			err = os.Mkdir(fullpath, 0755)
			if err != nil {
				logger.Sugar().Errorf("failed to create cache dir %q: %v", fullpath, err)
				return err
			}
		}

		// Create cache entry
		cache := &Cache{
			path:       fullpath,
			vectorSize: vectorSize,
		}

		// createCategoryIndexedCache
		err = db.createCategoryIndexedCache(ctx, bar, category.ID, cache)
		if err != nil {
			logger.Sugar().Errorf("failed to create category indexed cache for %d: %v", category.ID, err)
			return err
		}

		// add to map
		db.Cache[category.ID] = cache
	}
	bar.Finish()
	return nil
}

func (db *Database) createCategoryIndexedCache(ctx context.Context, bar *progressbar.ProgressBar, categoryID uint64, cache *Cache) (err error) {
	// get total documents
	var total int64
	result := db.Clauses(dbresolver.Read).WithContext(ctx).Model(&Document{}).Where("category_id = ?", categoryID).Count(&total)
	if result.Error == nil {
		// records found
	} else if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
		// context was canceled or deadline exceeded
		return result.Error
	} else {
		// request failed
		logger.Sugar().Errorf("database sample vectors count failed: %v", result.Error)
		return result.Error
	}

	// Calculate new centroid file count
	centroidFileCount := int(math.Ceil(float64(total) / float64(config.CACHE_TARGET_INDEX_SIZE)))

	// Fetch initial random centroids
	var initialCentroids []Document
	result = db.Clauses(dbresolver.Read).WithContext(ctx).Select("id", "vector").Where("category_id = ?", categoryID).Order("RANDOM()").Limit(centroidFileCount).Find(&initialCentroids)
	if result.Error == nil {
		// records found
	} else if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
		// context was canceled or deadline exceeded
		return result.Error
	} else {
		// request failed
		logger.Sugar().Errorf("database sample vectors retrieval failed: %v", result.Error)
		return result.Error
	}
	centroidIds := make([]uint64, centroidFileCount)
	centroidDocuments := make([][]uint8, centroidFileCount)
	for idx, document := range initialCentroids {
		centroidIds[idx] = document.ID
		centroidDocuments[idx] = document.Vector.Underlying()
	}
	logger.Sugar().Debugf("new centroids ids: %v", centroidIds)

	// Initialize IVFFlat index
	cache.ivf, err = compute.NewIVFFlat(centroidDocuments, config.CACHE_LEARNING_RATE)
	if err != nil {
		logger.Sugar().Errorf("database index initializing failed: %v", result.Error)
		return err
	}

	// Open index files and write streams
	cache.centroids = make([]*centroid, centroidFileCount)
	rowWriterList := make([]func(id uint64, vector []uint8), centroidFileCount)
	rowWriterCloserList := make([]func(), centroidFileCount)
	for idx := range centroidFileCount {
		path := filepath.Join(cache.path, fmt.Sprintf("centroid_%d.cache", idx))
		file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			logger.Sugar().Errorf("database index file opening failed: %v", err)
			return err
		}
		cache.centroids[idx] = &centroid{
			Idx:  idx,
			file: file,
		}
		rowWriterList[idx], rowWriterCloserList[idx] = cache.centroids[idx].createWriter(cache.vectorSize)
	}
	defer func() {
		for _, closer := range rowWriterCloserList {
			closer()
		}
	}()

	// Open IVF training
	ivfTraining, ivfDone := cache.ivf.TrainIVF()
	defer ivfDone()

	// Retrieve and train IVFFlat index
	var batch []Document
	result = db.Clauses(dbresolver.Read).WithContext(ctx).Select("id", "vector").FindInBatches(&batch, config.BATCH_SIZE_DATABASE, func(tx *gorm.DB, n int) error {
		// send batch for training index
		matrixTrain := make([][]uint8, len(batch))
		for idx, result := range batch {
			matrixTrain[idx] = result.Vector
		}
		assignments := ivfTraining(matrixTrain)

		// write each row to assigned centroid file
		for vectorIdx, centroidIdx := range assignments {
			result := batch[vectorIdx]
			rowWriterList[centroidIdx](result.ID, result.Vector)
		}

		bar.Add(len(batch))
		return nil
	})
	if result.Error != nil {
		if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
			return result.Error
		}
		logger.Sugar().Errorf("database vector retrieval failed: %v", result.Error)
		return result.Error
	}
	return nil
}
