package compute

import (
	"log"
	"slices"

	_ "github.com/expki/go-vectorsearch/env"
	"gorgonia.org/gorgonia"
	"gorgonia.org/tensor"
)

// VectorCosineSimilarity facilitates the computation of cosine similarity between two vector with a single graph.
func (vector1 Vector) VectorCosineSimilarity(vector2 Vector) float32 {
	g := gorgonia.NewGraph()

	// Create nodes for both vectors. Here we assume each vector is stored as node1 row vector.
	node1 := gorgonia.NewTensor(g, tensor.Float32, 2, gorgonia.WithValue(vector1.Dense), gorgonia.WithName("node1"))
	node2 := gorgonia.NewTensor(g, tensor.Float32, 2, gorgonia.WithValue(vector2.Dense), gorgonia.WithName("node2"))
	node1 = gorgonia.Must(gorgonia.Reshape(node1, tensor.Shape{vector1.Dense.Shape()[1]}))
	node2 = gorgonia.Must(gorgonia.Reshape(node2, tensor.Shape{vector2.Dense.Shape()[1]}))

	// Compute dot product manually:
	//   dot = sum(a[i] * b[i])
	// Element-wise multiplication: a * b
	prod, err := gorgonia.HadamardProd(node1, node2)
	if err != nil {
		log.Fatal(err)
	}
	// Sum the products to obtain the dot product
	dot, err := gorgonia.Sum(prod)
	if err != nil {
		log.Fatal(err)
	}

	// Compute norm of a: sqrt(sum(a[i]^2))
	aSquare, err := gorgonia.HadamardProd(node1, node1)
	if err != nil {
		log.Fatal(err)
	}
	sumASquare, err := gorgonia.Sum(aSquare)
	if err != nil {
		log.Fatal(err)
	}
	normA, err := gorgonia.Sqrt(sumASquare)
	if err != nil {
		log.Fatal(err)
	}

	// Compute norm of b: sqrt(sum(b[i]^2))
	bSquare, err := gorgonia.HadamardProd(node2, node2)
	if err != nil {
		log.Fatal(err)
	}
	sumBSquare, err := gorgonia.Sum(bSquare)
	if err != nil {
		log.Fatal(err)
	}
	normB, err := gorgonia.Sqrt(sumBSquare)
	if err != nil {
		log.Fatal(err)
	}

	// Multiply norms: normA * normB
	normsProduct, err := gorgonia.Mul(normA, normB)
	if err != nil {
		log.Fatal(err)
	}

	// Compute cosine similarity: dot / (normA * normB)
	cosineSim, err := gorgonia.Div(dot, normsProduct)
	if err != nil {
		log.Fatal(err)
	}

	// Execute the computation graph.
	machine := gorgonia.NewTapeMachine(g)
	if err := machine.RunAll(); err != nil {
		panic(err)
	}
	machine.Close()

	// Extract the scalar float32 value from the result.
	return cosineSim.Value().Data().(float32)
}

