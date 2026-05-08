package multigraph

import "fmt"

// TelemetrySnapshot is the per-Run summary the orchestrator logs at
// Run exit so operators can see how the multigraph layer performed
// (active set utilisation, eviction pressure, partial-typed-lane
// disclosure). Stable struct so logging / future telemetry pipeline
// integrations can read fields directly.
type TelemetrySnapshot struct {
	Discovered      int      // total sub-repos in topology
	Active          int      // sub-repos resident in LRU at snapshot time
	Cap             int      // current LRU cap
	Pending         []string // RootRel of sub-repos NOT in active set (R6 — RootRel only, never slug)
	Thrashing       bool     // true if eviction-rate threshold crossed during Run
	EvictionsInWindow int    // count of evictions in the last 60s window
	IsSingle        bool     // single-repo posture (degenerate)
}

// Snapshot collects current MultiGraph state into a TelemetrySnapshot.
// Safe to call from any goroutine; reads internal LRU + thrashing
// trackers under their own locks.
func (m *MultiGraph) Snapshot() TelemetrySnapshot {
	if m == nil {
		return TelemetrySnapshot{}
	}
	snap := TelemetrySnapshot{
		Cap:               m.Cap(),
		Pending:           m.PendingSubRepoNames(),
		Thrashing:         m.Thrashing(),
		IsSingle:          m.IsSingle(),
	}
	if m.thrash != nil {
		snap.EvictionsInWindow = m.thrash.CountInWindow()
	}
	if m.topo != nil {
		snap.Discovered = len(m.topo.Repos)
	}
	if m.active != nil {
		snap.Active = m.active.Len()
	}
	return snap
}

// FormatLogLine returns the canonical one-line log shape Phase 6
// uses: "multigraph: discovered=N active=N cap=N evicted_in_60s=N
// pending=[a,b] thrashing=false". Operators grep for "multigraph:"
// to find these.
func (s TelemetrySnapshot) FormatLogLine() string {
	pending := ""
	if len(s.Pending) > 0 {
		pending = " pending=" + fmt.Sprintf("%v", s.Pending)
	}
	mode := "multi"
	if s.IsSingle {
		mode = "single"
	}
	return fmt.Sprintf("multigraph: mode=%s discovered=%d active=%d cap=%d evicted_in_60s=%d%s thrashing=%v",
		mode, s.Discovered, s.Active, s.Cap, s.EvictionsInWindow, pending, s.Thrashing)
}
