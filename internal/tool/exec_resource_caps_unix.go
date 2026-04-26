//go:build !windows

package tool

import "fmt"

// wrapShellCommandWithCaps prefixes a shell command with ulimit
// directives so the resulting command tree honours the per-process
// memory and CPU caps. The prefix runs in the supervisor-spawned sh
// (or bash on Windows-via-MSYS), so its rlimits are inherited by
// every descendant the test runner forks.
//
// Why ulimit and not setrlimit via Cmd.SysProcAttr: Go's syscall
// package on Linux exposes RLIMIT_* but not via SysProcAttr (the
// child must set it AFTER fork+exec, which Go's exec stdlib doesn't
// support without a fork-helper). The ulimit shell builtin runs
// inside the freshly-forked shell BEFORE exec'ing the test runner,
// achieving the same effect with no fork-helper code.
//
// Why -v (virtual memory) and not -m (RSS): Linux silently ignores
// RLIMIT_RSS on modern kernels (it's been a no-op since 2.4). -v
// (RLIMIT_AS, address space) is the cap the kernel actually enforces;
// large mmap allocations fail at allocation time, which most test
// frameworks turn into a clean MemoryError / runtime panic.
//
// Why -t (CPU seconds): caps total CPU time consumed by the process.
// Triggers SIGXCPU when exceeded. Distinct from wall-time timeout:
// a 300s wall budget on an 8-core host could permit 2400s of CPU
// before this fires. Caps act on actual burn, not wall.
//
// On macOS the shell builtin and rlimits behave identically to
// Linux — same code path.
func wrapShellCommandWithCaps(cmdStr string, opts SupervisedRunOptions) string {
	prefix := ""
	if opts.MemoryLimitBytes > 0 {
		// ulimit -v takes KB; integer divide is fine because the cap
		// values originate as MB-aligned numbers.
		kb := opts.MemoryLimitBytes / 1024
		prefix += fmt.Sprintf("ulimit -v %d 2>/dev/null; ", kb)
	}
	if opts.CPULimitSeconds > 0 {
		prefix += fmt.Sprintf("ulimit -t %d 2>/dev/null; ", opts.CPULimitSeconds)
	}
	if prefix == "" {
		return cmdStr
	}
	return prefix + cmdStr
}
