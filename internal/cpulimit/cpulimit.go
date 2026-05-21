// Package cpulimit keeps codrax polite to the rest of the host while it
// does CPU-bound work.
//
// A repomap full scan of a very large repository parses tens of
// thousands of source files with tree-sitter, saturating every CPU
// core for the duration. On a small remote host that can starve
// interactive processes — most visibly sshd, whose missed keepalives
// drop the operator's SSH session mid-scan.
//
// The fix is scheduling niceness: the process (and every scan worker
// thread) runs at a raised nice value, so the kernel scheduler lets
// interactive, normal-priority processes preempt it. Niceness costs
// nothing when the host is otherwise idle — the scan still consumes
// every spare cycle — and only yields under contention, which is
// exactly the desired behaviour.
package cpulimit

import "fmt"

// niceMin / niceMax bound the configurable nice value. Only raising
// nice (lowering priority) is in scope; that never needs privilege.
const (
	niceMin = 0
	niceMax = 19
)

// Config holds the resolved CPU-politeness settings.
type Config struct {
	// Enabled is the master switch. When false, Apply is a no-op and
	// NiceCurrentThread does nothing.
	Enabled bool
	// Nice is the target scheduling nice value, clamped to [0, 19].
	Nice int
}

// Result describes what Apply did, for the caller to log.
type Result struct {
	Applied bool
	Nice    int
	Note    string
}

// String renders a Result as a single human-readable log line.
func (r Result) String() string {
	if !r.Applied {
		if r.Note != "" {
			return "CPU politeness not applied: " + r.Note
		}
		return "CPU politeness not applied"
	}
	msg := fmt.Sprintf("CPU politeness: nice=%d (scan work yields to interactive processes)", r.Nice)
	if r.Note != "" {
		msg += " — " + r.Note
	}
	return msg
}

// resolved is the politeness level set by Apply, read by
// NiceCurrentThread from scan worker goroutines. It is written once at
// startup before any scan runs, then only read.
var resolved struct {
	enabled bool
	nice    int
}

// Apply resolves the politeness level and raises the calling process's
// nice value. Call once at startup, as early as possible: OS threads
// the Go runtime spawns afterwards inherit the nice value, and scan
// workers additionally pin it per-thread via NiceCurrentThread. Apply
// never fails — a platform that cannot set niceness yields a Result
// with Applied=false and an explanatory Note.
func Apply(cfg Config) Result {
	if !cfg.Enabled {
		resolved.enabled = false
		return Result{Note: "disabled via cpu_politeness_enabled"}
	}
	nice := cfg.Nice
	if nice < niceMin {
		nice = niceMin
	}
	if nice > niceMax {
		nice = niceMax
	}
	resolved.enabled = true
	resolved.nice = nice

	if err := setThreadNice(nice); err != nil {
		return Result{Nice: nice, Note: "setpriority failed (" + err.Error() + ")"}
	}
	return Result{Applied: true, Nice: nice}
}

// NiceCurrentThread raises the calling OS thread's nice value to the
// level resolved by Apply. A scan worker goroutine calls this right
// after runtime.LockOSThread, so every CPU-bound thread is polite
// regardless of when the Go runtime created the underlying M. It is a
// no-op when politeness is disabled or unsupported, and never panics.
func NiceCurrentThread() {
	if !resolved.enabled {
		return
	}
	_ = setThreadNice(resolved.nice)
}
