//go:build !gonum && !gorgonia
// +build !gonum,!gorgonia

package compute

import (
	"math"

	"github.com/expki/go-vectorsearch/logger"
)

// MatrixCosineSimilarity facilitates the computation of cosine similarity between a vector and a matrix with single graph.
func (vector *vectorContainer) MatrixCosineSimilarity(matrix Matrix) (similarity []float32) {
	realMatrix := (matrix.(*matrixContainer))
	A := vector.data
	B := realMatrix.data
	AShape := vector.shape
	BShape := realMatrix.shape
	if AShape.cols != BShape.cols {
		logger.Sugar().Fatalf("vector/matrix column size does not match: %d != %d", AShape.cols, BShape.cols)
	}
	dim := AShape.cols
	n := BShape.rows

	// Normalize A in-place
	normalizeVector(A)

	// Normalize each row in B in-place
	for i := 0; i < n; i++ {
		start := i * dim
		end := start + dim
		normalizeVector(B[start:end])
	}

	// Allocate result slice
	sims := make([]float32, n)

	// Compute dot products (cosine similarity)
	var maxSim float64 = -1 // cosine similarity range is [-1, 1]

	for i := 0; i < n; i++ {
		start := i * dim

		// Dot product between A and B[i]
		var dot float64
		for j := 0; j < dim; j++ {
			dot += A[j] * B[start+j]
		}

		sims[i] = float32(dot)
		if dot > maxSim {
			maxSim = dot
		}
	}

	return sims
}

// VectorMatrixCosineSimilarity facilitates the computation of cosine similarity between a vector and a matrix with reusable graph.
func VectorMatrixCosineSimilarity() (calculate func(vector Vector, matrix Matrix) (similarity []float32), done func()) {
	return func(vector Vector, matrix Matrix) (similarity []float32) {
			return vector.MatrixCosineSimilarity(matrix)
		}, func() {
			return
		}
}

// MatrixCosineSimilarity facilitates the computation of cosine similarity between a matrix and a matrix with single graph.
// The first matrix is the input matrix and the second matrix is the batch of vectors to compare against.
func (matrix1 *matrixContainer) MatrixCosineSimilarity(matrix2 Matrix) (relativeSimilaritieList []float32, nearestIndexList []int) {
	realMatrix2 := (matrix2.(*matrixContainer))
	A := matrix1.data     // Centroids
	B := realMatrix2.data // Data points
	AShape := matrix1.shape
	BShape := realMatrix2.shape

	if AShape.cols != BShape.cols {
		logger.Sugar().Fatalf("matrix/matrix column size does not match: %d != %d", AShape.cols, BShape.cols)
	}

	dim := AShape.cols
	m := AShape.rows // Centroids
	n := BShape.rows // Data

	// Normalize all rows in A (centroids)
	for i := 0; i < m; i++ {
		normalizeVector(A[i*dim : (i+1)*dim])
	}

	// Normalize all rows in B (data)
	for i := 0; i < n; i++ {
		normalizeVector(B[i*dim : (i+1)*dim])
	}

	// Result: For each row in B, find best match in A
	sims := make([]float32, n)
	argmax := make([]int, n)

	for i := 0; i < n; i++ {
		Brow := B[i*dim : (i+1)*dim]

		maxVal := -1.0
		maxIdx := 0

		for j := 0; j < m; j++ {
			Arow := A[j*dim : (j+1)*dim]

			// Dot product (cosine similarity)
			var dot float64
			for k := 0; k < dim; k++ {
				dot += Arow[k] * Brow[k]
			}

			if dot > maxVal {
				maxVal = dot
				maxIdx = j
			}
		}

		sims[i] = float32(maxVal)
		argmax[i] = maxIdx
	}

	return sims, argmax
}

// MatrixCosineSimilarity facilitates the computation of cosine similarity between a matrix and a matrix with reusable graph.
// The first matrix is the input matrix and the second matrix is the batch of vectors to compare against.
func MatrixCosineSimilarity() (calculate func(matrix1 Matrix, matrix2 Matrix) (relativeSimilaritieList []float32, nearestIndexList []int), done func()) {
	return func(matrix1 Matrix, matrix2 Matrix) (relativeSimilaritieList []float32, nearestIndexList []int) {
			return matrix1.MatrixCosineSimilarity(matrix2)
		}, func() {
			return
		}
}

// normalizeMatrixRows normalizes each row vector by dividing each element by its L2 norm.
func normalizeVector(vec []float64) {
	var norm float64
	for _, v := range vec {
		norm += v * v
	}
	norm = math.Sqrt(norm)
	if norm != 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}
}
