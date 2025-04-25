package dnc

import (
	"bytes"
	"cmp"
	"fmt"
	"math/rand"
	"time"

	"slices"

	"github.com/expki/go-vectorsearch/compute"
	"github.com/expki/go-vectorsearch/config"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// assign data to k centroids
func kMeans(multibar *mpb.Progress, id uint64, data [][]uint8, k int) [][]uint8 {
	if k <= 0 {
		return nil
	}
	dlen := len(data)
	if dlen == 0 || dlen <= k {
		return data
	}

	// Step 1: Initialize utilities
	random := rand.New(rand.NewSource(time.Now().UnixNano()))
	chunkedDataMatrix := chunkData(data, config.BATCH_SIZE_CACHE)
	cosineSim, closeGraph := compute.MatrixCosineSimilarity()
	defer closeGraph()

	// Step 2: Randomly initialize unique centroids superset
	kS := min(len(data), k*config.SUPERSET_MUL)
	centroids := make([][]uint8, 0, kS)
	used := make(map[int]struct{}, kS)
	for len(centroids) < kS {
		i := random.Intn(dlen)
		if _, ok := used[i]; !ok {
			used[i] = struct{}{}
			centroids = append(centroids, data[i])
		}
	}
	used = nil

	// progress bar
	bar := multibar.AddBar(
		0,
		mpb.PrependDecorators(
			decor.Name(fmt.Sprintf("%d K-Means Superset: ", id)),
			decor.CountersNoUnit("%d / %d"),
		),
		mpb.BarRemoveOnComplete(),
	)

	// Step 3: Iterate superset until convergence
	vectorLen := len(centroids[0]) - 8
	counts := make([]int, kS)
	sumVectors := make([][]float32, kS)
	meanVectors := make([][]float32, kS)
	for i := range kS {
		sumVectors[i] = make([]float32, vectorLen)
		meanVectors[i] = make([]float32, vectorLen)
	}
	var converged bool
	for n := 0; n < config.KMEANS_ITTERATION_LIMIT && !converged; n++ {
		bar.Increment()
		// Create centroid matrix
		centroidMatrix := compute.NewMatrix(centroids)

		// Find nearest centroid for each data point
		centroidIndexes := make([]int, 0, len(centroids))
		for _, dataMatrix := range chunkedDataMatrix {
			_, chunkedCentroidIndexes := cosineSim(centroidMatrix.Clone(), dataMatrix.Clone())
			centroidIndexes = append(centroidIndexes, chunkedCentroidIndexes...)
		}

		// Accumulate vectors
		for i, centroidIdx := range centroidIndexes {
			vec := compute.DequantizeVectorFloat32(data[i])
			for j, val := range vec {
				sumVectors[centroidIdx][j] += val
			}
			counts[centroidIdx]++
		}

		// Compute means
		for i := range sumVectors {
			if counts[i] <= 0 {
				continue
			}
			for j, sum := range sumVectors[i] {
				meanVectors[i][j] = sum / float32(counts[i])
			}
		}

		// Quantize means to get new centroids
		newCentroids := compute.QuantizeMatrixFloat32(meanVectors)

		// Check for convergence
		converged = true
		for i, centroid := range centroids {
			if !bytes.Equal(newCentroids[i][8:], centroid[8:]) {
				converged = false
				break
			}
		}

		centroids = newCentroids
		for idx := range counts {
			counts[idx] = 0
		}
		for idx := range sumVectors {
			sumVectors[idx] = make([]float32, vectorLen)
		}
	}
	bar.EnableTriggerComplete()

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

	// progress bar
	bar = multibar.AddBar(
		0,
		mpb.PrependDecorators(
			decor.Name(fmt.Sprintf("%d K-Means Set: ", id)),
			decor.CountersNoUnit("%d / %d"),
		),
		mpb.BarRemoveOnComplete(),
	)

	// Step 6: Iterate set until convergence
	counts = make([]int, k)
	sumVectors = sumVectors[:k]
	meanVectors = meanVectors[:k]
	converged = false
	for n := 0; n < config.KMEANS_ITTERATION_LIMIT && !converged; n++ {
		bar.Increment()
		// Create centroid matrix
		centroidMatrix := compute.NewMatrix(centroids)

		// Find nearest centroid for each data point
		centroidIndexes := make([]int, 0, len(centroids))
		for _, dataMatrix := range chunkedDataMatrix {
			_, chunkedCentroidIndexes := cosineSim(centroidMatrix.Clone(), dataMatrix.Clone())
			centroidIndexes = append(centroidIndexes, chunkedCentroidIndexes...)
		}

		// Accumulate vectors
		for i, centroidIdx := range centroidIndexes {
			vec := compute.DequantizeVectorFloat32(data[i])
			for j, val := range vec {
				sumVectors[centroidIdx][j] += val
			}
			counts[centroidIdx]++
		}

		// Compute means
		for i := range sumVectors {
			if counts[i] <= 0 {
				continue
			}
			for j, sum := range sumVectors[i] {
				meanVectors[i][j] = sum / float32(counts[i])
			}
		}

		// Quantize means to get new centroids
		newCentroids := compute.QuantizeMatrixFloat32(meanVectors)

		// Check for convergence
		converged = true
		for i, centroid := range centroids {
			if !bytes.Equal(newCentroids[i][8:], centroid[8:]) {
				converged = false
				break
			}
		}

		centroids = newCentroids
		for idx := range counts {
			counts[idx] = 0
		}
		for idx := range sumVectors {
			sumVectors[idx] = make([]float32, vectorLen)
		}
	}
	bar.EnableTriggerComplete()

	// Step 7: Return converged set
	return centroids
}

func chunkData(input [][]uint8, size int) []compute.Matrix {
	chunks := make([]compute.Matrix, 0, (len(input)/size)+1)
	for i := 0; i < len(input); i += size {
		end := min(i+size, len(input))
		chunks = append(chunks, compute.NewMatrix(input[i:end]))
	}
	return chunks
}

// recenter centroid
func kMeansFinal(multibar *mpb.Progress, id uint64, centroids [][]uint8, dataSample [][]uint8) [][]uint8 {
	clen := len(centroids)
	if clen == 0 {
		return centroids
	}
	dlen := len(dataSample)
	if dlen == 0 || int(dlen) <= clen {
		return centroids
	}

	// Step 1: Initialize utilities
	chunkedDataMatrix := chunkData(dataSample, config.BATCH_SIZE_CACHE)
	cosineSim, closeGraph := compute.MatrixCosineSimilarity()
	defer closeGraph()

	// progress bar
	bar := multibar.AddBar(
		0,
		mpb.PrependDecorators(
			decor.Name(fmt.Sprintf("%d K-Means Final: ", id)),
			decor.CountersNoUnit("%d / %d"),
		),
		mpb.BarRemoveOnComplete(),
	)

	// Step 2: Iterate superset until convergence
	vectorLen := len(centroids[0]) - 8
	counts := make([]int, clen)
	sumVectors := make([][]float32, clen)
	meanVectors := make([][]float32, clen)
	for i := range clen {
		sumVectors[i] = make([]float32, vectorLen)
		meanVectors[i] = make([]float32, vectorLen)
	}
	var converged bool
	for n := 0; n < config.KMEANS_ITTERATION_LIMIT && !converged; n++ {
		bar.Increment()
		// Create centroid matrix
		centroidMatrix := compute.NewMatrix(centroids)

		// Find nearest centroid for each data point
		centroidIndexes := make([]int, 0, len(centroids))
		for _, dataMatrix := range chunkedDataMatrix {
			_, chunkedCentroidIndexes := cosineSim(centroidMatrix.Clone(), dataMatrix.Clone())
			centroidIndexes = append(centroidIndexes, chunkedCentroidIndexes...)
		}

		// Accumulate vectors
		for i, centroidIdx := range centroidIndexes {
			vec := compute.DequantizeVectorFloat32(dataSample[i])
			for j, val := range vec {
				sumVectors[centroidIdx][j] += val
			}
			counts[centroidIdx]++
		}

		// Compute means
		for i := range sumVectors {
			if counts[i] <= 0 {
				continue
			}
			for j, sum := range sumVectors[i] {
				meanVectors[i][j] = sum / float32(counts[i])
			}
		}

		// Quantize means to get new centroids
		newCentroids := compute.QuantizeMatrixFloat32(meanVectors)

		// Check for convergence
		converged = true
		for i, centroid := range centroids {
			if !bytes.Equal(newCentroids[i][8:], centroid[8:]) {
				converged = false
				break
			}
		}

		centroids = newCentroids
		for idx := range counts {
			counts[idx] = 0
		}
		for idx := range sumVectors {
			sumVectors[idx] = make([]float32, vectorLen)
		}
	}
	bar.EnableTriggerComplete()

	// Step 3: Return converged set
	return centroids
}
