package noop

import (
	crand "crypto/rand"
	"encoding/binary"
	"math/rand"
	"time"

	"github.com/expki/go-vectorsearch/compute"
)

type noivf struct {
	numberCentroids  int
	vectorDimentions int
	random           *rand.Rand
}

// NewNoIVF creates a new (empty) noivf struct.
func NewNoIVF(randomMatrix [][]uint8) (ivf compute.IVFFlat, err error) {
	var seed int64
	raw := make([]byte, 8)
	_, err = crand.Read(raw)
	if err == nil {
		raw[7] &= 0x7F // Ensure it is a positive number.
		seed = int64(binary.LittleEndian.Uint64(raw))
	} else {
		seed = time.Now().Unix()
	}
	return &noivf{
		numberCentroids:  len(randomMatrix),
		vectorDimentions: len(randomMatrix[0]),
		random:           rand.New(rand.NewSource(seed)),
	}, nil
}

// NearestCentroids finds the nearest centroids
func (ivf *noivf) NearestCentroids(query []uint8, topK int) (nearest []int, similarity []float32) {
	for range min(topK, ivf.numberCentroids) {
		nearest = append(nearest, ivf.random.Intn(ivf.numberCentroids))
		similarity = append(similarity, float32(ivf.random.Float64()))
	}
	return nearest, similarity
}

// TrainIVFStreaming performs batch assignment and mini-batch training from batches of data.
func (ivf *noivf) TrainIVFStreaming(batchChan <-chan *[][]uint8, assignmentChan chan<- []int) {
	for batch := range batchChan {
		if batch == nil {
			break
		}
		count := len(*batch)
		assingment := make([]int, 0, count)
		for range count {
			assingment = append(assingment, ivf.random.Intn(ivf.numberCentroids))
		}
		assignmentChan <- assingment
	}
	close(assignmentChan)
}
