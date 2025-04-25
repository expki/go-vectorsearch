package config

import "time"

const (
	BATCH_SIZE_DATABASE = 1_000
	BATCH_SIZE_CACHE    = 10_000
	CENTROID_SIZE       = 10_000

	SAMPLE_SIZE             = 5 * BATCH_SIZE_CACHE
	SPLIT_SIZE              = 5
	SUPERSET_MUL            = 5
	KMEANS_ITTERATION_LIMIT = 1_000

	CACHE_DURATION = 5 * time.Second
	CACHE_CLEANUP  = 15 * time.Second

	HTTP_CLIENT_MAX_REQUESTS uint64 = 500
)
