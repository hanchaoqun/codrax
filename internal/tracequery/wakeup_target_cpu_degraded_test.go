package tracequery

// CR-4 引擎件 pins (2026-07-12; tieba witness: 1697 in-window sched_wakeup
// rows all target_cpu=000 while wakeups were emitted from several CPUs —
// converter degradation that funnels the whole cross-thread runnable backlog
// into "CPU0" on every target_cpu-keyed per-CPU face). The census is a typed
// FACT disclosure (window-stats caveat), never a gate.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func wakeupDegradedTrace(t *testing.T, zeroTargets bool, emitters int, rows int) *Index {
	t.Helper()
	var b strings.Builder
	ts := 10.0
	for i := 0; i < rows; i++ {
		cpu := i % emitters
		target := 0
		if !zeroTargets {
			target = (i % 4) + 1
		}
		fmt.Fprintf(&b, "  worker-%d (100) [%03d] .... %.6f: sched_wakeup: comm=app pid=200 prio=120 target_cpu=%03d\n",
			100+i%3, cpu, ts, target)
		ts += 0.0001
	}
	path := filepath.Join(t.TempDir(), "wakeups.systrace")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

// TestWakeupTargetCPUDegradedCaveat — the witness shape: ≥floor all-zero
// targets emitted from ≥2 CPUs → the typed degradation caveat renders with
// the population count.
func TestWakeupTargetCPUDegradedCaveat(t *testing.T) {
	idx := wakeupDegradedTrace(t, true, 4, 160)
	stats := ComputeWindowStats(idx, Query{TimeStart: 9, TimeEnd: 11})
	if !containsSubstring(stats.Caveats, "wakeup_target_cpu_degraded=true total=160") {
		t.Fatalf("the all-zero degradation caveat must render: %v", stats.Caveats)
	}
}

// TestWakeupTargetCPUHealthyAndSingleCoreSilent — a healthy target spread
// stays silent; an all-zero SINGLE-emitter trace (genuine one-core device)
// stays silent; a sub-floor population stays silent.
func TestWakeupTargetCPUHealthyAndSingleCoreSilent(t *testing.T) {
	healthy := wakeupDegradedTrace(t, false, 4, 160)
	if stats := ComputeWindowStats(healthy, Query{TimeStart: 9, TimeEnd: 11}); containsSubstring(stats.Caveats, "wakeup_target_cpu_degraded") {
		t.Fatalf("healthy target spread must stay silent: %v", stats.Caveats)
	}
	singleCore := wakeupDegradedTrace(t, true, 1, 160)
	if stats := ComputeWindowStats(singleCore, Query{TimeStart: 9, TimeEnd: 11}); containsSubstring(stats.Caveats, "wakeup_target_cpu_degraded") {
		t.Fatalf("a single-emitter trace legitimately targets cpu0: %v", stats.Caveats)
	}
	small := wakeupDegradedTrace(t, true, 4, 20)
	if stats := ComputeWindowStats(small, Query{TimeStart: 9, TimeEnd: 11}); containsSubstring(stats.Caveats, "wakeup_target_cpu_degraded") {
		t.Fatalf("a sub-floor population must stay silent: %v", stats.Caveats)
	}
}
