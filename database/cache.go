package database

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/expki/go-vectorsearch/compute"
	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/logger"
	"github.com/klauspost/compress/zstd"
	"github.com/schollz/progressbar/v3"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

type Cache struct {
	path       string
	fileLock   sync.Mutex
	file       *os.File
	ivfLock    sync.RWMutex
	ivf        *compute.IVFFlat
	centroids  []*centroid
	vectorSize int
}

type centroid struct {
	Idx      int
	fileLock sync.Mutex
	file     *os.File
}

func (c *Cache) LastUpdated() time.Time {
	c.ivfLock.RLock()
	defer c.ivfLock.RUnlock()
	if len(c.centroids) != 0 {
		c.centroids[0].fileLock.Lock()
		defer c.centroids[0].fileLock.Unlock()
		return lastUpdated(c.centroids[0].file)
	}

	c.fileLock.Lock()
	defer c.fileLock.Unlock()
	if c.file != nil {
		return lastUpdated(c.file)
	}

	return time.Time{}
}

func (c *Cache) Count() uint64 {
	c.ivfLock.RLock()
	defer c.ivfLock.RUnlock()
	if len(c.centroids) != 0 {
		var total uint64
		for _, centroid := range c.centroids {
			centroid.fileLock.Lock()
			total += count(centroid.file)
			centroid.fileLock.Unlock()
		}
		return total
	}

	c.fileLock.Lock()
	defer c.fileLock.Unlock()
	if c.file != nil {
		return count(c.file)
	}

	return 0
}

func (c *Cache) ReadInBatches(ctx context.Context, target []uint8, centroids int) (stream <-chan *[][]uint8) {
	c.ivfLock.RLock()
	defer c.ivfLock.RUnlock()
	writeStream := make(chan *[][]uint8, config.STREAM_QUEUE_SIZE)
	go func() {
		defer close(writeStream)
		var wg sync.WaitGroup
		if c.file == nil && len(c.centroids) == 0 {
			return
		} else if len(c.centroids) == 0 {
			// full
			wg.Add(1)
			go func() {
				c.fileLock.Lock()
				readInBatches(ctx, c.file, c.vectorSize, writeStream)
				c.fileLock.Unlock()
				wg.Done()
			}()
		} else {
			// indexed
			centroids, _ := c.ivf.NearestCentroids(target, min(centroids, len(c.centroids)))
			wg.Add(len(centroids))
			for _, centroid := range centroids {
				go func() {
					c.centroids[centroid].fileLock.Lock()
					readInBatches(ctx, c.centroids[centroid].file, c.vectorSize, writeStream)
					c.centroids[centroid].fileLock.Unlock()
					wg.Done()
				}()
			}
		}
		wg.Wait()
	}()
	return writeStream
}

func (c *Cache) Close() {
	c.ivfLock.Lock()
	defer c.ivfLock.Unlock()
	c.fileLock.Lock()
	defer c.fileLock.Unlock()
	if c.file != nil {
		c.file.Close()
	}
	for _, centroid := range c.centroids {
		if centroid.file != nil {
			centroid.file.Close()
		}
	}
	c.file = nil
	c.centroids = nil
}

func (db *Database) RefreshCache(ctx context.Context) error {
	// close current files
	db.Cache.Close()
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
	if total < config.CACHE_TARGET_INDEX_SIZE*config.CACHE_MIN_INDEXES {
		logger.Sugar().Debugf("full cache because %d < %d required documents", total, config.CACHE_TARGET_INDEX_SIZE*config.CACHE_MIN_INDEXES)
		return db.createFullCache(ctx, total)
	}
	logger.Sugar().Debugf("indexed cache because %d > %d documents", total, config.CACHE_TARGET_INDEX_SIZE*config.CACHE_MIN_INDEXES)
	return db.createIndexedCache(ctx, total)
}

