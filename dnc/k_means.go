package dnc

import (
	"bytes"
	"cmp"
	"math/rand"
	"time"

	"slices"

	"github.com/expki/go-vectorsearch/compute"
)

// assign data to k centroids
func kMeans(data [][]uint8, k int) [][]uint8 {
	if len(data) == 0 || k <= 0 {
		return nil
	}

	// Step 1: Initialize utilities
	random := rand.New(rand.NewSource(time.Now().UnixNano()))
	dataMatrix := compute.NewMatrix(data)
	cosineSim, closeGraph := compute.MatrixCosineSimilarity()
	defer closeGraph()

	// Step 2: Randomly initialize unique centroids superset
	centroids := make([][]uint8, 0, min(len(data), k*2))
	used := make(map[int]struct{}, k)
	for len(centroids) < k {
		i := random.Intn(len(data))
		if _, ok := used[i]; !ok {
			used[i] = struct{}{}
			centroids = append(centroids, data[i])
		}
	}

	// Step 3: Iterate superset until convergence
	vectorLen := len(centroids[0])
	counts := make([]int, k)
	var converged bool
	for !converged {
		// Create centroid matrix
		centroidMatrix := compute.NewMatrix(centroids)

		// Find nearest centroid for each data point
		_, centroidIndexes := cosineSim(centroidMatrix.Clone(), dataMatrix.Clone())

		// Prepare new centroids
		sumVectors := make([][]float32, k)

		for i := range sumVectors {
			sumVectors[i] = make([]float32, vectorLen)
		}

		// Accumulate vectors
		for i, centroidIdx := range centroidIndexes {
			vec := compute.DequantizeVector(data[i], -1, 1)
			for j, val := range vec {
				sumVectors[centroidIdx][j] += val
			}
			counts[centroidIdx]++
		}

		// Compute means
		meanVectors := make([][]float32, k)
		for i := range sumVectors {
			meanVectors[i] = make([]float32, vectorLen)
			if counts[i] > 0 {
				for j, sum := range sumVectors[i] {
					meanVectors[i][j] = sum / float32(counts[i])
				}
			}
		}

		// Quantize means to get new centroids
		newCentroids := compute.QuantizeMatrix(meanVectors, -1, 1)

		// Check for convergence
		converged = true
		for i := range centroids {
			if !bytes.Equal(newCentroids[i], centroids[i]) {
				converged = false
				break
			}
		}

		centroids = newCentroids
		counts = make([]int, k)
	}

	// Step 4: Order superset by size desc
	type result struct {
		vector []uint8
		count  int
	}
	results := make([]result, len(centroids))
	for idx, centroid := range centroids {
		results[idx] = result{
			vector: centroid,
			count:  counts[idx],
		}
	}
	slices.SortFunc(results, func(a, b result) int {
		return cmp.Compare(b.count, a.count)
	})

	// Step 5: Truncate superset to set
	centroids = make([][]uint8, k)
	for idx := range k {
		centroids[idx] = results[idx].vector
	}

	// Step 6: Iterate set until convergence
	counts = make([]int, k)
	converged = false
	for !converged {
		// Create centroid matrix
		centroidMatrix := compute.NewMatrix(centroids)

		// Find nearest centroid for each data point
		_, centroidIndexes := cosineSim(centroidMatrix.Clone(), dataMatrix.Clone())

		// Prepare new centroids
		sumVectors := make([][]float32, k)

		for i := range sumVectors {
			sumVectors[i] = make([]float32, vectorLen)
		}

		// Accumulate vectors
		for i, centroidIdx := range centroidIndexes {
			vec := compute.DequantizeVector(data[i], -1, 1)
			for j, val := range vec {
				sumVectors[centroidIdx][j] += val
			}
			counts[centroidIdx]++
		}

		// Compute means
		meanVectors := make([][]float32, k)
		for i := range sumVectors {
			meanVectors[i] = make([]float32, vectorLen)
			if counts[i] > 0 {
				for j, sum := range sumVectors[i] {
					meanVectors[i][j] = sum / float32(counts[i])
				}
			}
		}

		// Quantize means to get new centroids
		newCentroids := compute.QuantizeMatrix(meanVectors, -1, 1)

		// Check for convergence
		converged = true
		for i := range centroids {
			if !bytes.Equal(newCentroids[i], centroids[i]) {
				converged = false
				break
			}
		}

		centroids = newCentroids
		counts = make([]int, k)
	}

	// Step 7: Return converged set
	return centroids
}
