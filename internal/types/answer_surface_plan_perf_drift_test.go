package types

import (
	"testing"
)

// TestCollectArtifactObservedAnchors_PerfStallProducesAnchor: round-8
// trace-symmetry. With ONLY a perf bundle (no log), a stall whose
// file/line don't match current code must still produce a derived
// anchor. Pre-fix the derive pipeline ignored perf bundles entirely.
func TestCollectArtifactObservedAnchors_PerfStallProducesAnchor(t *testing.T) {
	perf := &PerfBundle{
		Stalls: []PerfStall{
			{
				Symbol: "Render",
				File:   "internal/ui/view.go",
				Line:   60, // trace observed at 60
			},
		},
	}
	items := []EvidenceItem{
		{
			Kind:            EvidenceDirect,
			Scope:           ScopeLine,
			Source:          "internal/ui/view.go",
			LineStart:       180, // current code at 180
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "Render",
			GroundingStatus: GroundingGrounded,
		},
	}
	anchors := CollectArtifactObservedAnchors(RequestModel{}, nil, perf, items)
	if len(anchors) != 1 {
		t.Fatalf("perf-only run: got %d anchors; want 1", len(anchors))
	}
	got := anchors[0]
	if got.File != "internal/ui/view.go" || got.AnchoredLine != 180 || got.ObservedLine != 60 {
		t.Errorf("perf anchor mis-derived: %+v", got)
	}
	if got.Reason != DriftReasonLineDrift {
		t.Errorf("Reason = %q; want line_drift", got.Reason)
	}
}

// TestCollectArtifactSourceDriftAnchors_PerfFiltersToDriftedOnly:
// perf observed-cross-source items are NOT in the drift subset;
// drifted ones are.
func TestCollectArtifactSourceDriftAnchors_PerfFiltersToDriftedOnly(t *testing.T) {
	perf := &PerfBundle{
		Stalls: []PerfStall{
			{Symbol: "Aligned", File: "a.go", Line: 10},
			{Symbol: "Drifted", File: "b.go", Line: 20},
		},
	}
	items := []EvidenceItem{
		// Aligned: same line as stall → cross-source factual.
		{Source: "a.go", LineStart: 10, AnchorSymbol: "Aligned", AnchorKind: AnchorDefinition, GroundingStatus: GroundingGrounded},
		// Drifted: far from stall's line → conditional/line_drift.
		{Source: "b.go", LineStart: 200, AnchorSymbol: "Drifted", AnchorKind: AnchorDefinition, GroundingStatus: GroundingGrounded},
	}
	observed := CollectArtifactObservedAnchors(RequestModel{}, nil, perf, items)
	if len(observed) != 2 {
		t.Errorf("observed count = %d; want 2", len(observed))
	}
	drift := CollectArtifactSourceDriftAnchors(RequestModel{}, nil, perf, items)
	if len(drift) != 1 {
		t.Fatalf("drift count = %d; want 1", len(drift))
	}
	if drift[0].File != "b.go" {
		t.Errorf("drift anchor wrong file: %+v", drift[0])
	}
}

// TestCollectArtifactObservedAnchors_LogPlusPerfMixed: when both
// log frames and perf stalls are present, both contribute to the
// frame source. Demonstrates symmetric coverage.
func TestCollectArtifactObservedAnchors_LogPlusPerfMixed(t *testing.T) {
	logBundle := &LogBundle{
		Errors: []LogError{{Frames: []LogFrame{
			{File: "log.go", Line: 50, Func: "LogFunc", Raw: "log.go:50"},
		}}},
	}
	perf := &PerfBundle{
		Stalls: []PerfStall{
			{Symbol: "PerfFunc", File: "perf.go", Line: 100},
		},
	}
	items := []EvidenceItem{
		{Source: "log.go", LineStart: 60, AnchorSymbol: "LogFunc", AnchorKind: AnchorDefinition, GroundingStatus: GroundingGrounded},
		{Source: "perf.go", LineStart: 110, AnchorSymbol: "PerfFunc", AnchorKind: AnchorDefinition, GroundingStatus: GroundingGrounded},
	}
	anchors := CollectArtifactObservedAnchors(RequestModel{}, logBundle, perf, items)
	if len(anchors) != 2 {
		t.Errorf("mixed run: anchors = %d; want 2", len(anchors))
	}
	gotFiles := make(map[string]bool)
	for _, a := range anchors {
		gotFiles[a.File] = true
	}
	if !gotFiles["log.go"] || !gotFiles["perf.go"] {
		t.Errorf("expected both log.go and perf.go anchors; got %v", gotFiles)
	}
}

// TestBackfillUnprojectedItems_PerfStallTagsItemAsPerfOrigin: when
// a perf stall produces drift, the projected item's Origin must
// be ClaimOriginPerf (not Log). Pre-fix all backfill projections
// collapsed to ClaimOriginLog.
func TestBackfillUnprojectedItems_PerfStallTagsItemAsPerfOrigin(t *testing.T) {
	perf := &PerfBundle{
		Stalls: []PerfStall{
			{Symbol: "Render", File: "ui.go", Line: 60},
		},
	}
	items := []EvidenceItem{
		{Source: "ui.go", LineStart: 200, AnchorSymbol: "Render", AnchorKind: AnchorDefinition, GroundingStatus: GroundingGrounded},
	}
	out := backfillUnprojectedItemsForDeriveArtifacts(items, nil, perf)
	if len(out) != 1 {
		t.Fatalf("len = %d; want 1", len(out))
	}
	if out[0].Origin != ClaimOriginPerf {
		t.Errorf("Origin = %q; want perf", out[0].Origin)
	}
	if out[0].DriftReason != DriftReasonLineDrift {
		t.Errorf("DriftReason = %q; want line_drift", out[0].DriftReason)
	}
}
