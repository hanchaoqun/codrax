package tool

import (
	"sync"
	"sync/atomic"
)

// Default resource caps for supervised exec — picked so adversarial
// LLM-generated test code (the OOM event that motivated this code:
// pytest at 2.47 GiB / 9 hours of CPU) is bounded to operator-friendly
// numbers without rejecting legit-but-heavy real test suites:
//
//   - Memory: 2 GiB. Big enough for most real test suites (Java
//     gradle, large Go monorepos), tight enough that a 4 GiB host
//     keeps Codrax + Claude Code + the test process alive together.
//   - CPU seconds: 600 (10 min). Wall-time is already capped by the
//     runTests timeout (default 300s). CPU-time cap is the
//     belt-and-suspenders that catches genuinely-burning loops on
//     multi-core hosts where 300s wall could mean 2400s of CPU.
const (
	defaultVerifyMemLimitBytes uint64 = 2 * 1024 * 1024 * 1024
	defaultVerifyCPULimitSec   uint64 = 600
)

var (
	// verifyMemLimitBytes is the memory cap applied to every
	// run_tests + exec_command supervised run. cmd/root.go sets it
	// from codrax.yaml :: verify_mem_limit_mb at startup; runs
	// before any tool dispatch.
	verifyMemLimitBytes atomic.Uint64

	// verifyCPULimitSec mirrors verifyMemLimitBytes for CPU seconds.
	verifyCPULimitSec atomic.Uint64

	verifyCapsOnce sync.Once
)

// SetVerifyResourceCaps installs the operator-supplied caps. memMB
// and cpuSec come from codrax.yaml :: verify_mem_limit_mb /
// verify_cpu_limit_seconds. Either zero leaves the corresponding cap
// at the package default; non-zero values override.
//
// Called once from cmd/root.go at startup. Tool implementations
// read via verifyResourceCaps() at execute time so a future
// REPL-driven knob change can take effect without restart.
func SetVerifyResourceCaps(memMB, cpuSec int) {
	verifyCapsOnce.Do(func() {
		verifyMemLimitBytes.Store(defaultVerifyMemLimitBytes)
		verifyCPULimitSec.Store(defaultVerifyCPULimitSec)
	})
	if memMB > 0 {
		verifyMemLimitBytes.Store(uint64(memMB) * 1024 * 1024)
	}
	if cpuSec > 0 {
		verifyCPULimitSec.Store(uint64(cpuSec))
	}
}

// verifyResourceCaps returns the currently-installed caps. Lazily
// initialises with package defaults the first time it's called from
// any goroutine, so tests that don't call SetVerifyResourceCaps still
// get the safety floor.
func verifyResourceCaps() SupervisedRunOptions {
	if verifyMemLimitBytes.Load() == 0 {
		verifyCapsOnce.Do(func() {
			verifyMemLimitBytes.Store(defaultVerifyMemLimitBytes)
			verifyCPULimitSec.Store(defaultVerifyCPULimitSec)
		})
	}
	return SupervisedRunOptions{
		MemoryLimitBytes: verifyMemLimitBytes.Load(),
		CPULimitSeconds:  verifyCPULimitSec.Load(),
	}
}
