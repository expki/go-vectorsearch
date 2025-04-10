package config

import "time"

const (
	BATCH_SIZE_DATABASE = 1_000
	BATCH_SIZE_CACHE    = 10_000
	CENTROID_SIZE       = 10_000

	CACHE_DURATION = 5 * time.Second
	CACHE_CLEANUP  = 15 * time.Second
)
