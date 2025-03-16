package config

import "time"

const (
	BATCH_SIZE_DATABASE          = 1_000
	MAX_CENTROID_SIZE            = 10_000
	CENTROID_REBALANCE_THRESHOLD = 500
	CENTROID_REFRESH_INTERVAL    = 5 * time.Minute
)
