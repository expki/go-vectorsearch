package compute

import (
	"encoding/binary"
	"math"
)

func Quantize[T float32 | float64](value T, min T, max T) (valueQuantized uint8) {
	if value < min {
		value = min
	} else if value > max {
		value = max
	}
	// Normalize the value to the range [0, 1]
	normalized := (value - min) / (max - min)
	// Scale to [0, 255] and convert to uint8
	valueQuantized = uint8(normalized * 255)
	return valueQuantized
}

func QuantizeFloat32(value float32, min float32, max float32) (valueQuantized uint8) {
	if value < min {
		value = min
	} else if value > max {
		value = max
	}
	// Normalize the value to the range [0, 1]
	normalized := (value - min) / (max - min)
	// Scale to [0, 255] and convert to uint8
	valueQuantized = uint8(normalized * 255)
	return valueQuantized
}

func QuantizeFloat64(value float64, min float64, max float64) (valueQuantized uint8) {
	if value < min {
		value = min
	} else if value > max {
		value = max
	}
	// Normalize the value to the range [0, 1]
	normalized := (value - min) / (max - min)
	// Scale to [0, 255] and convert to uint8
	valueQuantized = uint8(normalized * 255)
	return valueQuantized
}

func Dequantize[T float32 | float64](valueQuantized uint8, min T, max T) (value T) {
	// Normalize the uint8 value to the range [0, 1]
	normalized := T(valueQuantized) / 255.0
	// Scale back to the original range [min, max]
	value = min + normalized*(max-min)
	return value
}

func DequantizeFloat32(valueQuantized uint8, min float32, max float32) (value float32) {
	// Normalize the uint8 value to the range [0, 1]
	normalized := float32(valueQuantized) / 255.0
	// Scale back to the original range [min, max]
	value = min + normalized*(max-min)
	return value
}

func DequantizeFloat64(valueQuantized uint8, min float64, max float64) (value float64) {
	// Normalize the uint8 value to the range [0, 1]
	normalized := float64(valueQuantized) / 255.0
	// Scale back to the original range [min, max]
	value = min + normalized*(max-min)
	return value
}

func QuantizeVector[T float32 | float64](vector []T) (vectorQuantized []uint8) {
	vectorQuantized = make([]uint8, 8+len(vector))
	min, max := rangeFloat(vector)
	binary.LittleEndian.PutUint32(vectorQuantized, math.Float32bits(float32(min)))
	binary.LittleEndian.PutUint32(vectorQuantized[4:], math.Float32bits(float32(max)))
	for i, value := range vector {
		vectorQuantized[8+i] = Quantize(value, T(min), T(max))
	}
	return vectorQuantized
}

func QuantizeVectorFloat32(vector []float32) (vectorQuantized []uint8) {
	vectorQuantized = make([]uint8, 8+len(vector))
	min, max := rangeFloat32(vector)
	binary.LittleEndian.PutUint32(vectorQuantized, math.Float32bits(min))
	binary.LittleEndian.PutUint32(vectorQuantized[4:], math.Float32bits(max))
	for i, value := range vector {
		vectorQuantized[8+i] = QuantizeFloat32(value, min, max)
	}
	return vectorQuantized
}

func QuantizeVectorFloat64(vector []float64) (vectorQuantized []uint8) {
	vectorQuantized = make([]uint8, 8+len(vector))
	min, max := rangeFloat64(vector)
	binary.LittleEndian.PutUint32(vectorQuantized, math.Float32bits(float32(min)))
	binary.LittleEndian.PutUint32(vectorQuantized[4:], math.Float32bits(float32(max)))
	for i, value := range vector {
		vectorQuantized[8+i] = QuantizeFloat64(value, min, max)
	}
	return vectorQuantized
}

func DequantizeVector[T float32 | float64](vectorQuantized []uint8) (vector []T) {
	vector = make([]T, len(vectorQuantized)-8)
	min := T(math.Float32frombits(binary.LittleEndian.Uint32(vectorQuantized)))
	max := T(math.Float32frombits(binary.LittleEndian.Uint32(vectorQuantized[4:])))
	for i, value := range vectorQuantized[8:] {
		vector[i] = Dequantize(value, min, max)
	}
	return vector
}

func DequantizeVectorFloat32(vectorQuantized []uint8) (vector []float32) {
	min := math.Float32frombits(binary.LittleEndian.Uint32(vectorQuantized))
	max := math.Float32frombits(binary.LittleEndian.Uint32(vectorQuantized[4:]))
	vector = make([]float32, len(vectorQuantized)-8)
	for i, value := range vectorQuantized[8:] {
		vector[i] = DequantizeFloat32(value, min, max)
	}
	return vector
}

func DequantizeVectorFloat64(vectorQuantized []uint8) (vector []float64) {
	min := float64(math.Float32frombits(binary.LittleEndian.Uint32(vectorQuantized)))
	max := float64(math.Float32frombits(binary.LittleEndian.Uint32(vectorQuantized[4:])))
	vector = make([]float64, len(vectorQuantized)-8)
	for i, value := range vectorQuantized[8:] {
		vector[i] = DequantizeFloat64(value, min, max)
	}
	return vector
}

func QuantizeMatrix[T float32 | float64](matrix [][]T) (matrixQuantized [][]uint8) {
	matrixQuantized = make([][]uint8, len(matrix))
	for i, vector := range matrix {
		matrixQuantized[i] = QuantizeVector(vector)
	}
	return matrixQuantized
}

func QuantizeMatrixFloat32(matrix [][]float32) (matrixQuantized [][]uint8) {
	matrixQuantized = make([][]uint8, len(matrix))
	for i, vector := range matrix {
		matrixQuantized[i] = QuantizeVectorFloat32(vector)
	}
	return matrixQuantized
}

func QuantizeMatrixFloat64(matrix [][]float64) (matrixQuantized [][]uint8) {
	matrixQuantized = make([][]uint8, len(matrix))
	for i, vector := range matrix {
		matrixQuantized[i] = QuantizeVectorFloat64(vector)
	}
	return matrixQuantized
}

func DequantizeMatrix[T float32 | float64](matrixQuantized [][]uint8) (matrix [][]T) {
	matrix = make([][]T, len(matrixQuantized))
	for i, vector := range matrixQuantized {
		matrix[i] = DequantizeVector[T](vector)
	}
	return matrix
}

func DequantizeMatrixFloat32(matrixQuantized [][]uint8) (matrix [][]float32) {
	matrix = make([][]float32, len(matrixQuantized))
	for i, vector := range matrixQuantized {
		matrix[i] = DequantizeVectorFloat32(vector)
	}
	return matrix
}

func DequantizeMatrixFloat64(matrixQuantized [][]uint8) (matrix [][]float64) {
	matrix = make([][]float64, len(matrixQuantized))
	for i, vector := range matrixQuantized {
		matrix[i] = DequantizeVectorFloat64(vector)
	}
	return matrix
}

func rangeFloat[T float32 | float64](slice []T) (min T, max T) {
	for _, v := range slice {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}

func rangeFloat32(slice []float32) (min float32, max float32) {
	for _, v := range slice {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}

func rangeFloat64(slice []float64) (min float64, max float64) {
	for _, v := range slice {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}
