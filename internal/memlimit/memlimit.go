// Package memlimit derives and installs a soft heap limit (GOMEMLIMIT)
// for the codrax process at startup.
//
// On a memory-constrained host a full repomap scan of a very large
// repository (e.g. the Linux kernel: ~64k parsed source files) can grow
// the Go heap past available RAM and get the process OOM-killed
// mid-scan. A soft limit makes the garbage collector run more
// aggressively as the heap approaches the limit, trading scan latency
// for staying alive.
//
// The limit is SOFT: it bounds the GC's target, not a hard ceiling. If
// the live (non-garbage) working set genuinely exceeds the limit the GC
// cannot reclaim it and the process may still OOM — but the dominant
// cost of a large scan is transient parse garbage, which a soft limit
// reins in effectively.
package memlimit

import (
	"fmt"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

// minLimitBytes is the floor for an applied soft limit. A fraction or
// explicit value resolving below this is raised to it: a sub-512MiB
// limit would strangle even modest workloads without preventing any
// realistic OOM.
const minLimitBytes int64 = 512 << 20

// defaultFraction is used when the configured fraction is out of the
// (0, 1] range.
const defaultFraction = 0.8

// Config holds the resolved memory-limit settings (from codrax.yaml or
// code defaults).
type Config struct {
	// Enabled is the master switch. When false, Apply is a no-op.
	Enabled bool
	// Fraction of host RAM to target when no explicit byte count is
	// given. Clamped to (0, 1]; out-of-range values fall back to
	// defaultFraction.
	Fraction float64
	// ExplicitBytes, when > 0, is used verbatim as the limit and the
	// host-RAM path is skipped entirely.
	ExplicitBytes int64
}

// Result describes what Apply did, for the caller to log.
type Result struct {
	// Applied is true when a soft limit was installed this call.
	Applied bool
	// LimitBytes is the installed limit (valid only when Applied).
	LimitBytes int64
	// TotalBytes is the detected host RAM (0 when undetected).
	TotalBytes uint64
	// Source identifies how the limit was derived: "config-bytes",
	// "host-fraction", or "" when nothing was applied.
	Source string
	// Note explains why nothing was applied, or carries a benign
	// remark; empty when a limit was applied cleanly.
	Note string
}

// String renders a Result as a single human-readable log line.
func (r Result) String() string {
	if !r.Applied {
		if r.Note != "" {
			return "soft heap limit not applied: " + r.Note
		}
		return "soft heap limit not applied"
	}
	msg := fmt.Sprintf("soft heap limit %s (source=%s", humanBytes(r.LimitBytes), r.Source)
	if r.TotalBytes > 0 {
		msg += ", host=" + humanBytes(int64(r.TotalBytes))
	}
	msg += ")"
	if r.Note != "" {
		msg += " — " + r.Note
	}
	return msg
}

// Apply resolves and installs the soft heap limit. It is safe to call
// once at startup before any heavy work runs. The returned Result is
// purely informational; Apply itself never fails.
func Apply(cfg Config) Result {
	if !cfg.Enabled {
		return Result{Note: "disabled via memory_soft_limit_enabled"}
	}

	// The Go runtime already honours a GOMEMLIMIT environment variable
	// natively. If the operator set one, defer to it rather than
	// silently overriding an explicit choice.
	if env := strings.TrimSpace(os.Getenv("GOMEMLIMIT")); env != "" {
		return Result{Note: "GOMEMLIMIT already set in environment (" + env + ")"}
	}

	var (
		limit  int64
		source string
		total  uint64
		note   string
	)

	switch {
	case cfg.ExplicitBytes > 0:
		limit = cfg.ExplicitBytes
		source = "config-bytes"
	default:
		t, ok := systemTotalMemory()
		if !ok {
			return Result{Note: "host RAM undetected on this platform; set memory_soft_limit_bytes to enable"}
		}
		total = t
		frac := cfg.Fraction
		if frac <= 0 || frac > 1 {
			frac = defaultFraction
		}
		limit = int64(float64(t) * frac)
		source = "host-fraction"
	}

	if limit < minLimitBytes {
		note = fmt.Sprintf("raised to %s floor", humanBytes(minLimitBytes))
		limit = minLimitBytes
	}

	debug.SetMemoryLimit(limit)
	return Result{
		Applied:    true,
		LimitBytes: limit,
		TotalBytes: total,
		Source:     source,
		Note:       note,
	}
}

// systemTotalMemory reports total host RAM in bytes. It reads
// /proc/meminfo and is therefore Linux-only; on other platforms it
// returns ok=false and callers fall back to an explicit byte count.
func systemTotalMemory() (uint64, bool) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		// Expected shape: "MemTotal:  3902345 kB"
		if len(fields) < 2 {
			return 0, false
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil || kb == 0 {
			return 0, false
		}
		return kb * 1024, true
	}
	return 0, false
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f%ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
