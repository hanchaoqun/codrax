package cmd

import "testing"

// TestClampReflectorTimeout pins the safety-clamp contract behind
// the reflector adapter wiring. The reflector is a side-channel
// LLM call gated by a graceful-degrade fallback on the orchestrator
// side; what we MUST avoid is the reflector blocking the retry loop
// on a stalled upstream for the default 120 s HTTP timeout. The
// clamp enforces 30 s default unless the operator explicitly opted
// into a longer timeout via providers.yaml :: agents.reflector.
//
// Bug provenance: pig-latin attempt 2 in eval Batch F waited
// ~2.5 min on a stalled Chat call before http.Client.Timeout fired,
// burning a full retry slot for zero signal. The clamp prevents
// this regression class.
func TestClampReflectorTimeout(t *testing.T) {
	cases := []struct {
		name         string
		resolved     int
		hasExplicit  bool
		want         int
	}{
		// No agents.reflector: ResolveProvider returns the default
		// LLM's value (typically 0 → factory default 120s would
		// later kick in). Clamp must override to 30s.
		{"no explicit config + zero resolved → clamp", 0, false, 30},
		// No explicit config but the default carried a timeout
		// (operator set it on llm.default for the main LLM). Still
		// clamp — reflector should NOT inherit a 600s research-
		// model timeout.
		{"no explicit config + nonzero resolved → still clamp", 600, false, 30},
		// Operator explicitly set agents.reflector but didn't set
		// request_timeout_seconds. ResolveProvider's merge will
		// keep the field at zero (non-zero-overrides rule). Apply
		// the safety clamp.
		{"explicit config + zero seconds → clamp", 0, true, 30},
		// Operator explicitly set both agents.reflector AND a
		// positive request_timeout_seconds. Their decision wins.
		{"explicit config + 60s → operator wins", 60, true, 60},
		{"explicit config + 5s → operator wins", 5, true, 5},
		{"explicit config + 600s → operator wins", 600, true, 600},
		// Negative seconds is pathological; treat as unset and
		// clamp.
		{"explicit config + negative → clamp", -10, true, 30},
		{"no explicit config + negative → clamp", -1, false, 30},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := clampReflectorTimeout(c.resolved, c.hasExplicit)
			if got != c.want {
				t.Errorf("clampReflectorTimeout(%d, %v) = %d, want %d",
					c.resolved, c.hasExplicit, got, c.want)
			}
		})
	}
}

// TestClampReflectorTimeout_NeverReturnsZero is a structural
// invariant: every code path returns a positive value, so the
// downstream NewOpenAIAdapter never panics on a zero timeout
// (which it currently treats as a misconfiguration that should
// fail loud).
func TestClampReflectorTimeout_NeverReturnsZero(t *testing.T) {
	for resolved := -100; resolved <= 1000; resolved += 50 {
		for _, explicit := range []bool{true, false} {
			got := clampReflectorTimeout(resolved, explicit)
			if got <= 0 {
				t.Errorf("clamp returned non-positive %d for (resolved=%d, explicit=%v) — adapter constructor would panic",
					got, resolved, explicit)
			}
		}
	}
}
