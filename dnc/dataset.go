package dnc

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/logger"
	"github.com/klauspost/compress/zstd"
)

type dataset struct {
	vectorsize int
	folderpath string

	vector    []uint8
	total     uint64
	rowReader func() (vector []uint8)
	restart   func()
	close     func()
}

func newDataset(vectorSize int, folderPath string) (
	rowWriter func(vector []uint8),
	finalize func() dataset,
	err error,
) {
	// create empty cache file
	path := filepath.Join(folderPath, fmt.Sprintf("%d.cache", rand.Uint64()))
	file, err := os.Create(path)
	if err != nil {
		return nil, nil, errors.Join(errors.New("failed to create cache file"), err)
	}

	// create encoder
	encoder, err := zstd.NewWriter(
		file,
		zstd.WithEncoderLevel(zstd.SpeedFastest),
		zstd.WithEncoderCRC(false),
	)
	if err != nil {
		file.Close()
		os.Remove(path)
		return nil, nil, errors.Join(errors.New("failed to create zstd cache encoder"), err)
	}

	// buffer writes
	encoderBuffer := bufio.NewWriterSize(encoder, vectorSize*config.BATCH_SIZE_CACHE)

	// create writer
	totalCount := uint64(0)
	rowWriter = func(vector []uint8) {
		encoderBuffer.Write(vector)
		totalCount++
	}

	// create finializer
	finalize = func() dataset {
		// finish writing to file
		encoderBuffer.Flush()
		encoder.Close()
		file.Sync()

		// move to start of cache file
		file.Seek(0, io.SeekStart)
		// create decoder
		decoder, err := zstd.NewReader(
			file,
			zstd.IgnoreChecksum(true),
		)
		if err != nil {
			logger.Sugar().Fatalf("Failed to create zstd cache decoder: %v", err)
		}

		// Buffer read
		decoderBuffer := bufio.NewReaderSize(decoder, vectorSize*config.BATCH_SIZE_CACHE)

		// create reader
		rowReader := func() (vector []uint8) {
			vector = make([]uint8, vectorSize)
			_, err := io.ReadFull(decoderBuffer, vector)
			if err == io.EOF {
				return nil
			}
			if err != nil {
				logger.Sugar().Fatalf("Failed to read vector from zstd cache decoder: %v", err)
			}
			return vector
		}

		// create restart
		restart := func() {
			decoder.Close()
			// move to start of cache file
			file.Seek(0, io.SeekStart)
			// create decoder
			decoder, err = zstd.NewReader(
				file,
				zstd.IgnoreChecksum(true),
			)
			if err != nil {
				logger.Sugar().Fatalf("Failed to create zstd cache decoder: %v", err)
			}
			decoderBuffer = bufio.NewReaderSize(decoder, vectorSize*config.BATCH_SIZE_CACHE)
		}

		// create closer
		close := func() {
			decoder.Close()
			file.Close()
			os.Remove(path)
		}

		// calculate centroid
		vector := kMeans(sample(rowReader, int(totalCount), 50_000), 1)[0]
		restart()

		return dataset{
			vectorSize, folderPath,
			vector, totalCount, rowReader, restart, close,
		}
	}

	return rowWriter, finalize, nil
}
