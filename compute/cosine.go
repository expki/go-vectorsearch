package compute

import (
	_ "github.com/expki/go-vectorsearch/env"
	"gorgonia.org/gorgonia"
	"gorgonia.org/tensor"
)

// TODO: Reuse graph and machine
func (vector Vector) CosineSimilarity(matrix Matrix) []float32 {
	g := gorgonia.NewGraph()

	// Input vector
	inputNode := gorgonia.NewTensor(g, tensor.Float32, 2, gorgonia.WithValue(vector.Dense.Clone()))

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
	err := machine.RunAll()
	if err != nil {
		panic(err)
	}
	machine.Close()

	// Return data
	return cosineSim.Value().Data().([]float32)
}

// TODO: This function does not handle matrixes of different sizes
func (matrix1 Matrix) CosineSimilarity(matrix2 Matrix) (similarities []float32, bestMatches []int) {
	g := gorgonia.NewGraph()

	// Create tensor nodes to hold M1 and M2 (rank=2)
	M1 := gorgonia.NewTensor(g, tensor.Float32, 2, gorgonia.WithValue(matrix1.Dense.Clone()))

	M2 := gorgonia.NewTensor(g, tensor.Float32, 2, gorgonia.WithValue(matrix2.Dense.Clone()))

	// Step 1: Dot Product => shape [N1, N2]
	// M2^T: shape [dim, N2]
	M2T := gorgonia.Must(gorgonia.Transpose(M2, 1, 0))
	dot := gorgonia.Must(gorgonia.BatchedMatMul(M1, M2T))

	// Step 2: Compute row-wise L2 norms of M1 and M2
	M1Norms, err := rowWiseL2Norm(M1) // shape [N1]
	if err != nil {
		panic(err)
	}
	M2Norms, err := rowWiseL2Norm(M2) // shape [N2]
	if err != nil {
		panic(err)
	}

	// Broadcast M1Norms, M2Norms to get shape [N1, N2]
	M1NormsCol, err := gorgonia.Reshape(M1Norms, tensor.Shape{matrix1.Shape[0], 1})
	if err != nil {
		panic(err)
	}
	M2NormsRow, err := gorgonia.Reshape(M2Norms, tensor.Shape{1, matrix1.Shape[0]})
	if err != nil {
		panic(err)
	}

	normsProduct, err := gorgonia.BroadcastHadamardProd(M1NormsCol, M2NormsRow, nil, []byte{1})
	if err != nil {
		panic(err)
	}

	// Step 3: Cosine similarity = dot / (||M1|| * ||M2||)
	cosSim, err := gorgonia.HadamardDiv(dot, normsProduct)
	if err != nil {
		panic(err)
	}

	// Build and run the VM
	machine := gorgonia.NewTapeMachine(g)
	err = machine.RunAll()
	if err != nil {
		panic(err)
	}
	machine.Close()

	// Retrieve cosSim as a tensor
	cosVal := cosSim.Value().(tensor.Tensor)

	// Step 4: Argmax across axis=0 => for each column in cosVal, find the row with max
	argmaxTensor, err := tensor.Argmax(cosVal, 0)
	if err != nil {
		panic(err)
	}

	// Convert to Go slice
	bestMatches = argmaxTensor.Data().([]int)

	return cosSim.Value().Data().([]float32), bestMatches
}

// rowWiseL2Norm computes the row-wise L2-norms for a matrix node [N, D], returning a node of shape [N].
func rowWiseL2Norm(mat *gorgonia.Node) (*gorgonia.Node, error) {
	// square each element
	squared, err := gorgonia.Square(mat)
	if err != nil {
		return nil, err
	}
	// sum across dim=1 (each row)
	rowSums, err := gorgonia.Sum(squared, 1)
	if err != nil {
		return nil, err
	}
	// sqrt the sums -> L2 norms
	norms, err := gorgonia.Sqrt(rowSums)
	if err != nil {
		return nil, err
	}
	return norms, nil
}
