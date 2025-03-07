package compute

func Quantize(value float32, min float32, max float32) (valueQuantized uint8) {
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

func Dequantize(valueQuantized uint8, min float32, max float32) (value float32) {
	// Normalize the uint8 value to the range [0, 1]
	normalized := float32(valueQuantized) / 255.0
	// Scale back to the original range [min, max]
	return min + normalized*(max-min)
}

func QuantizeVector(vector []float32, min float32, max float32) (vectorQuantized []uint8) {
	vectorQuantized = make([]uint8, len(vector))
	for i, value := range vector {
		vectorQuantized[i] = Quantize(value, min, max)
	}
	return vectorQuantized
}

func DequantizeVector(vectorQuantized []uint8, min float32, max float32) (vector []float32) {
	vector = make([]float32, len(vectorQuantized))
	for i, value := range vectorQuantized {
		vector[i] = Dequantize(value, min, max)
	}
	return vector
}

func QuantizeMatrix(matrix [][]float32, min float32, max float32) (matrixQuantized [][]uint8) {
	matrixQuantized = make([][]uint8, len(matrix))
	for i, vector := range matrix {
		matrixQuantized[i] = QuantizeVector(vector, min, max)
	}
	return matrixQuantized
}

func DequantizeMatrix(matrixQuantized [][]uint8, min float32, max float32) (matrix [][]float32) {
	matrix = make([][]float32, len(matrixQuantized))
	for i, vector := range matrixQuantized {
		matrix[i] = DequantizeVector(vector, min, max)
	}
	return matrix
}
