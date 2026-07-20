package tracequery

// cpu_constraint_r10wire_test.go — R10WIRE-1 (checklist R-10, §29.150⑫,
// 2026-07-20): the binding tier pair's dual-unit shape converges. Census
// verdict (M18 template, recorded before edits): the cpu_constraint_*_khz
// wire notes and the *_khz JSON payload fields keep exact kHz ints (the _khz
// key suffix is the typed unit self-description; a *_ghz rename would force
// lossy float wire = the §29.42 float64-zero trap), while the ONE free-text
// kHz face — the engine Summary k=v row, single-minted in
// renderCPUConstraintSummary and copied verbatim onto the rank seat — now
// speaks the B11 reader convention (%.2fGHz, ÷1e6). These pins hold the
// cross-face unit-consistency identity: every GHz utterance is the ÷1e6 %.2f
// image of the SAME typed kHz pair, and no face regrows a raw-kHz tier text.

import (
	"fmt"
	"strings"
	"testing"
)

// Pin ① (synthetic, both arms): the tier-pair token wears the GHz form with
// the exact ÷1e6 %.2f conversion and stays a space-free k=v token (spaces
// would split the field in the observation-ledger k=v parsing; the spaced `<`
// belongs to the display-face grammar only). The zero-pair negative arm mints
// no token at all (禁无中生有 — the render gate mirrors the proof-pair mint).
func TestR10WireTierPairSummarySpeaksGHz(t *testing.T) {
	item := CPUConstraintSummary{
		Thread:            ThreadRef{Comm: "JankManager", PID: 9655},
		Kind:              "sched_switch_next_info",
		AllowedCPUs:       []int{0, 1, 3},
		AllowedMaxTierKHz: 2270000,
		GlobalMaxTierKHz:  2750000,
	}
	summary := renderCPUConstraintSummary(item)
	if !strings.Contains(summary, "excludes_bigger_core_tier=allowed_max_tier=2.27GHz<global_max_tier=2.75GHz") {
		t.Fatalf("tier token must speak the B11 GHz convention as one space-free k=v token, got: %s", summary)
	}
	if strings.Contains(summary, "kHz") {
		t.Fatalf("no raw-kHz residue may survive on the constraint summary face, got: %s", summary)
	}
	// Cross-face identity by formula, not by literal: the GHz text is the
	// ÷1e6 %.2f image of the SAME typed ints the wire lane carries.
	wantAllowed := fmt.Sprintf("allowed_max_tier=%.2fGHz", float64(item.AllowedMaxTierKHz)/1e6)
	wantGlobal := fmt.Sprintf("global_max_tier=%.2fGHz", float64(item.GlobalMaxTierKHz)/1e6)
	if !strings.Contains(summary, wantAllowed) || !strings.Contains(summary, wantGlobal) {
		t.Fatalf("GHz text must be the ÷1e6 %%.2f image of the typed kHz pair (%s / %s), got: %s",
			wantAllowed, wantGlobal, summary)
	}
	// Negative arm: an incomplete pair (mint is all-or-nothing; render gate
	// mirrors it) speaks no exclusion claim in either unit.
	for _, broken := range []CPUConstraintSummary{
		{Thread: item.Thread, Kind: item.Kind, AllowedCPUs: item.AllowedCPUs},
		{Thread: item.Thread, Kind: item.Kind, AllowedCPUs: item.AllowedCPUs, AllowedMaxTierKHz: 2270000},
		{Thread: item.Thread, Kind: item.Kind, AllowedCPUs: item.AllowedCPUs, GlobalMaxTierKHz: 2750000},
	} {
		if got := renderCPUConstraintSummary(broken); strings.Contains(got, "excludes_bigger_core_tier") ||
			strings.Contains(got, "GHz") || strings.Contains(got, "kHz") {
			t.Fatalf("pair-less summary must carry no tier claim in any unit, got: %s", got)
		}
	}
}

// Pin ② (engine-real, donghu live witness — the §29.88.4 mask=ffb
// JankManager-9655 shape): the rank seat carries BOTH faces of the same
// measurement — exact kHz ints on the typed/wire lane (2270000 < 2750000,
// untouched by this batch) and the GHz free-text form on the Summary lane,
// related by the ÷1e6 %.2f identity. The pre-batch raw form
// (allowed_max_tier=2270000kHz) is dead on every text face.
func TestR10WireDonghuWitnessCrossFaceUnitIdentity(t *testing.T) {
	idx := rnb4DonghuIndex(t)
	q := Query{PID: 9655, TimeStart: 13762.934161, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
	rank := BuildRootCauseRank(idx, q)
	for _, item := range rank.Items {
		if item.Type != "cpu_affinity_or_cpuset" || item.Thread.PID != 9655 {
			continue
		}
		// Typed/wire lane: exact kHz ints, value semantics untouched.
		if item.CPUConstraintAllowedMaxTierKHz != 2270000 || item.CPUConstraintGlobalMaxTierKHz != 2750000 {
			t.Fatalf("wire lane must keep the exact kHz ints 2270000<2750000, got %d/%d",
				item.CPUConstraintAllowedMaxTierKHz, item.CPUConstraintGlobalMaxTierKHz)
		}
		// Free-text lane: the ÷1e6 %.2f image of those SAME ints.
		wantToken := fmt.Sprintf("excludes_bigger_core_tier=allowed_max_tier=%.2fGHz<global_max_tier=%.2fGHz",
			float64(item.CPUConstraintAllowedMaxTierKHz)/1e6, float64(item.CPUConstraintGlobalMaxTierKHz)/1e6)
		if !strings.Contains(item.Summary, wantToken) {
			t.Fatalf("summary must carry the GHz image of the wire ints (%s), got: %s", wantToken, item.Summary)
		}
		if strings.Contains(item.Summary, "2270000kHz") || strings.Contains(item.Summary, "2750000kHz") {
			t.Fatalf("pre-batch raw-kHz tier text must be dead, got: %s", item.Summary)
		}
		return
	}
	t.Fatalf("affinity seat missing (fixture drifted): %+v", rank.Items)
}
