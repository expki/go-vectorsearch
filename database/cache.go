package database

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
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
	c.lock.RLock()
	defer c.lock.RUnlock()

	if len(c.centroids) == 0 {
		return time.Time{}
	}

	return c.centroids[0].lastUpdated()
}

func (c *Cache) Count() uint64 {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if len(c.centroids) == 0 {
		return 0
	}

	return c.centroids[0].count()
}

func (c *Cache) ReadInBatches(ctx context.Context, target []uint8, centroidCount int) (stream <-chan *[][]uint8) {
	c.lock.RLock()
	topk := min(centroidCount, len(c.centroids))

	writeStream := make(chan *[][]uint8, topk+1)
	go func() {
		defer c.lock.RUnlock()
		defer close(writeStream)
		var wg sync.WaitGroup
		if len(c.centroids) == 0 {
			return
		}
		matchedCentroidIndexes, _ := c.ivf.NearestCentroids(target, topk)
		wg.Add(len(matchedCentroidIndexes))
		for _, centroidIdx := range matchedCentroidIndexes {
			go func() {
				c.centroids[centroidIdx].readInBatches(ctx, c.vectorSize, writeStream)
				wg.Done()
			}()
		}
		wg.Wait()
	}()
	return writeStream
}

func (c *Cache) Close() {
	c.lock.Lock()
	defer c.lock.Unlock()
	for _, centroid := range c.centroids {
		centroid.lock.Lock()
		defer centroid.lock.Unlock()
		centroid.file.Close()
	}
	c.centroids = nil
}

func (db *Database) RefreshCache(ctx context.Context) error {
	// close current files
	db.Cache.Close()

	// get total documents
	var total int64
	result := db.Clauses(dbresolver.Read).WithContext(ctx).Model(&Document{}).Count(&total)
	if result.Error != nil {
		if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
			return result.Error
		}
		logger.Sugar().Errorf("database vector count retrieval failed: %v", result.Error)
		return result.Error
	}
	if total == 0 {
		logger.Sugar().Debug("no cache because database has 0 documents")
		return nil
	}

	// create new cache
	return db.createIndexedCache(ctx, total)
}

func (db *Database) createIndexedCache(ctx context.Context, total int64) (err error) {
	db.Cache.lock.Lock()
	defer db.Cache.lock.Unlock()

	// Calculate new centroid file count
	centroidFileCount := int(math.Ceil(float64(total) / float64(config.CACHE_TARGET_INDEX_SIZE)))

	// Fetch initial random centroids
	var initialCentroids []Document
	result := db.Clauses(dbresolver.Read).WithContext(ctx).Select("id", "vector").Order("RANDOM()").Limit(centroidFileCount).Find(&initialCentroids)
	if result.Error != nil {
		if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
			return result.Error
		}
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
	db.Cache.ivf, err = compute.NewIVFFlat(centroidDocuments, config.CACHE_LEARNING_RATE)
	if err != nil {
		logger.Sugar().Errorf("database index initializing failed: %v", result.Error)
		return err
	}

	// Open index files and write streams
	db.Cache.centroids = make([]*centroid, centroidFileCount)
	rowWriterList := make([]func(row []uint8), centroidFileCount)
	rowWriterCloserList := make([]func(), centroidFileCount)
	for idx := range centroidFileCount {
		path := filepath.Join(db.Cache.path, fmt.Sprintf("centroid_%d.cache", idx))
		file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			logger.Sugar().Errorf("database index file opening failed: %v", err)
			return err
		}
		db.Cache.centroids[idx] = &centroid{
			Idx:  idx,
			file: file,
		}
		rowWriterList[idx], rowWriterCloserList[idx] = db.Cache.centroids[idx].createRowWriter(db.Cache.vectorSize)
	}
	defer func() {
		for _, closer := range rowWriterCloserList {
			closer()
		}
	}()

	// Open IVF training
	ivfTraining, ivfDone := db.Cache.ivf.TrainIVF()
	defer ivfDone()

	// Retrieve and train IVFFlat index
	bar := progressbar.Default(total, "Building IVF Flat Index Cache...")
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
			row := make([]byte, 8+db.Cache.vectorSize)
			binary.LittleEndian.PutUint64(row[:8], result.ID)
			copy(row[8:], result.Vector)
			rowWriterList[centroidIdx](row)
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
	bar.Finish()
	return nil
}
