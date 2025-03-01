package compute

import (
	_ "github.com/expki/govecdb/env"
	"gorgonia.org/tensor"
)

type Vector struct{ Dense *tensor.Dense }

func NewTensor(vector []uint8) Vector {
	if len(vector) == 0 {
		panic("empty vector provided")
	}
	return Vector{
		Dense: tensor.New(tensor.WithBacking(Uint8SliceToFloat32Slice(vector, -1, 1)), tensor.WithShape(1, len(vector))),
	}
}

type Matrix struct{ Dense *tensor.Dense }

func NewMatrix(matrix [][]uint8) Matrix {
	if len(matrix) == 0 || len(matrix[0]) == 0 {
		panic("empty matrix provided")
	}
	vector := make([]float32, 0, len(matrix[0])*len(matrix))
	for _, row := range matrix {
		vector = append(vector, Uint8SliceToFloat32Slice(row, -1, 1)...)
	}
	return Matrix{
		Dense: tensor.New(tensor.WithBacking(vector), tensor.WithShape(len(matrix), len(matrix[0]))),
	}
}

func Float32SliceToUint8Slice(floatSlice []float32, min float32, max float32) []uint8 {
	uint8Slice := make([]uint8, len(floatSlice))
	for i, value := range floatSlice {
		uint8Slice[i] = Float32ToUint8(value, min, max)
	}
	return uint8Slice
}

func Uint8SliceToFloat32Slice(uint8Slice []uint8, min float32, max float32) []float32 {
	float32Slice := make([]float32, len(uint8Slice))
	for i, value := range uint8Slice {
		float32Slice[i] = Uint8ToFloat32(value, min, max)
	}
	return float32Slice
}

func Float32ToUint8(value float32, min float32, max float32) uint8 {
	if value < min {
		value = min
	} else if value > max {
		value = max
	}
	// Normalize the value to the range [0, 1]
	normalized := (value - min) / (max - min)
	// Scale to [0, 255] and convert to uint8
	return uint8(normalized * 255)
}

func Uint8ToFloat32(value uint8, min float32, max float32) float32 {
	// Normalize the uint8 value to the range [0, 1]
	normalized := float32(value) / 255.0
	// Scale back to the original range [min, max]
	return min + normalized*(max-min)
}
