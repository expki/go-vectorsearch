//go:build gorgonia
// +build gorgonia

package compute

import (
	_ "github.com/expki/go-vectorsearch/env"
	"gorgonia.org/tensor"
)

func NewVector(vectorQuantized []uint8) Vector {
	cols := len(vectorQuantized) - 8
	if cols <= 0 {
		panic("vector columns are empty")
	}
	vector := dequantizeVectorFloat64(vectorQuantized)
	return &vectorContainer{
		dense: tensor.New(tensor.WithBacking(vector), tensor.WithShape(1, cols)),
		shape: tensor.Shape{cols},
	}
}

func NewMatrix(matrixQuantized [][]uint8) Matrix {
	rows := len(matrixQuantized)
	if rows == 0 {
		panic("matrix rows are empty")
	}
	cols := len(matrixQuantized[0]) - 8
	if cols <= 0 {
		panic("matrix columns are empty")
	}
	matrix := dequantizeMatrixFloat64(matrixQuantized)
	flat := make([]float32, rows*cols)
	for i, row := range matrix {
		copy(flat[i*cols:], row)
	}
	return &matrixContainer{
		dense: tensor.New(tensor.WithBacking(flat), tensor.WithShape(rows, cols)),
		shape: tensor.Shape{rows, cols},
	}
}

type vectorContainer struct {
	dense *tensor.Dense
	shape tensor.Shape
}

type matrixContainer struct {
	dense *tensor.Dense
	shape tensor.Shape
}

func (v *vectorContainer) Clone() (clone Vector) {
	return &vectorContainer{
		dense: v.dense.Clone().(*tensor.Dense),
		shape: v.shape,
	}
}

func (m *matrixContainer) Clone() (clone Matrix) {
	return &matrixContainer{
		dense: m.dense.Clone().(*tensor.Dense),
		shape: m.shape,
	}
}
