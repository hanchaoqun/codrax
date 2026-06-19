package tool

import (
	"sync/atomic"
	"time"
)

const defaultRunTestsTimeoutSeconds = 900

var runTestsDefaultTimeoutSec atomic.Uint64

// SetRunTestsDefaultTimeoutSeconds installs the process-wide default
// wall-clock budget for run_tests suite executions. Non-positive values
// reset to the code default so codrax.yaml :: verify_wall_timeout_seconds: 0
// preserves the safety cap instead of disabling it.
func SetRunTestsDefaultTimeoutSeconds(seconds int) {
	if seconds <= 0 {
		runTestsDefaultTimeoutSec.Store(defaultRunTestsTimeoutSeconds)
		return
	}
	runTestsDefaultTimeoutSec.Store(uint64(seconds))
}

// RunTestsDefaultTimeoutSeconds returns the configured run_tests wall budget.
func RunTestsDefaultTimeoutSeconds() int {
	if v := runTestsDefaultTimeoutSec.Load(); v > 0 {
		return int(v)
	}
	return defaultRunTestsTimeoutSeconds
}

func runTestsDefaultTimeout() time.Duration {
	return time.Duration(RunTestsDefaultTimeoutSeconds()) * time.Second
}
