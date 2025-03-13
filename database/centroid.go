package database

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"os"
	"sync"
	"time"

	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/logger"
	"github.com/klauspost/compress/zstd"
)

type centroid struct {
	Idx int

	lock sync.Mutex
	file *os.File
}

func (c *centroid) writeInBatches(stream <-chan *[][]uint8) {
	c.lock.Lock()
	defer c.lock.Unlock()

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

	// read first batch
	batchPointer, hasMore := <-stream
	if batchPointer == nil {
		writeTotal(c.file, 0)
		encoder.Close()
		c.file.Sync()
		return
	}
	batch := *batchPointer
	if len(batch) == 0 {
		writeTotal(c.file, 0)
		encoder.Close()
		c.file.Sync()
		return
	}

	encoderBuffer := bufio.NewWriterSize(encoder, (len(batch[0])+8)*config.BATCH_SIZE_DATABASE)

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
	c.file.Seek(8, io.SeekStart)
	writeTotal(c.file, total)

	encoderBuffer.Flush()
	encoder.Close()
	c.file.Sync()
}

func (c *centroid) readInBatches(ctx context.Context, vectorSize int, openStream chan<- *[][]uint8) {
	c.lock.Lock()
	defer c.lock.Unlock()

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
	decoderBuffer := bufio.NewReaderSize(decoder, (vectorSize+8)*config.BATCH_SIZE_CACHE)

	// read the data
	for {
		// stop if request is canceled
		select {
		case <-ctx.Done():
			decoder.Close()
			return
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
				decoder.Close()
				return
			}

			// if error is not nil, fatal
			if err != nil {
				decoder.Close()
				logger.Sugar().Errorf("Failed to read row from zstd cache decoder: %v", err)
				return
			}

			// add row to batchs
			batch = append(batch, row)
		}

		// if batch is empty, stop
		if len(batch) == 0 {
			decoder.Close()
			return
		}

		// send batch
		openStream <- &batch
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
