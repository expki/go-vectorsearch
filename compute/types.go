package compute

type Vector interface {
	Clone() Vector
	MatrixCosineSimilarity(matrix Matrix) (similarity []float32)
}

type Matrix interface {
	Clone() Matrix
	MatrixCosineSimilarity(matrix Matrix) (relativeSimilaritieList []float32, nearestIndexList []int)
}
