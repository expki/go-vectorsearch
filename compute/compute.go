package compute

import (
	_ "github.com/expki/go-vectorsearch/env"
	"gorgonia.org/tensor"
)

type Vector struct{ Dense *tensor.Dense }

func NewTensor(vector []uint8) Vector {
	if len(vector) == 0 {
		panic("empty vector provided")
	}
	return Vector{
		Dense: tensor.New(tensor.WithBacking(DequantizeVector(vector, -1, 1)), tensor.WithShape(1, len(vector))),
	}
}

type Matrix struct {
	Dense *tensor.Dense
	Shape tensor.Shape
}

func NewMatrix(matrix [][]uint8) Matrix {
	if len(matrix) == 0 || len(matrix[0]) == 0 {
		panic("empty matrix provided")
	}
	vector := make([]float32, 0, len(matrix[0])*len(matrix))
	for _, row := range matrix {
		vector = append(vector, DequantizeVector(row, -1, 1)...)
	}
	return Matrix{
		Dense: tensor.New(tensor.WithBacking(vector), tensor.WithShape(len(matrix), len(matrix[0]))),
		Shape: tensor.Shape{len(matrix), len(matrix[0])},
	}
}
