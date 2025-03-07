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

	"github.com/expki/go-vectorsearch/config"
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

func (c *Cache) Count() uint64 {
	// prevent other file operations
	c.fileLock.Lock()
	defer c.fileLock.Unlock()

	// move to start of file
	c.file.Seek(0, io.SeekStart)

	// read the date + count
	headerBytes := make([]byte, 16)
	_, err := io.ReadFull(c.file, headerBytes)

	// if error is EOF, return empty time.Time
	if err == io.EOF {
		return 0
	}

	// if error is not nil, fatal
	if err != nil {
		logger.Sugar().Fatalf("Failed to read count from cache file: %v", err)
	}

	// parse the epoch
	count := binary.LittleEndian.Uint64(headerBytes[8:])

	return count
}

func (c *Cache) ReadInBatches(ctx context.Context) <-chan *[][]uint8 {
	// prevent other file operations
	c.fileLock.Lock()
	stream := make(chan *[][]uint8, config.STREAM_QUEUE_SIZE)
	go func(c *Cache) {
		defer c.fileLock.Unlock()
		defer close(stream)

		// move to start of file after date & count
		c.file.Seek(16, io.SeekStart)

		// create decoder
		decoder, err := zstd.NewReader(
			c.file,
			zstd.WithDecoderConcurrency(runtime.NumCPU()),
			zstd.IgnoreChecksum(true),
		)
		if err != nil {
			logger.Sugar().Fatalf("Failed to create zstd cache decoder: %v", err)
		}
		decoderBuffer := bufio.NewReaderSize(decoder, c.vectorSize*config.BATCH_SIZE_CACHE)

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

func (c *Cache) WriteInBatches(total uint64, stream <-chan *[][]uint8) {
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

		// write total count

		totalBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(totalBytes, total)
		c.file.Write(totalBytes)

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

		encoderBuffer := bufio.NewWriterSize(encoder, c.vectorSize*config.BATCH_SIZE_DATABASE)

		// write the data
		for {
			// read batch
			batchPointer, hasMore := <-stream

			if batchPointer == nil {
				break
			}

			for _, row := range *batchPointer {
				encoderBuffer.Write(row)
			}

			if !hasMore {
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
		logger.Sugar().Debug("database is empty")
		return nil
	}

	stream := make(chan *[][]uint8, config.STREAM_QUEUE_SIZE)
	defer close(stream)
	db.Cache.WriteInBatches(uint64(total), stream)

	bar := progressbar.Default(total, "Refreshing Cache")
	var batch []Document
	result = db.Clauses(dbresolver.Read).WithContext(ctx).Select("id", "vector").FindInBatches(&batch, config.BATCH_SIZE_DATABASE, func(tx *gorm.DB, n int) error {
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