// VectorCosineSimilarity facilitates the computation of cosine similarity between two vector with a reusable graph.
func VectorCosineSimilarity(vector1 Vector, vector2 Vector) (computer func(vector1, vector2 Vector) (similarity float32), done func()) {
	buildGraph := func(vectorShape1, vectorShape2 tensor.Shape) (node1, node2, cosineSim *gorgonia.Node, machine partialTapeMachine) {
		g := gorgonia.NewGraph()

		// Create nodes for both vectors. Here we assume each vector is stored as a row vector.
		node1 = gorgonia.NewTensor(g, tensor.Float32, 2, gorgonia.WithShape(vectorShape1...), gorgonia.WithName("node1"))
		node2 = gorgonia.NewTensor(g, tensor.Float32, 2, gorgonia.WithShape(vectorShape2...), gorgonia.WithName("node2"))
		node1 = gorgonia.Must(gorgonia.Reshape(node1, tensor.Shape{vector1.Dense.Shape()[1]}))
		node2 = gorgonia.Must(gorgonia.Reshape(node2, tensor.Shape{vector2.Dense.Shape()[1]}))

		// Compute element-wise multiplication then sum over axis 1 to get the dot product.
		elementwiseProd := gorgonia.Must(gorgonia.HadamardProd(node1, node2))
		dotProduct := gorgonia.Must(gorgonia.Sum(elementwiseProd))

		// Compute the norm of v1: sqrt(sum(v1^2))
		squared1 := gorgonia.Must(gorgonia.Square(node1))
		sumSquares1 := gorgonia.Must(gorgonia.Sum(squared1))
		norm1 := gorgonia.Must(gorgonia.Sqrt(sumSquares1))

		// Compute the norm of v2: sqrt(sum(v2^2))
		squared2 := gorgonia.Must(gorgonia.Square(node2))
		sumSquares2 := gorgonia.Must(gorgonia.Sum(squared2))
		norm2 := gorgonia.Must(gorgonia.Sqrt(sumSquares2))

		// Multiply the norms (both are 1-element tensors)
		normProduct := gorgonia.Must(gorgonia.Mul(norm1, norm2))

		// Divide dot product by the product of the norms to get the cosine similarity.
		cosineSim = gorgonia.Must(gorgonia.Div(dotProduct, normProduct))

		// Execute the computation graph.
		machine = gorgonia.NewTapeMachine(g)

		return
	}

	// initial state
	var (
		node1, node2, cosineSim    *gorgonia.Node
		machine                    partialTapeMachine
		vectorShape1, vectorShape2 tensor.Shape
	)

	return func(vector1, vector2 Vector) (similarity float32) {
			// Check if shapes are compatible with current machine
			if !slices.Equal(vectorShape1, vector1.Dense.Shape()) || !slices.Equal(vectorShape2, vector2.Dense.Shape()) {
				if machine != nil {
					machine.Close()
				}
				node1, node2, cosineSim, machine = buildGraph(vector1.Dense.Shape(), vector2.Dense.Shape())
			}

			// Update node values. Each call to Let updates the value for the next run
			if err := gorgonia.Let(node1, vector1.Dense); err != nil {
				panic(err)
			}
			if err := gorgonia.Let(node2, vector2.Dense); err != nil {
				panic(err)
			}

			// Execute the graph.
			if err := machine.RunAll(); err != nil {
				panic(err)
			}

			// Reset the machine to clear the tape for the next run
			machine.Reset()

			// Return data
			return cosineSim.Value().Data().([]float32)[0]
		}, func() {
			if machine != nil {
				machine.Close()
			}
		}
}

// MatrixCosineSimilarity facilitates the computation of cosine similarity between a vector and a matrix with single graph.
func (vector Vector) MatrixCosineSimilarity(matrix Matrix) (similarity []float32) {
	g := gorgonia.NewGraph()

	// Input vector
	inputNode := gorgonia.NewTensor(g, tensor.Float32, 2, gorgonia.WithValue(vector.Dense), gorgonia.WithName("node1"))

	// Batch matrix
	batchNode := gorgonia.NewTensor(g, tensor.Float32, 2, gorgonia.WithValue(matrix.Dense), gorgonia.WithName("node2"))

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
		inputShape := []int{1, vectorShape[1]} // assuming a 1×N vector
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
			// Check if shapes are compatible with current machine
			if !slices.Equal(vectorShape, vector.Dense.Shape()) || !slices.Equal(matrixShape, matrix.Shape) {
				if machine != nil {
					machine.Close()
				}
				inputNode, batchNode, cosineSim, machine = buildGraph(vector.Dense.Shape(), matrix.Shape)
			}

			// Update node values. Each call to Let updates the value for the next run
			if err := gorgonia.Let(inputNode, vector.Dense); err != nil {
				panic(err)
			}
			if err := gorgonia.Let(batchNode, matrix.Dense); err != nil {
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
func (matrix1 Matrix) MatrixCosineSimilarity(matrix2 Matrix) (relativeSimilaritieList []float32, nearestIndexList []int) {
	g := gorgonia.NewGraph()

	// Create tensor nodes to hold M1 and M2 (rank=2)
	M1 := gorgonia.NewTensor(g, tensor.Float32, 2, gorgonia.WithValue(matrix1.Dense), gorgonia.WithName("node1"))

	M2 := gorgonia.NewTensor(g, tensor.Float32, 2, gorgonia.WithValue(matrix2.Dense), gorgonia.WithName("node2"))

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
	M2NormsRow, err := gorgonia.Reshape(M2Norms, tensor.Shape{1, matrix2.Shape[0]})
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
			// Check if shapes are compatible with current machine
			if !slices.Equal(matrixShape1, matrix1.Shape) || !slices.Equal(matrixShape2, matrix2.Shape) {
				if machine != nil {
					machine.Close()
				}
				M1, M2, cosineSim, machine = buildGraph(matrix1.Shape, matrix2.Shape)
			}

			// Update node values. Each call to Let updates the value for the next run
			if err := gorgonia.Let(M1, matrix1.Dense); err != nil {
				panic(err)
			}
			if err := gorgonia.Let(M2, matrix2.Dense); err != nil {
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