func (db *Database) createFullCache(ctx context.Context, total int64) (err error) {
	db.Cache.fileLock.Lock()
	defer db.Cache.fileLock.Unlock()
	stream := make(chan *[][]uint8, config.STREAM_QUEUE_SIZE)
	defer close(stream)

	// Create file
	path := filepath.Join(db.Cache.path, "database.cache")
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		logger.Sugar().Errorf("database index file opening failed: %v", err)
		return err
	}
	db.Cache.file = file
	go writeInBatches(file, stream)

	// Fetch in any order because we will read it all anyways.
	bar := progressbar.Default(total, "Full Database Cache")
	var batch []Document
	result := db.Clauses(dbresolver.Read).WithContext(ctx).Select("id", "vector").FindInBatches(&batch, config.BATCH_SIZE_DATABASE, func(tx *gorm.DB, n int) error {
		matrix := make([][]uint8, len(batch))
		for idx, result := range batch {
			matrix[idx] = make([]byte, 8, 8+len(result.Vector))
			binary.LittleEndian.PutUint64(matrix[idx], result.ID)
			matrix[idx] = append(matrix[idx], result.Vector...)
		}

		stream <- &matrix
		bar.Add(len(matrix))
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

func (db *Database) createIndexedCache(ctx context.Context, total int64) (err error) {
	db.Cache.ivfLock.Lock()
	defer db.Cache.ivfLock.Unlock()

	centroids := int(total / config.CACHE_TARGET_INDEX_SIZE)

	// Fetch initial centroids
	var initialCentroids []Document
	result := db.Clauses(dbresolver.Read).WithContext(ctx).Select("id", "vector").Order("RANDOM()").Limit(centroids).Find(&initialCentroids)
	if result.Error != nil {
		if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
			return result.Error
		}
		logger.Sugar().Errorf("database sample vectors retrieval failed: %v", result.Error)
		return result.Error
	}
	centroidDocuments := make([][]uint8, centroids)
	for idx, document := range initialCentroids {
		centroidDocuments[idx] = document.Vector.Underlying()
	}

	// Initialize IVFFlat index
	db.Cache.ivf, err = compute.NewIVFFlat(centroidDocuments, config.CACHE_LEARNING_RATE)
	if err != nil {
		logger.Sugar().Errorf("database index initializing failed: %v", result.Error)
		return err
	}

	// Open index files and write streams
	db.Cache.centroids = make([]*centroid, 0, centroids)
	streamList := make([]chan *[][]uint8, centroids)
	for idx := range centroidDocuments {
		centroid := &centroid{
			Idx: idx,
		}
		path := filepath.Join(db.Cache.path, fmt.Sprintf("centroid_%d.cache", idx))
		centroid.file, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			logger.Sugar().Errorf("database index file opening failed: %v", err)
			return err
		}
		streamList[idx] = make(chan *[][]uint8, config.STREAM_QUEUE_SIZE)
		defer close(streamList[idx])
		go writeInBatches(centroid.file, streamList[idx])
		db.Cache.centroids = append(db.Cache.centroids, centroid)
	}

	// Open IVF training stream
	batchChan := make(chan *[][]uint8, 1)
	defer close(batchChan)
	assignmentChan := make(chan []int, 1)
	go db.Cache.ivf.TrainIVFStreaming(batchChan, assignmentChan)

	// Fetch in random order to ensure randomness in IVFFlat index training.
	bar := progressbar.Default(total, "Indexed Database Cache")
	var batch []Document
	result = db.Clauses(dbresolver.Read).WithContext(ctx).Select("id", "vector").Order("RANDOM()").FindInBatches(&batch, config.BATCH_SIZE_DATABASE, func(tx *gorm.DB, n int) error {
		matrixTrain := make([][]uint8, len(batch))
		for idx, result := range batch {
			matrixTrain[idx] = result.Vector
		}
		batchChan <- &matrixTrain
		matrix := make([][]uint8, len(batch))
		for idx, result := range batch {
			matrix[idx] = make([]byte, 8, 8+len(result.Vector))
			binary.LittleEndian.PutUint64(matrix[idx], result.ID)
			matrix[idx] = append(matrix[idx], result.Vector...)
		}
		matrixMap := make(map[int][][]uint8, len(batch))
		for idx := range centroids {
			matrixMap[idx] = make([][]uint8, 0, len(batch))
		}
		for vectorIdx, centroidIdx := range <-assignmentChan {
			matrixMap[centroidIdx] = append(matrixMap[centroidIdx], matrix[vectorIdx])
		}
		for idx := range centroids {
			copy := matrixMap[idx]
			streamList[idx] <- &copy
		}
		bar.Add(len(matrix))
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

func writeInBatches(file *os.File, stream <-chan *[][]uint8) {
	// move to start of file
	file.Seek(0, io.SeekStart)

	// clear file
	file.Truncate(0)

	// write current date
	epoch := time.Now().Unix()
	epochBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(epochBytes, uint64(epoch))
	file.Write(epochBytes)

	// initial count
	writeCount(file, 0)

	// create encoder
	encoder, err := zstd.NewWriter(
		file,
		zstd.WithEncoderLevel(zstd.SpeedFastest),
		zstd.WithEncoderCRC(false),
		zstd.WithEncoderConcurrency(runtime.NumCPU()),
		zstd.WithEncoderPadding(1),
		zstd.WithNoEntropyCompression(true),
	)
	if err != nil {
		logger.Sugar().Fatalf("Failed to create zstd cache encoder: %v", err)
	}

	// read first batch
	batchPointer, hasMore := <-stream
	if batchPointer == nil {
		writeCount(file, 0)
		encoder.Close()
		file.Sync()
		return
	}
	batch := *batchPointer
	if len(batch) == 0 {
		writeCount(file, 0)
		encoder.Close()
		file.Sync()
		return
	}

	encoderBuffer := bufio.NewWriterSize(encoder, len(batch[0])*config.BATCH_SIZE_DATABASE)

	// write the data
	var total uint64
	for {
		for _, row := range batch {
			encoderBuffer.Write(row)
			total++
		}

		// stop if no more data is available
		if !hasMore {
			break
		}

		// read next batch
		batchPointer, hasMore = <-stream
		if batchPointer == nil {
			break
		}
		batch = *batchPointer
	}

	// update total count
	file.Seek(8, io.SeekStart)
	writeCount(file, total)

	encoderBuffer.Flush()
	encoder.Close()
	file.Sync()
}

func writeCount(file *os.File, count uint64) {
	totalBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(totalBytes, count)
	file.Write(totalBytes)
}

func readInBatches(ctx context.Context, file *os.File, vectorSize int, stream chan<- *[][]uint8) {
	// prevent other file operations
	if file == nil {
		return
	}

	// move to start of file after date & count
	file.Seek(16, io.SeekStart)

	// create decoder
	decoder, err := zstd.NewReader(
		file,
		zstd.WithDecoderConcurrency(runtime.NumCPU()),
		zstd.IgnoreChecksum(true),
	)
	if err != nil {
		logger.Sugar().Fatalf("Failed to create zstd cache decoder: %v", err)
	}
	decoderBuffer := bufio.NewReaderSize(decoder, (vectorSize+8)*config.BATCH_SIZE_CACHE)

	// read the data
	for {
		// stop if request is canceled
		select {
		case <-ctx.Done():
		default:
			// continue with read
		}

		// read batch
		batch := make([][]uint8, 0, config.BATCH_SIZE_CACHE)
		for range config.BATCH_SIZE_CACHE {
			// read row
			row := make([]uint8, vectorSize+8)
			_, err := io.ReadFull(decoderBuffer, row)

			// if error is EOF, return you are done
			if err == io.EOF {
				break
			}

			// if error is not nil, fatal
			if err != nil {
				logger.Sugar().Fatalf("Failed to read row from zstd cache decoder: %v", err)
			}

			// add row to batchs
			batch = append(batch, row)
		}

		// if batch is empty, stop
		if len(batch) == 0 {
			break
		}

		// send batch
		stream <- &batch
	}
}

func lastUpdated(file *os.File) time.Time {
	// move to start of file
	file.Seek(0, io.SeekStart)

	// read the date
	epochBytes := make([]byte, 8)
	_, err := io.ReadFull(file, epochBytes)
	if err != nil {
		return time.Time{}
	}

	// parse the epoch
	epoch := binary.LittleEndian.Uint64(epochBytes)

	// parse the date from epoch
	return time.Unix(int64(epoch), 0)
}

func count(file *os.File) uint64 {
	// move to end of file
	_, err := file.Seek(8, io.SeekStart)
	if err != nil {
		return 0
	}

	// read the count
	epochBytes := make([]byte, 8)
	_, err = io.ReadFull(file, epochBytes)
	if err != nil {
		return 0
	}

	// parse the epoch
	return binary.LittleEndian.Uint64(epochBytes)
}
