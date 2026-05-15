package types

import "testing"

// TestAllLogSignals_IncludesPerformance pins the 2026-05-02 enum
// expansion: SignalPerformance must be present in AllLogSignals so
// (a) the emit_log_triage tool's auto-derived JSON schema offers it
// to the LLM and (b) per-signal-aware downstream consumers (the
// analyzer's reconcileIntent and artifact observation support lanes)
// include it without per-call enumeration drift.
//
// Pre-fix the enum had 10 values; "the system was operational but
// noticeably slow / blocked / lost frames" had no fitting category
// and was forced into either timeout (prefix-misuse — no operation
// was actually cancelled) or other (information loss).
func TestAllLogSignals_IncludesPerformance(t *testing.T) {
	all := AllLogSignals()
	wantSize := 11
	if len(all) != wantSize {
		t.Errorf("AllLogSignals() returned %d signals, want %d (added SignalPerformance 2026-05-02)", len(all), wantSize)
	}
	hasPerf := false
	for _, sig := range all {
		if sig == SignalPerformance {
			hasPerf = true
			break
		}
	}
	if !hasPerf {
		t.Errorf("SignalPerformance missing from AllLogSignals(); got %+v", all)
	}
	if string(SignalPerformance) != "performance" {
		t.Errorf("SignalPerformance value = %q, want %q (load-bearing for skill prompt + tool schema)",
			string(SignalPerformance), "performance")
	}
}

// TestAllLogSignals_NoDuplicates is a structural guard — adding
// SignalPerformance close to SignalOther made it easy to typo a
// duplicate in either AllLogSignals or the const block.
func TestAllLogSignals_NoDuplicates(t *testing.T) {
	seen := make(map[LogSignal]bool)
	for _, sig := range AllLogSignals() {
		if seen[sig] {
			t.Errorf("AllLogSignals() contains duplicate %q", sig)
		}
		seen[sig] = true
	}
}

// TestAllLogSignals_OtherIsLast pins the canonical-fallback
// invariant: SignalOther sits at the end of the enumeration so the
// LLM-facing prompt's "everything-else" category remains visually
// distinct from the typed signals when the schema is rendered.
func TestAllLogSignals_OtherIsLast(t *testing.T) {
	all := AllLogSignals()
	if len(all) == 0 {
		t.Fatal("AllLogSignals() returned empty slice")
	}
	if all[len(all)-1] != SignalOther {
		t.Errorf("SignalOther must be last; got %v at tail", all[len(all)-1])
	}
}
