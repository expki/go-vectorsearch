//go:build gorgonia
// +build gorgonia

package compute

import (
	"slices"

	_ "github.com/expki/go-vectorsearch/env"
	"gorgonia.org/gorgonia"
	"gorgonia.org/tensor"
)

// MatrixCosineSimilarity facilitates the computation of cosine similarity between a vector and a matrix with single graph.
func (vector *vectorContainer) MatrixCosineSimilarity(matrix Matrix) (similarity []float32) {
	realMatrix := matrix.(*matrixContainer)
	g := gorgonia.NewGraph()

	// Input vector
	inputNode := gorgonia.NewTensor(g, tensor.Float32, 2, gorgonia.WithValue(vector.dense), gorgonia.WithName("node1"))

	// Batch matrix
	batchNode := gorgonia.NewTensor(g, tensor.Float32, 2, gorgonia.WithValue(realMatrix.dense), gorgonia.WithName("node2"))

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

// VectorMatrixCosineSimilarity facilitates the computation of cosine similarity between a vector and a matrix with reusable graph.
func VectorMatrixCosineSimilarity() (calculate func(vector Vector, matrix Matrix) (similarity []float32), done func()) {
	buildGraph := func(vectorShape, matrixShape tensor.Shape) (inputNode, batchNode, cosineSim *gorgonia.Node, machine partialTapeMachine) {
		g := gorgonia.NewGraph()

		// Create nodes with fixed shapes
		inputShape := []int{1, vectorShape[0]} // assuming a 1×N vector
		batchShape := []int{matrixShape[0], matrixShape[1]}

		// Input vector
		inputNode = gorgonia.NewTensor(g, tensor.Float32, 2, gorgonia.WithShape(inputShape...), gorgonia.WithName("node1"))

		// Batch matrix
		batchNode = gorgonia.NewTensor(g, tensor.Float32, 2, gorgonia.WithShape(batchShape...), gorgonia.WithName("node2"))

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
		cosineSim = gorgonia.Must(gorgonia.Div(dotProduct, denominator))

		// Execute the graph
		machine = gorgonia.NewTapeMachine(g)

		return
	}

	// initial state
	var (
		inputNode, batchNode, cosineSim *gorgonia.Node
		machine                         partialTapeMachine
		vectorShape, matrixShape        tensor.Shape
	)

	return func(vector Vector, matrix Matrix) (similarity []float32) {
			realVector := vector.(*vectorContainer)
			realMatrix := matrix.(*matrixContainer)
			// Check if shapes are compatible with current machine
			if !slices.Equal(vectorShape, realVector.shape) || !slices.Equal(matrixShape, realMatrix.shape) {
				if machine != nil {
					machine.Close()
				}
				inputNode, batchNode, cosineSim, machine = buildGraph(realVector.shape, realMatrix.shape)
				vectorShape = realVector.shape
				matrixShape = realMatrix.shape
			}

			// Update node values. Each call to Let updates the value for the next run
			if err := gorgonia.Let(inputNode, realVector.dense); err != nil {
				panic(err)
			}
			if err := gorgonia.Let(batchNode, realMatrix.dense); err != nil {
				panic(err)
			}

			// Execute the graph.
			if err := machine.RunAll(); err != nil {
				panic(err)
			}

			// Reset the machine to clear the tape for the next run
			machine.Reset()

			// Return data
			return cosineSim.Value().Data().([]float32)
		}, func() {
			if machine != nil {
				machine.Close()
			}
		}
}

// MatrixCosineSimilarity facilitates the computation of cosine similarity between a matrix and a matrix with single graph.
// The first matrix is the input matrix and the second matrix is the batch of vectors to compare against.
func (matrix1 *matrixContainer) MatrixCosineSimilarity(matrix2 Matrix) (relativeSimilaritieList []float32, nearestIndexList []int) {
	realMatrix2 := matrix2.(*matrixContainer)
	g := gorgonia.NewGraph()

	// Create tensor nodes to hold M1 and M2 (rank=2)
	M1 := gorgonia.NewTensor(g, tensor.Float32, 2, gorgonia.WithValue(matrix1.dense), gorgonia.WithName("node1"))

	M2 := gorgonia.NewTensor(g, tensor.Float32, 2, gorgonia.WithValue(realMatrix2.dense), gorgonia.WithName("node2"))

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
	M1NormsCol, err := gorgonia.Reshape(M1Norms, tensor.Shape{matrix1.shape[0], 1})
	if err != nil {
		panic(err)
	}
	M2NormsRow, err := gorgonia.Reshape(M2Norms, tensor.Shape{1, realMatrix2.shape[0]})
	if err != nil {
		panic(err)
	}

	normsProduct, err := gorgonia.BroadcastHadamardProd(M1NormsCol, M2NormsRow, []byte{1}, []byte{0})
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
	nearestIndexList = argmaxTensor.Data().([]int)

	return cosSim.Value().Data().([]float32), nearestIndexList
}

// MatrixCosineSimilarity facilitates the computation of cosine similarity between a matrix and a matrix with reusable graph.
// The first matrix is the input matrix and the second matrix is the batch of vectors to compare against.
func MatrixCosineSimilarity() (calculate func(matrix1 Matrix, matrix2 Matrix) (relativeSimilaritieList []float32, nearestIndexList []int), done func()) {
	buildGraph := func(matrixShape1, matrixShape2 tensor.Shape) (M1, M2, cosineSim *gorgonia.Node, machine partialTapeMachine) {
		g := gorgonia.NewGraph()

		// Create tensor nodes to hold M1 and M2 (rank=2)
		M1 = gorgonia.NewTensor(g, tensor.Float32, 2, gorgonia.WithShape(matrixShape1...), gorgonia.WithName("node1"))

		M2 = gorgonia.NewTensor(g, tensor.Float32, 2, gorgonia.WithShape(matrixShape2...), gorgonia.WithName("node2"))

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
		M1NormsCol, err := gorgonia.Reshape(M1Norms, tensor.Shape{matrixShape1[0], 1})
		if err != nil {
			panic(err)
		}
		M2NormsRow, err := gorgonia.Reshape(M2Norms, tensor.Shape{1, matrixShape2[0]})
		if err != nil {
			panic(err)
		}

		normsProduct, err := gorgonia.BroadcastHadamardProd(M1NormsCol, M2NormsRow, []byte{1}, []byte{0})
		if err != nil {
			panic(err)
		}

		// Step 3: Cosine similarity = dot / (||M1|| * ||M2||)
		cosineSim, err = gorgonia.HadamardDiv(dot, normsProduct)
		if err != nil {
			panic(err)
		}

		// Execute the graph
		machine = gorgonia.NewTapeMachine(g)

		return
	}

	// initial state
	var (
		M1, M2, cosineSim          *gorgonia.Node
		machine                    partialTapeMachine
		matrixShape1, matrixShape2 tensor.Shape
	)

	return func(matrix1 Matrix, matrix2 Matrix) (relativeSimilaritieList []float32, nearestIndexList []int) {
			realMatrix1 := matrix1.(*matrixContainer)
			realMatrix2 := matrix2.(*matrixContainer)
			// Check if shapes are compatible with current machine
			if !slices.Equal(matrixShape1, realMatrix1.shape) || !slices.Equal(matrixShape2, realMatrix2.shape) {
				if machine != nil {
					machine.Close()
				}
				M1, M2, cosineSim, machine = buildGraph(realMatrix1.shape, realMatrix2.shape)
				matrixShape1 = realMatrix1.shape
				matrixShape2 = realMatrix2.shape
			}

			// Update node values. Each call to Let updates the value for the next run
			if err := gorgonia.Let(M1, realMatrix1.dense); err != nil {
				panic(err)
			}
			if err := gorgonia.Let(M2, realMatrix2.dense); err != nil {
				panic(err)
			}

			// Execute the graph.
			if err := machine.RunAll(); err != nil {
				panic(err)
			}

			// Retrieve cosSim as a tensor
			cosVal := cosineSim.Value().(tensor.Tensor)

			// Step 4: Argmax across axis=0 => for each column in cosVal, find the row with max
			argmaxTensor, err := tensor.Argmax(cosVal, 0)
			if err != nil {
				panic(err)
			}

			// Convert to Go slice
			nearestIndexList = argmaxTensor.Data().([]int)
			relativeSimilaritieList = cosineSim.Value().Data().([]float32)

			// Reset the machine to clear the tape for the next run
			machine.Reset()

			return relativeSimilaritieList, nearestIndexList
		}, func() {
			if machine != nil {
				machine.Close()
			}
		}
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

type partialTapeMachine interface {
	Close() error
	Reset()
	RunAll() error
}
