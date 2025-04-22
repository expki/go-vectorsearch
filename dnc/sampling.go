package dnc

import (
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/expki/go-vectorsearch/logger"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// sample number of vectors
func sample(
	multibar *mpb.Progress,
	id uint64,
	rowReader func() (vector []uint8),
	total int,
	size int,
) (output [][]uint8) {
	// progress bar
	bar := multibar.AddBar(
		0,
		mpb.PrependDecorators(
			decor.Name(fmt.Sprintf("%d Selecting Samples: ", id)),
			decor.CountersNoUnit("%d / %d"),
		),
		mpb.BarRemoveOnComplete(),
	)

	var indexes []int

	if total <= size {
		// smaller than target return all
		indexes = make([]int, total)
		for i := range total {
			indexes[i] = i
		}
		bar.IncrBy(total)
	} else {
		// generate random unique indexes
		indexes = generateUniqueRandom(bar, total, size)
		if len(indexes) != size {
			logger.Sugar().Fatalf("generateUniqueRandom returned incorrect size: %d != %d", len(indexes), size)
		}

		// sort indexes by ascending
		sort.Ints(indexes)
	}
	bar.EnableTriggerComplete()

	// progress bar
	bar = multibar.AddBar(
		0,
		mpb.PrependDecorators(
			decor.Name(fmt.Sprintf("%d Reading Samples: ", id)),
			decor.CountersNoUnit("%d / %d"),
		),
		mpb.BarRemoveOnComplete(),
	)

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
		bar.Increment()
	}
	bar.EnableTriggerComplete()

	return output
}

// Fisher-Yates shuffle algorithm for generating unique random numbers
func generateUniqueRandom(bar *mpb.Bar, n int, count int) []int {
	random := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Fisher-Yates partial shuffle algorithm
	numbers := make([]int, n)
	for i := range numbers {
		numbers[i] = i
	}
	for i := range count {
		j := i + random.Intn(n-i)
		numbers[i], numbers[j] = numbers[j], numbers[i]
		bar.Increment()
	}

	return numbers[:count]
}
