package compute

import (
	"errors"
	"sort"

	_ "github.com/expki/go-vectorsearch/env"
	"gorgonia.org/gorgonia"
	"gorgonia.org/tensor"
)

// IVFFlat holds data needed for an IVF-Flat index.
type IVFFlat interface {
	// NearestCentroids finds the nearest centroids
	NearestCentroids(query []uint8, topK int) (nearest []int, similarity []float32)
	// TrainIVFStreaming performs batch assignment and mini-batch training from batches of data.
	TrainIVFStreaming(batchChan <-chan *[][]uint8, assignmentChan chan<- []int)
}

// Newivfflat creates a new (empty) ivfflat struct.
func NewIVFFlat(randomMatrix [][]uint8, learningRate float32) (ivf IVFFlat, err error) {
	if len(randomMatrix) == 0 {
		return nil, errors.New("random matrix is empty")
	}
	ivf = &ivfflat{
		learningRate:     learningRate,
		numberCentroids:  len(randomMatrix),
		vectorDimentions: len(randomMatrix[0]),
		centroids:        NewMatrix(randomMatrix),
	}
	return ivf, nil
}

type ivfflat struct {
	learningRate     float32
	numberCentroids  int    // Number of centroids (clusters)
	vectorDimentions int    // Dimension of each vector
	centroids        Matrix // Shape: flat<[numberCentroids, vectorDimentions]>
}

// NearestCentroids finds the nearest centroids
func (ivf *ivfflat) NearestCentroids(query []uint8, topK int) (nearest []int, similarity []float32) {
	centroidSimilarities := NewTensor(query).CosineSimilarity(ivf.centroids)
	type results struct {
		index      int
		similarity float32
	}
	list := make([]results, ivf.numberCentroids)
	for idx, centroidSimilarity := range centroidSimilarities {
		list[idx] = results{index: idx, similarity: centroidSimilarity}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].similarity > list[j].similarity // descending order
	})
	nearest = make([]int, topK)
	similarity = make([]float32, topK)
	for idx := range topK {
		nearest[idx] = list[idx].index
		similarity[idx] = list[idx].similarity
	}
	return nearest, similarity
}

// TrainIVFStreaming performs batch assignment and mini-batch training from batches of data.
func (ivf *ivfflat) TrainIVFStreaming(batchChan <-chan *[][]uint8, assignmentChan chan<- []int) {
	for batchPointer := range batchChan {
		if batchPointer == nil {
			break
		}
		batch := *batchPointer
		if len(batch) == 0 {
			break
		}

		// Now we have a batch of shape [batchSize][dimension].
		batchSize := len(batch)

		// Dequantize the batch
		matrix := DequantizeMatrix(batch, -1, 1)

		// Flatten batch data
		dataFlat := make([]float32, 0, batchSize*ivf.vectorDimentions)
		for _, vector := range matrix {
			dataFlat = append(dataFlat, vector...)
		}

		// Build a Gorgonia graph to compute assignments for this batch.
		g := gorgonia.NewGraph()

		// New data
		dataTensor := tensor.New(tensor.WithShape(batchSize, ivf.vectorDimentions), tensor.WithBacking(dataFlat))
		dataNode := gorgonia.NewTensor(g, gorgonia.Float32, 2, gorgonia.WithShape(batchSize, ivf.vectorDimentions), gorgonia.WithName("data"))
		err := gorgonia.Let(dataNode, dataTensor)
		if err != nil {
			panic(err)
		}

		// Current centroids
		centroidsNode := gorgonia.NewTensor(g, gorgonia.Float32, 2, gorgonia.WithShape(ivf.numberCentroids, ivf.vectorDimentions), gorgonia.WithName("centroids"))
		centroidCopy := ivf.centroids.Dense.Clone().(*tensor.Dense)
		err = gorgonia.Let(centroidsNode, centroidCopy)
		if err != nil {
			panic(err)
		}

		// Reshape
		dataExp := gorgonia.Must(gorgonia.Reshape(dataNode, []int{1, batchSize, ivf.vectorDimentions}))
		centExp := gorgonia.Must(gorgonia.Reshape(centroidsNode, []int{ivf.numberCentroids, 1, ivf.vectorDimentions}))

		// Broadcast
		dataBroadcasted, centBroadcasted, err := gorgonia.Broadcast(dataExp, centExp, gorgonia.NewBroadcastPattern([]byte{0}, []byte{1})) // Broadcast along the first dimension
		if err != nil {
			panic(err)
		}

		// Distances
		diff := gorgonia.Must(gorgonia.Sub(dataBroadcasted, centBroadcasted))
		sq := gorgonia.Must(gorgonia.Square(diff))
		distances := gorgonia.Must(gorgonia.Sum(sq, 2)) // shape: [batchSize, nlist]

		// Compute
		machine := gorgonia.NewTapeMachine(g)
		err = machine.RunAll()
		if err != nil {
			panic(err)
		}
		machine.Close()

		// Argmax to get cluster assignments for this batch.
		argmaxAssignments, err := tensor.Argmin(distances.Value().(tensor.Tensor), 0)
		if err != nil {
			panic(err)
		}
		batchAssignments := argmaxAssignments.Data().([]int)

		// Calculate mini-batch centroids
		newCentroids, counts := ivf.computeBatchAverages(matrix, batchAssignments)

		// Update centroids with mini-batch centroids
		ivf.updateCentroidsMiniBatch(batchSize, newCentroids, counts)

		// Send the assignments to the main thread
		assignmentChan <- batchAssignments
	}
	close(assignmentChan)
}

// computeBatchAverages returns (avgVectors, counts) where:
// avgVectors[k] is the sum of all vectors assigned to cluster k (we'll divide later) and
// counts[k] is how many vectors go to cluster k.
func (ivf *ivfflat) computeBatchAverages(batch [][]float32, assignments []int) ([][]float32, []int) {
	newCentroids := make([][]float32, ivf.numberCentroids)
	for i := range ivf.numberCentroids {
		newCentroids[i] = make([]float32, ivf.vectorDimentions)
	}
	counts := make([]int, ivf.numberCentroids)

	// Sum up all vectors assigned to each cluster.
	for i, vec := range batch {
		c := assignments[i]
		counts[c]++
		for d := range ivf.vectorDimentions {
			newCentroids[c][d] += vec[d]
		}
	}

	// Devide by the number of vectors in each cluster to get the average.
	for i, centroid := range newCentroids {
		if counts[i] > 0 {
			for j := range ivf.vectorDimentions {
				centroid[j] /= float32(counts[i])
			}
		}
	}

	// newCentroids[k] is still the SUM of the cluster k's members
	return newCentroids, counts
}

// updateCentroidsMiniBatch adjusts each centroid using a “learningRate” approach.
func (ivf *ivfflat) updateCentroidsMiniBatch(batchSize int, newCentroids [][]float32, counts []int) {
	// We take the average and shift the centroid in its direction.
	centData := ivf.centroids.Dense.Data().([]float32)
	for k := range ivf.numberCentroids {
		for d := range ivf.vectorDimentions {
			oldVal := centData[k*ivf.vectorDimentions+d]
			avgVal := newCentroids[k][d]
			// Compute a weighted learning rate based on the number of vectors in the batch compared to the total batch size.
			lrWeighted := ivf.learningRate * (float32(counts[k]) / float32(batchSize))
			// Move centroid toward the batch average accrording to a weighted learning rate.
			centData[k*ivf.vectorDimentions+d] = oldVal - (lrWeighted * (oldVal - avgVal))
		}
	}
}
