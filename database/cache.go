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
	"sync"
	"time"

	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/logger"
	"github.com/klauspost/compress/zstd"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

type cache struct {
	lock       sync.Mutex
	path       string
	vectorSize int
	total      int
	file       *os.File
}

func (d *Database) newCache(ctx context.Context, categoryID uint64) (*cache, error) {
	// get vector size
	var document Document
	err := d.DB.WithContext(ctx).Clauses(dbresolver.Read).Take(&document).Error
	if err != nil {
		return nil, errors.Join(errors.New("failed to get vector size"), err)
	}
	vectorSize := len(document.Vector)

	// create empty cache file
	path := filepath.Join(d.cfg.Cache, fmt.Sprintf("%d_%s.cache", categoryID, time.Now().Format(time.RFC3339)))
	file, err := os.Create(path)
	if err != nil {
		return nil, errors.Join(errors.New("failed to create cache file"), err)
	}
	c := &cache{file: file, path: path, vectorSize: vectorSize, total: 0}

	// create encoder
	encoder, err := zstd.NewWriter(
		c.file,
		zstd.WithEncoderLevel(zstd.SpeedFastest),
		zstd.WithEncoderCRC(false),
		zstd.WithEncoderPadding(1),
		zstd.WithNoEntropyCompression(true),
	)
	if err != nil {
		c.Close()
		return nil, errors.Join(errors.New("failed to create zstd cache encoder"), err)
	}

	// buffer writes
	encoderBuffer := bufio.NewWriterSize(encoder, (8+vectorSize)*config.BATCH_SIZE_DATABASE)

	// retrieve documents in batches
	var documents []Document
	err = d.DB.WithContext(ctx).Clauses(dbresolver.Read).Where("category_id = ?", categoryID).Select("id", "vector").FindInBatches(&documents, config.BATCH_SIZE_DATABASE, func(tx *gorm.DB, batch int) (err error) {
		// write documents to cache file
		for _, document := range documents {
			row := make([]byte, 8+vectorSize)
			binary.LittleEndian.PutUint64(row[:8], document.ID)
			copy(row[8:], document.Vector)
			encoderBuffer.Write(row)
			c.total++
		}
		return nil
	}).Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		c.Close()
		return nil, err
	} else {
		c.Close()
		return nil, errors.Join(errors.New("failed to retrieve documents from database"), err)
	}

	// finish writing to file
	encoderBuffer.Flush()
	encoder.Close()
	c.file.Sync()

	return c, nil
}

func (c *cache) readRows() (rowReader func() (id uint64, vector []uint8), closeRowReader func()) {
	c.lock.Lock()

	// move to start of cache file
	c.file.Seek(0, io.SeekStart)

	// create decoder
	decoder, err := zstd.NewReader(
		c.file,
		zstd.IgnoreChecksum(true),
	)
	if err != nil {
		logger.Sugar().Fatalf("Failed to create zstd cache decoder: %v", err)
	}

	// Buffer read
	decoderBuffer := bufio.NewReaderSize(decoder, (8+c.vectorSize)*config.BATCH_SIZE_CACHE)

	// read the data
	return func() (id uint64, vector []uint8) {
			row := make([]byte, 8+c.vectorSize) // id + vector
			_, err := io.ReadFull(decoderBuffer, row)
			if err == io.EOF {
				return 0, nil
			}
			if err != nil {
				logger.Sugar().Fatalf("Failed to read full id from zstd cache decoder: %v", err)
			}
			id = binary.LittleEndian.Uint64(row[:8])
			vector = row[8:]
			return id, vector
		}, func() {
			decoder.Close()
			c.lock.Unlock()
		}
}

func (c *cache) Close() error {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.file == nil {
		return nil
	}
	c.file.Close()
	c.file = nil
	return os.Remove(c.path)
}
