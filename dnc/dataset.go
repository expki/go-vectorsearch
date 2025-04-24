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
	"github.com/vbauerster/mpb/v8"
)

func newDataset(concurrent *atomic.Int64, vectorSize int, folderPath string) (*createDataset, error) {
	// create empty cache file
	path := filepath.Join(folderPath, fmt.Sprintf("%d.cache", rand.Uint64()))
	file, err := os.Create(path)
	if err != nil {
		return nil, errors.Join(errors.New("failed to create cache file"), err)
	}

	// buffer writes
	encoderBuffer := bufio.NewWriterSize(file, (8+vectorSize)*config.BATCH_SIZE_CACHE)

	// dataset creator
	return &createDataset{
		vectorsize: vectorSize,
		folderpath: folderPath,
		filepath:   path,
		file:       file,
		fileBuffer: encoderBuffer,
		concurrent: concurrent,
	}, nil
}

type createDataset struct {
	vectorsize int
	folderpath string
	filepath   string

	file       *os.File
	fileBuffer *bufio.Writer
	concurrent *atomic.Int64

	total uint64
}

func (c *createDataset) WriteRow(row []uint8) {
	c.fileBuffer.Write(row)
	c.total++
}

func (c *createDataset) Finalize(multibar *mpb.Progress, id uint64) (initialize func() *dataset) {
	// finish writing to file
	c.fileBuffer.Flush()
	c.file.Sync()

	// create dataset
	X := &dataset{
		vectorsize: c.vectorsize,
		folderpath: c.folderpath,
		filepath:   c.filepath,
		file:       c.file,
		fileBuffer: nil,
		concurrent: c.concurrent,
		centroid:   nil,
		total:      c.total,
	}

	// clear writer
	c.folderpath = ""
	c.filepath = ""
	c.fileBuffer = nil
	c.file = nil
	c.concurrent = nil
	c = nil

	return func() *dataset {
		// move reader to start and set buffer
		X.Reset()

		// set centroid vector
		X.centroid = kMeans(
			multibar,
			id,
			sample(multibar, id, X.ReadRow, int(X.total), config.SAMPLE_SIZE),
			1,
		)[0]

		// move reader to start
		X.Reset()

		return X
	}
}

type dataset struct {
	vectorsize int
	folderpath string
	filepath   string

	file       *os.File
	fileBuffer *bufio.Reader
	concurrent *atomic.Int64

	centroid []uint8
	total    uint64
}

func (d *dataset) ReadRow() (row []uint8) {
	if d.fileBuffer == nil {
		logger.Sugar().Fatalf("File is not open for decoder")
	}
	row = make([]uint8, 8+d.vectorsize)
	_, err := io.ReadFull(d.fileBuffer, row)
	if err == io.EOF {
		return nil
	}
	if err != nil {
		logger.Sugar().Fatalf("Failed to read row from decoder: %v", err)
	}
	return row
}

func (d *dataset) Reset() (err error) {
	if d.file == nil {
		return errors.New("file is not open")
	}
	// move to start of cache file
	d.file.Seek(0, io.SeekStart)
	// create buffer
	d.fileBuffer = bufio.NewReaderSize(d.file, (8+d.vectorsize)*config.BATCH_SIZE_CACHE)
	return nil
}

func (d *dataset) Close() {
	d.folderpath = ""
	d.fileBuffer = nil
	d.vectorsize = 0
	d.centroid = nil
	d.total = 0
	if d.fileBuffer != nil {
		d.fileBuffer = nil
	}
	if d.file != nil {
		d.file.Close()
		d.file = nil
	}
	if d.filepath != "" {
		os.Remove(d.filepath)
		d.filepath = ""
	}
	runtime.GC()
}
