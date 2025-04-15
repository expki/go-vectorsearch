package dnc

import (
	"math/rand"
	"sort"
	"time"
)

// sample number of vectors
func sample(
	rowReader func() (vector []uint8),
	total int,
	size int,
) (output [][]uint8) {
	var indexes []int

	if total <= size {
		// smaller than target return all
		indexes = make([]int, total)
		for i := range total {
			indexes[i] = i
		}
	} else {
		// generate random unique indexes
		indexes = generateUniqueRandom(total, size)

		// sort indexes by ascending
		sort.Ints(indexes)
	}

	// read sorted indexes
	output = make([][]uint8, len(indexes))
	readerIdx := 0
	for outputIdx, targetIdx := range indexes {
		for readerIdx < targetIdx {
			rowReader()
			readerIdx++
		}
		output[outputIdx] = rowReader()
		readerIdx++
	}

	return output
}

// Fisher-Yates shuffle algorithm for generating unique random numbers
func generateUniqueRandom(n int, count int) []int {
	random := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Fisher-Yates partial shuffle algorithm
	numbers := make([]int, n+1)
	for i := range numbers {
		numbers[i] = i
	}
	for i := 0; i < count; i++ {
		j := i + random.Intn(n+1-i)
		numbers[i], numbers[j] = numbers[j], numbers[i]
	}

	return numbers[:count]
}
