//go:build !gonum && !gorgonia
// +build !gonum,!gorgonia

package compute

import (
	"slices"
)

func NewVector(vectorQuantized []uint8) Vector {
	cols := len(vectorQuantized) - 8
	if cols <= 0 {
		panic("vector columns are empty")
	}
	return &vectorContainer{
		data: DequantizeVectorFloat64(vectorQuantized),
		shape: vectorShape{
			cols: cols,
		},
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
	matrix := DequantizeMatrixFloat64(matrixQuantized)
	flat := make([]float64, rows*cols)
	for i, row := range matrix {
		copy(flat[i*cols:], row)
	}
	return &matrixContainer{
		data: flat,
		shape: matrixShape{
			rows: rows,
			cols: cols,
		},
	}
}

type vectorContainer struct {
	data  []float64
	shape vectorShape
}

type matrixContainer struct {
	data  []float64
	shape matrixShape
}

type vectorShape struct {
	cols int
}

type matrixShape struct {
	rows int
	cols int
}

func (v *vectorContainer) Clone() (clone Vector) {
	return &vectorContainer{
		data:  slices.Clone(v.data),
		shape: v.shape,
	}
}

func (m *matrixContainer) Clone() (clone Matrix) {
	return &matrixContainer{
		data:  slices.Clone(m.data),
		shape: m.shape,
	}
}
