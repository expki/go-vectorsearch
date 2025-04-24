//go:build gonum
// +build gonum

package compute

import (
	"github.com/expki/go-vectorsearch/logger"
	"gonum.org/v1/gonum/blas"
	"gonum.org/v1/gonum/blas/blas64"
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

	impl := blas64.Implementation()

	// Normalize A and B
	normalizeVector(A, dim)
	normalizeMatrixRows(B, n, dim)

	// Output similarity scores
	scores := make([]float64, n)

	// scores = B * Aᵗ (each row of B ⋅ A)
	for i := 0; i < n; i++ {
		scores[i] = impl.Ddot(dim, B[i*dim:], 1, A, 1)
	}

	// Convert to float32 and find argmax
	sims := make([]float32, n)

	for i := 0; i < n; i++ {
		sims[i] = float32(scores[i])
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
	B := realMatrix2.data // Data
	AShape := matrix1.shape
	BShape := realMatrix2.shape

	if AShape.cols != BShape.cols {
		logger.Sugar().Fatalf("matrix/matrix column size does not match: %d != %d", AShape.cols, BShape.cols)
	}

	dim := AShape.cols
	m := AShape.rows // Centroids
	n := BShape.rows // Data points

	impl := blas64.Implementation()

	// Normalize all rows
	normalizeMatrixRows(A, m, dim)
	normalizeMatrixRows(B, n, dim)

	// Allocate output buffer C (n x m), since we want B x Aᵗ
	C := make([]float64, n*m)

	// Compute C = B × Aᵗ
	impl.Dgemm(
		blas.NoTrans, blas.Trans,
		n,   // rows of B
		m,   // cols of Aᵗ (rows of A)
		dim, // shared dimension
		1.0, B, dim,
		A, dim,
		0.0, C, m, // note: row-major, so ldc is m (the inner dimension)
	)

	// Extract results: for each row in B, find best match in A
	sims := make([]float32, len(C)) // n * m
	argmax := make([]int, n)        // one best match per row of B

	for i := 0; i < n; i++ {
		rowOffset := i * m
		maxIdx := 0
		maxVal := C[rowOffset]

		for j := 0; j < m; j++ {
			v := C[rowOffset+j]
			sims[rowOffset+j] = float32(v)
			if v > maxVal {
				maxVal = v
				maxIdx = j
			}
		}
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
func normalizeMatrixRows(data []float64, rows, cols int) {
	impl := blas64.Implementation()
	for i := 0; i < rows; i++ {
		offset := i * cols
		row := data[offset : offset+cols]

		norm := impl.Dnrm2(cols, row, 1)
		if norm != 0 {
			impl.Dscal(cols, 1/norm, row, 1)
		}
	}
}

// normalizeVector normalizes a single vector by dividing each element by its L2 norm.
func normalizeVector(vec []float64, cols int) {
	impl := blas64.Implementation()
	norm := impl.Dnrm2(cols, vec, 1)
	if norm != 0 {
		impl.Dscal(cols, 1/norm, vec, 1)
	}
}
