package database

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/expki/go-vectorsearch/logger"
	"github.com/klauspost/compress/zstd"
	"github.com/schollz/progressbar/v3"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

type Cache struct {
	fileLock   sync.Mutex
	file       *os.File
	vectorSize int
}

func NewCache(cachePath string, vectorSize int) (*Cache, error) {
	cacheFile, err := os.OpenFile(cachePath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		logger.Sugar().Errorf("failed to open cache file: %v", err)
		return nil, err
	}
	return &Cache{
		file:       cacheFile,
		vectorSize: vectorSize,
	}, nil
}

func (c *Cache) LastUpdated() time.Time {
	// prevent other file operations
	c.fileLock.Lock()
	defer c.fileLock.Unlock()

	// move to start of file
	c.file.Seek(0, io.SeekStart)

	// read the date
	epochBytes := make([]byte, 8)
	_, err := io.ReadFull(c.file, epochBytes)

	// if error is EOF, return empty time.Time
	if err == io.EOF {
		return time.Time{}
	}

	// if error is not nil, fatal
	if err != nil {
		logger.Sugar().Fatalf("Failed to read last updated date from cache file: %v", err)
	}

	// parse the epoch
	epoch := binary.LittleEndian.Uint64(epochBytes)

	// parse the date from epoch
	date := time.Unix(int64(epoch), 0)

	return date
}

func (c *Cache) ReadInBatches(ctx context.Context, batchSize int) <-chan *[][]uint8 {
	// prevent other file operations
	c.fileLock.Lock()
	stream := make(chan *[][]uint8, 3)
	go func(c *Cache) {
		defer c.fileLock.Unlock()
		defer close(stream)

		// move to start of file after date
		c.file.Seek(8, io.SeekStart)

		// create decoder
		decoder, err := zstd.NewReader(
			c.file,
			zstd.WithDecoderConcurrency(runtime.NumCPU()),
			zstd.IgnoreChecksum(true),
		)
		if err != nil {
			logger.Sugar().Fatalf("Failed to create zstd cache decoder: %v", err)
		}
		decoderBuffer := bufio.NewReaderSize(decoder, c.vectorSize*batchSize)

		// read the data
		for {
			// stop if request is canceled
			select {
			case <-ctx.Done():
			default:
				// continue with read
			}

			// read batch
			batch := make([][]uint8, 0, batchSize)
			for range batchSize {
				// read row
				row := make([]uint8, c.vectorSize+8)
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
	}(c)

	return stream
}

func (c *Cache) WriteInBatches(batchSize int, stream <-chan *[][]uint8) {
	// prevent other file operations
	c.fileLock.Lock()
	go func(c *Cache) {
		defer c.fileLock.Unlock()

		// move to start of file
		c.file.Seek(0, io.SeekStart)

		// clear file
		c.file.Truncate(0)

		// write current date
		epoch := time.Now().Unix()
		epochBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(epochBytes, uint64(epoch))
		c.file.Write(epochBytes)

		// create encoder
		encoder, err := zstd.NewWriter(
			c.file,
			zstd.WithEncoderLevel(zstd.SpeedFastest),
			zstd.WithEncoderCRC(false),
			zstd.WithEncoderConcurrency(runtime.NumCPU()),
			zstd.WithEncoderPadding(1),
			zstd.WithNoEntropyCompression(true),
		)
		if err != nil {
			logger.Sugar().Fatalf("Failed to create zstd cache encoder: %v", err)
		}
		encoderBuffer := bufio.NewWriterSize(encoder, c.vectorSize*batchSize)

		// write the data
		for {
			// read batch
			batchPointer, done := <-stream
			if batchPointer == nil {
				break
			}

			for _, row := range *batchPointer {
				encoderBuffer.Write(row)
			}

			if done {
				break
			}
		}
		encoderBuffer.Flush()
		encoder.Close()
		c.file.Sync()
	}(c)
}

func (c *Cache) Close() error {
	c.fileLock.Lock()
	defer c.fileLock.Unlock()
	return c.file.Close()
}

func (db *Database) RefreshCache(ctx context.Context) error {
	stream := make(chan *[][]uint8, 3)
	defer close(stream)
	db.Cache.WriteInBatches(1000, stream)

	var total int64
	result := db.Clauses(dbresolver.Read).WithContext(ctx).Model(&Document{}).Count(&total)
	if result.Error != nil {
		if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, os.ErrDeadlineExceeded) {
			return result.Error
		}
		logger.Sugar().Errorf("database vector count retrieval failed: %v", result.Error)
		return result.Error
	}

	bar := progressbar.Default(total, "Refreshing Cache")
	var batch []Document
	result = db.Clauses(dbresolver.Read).WithContext(ctx).Select("id", "vector").FindInBatches(&batch, 1000, func(tx *gorm.DB, n int) error {
		matrix := make([][]uint8, len(batch))
		for idx, result := range batch {
			matrix[idx] = make([]byte, 8, 8+len(result.Vector))
			binary.LittleEndian.PutUint64(matrix[idx], result.ID)
			matrix[idx] = append(matrix[idx], result.Vector...)
		}
		stream <- &matrix
		bar.Add(n)
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
