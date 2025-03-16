package config

import "time"

const (
	BATCH_SIZE_DATABASE       = 1_000
	MAX_CENTROID_SIZE         = 10_000
	CENTROID_REFRESH_INTERVAL = time.Minute
)
