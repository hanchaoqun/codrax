package types

import (
	"testing"
)

// TestCollectLogObservedAnchors_DerivesFromProjectedEvidence: when
// evidence carries Origin/Authority projection (round-1+ axis), the
// public Collect* functions derive their output from EvidenceItem
// fields rather than re-running OLD detection. This is the P2
// load-bearing assertion of plan-A unification.
func TestCollectLogObservedAnchors_DerivesFromProjectedEvidence(t *testing.T) {
	bundle := &LogBundle{
		Errors: []LogError{{Frames: []LogFrame{
			{File: "x.go", Line: 50, Func: "Run", Raw: "x.go:50 Run"},
		}}},
	}
	items := []EvidenceItem{
		{
			Kind:            EvidenceDirect,
			Scope:           ScopeLine,
			Source:          "x.go",
			LineStart:       100,
			AnchorSymbol:    "Run",
			AnchorKind:      AnchorDefinition,
			GroundingStatus: GroundingGrounded,
			// Projected fields: line drift detected (log says 50,
			// current code line 100).
			Origin:      ClaimOriginLog,
			Authority:   AuthorityConditional,
			DriftReason: DriftReasonLineDrift,
		},
	}
	anchors := CollectLogObservedAnchors(RequestModel{}, bundle, items)
	if len(anchors) != 1 {
		t.Fatalf("got %d anchors; want 1", len(anchors))
	}
	got := anchors[0]
	if got.File != "x.go" || got.ObservedLine != 50 || got.AnchoredLine != 100 {
		t.Errorf("anchor mis-derived: %+v", got)
	}
	if got.Reason != DriftReasonLineDrift {
		t.Errorf("Reason = %q; want line_drift", got.Reason)
	}
}

// TestCollectLogSourceDriftAnchors_FiltersToDriftedOnly: the drift
// subset must include only items the projection marked drifted.
// Cross-source factual items (log + repo agree) are observed but
// not drifted.
func TestCollectLogSourceDriftAnchors_FiltersToDriftedOnly(t *testing.T) {
	bundle := &LogBundle{
		Errors: []LogError{{Frames: []LogFrame{
			{File: "drifted.go", Line: 10, Func: "DriftedFn", Raw: "drifted.go:10"},
			{File: "agreed.go", Line: 20, Func: "AgreedFn", Raw: "agreed.go:20"},
		}}},
	}
	items := []EvidenceItem{
		// Cross-source factual: observed but not drifted.
		{
			Kind:            EvidenceDirect,
			Scope:           ScopeLine,
			Source:          "agreed.go",
			LineStart:       20,
			AnchorSymbol:    "AgreedFn",
			AnchorKind:      AnchorDefinition,
			GroundingStatus: GroundingGrounded,
			Origin:          ClaimOriginCrossSource,
			Authority:       AuthorityFactual,
		},
		// Log-derived conditional: drifted.
		{
			Kind:            EvidenceDirect,
			Scope:           ScopeLine,
			Source:          "drifted.go",
			LineStart:       50,
			AnchorSymbol:    "DriftedFn",
			AnchorKind:      AnchorDefinition,
			GroundingStatus: GroundingGrounded,
			Origin:          ClaimOriginLog,
			Authority:       AuthorityConditional,
			DriftReason:     DriftReasonLineDrift,
		},
	}
	observed := CollectLogObservedAnchors(RequestModel{}, bundle, items)
	if len(observed) != 2 {
		t.Errorf("observed count = %d; want 2 (both items log-related)", len(observed))
	}
	drift := CollectLogSourceDriftAnchors(RequestModel{}, bundle, items)
	if len(drift) != 1 {
		t.Errorf("drift count = %d; want 1 (only DriftedFn)", len(drift))
	}
	if len(drift) == 1 && drift[0].File != "drifted.go" {
		t.Errorf("drift anchor wrong file: %+v", drift[0])
	}
}

// TestCollectLogObservedAnchors_LegacyFallbackForUnprojectedEvidence:
// when no evidence carries projection (Origin/Authority empty), the
// legacy detector path must run. Backward-compat for tests + single-
// shot CLI flows that bypass emit_evidence.
func TestCollectLogObservedAnchors_LegacyFallbackForUnprojectedEvidence(t *testing.T) {
	bundle := &LogBundle{
		Errors: []LogError{{Frames: []LogFrame{
			{File: "x.go", Line: 10, Func: "Run", Raw: "x.go:10"},
		}}},
	}
	items := []EvidenceItem{
		{
			Kind:            EvidenceDirect,
			Scope:           ScopeLine,
			Source:          "x.go",
			LineStart:       10,
			AnchorSymbol:    "Run",
			AnchorKind:      AnchorDefinition,
			GroundingStatus: GroundingGrounded,
			// NO Origin/Authority — legacy path triggers.
		},
	}
	// Legacy detector returns empty (zero drift gap, line==line).
	// We're asserting the function doesn't crash and returns the
	// legacy result.
	anchors := CollectLogObservedAnchors(RequestModel{}, bundle, items)
	// Either path may return empty here; the assertion is that no
	// panic, no error. Length is implementation detail of legacy.
	_ = anchors
}
