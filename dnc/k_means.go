package dnc

import (
	"bytes"
	"math/rand"
	"time"

	"github.com/expki/go-vectorsearch/compute"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// assign data to k centroids
func kMeans(multibar *mpb.Progress, data [][]uint8, k int) [][]uint8 {
	if len(data) == 0 || k <= 0 {
		return nil
	}

	random := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Step 1: Randomly initialize unique centroids
	centroids := make([][]uint8, 0, min(len(data), k))
	used := make(map[int]struct{}, k)
	for len(centroids) < k {
		i := random.Intn(len(data))
		if _, ok := used[i]; !ok {
			used[i] = struct{}{}
			centroids = append(centroids, data[i])
		}
	}

	// Step 2: Initialize utilities
	dataMatrix := compute.NewMatrix(data)
	cosineSim, closeGraph := compute.MatrixCosineSimilarity()
	defer closeGraph()

	// Step 3: Iterate until convergence
	var bar *mpb.Bar
	if multibar != nil {
		bar = multibar.AddBar(
			-1,
			mpb.PrependDecorators(
				decor.Name("K-Means Clustering"),
			),
		)
	}

	vectorLen := len(centroids[0])
	var converged bool
	for !converged {
		// Create centroid matrix
		centroidMatrix := compute.NewMatrix(centroids)

		// Find nearest centroid for each data point
		_, assignments := cosineSim(centroidMatrix.Clone(), dataMatrix.Clone())

		// Prepare new centroids
		sumVectors := make([][]float32, k)
		counts := make([]int, k)
		for i := range sumVectors {
			sumVectors[i] = make([]float32, vectorLen)
		}

		// Accumulate vectors
		for i, cIdx := range assignments {
			vec := compute.DequantizeVector(data[i], -1, 1)
			for j, val := range vec {
				sumVectors[cIdx][j] += val
			}
			counts[cIdx]++
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
		if bar != nil {
			bar.Increment()
		}
	}
	if bar != nil {
		bar.EnableTriggerComplete()
	}

	return centroids
}
