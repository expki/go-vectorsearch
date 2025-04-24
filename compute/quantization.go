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

func Dequantize[T float32 | float64](valueQuantized uint8, min T, max T) (value T) {
	// Normalize the uint8 value to the range [0, 1]
	normalized := T(valueQuantized) / 255.0
	// Scale back to the original range [min, max]
	value = min + normalized*(max-min)
	return value
}

func QuantizeVector[T float32 | float64](vector []T) (vectorQuantized []uint8) {
	vectorQuantized = make([]uint8, len(vector)+8)
	min, max := rangeFloat(vector)
	binary.LittleEndian.PutUint32(vectorQuantized, math.Float32bits(float32(min)))
	binary.LittleEndian.PutUint32(vectorQuantized[4:], math.Float32bits(float32(max)))
	for i, value := range vector {
		vectorQuantized[i+8] = Quantize(value, T(min), T(max))
	}
	return vectorQuantized
}

func DequantizeVector[T float32 | float64](vectorQuantized []uint8) (vector []T) {
	min := T(math.Float32frombits(binary.LittleEndian.Uint32(vectorQuantized)))
	max := T(math.Float32frombits(binary.LittleEndian.Uint32(vectorQuantized[4:])))
	vector = make([]T, len(vectorQuantized)-8)
	for i, value := range vectorQuantized[8:] {
		vector[i] = Dequantize[T](value, min, max)
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

func DequantizeMatrix[T float32 | float64](matrixQuantized [][]uint8) (matrix [][]T) {
	matrix = make([][]T, len(matrixQuantized))
	for i, vector := range matrixQuantized {
		matrix[i] = DequantizeVector[T](vector)
	}
	return matrix
}

func bytesToFloat32(bytes []byte) float32 {
	bits := binary.LittleEndian.Uint32(bytes)
	return math.Float32frombits(bits)
}

func floatToBytes(f float32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, math.Float32bits(f))
	return buf
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
