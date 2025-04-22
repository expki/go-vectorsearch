package config

import "time"

const (
	BATCH_SIZE_DATABASE = 1_000
	BATCH_SIZE_CACHE    = 5_000
	CENTROID_SIZE       = 10_000

	SAMPLE_SIZE = 50_000
	SPLIT_SIZE  = 3

	CACHE_DURATION = 5 * time.Second
	CACHE_CLEANUP  = 15 * time.Second

	HTTP_CLIENT_MAX_REQUESTS uint64 = 500
)
