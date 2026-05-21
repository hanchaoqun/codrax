//go:build !linux && !darwin && !windows

package memlimit

// systemTotalMemory has no implementation on this platform; callers
// fall back to an explicit memory_soft_limit_bytes value.
func systemTotalMemory() (uint64, bool) { return 0, false }
