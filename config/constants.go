package config

import "time"

const (
	BATCH_SIZE_DATABASE       = 1_000
	BATCH_SIZE_CACHE          = 10_000
	CENTROID_SIZE             = 20_000
	CENTROID_REFRESH_INTERVAL = time.Hour
)
