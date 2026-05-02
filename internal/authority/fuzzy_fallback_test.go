package authority

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestComputeForEvidence_PinsDriftReason: every drift status returned
// by the projection must pin a typed DriftReason on the evidence so
// downstream derivers (P2 will use this) can rebuild the
// LogSourceDriftAnchor list without re-running detection.
func TestComputeForEvidence_PinsDriftReason(t *testing.T) {
	cases := []struct {
		name      string
		setup     func(bus *types.BusContext)
		locator   types.SymbolLocator
		item      types.EvidenceItem
		wantDrift types.DriftReason
	}{
		{
			name: "line drift -> line_drift",
			setup: func(bus *types.BusContext) {
				bus.Mutable.SetSearchGraph(struct{}{})
				bus.Mutable.SetLogTriage(&types.LogBundle{
					Errors: []types.LogError{{Frames: []types.LogFrame{
						{File: "x.go", Line: 50, Func: "Run", Raw: "x.go:50 Run"},
					}}},
				})
			},
			locator: &fakeLocator{
				defs: map[string][]types.SymbolLocation{
					"Run": {{File: "x.go", Line: 100, EndLine: 110}},
				},
			},
			item: types.EvidenceItem{
				Scope:           types.ScopeLine,
				Source:          "x.go",
				LineStart:       50,
				AnchorSymbol:    "Run",
				AnchorKind:      types.AnchorDefinition,
				GroundingStatus: types.GroundingGrounded,
			},
			wantDrift: types.DriftReasonLineDrift,
		},
		{
			name: "file moved -> file_moved",
			setup: func(bus *types.BusContext) {
				bus.Mutable.SetSearchGraph(struct{}{})
				bus.Mutable.SetLogTriage(&types.LogBundle{
					Errors: []types.LogError{{Frames: []types.LogFrame{
						{File: "old/x.go", Line: 30, Func: "Run", Raw: "old/x.go:30 Run"},
					}}},
				})
			},
			locator: &fakeLocator{
				defs: map[string][]types.SymbolLocation{
					"Run": {{File: "new/y.go", Line: 200, EndLine: 250}},
				},
			},
			item: types.EvidenceItem{
				Scope:           types.ScopeLine,
				Source:          "old/x.go",
				LineStart:       30,
				AnchorSymbol:    "Run",
				AnchorKind:      types.AnchorDefinition,
				GroundingStatus: types.GroundingGrounded,
			},
			wantDrift: types.DriftReasonFileMoved,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			SetSymbolLocatorProvider(func(graph any) types.SymbolLocator { return tc.locator })
			defer SetSymbolLocatorProvider(nil)
			bus := &types.BusContext{Mutable: types.NewMutableState("test")}
			tc.setup(bus)
			p := ComputeForEvidence(tc.item, bus)
			if p.DriftReason != tc.wantDrift {
				t.Errorf("DriftReason = %q; want %q", p.DriftReason, tc.wantDrift)
			}
		})
	}
}

// TestFuzzyTailRenameFallback_RecoversRename: graph lookup fails for
// the old func name (renamed since log was captured), but the
// evidence pool has a same-file definition anchor with a similar
// name → fallback upgrades to TailRename.
func TestFuzzyTailRenameFallback_RecoversRename(t *testing.T) {
	frame := types.LogFrame{File: "x.go", Line: 10, Func: "oldHandler"}
	evidence := []types.EvidenceItem{
		{
			Source:          "x.go",
			LineStart:       12,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "newHandler",
			GroundingStatus: types.GroundingGrounded,
		},
	}
	got, ok := fuzzyTailRenameFallback(frame, evidence)
	if !ok {
		t.Fatalf("fuzzy fallback did not fire for similar tail")
	}
	if got.Status != types.DriftStatusTailRename {
		t.Errorf("Status = %q; want TailRename", got.Status)
	}
	if got.AnchoredFile != "x.go" || got.AnchoredLine != 12 {
		t.Errorf("AnchoredFile/Line = %q:%d; want x.go:12", got.AnchoredFile, got.AnchoredLine)
	}
}

// TestFuzzyTailRenameFallback_RejectsUnrelatedNames: when names
// share no prefix/suffix/substring, fallback declines so we don't
// invent a rename. Conservative on purpose.
func TestFuzzyTailRenameFallback_RejectsUnrelatedNames(t *testing.T) {
	frame := types.LogFrame{File: "x.go", Line: 10, Func: "oldHandler"}
	evidence := []types.EvidenceItem{
		{
			Source:          "x.go",
			LineStart:       12,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "Foo",
			GroundingStatus: types.GroundingGrounded,
		},
	}
	if _, ok := fuzzyTailRenameFallback(frame, evidence); ok {
		t.Errorf("fuzzy fallback should NOT have fired for unrelated names")
	}
}
