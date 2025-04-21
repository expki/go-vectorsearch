package dnc

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"

	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/logger"
	"github.com/klauspost/compress/zstd"
)

func newDataset(concurrent *atomic.Int64, vectorSize int, folderPath string) (*createDataset, error) {
	// create empty cache file
	path := filepath.Join(folderPath, fmt.Sprintf("%d.cache", rand.Uint64()))
	file, err := os.Create(path)
	if err != nil {
		return nil, errors.Join(errors.New("failed to create cache file"), err)
	}

	// create encoder
	encoder, err := zstd.NewWriter(
		file,
		zstd.WithEncoderLevel(zstd.SpeedFastest),
		zstd.WithEncoderCRC(false),
		zstd.WithEncoderConcurrency(
			max(
				1,
				runtime.NumCPU()-int(concurrent.Load()),
			),
		),
		zstd.WithLowerEncoderMem(true),
	)
	if err != nil {
		file.Close()
		os.Remove(path)
		return nil, errors.Join(errors.New("failed to create zstd cache encoder"), err)
	}

	// buffer writes
	encoderBuffer := bufio.NewWriterSize(encoder, vectorSize*config.BATCH_SIZE_CACHE)

	// dataset creator
	return &createDataset{
		vectorsize:    vectorSize,
		folderpath:    folderPath,
		file:          file,
		encoder:       encoder,
		encoderBuffer: encoderBuffer,
		concurrent:    concurrent,
	}, nil
}

type createDataset struct {
	vectorsize int
	folderpath string

	file          *os.File
	encoder       *zstd.Encoder
	encoderBuffer *bufio.Writer
	concurrent    *atomic.Int64

	total uint64
}

func (c *createDataset) WriteRow(vector []uint8) {
	c.encoderBuffer.Write(vector)
	c.total++
}

func (c *createDataset) Finalize() *dataset {
	// finish writing to file
	c.encoderBuffer.Flush()
	c.encoder.Close()
	c.file.Sync()

	// create dataset
	X := &dataset{
		vectorsize:    c.vectorsize,
		folderpath:    c.folderpath,
		file:          c.file,
		decoder:       nil,
		decoderBuffer: nil,
		concurrent:    c.concurrent,
		centroid:      nil,
		total:         c.total,
	}

	// move reader to start
	X.Reset()

	// set centroid vector
	X.centroid = kMeans(sample(X.ReadRow, int(c.total), 50_000), 1)[0]

	// move reader to start
	X.Reset()

	// clear writer
	c.encoderBuffer = nil
	c.encoder = nil
	c.total = 0

	return X
}

type dataset struct {
	vectorsize int
	folderpath string

	file          *os.File
	decoder       *zstd.Decoder
	decoderBuffer *bufio.Reader
	concurrent    *atomic.Int64

	centroid []uint8
	total    uint64
}

func (d *dataset) ReadRow() []uint8 {
	if d.decoderBuffer == nil {
		logger.Sugar().Fatalf("File is not open for decoder")
	}
	vector := make([]uint8, d.vectorsize)
	_, err := io.ReadFull(d.decoderBuffer, vector)
	if err == io.EOF {
		return nil
	}
	if err != nil {
		logger.Sugar().Fatalf("Failed to read vector from decoder: %v", err)
	}
	return vector
}

func (d *dataset) Reset() (err error) {
	if d.decoder != nil {
		d.decoder.Close()
	}
	if d.file == nil {
		return errors.New("file is not open")
	}
	// move to start of cache file
	d.file.Seek(0, io.SeekStart)
	// create decoder
	d.decoder, err = zstd.NewReader(
		d.file,
		zstd.IgnoreChecksum(true),
		zstd.WithDecoderLowmem(true),
		zstd.WithDecoderConcurrency(
			max(
				1,
				min(4, runtime.NumCPU()-int(d.concurrent.Load())),
			),
		),
	)
	if err != nil {
		return errors.Join(errors.New("create zstd file decoder"), err)
	}
	d.decoderBuffer = bufio.NewReaderSize(d.decoder, d.vectorsize*config.BATCH_SIZE_CACHE)
	return nil
}

func (d *dataset) Close() {
	d.decoderBuffer = nil
	d.vectorsize = 0
	d.centroid = nil
	d.total = 0
	if d.decoder != nil {
		d.decoder.Close()
		d.decoder = nil
	}
	if d.file != nil {
		d.file.Close()
		d.file = nil
	}
	if d.folderpath != "" {
		os.Remove(d.folderpath)
		d.folderpath = ""
	}
}
