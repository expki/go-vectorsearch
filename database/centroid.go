package database

import (
	"bufio"
	"encoding/binary"
	"io"
	"os"
	"sync"
	"time"

	"github.com/expki/go-vectorsearch/config"
	_ "github.com/expki/go-vectorsearch/env"
	"github.com/expki/go-vectorsearch/logger"
	"github.com/klauspost/compress/zstd"
)

type centroid struct {
	Idx int

	lock sync.Mutex
	file *os.File
}

func (c *centroid) createWriter(vectorSize int) (rowWriter func(id uint64, vector []uint8), done func()) {
	c.lock.Lock()

	// move to start of file
	c.file.Seek(0, io.SeekStart)

	// clear file
	c.file.Truncate(0)

	// write current date
	writeDateTime(c.file, time.Now())

	// initial count
	writeTotal(c.file, 0)

	// create encoder
	encoder, err := zstd.NewWriter(
		c.file,
		zstd.WithEncoderLevel(zstd.SpeedFastest),
		zstd.WithEncoderCRC(false),
		zstd.WithEncoderPadding(1),
		zstd.WithNoEntropyCompression(true), // entropy seems to increase final size 5% as well as being 10% slower, there is no benefit
	)
	if err != nil {
		logger.Sugar().Errorf("Failed to create zstd cache encoder: %v", err)
		return
	}

	// Buffer
	encoderBuffer := bufio.NewWriterSize(encoder, (8+vectorSize)*config.BATCH_SIZE_CACHE)

	// Track total
	var total uint64

	return func(id uint64, vector []uint8) {
			idBytes := make([]byte, 8)
			binary.LittleEndian.PutUint64(idBytes, id)
			encoderBuffer.Write(idBytes) // write id
			encoderBuffer.Write(vector)  // write vector
			total++
		}, func() {
			// close the encoder
			encoderBuffer.Flush()
			encoder.Close()

			// update total count
			c.file.Seek(8, io.SeekStart)
			writeTotal(c.file, total)

			// unlock the file
			c.file.Sync()
			c.lock.Unlock()
		}
}

func (c *centroid) createReader(vectorSize int) (rowReader func() (id uint64, vector []uint8), done func()) {
	total := c.count()
	if total == 0 {
		return func() (uint64, []uint8) { return 0, nil }, func() {}
	}
	c.lock.Lock()

	// move to start of file after date & count
	c.file.Seek(16, io.SeekStart)

	// create decoder
	decoder, err := zstd.NewReader(
		c.file,
		zstd.IgnoreChecksum(true),
	)
	if err != nil {
		logger.Sugar().Fatalf("Failed to create zstd cache decoder: %v", err)
	}

	// Buffer read
	decoderBuffer := bufio.NewReaderSize(decoder, (8+vectorSize)*config.BATCH_SIZE_CACHE)

	// read the data
	return func() (id uint64, vector []uint8) {
			idBytes := make([]byte, 8)
			_, err := io.ReadFull(decoderBuffer, idBytes)
			if err == io.EOF {
				return 0, nil
			}
			if err != nil {
				logger.Sugar().Fatalf("Failed to read full id from zstd cache decoder: %v", err)
			}
			vector = make([]uint8, vectorSize)
			_, err = io.ReadFull(decoderBuffer, vector)
			if err != nil {
				logger.Sugar().Fatalf("Failed to read full vector from zstd cache decoder: %v", err)
			}
			return binary.LittleEndian.Uint64(idBytes), vector
		}, func() {
			decoder.Close()
			c.lock.Unlock()
		}
}

func (c *centroid) lastUpdated() time.Time {
	c.lock.Lock()
	defer c.lock.Unlock()

	// move to start of file
	c.file.Seek(0, io.SeekStart)

	// read the date
	epochBytes := make([]byte, 8)
	_, err := io.ReadFull(c.file, epochBytes)
	if err != nil {
		return time.Time{}
	}

	// parse the epoch
	epoch := binary.LittleEndian.Uint64(epochBytes)

	// parse the date from epoch
	return time.Unix(int64(epoch), 0)
}

func (c *centroid) count() uint64 {
	c.lock.Lock()
	defer c.lock.Unlock()

	// move to start of file after date
	_, err := c.file.Seek(8, io.SeekStart)
	if err != nil {
		return 0
	}

	// read the count
	epochBytes := make([]byte, 8)
	_, err = io.ReadFull(c.file, epochBytes)
	if err != nil {
		return 0
	}

	// parse the epoch
	return binary.LittleEndian.Uint64(epochBytes)
}

func writeDateTime(file *os.File, datetime time.Time) {
	epoch := datetime.Unix()
	epochBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(epochBytes, uint64(epoch))
	file.Write(epochBytes)
}

func writeTotal(file *os.File, count uint64) {
	totalBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(totalBytes, count)
	file.Write(totalBytes)
}

func ReadCentroidBatch(rowReader func() (id uint64, vector []uint8), size int) (ids []uint64, matrix [][]uint8) {
	ids = make([]uint64, size)
	matrix = make([][]uint8, size)
	for idx := range size {
		id, vector := rowReader()
		if vector == nil {
			return ids[:idx], matrix[:idx]
		}
		ids[idx] = id
		matrix[idx] = vector
	}
	return ids, matrix
}
