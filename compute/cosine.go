package compute

import (
	_ "github.com/expki/go-vectorsearch/env"
	"gorgonia.org/gorgonia"
	"gorgonia.org/tensor"
)

func (vector Vector) CosineSimilarity(matrix Matrix) []float32 {
	g := gorgonia.NewGraph()

	// Input vector
	inputNode := gorgonia.NewTensor(g, tensor.Float32, 2, gorgonia.WithValue(vector.Dense))

	// Batch matrix
	batchNode := gorgonia.NewTensor(g, tensor.Float32, 2, gorgonia.WithValue(matrix.Dense))

	// Compute norms
	inputSquared := gorgonia.Must(gorgonia.Square(inputNode))
	inputSumSquares := gorgonia.Must(gorgonia.Sum(inputSquared, 1))
	inputNorm := gorgonia.Must(gorgonia.Sqrt(inputSumSquares))

	batchSquared := gorgonia.Must(gorgonia.Square(batchNode))
	batchSumSquares := gorgonia.Must(gorgonia.Sum(batchSquared, 1))
	batchNorms := gorgonia.Must(gorgonia.Sqrt(batchSumSquares))

	// Matrix multiplication
	batchTransposed := gorgonia.Must(gorgonia.Transpose(batchNode, 1, 0))
	dotProduct := gorgonia.Must(gorgonia.BatchedMatMul(inputNode, batchTransposed))

	// Calculate denominator
	denominator := gorgonia.Must(gorgonia.OuterProd(inputNorm, batchNorms))

	// Compute cosine similarity
	cosineSim := gorgonia.Must(gorgonia.Div(dotProduct, denominator))

	// Execute the graph
	machine := gorgonia.NewTapeMachine(g)
	defer machine.Close()

	if err := machine.RunAll(); err != nil {
		panic(err)
	}

	// Return data
	return cosineSim.Value().Data().([]float32)
}
