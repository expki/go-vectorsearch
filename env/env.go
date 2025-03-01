package env

import (
	"os"
)

func init() {
	// Set the environment variable to bypass the check for moving GC in CUDA environments. This is a workaround for known issues with CUDA and Go's garbage collector.
	os.Setenv("ASSUME_NO_MOVING_GC_UNSAFE_RISK_IT_WITH", "go1.24")
}
