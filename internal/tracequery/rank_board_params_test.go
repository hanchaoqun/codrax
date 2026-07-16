package tracequery

// XLANE-3 件1 (§29.104.2 定谳③, 2026-07-16): the rank BOARD identity triple's
// params half — engine mint pins.

import "testing"

// TestRootCauseBoardParamsFingerprintNormalizedIdentity: an unset knob and its
// explicit default fingerprint IDENTICALLY (Run normalizes before any build),
// so "same spec, different spelling" can never split one board in two.
func TestRootCauseBoardParamsFingerprintNormalizedIdentity(t *testing.T) {
	base := Query{View: "root_cause_rank", PID: 1, TimeStart: 1, TimeEnd: 2, TimeStartSet: true, TimeEndSet: true}
	unset := normalizeQuery(nil, base)
	explicit := base
	explicit.MaxDepth = wakeupChainDefaultMaxDepth
	explicit.MaxBranches = wakeupChainDefaultMaxBranches
	explicit.MinDurationMs = 1
	explicit.Limit = sharedDefaultResultLimit
	explicit = normalizeQuery(nil, explicit)
	a, b := rootCauseBoardParamsFingerprint(unset), rootCauseBoardParamsFingerprint(explicit)
	if a != b {
		t.Fatalf("unset vs explicit-default knobs must fingerprint identically: %s vs %s", a, b)
	}
	if len(a) != 8 {
		t.Fatalf("the fingerprint is the fixed 8-hex form, got %q", a)
	}
}

// TestRootCauseBoardParamsFingerprintSplitsOnKnobChange: every knob of the
// closed set changes the fingerprint — two boards ranked under different
// knobs are different ordinal domains. 修补轮 件D (2026-07-16): the closed
// set gains every remaining rank-shaping Query knob (core_topology witnessed
// on donghu 9163: r8 eff 4.783→3.806 under an unchanged fingerprint = silent
// board collision).
func TestRootCauseBoardParamsFingerprintSplitsOnKnobChange(t *testing.T) {
	base := normalizeQuery(nil, Query{View: "root_cause_rank", PID: 1, TimeStart: 1, TimeEnd: 2, TimeStartSet: true, TimeEndSet: true})
	ref := rootCauseBoardParamsFingerprint(base)
	for name, mutate := range map[string]func(*Query){
		"MaxDepth":      func(q *Query) { q.MaxDepth += 2 },
		"MaxBranches":   func(q *Query) { q.MaxBranches += 2 },
		"MinDurationMs": func(q *Query) { q.MinDurationMs += 0.5 },
		"Limit":         func(q *Query) { q.Limit += 4 },
		"CoreTopology":  func(q *Query) { q.CoreTopology = "big=10-13;middle=4-9;little=0-3" },
		"ViaThread":     func(q *Query) { q.ViaThread = "logd.writer-9163" },
		"LineStart":     func(q *Query) { q.LineStart = 100 },
		"LineEnd":       func(q *Query) { q.LineEnd = 900 },
		"TraceFlavor":   func(q *Query) { q.TraceFlavor = TraceFlavorAndroidAtrace },
		"TracePlatform": func(q *Query) { q.TracePlatform = TracePlatformDonghu },
	} {
		q := base
		mutate(&q)
		if got := rootCauseBoardParamsFingerprint(q); got == ref {
			t.Fatalf("%s change must split the board fingerprint", name)
		}
	}
	// Window and target are the triple's OTHER components — deliberately not
	// fingerprinted (they would double-count identity halves).
	q := base
	q.PID, q.TimeStart, q.TimeEnd = 99, 5, 6
	if got := rootCauseBoardParamsFingerprint(q); got != ref {
		t.Fatalf("window/target must not enter the params fingerprint")
	}
}

// TestRootCauseBoardParamsFingerprintCanonicalization — 件D 恒等纪律 pins:
// spellings the engine treats identically fingerprint identically, and only
// those.
func TestRootCauseBoardParamsFingerprintCanonicalization(t *testing.T) {
	base := normalizeQuery(nil, Query{View: "root_cause_rank", PID: 1, TimeStart: 1, TimeEnd: 2, TimeStartSet: true, TimeEndSet: true})
	ref := rootCauseBoardParamsFingerprint(base)
	// core_topology: spelling variants of ONE parsed topology are one board;
	// an unparseable string behaves as absent (the engine falls back to the
	// derived topology in exactly those cases).
	a, b := base, base
	a.CoreTopology = "big=10-13;middle=4-9;little=0-3"
	b.CoreTopology = " little = 0-3 , middle = 4-9 , big = 10-13 "
	fa, fb := rootCauseBoardParamsFingerprint(a), rootCauseBoardParamsFingerprint(b)
	if fa != fb {
		t.Fatalf("one parsed topology must fingerprint identically: %s vs %s", fa, fb)
	}
	if fa == ref {
		t.Fatalf("an explicit topology must split from the topology-less board")
	}
	garbage := base
	garbage.CoreTopology = "???not-a-topology"
	if got := rootCauseBoardParamsFingerprint(garbage); got != ref {
		t.Fatalf("an unparseable topology behaves as absent and must not split: %s vs %s", got, ref)
	}
	// via selector: "9163" and "pid=9163" parse to one selector.
	v1, v2 := base, base
	v1.ViaThread = "9163"
	v2.ViaThread = "pid=9163"
	if rootCauseBoardParamsFingerprint(v1) != rootCauseBoardParamsFingerprint(v2) {
		t.Fatalf("one parsed via selector must fingerprint identically")
	}
	if rootCauseBoardParamsFingerprint(v1) == ref {
		t.Fatalf("a via selector must split from the via-less board")
	}
}
