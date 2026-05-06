package authority

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestComputeForEvidence_PerfStallDriftDetected: with PerfBundle
// stalls (no log) and a graph that places the symbol at a different
// line, ComputeForEvidence must classify the repo-backed item as
// cross_source + drift rather than pure perf-origin observation.
func TestComputeForEvidence_PerfStallDriftDetected(t *testing.T) {
	SetSymbolLocatorProvider(func(graph any) types.SymbolLocator {
		return &fakeLocator{
			defs: map[string][]types.SymbolLocation{
				"Render": {{File: "ui.go", Line: 200, EndLine: 250}},
			},
		}
	})
	defer SetSymbolLocatorProvider(nil)

	bus := &types.BusContext{Mutable: types.NewMutableState("test")}
	bus.Mutable.SetSearchGraph(struct{}{})
	bus.Mutable.SetPerfTrace(&types.PerfBundle{
		Stalls: []types.PerfStall{
			{Symbol: "Render", File: "ui.go", Line: 60},
		},
	})
	item := types.EvidenceItem{
		Scope:           types.ScopeLine,
		Source:          "ui.go",
		LineStart:       60,
		AnchorSymbol:    "Render",
		AnchorKind:      types.AnchorDefinition,
		GroundingStatus: types.GroundingGrounded,
	}
	p := ComputeForEvidence(item, bus)
	if p.Origin != types.ClaimOriginCrossSource {
		t.Errorf("Origin = %q; want cross_source (reason=%q)", p.Origin, p.Reason)
	}
	if p.Authority != types.AuthorityConditional {
		t.Errorf("Authority = %q; want conditional (line drift)", p.Authority)
	}
	if p.DriftReason != types.DriftReasonLineDrift {
		t.Errorf("DriftReason = %q; want line_drift", p.DriftReason)
	}
}

// TestComputeForEvidence_PerfStallEmptyFileSymbolAxis: when a perf
// stall has Symbol but NO File (unsymbolicated trace), the matcher
// finds the item via symbol-axis fallback (round-8). With no line
// to compare against AND graph confirming the symbol exists at the
// item's source, this is correctly classified as cross-source
// factual (no detectable drift). The TEST contract is that the
// matcher fires AT ALL (pre-fix it didn't because empty File
// caused early-skip).
func TestComputeForEvidence_PerfStallEmptyFileSymbolAxis(t *testing.T) {
	SetSymbolLocatorProvider(func(graph any) types.SymbolLocator {
		return &fakeLocator{
			defs: map[string][]types.SymbolLocation{
				"Render": {{File: "ui.go", Line: 50, EndLine: 80}},
			},
		}
	})
	defer SetSymbolLocatorProvider(nil)

	bus := &types.BusContext{Mutable: types.NewMutableState("test")}
	bus.Mutable.SetSearchGraph(struct{}{})
	bus.Mutable.SetPerfTrace(&types.PerfBundle{
		Stalls: []types.PerfStall{
			{Symbol: "Render"}, // no File / Line
		},
	})
	item := types.EvidenceItem{
		Scope:           types.ScopeLine,
		Source:          "ui.go",
		LineStart:       50,
		AnchorSymbol:    "Render",
		AnchorKind:      types.AnchorDefinition,
		GroundingStatus: types.GroundingGrounded,
	}
	p := ComputeForEvidence(item, bus)
	// Symbol-axis fallback fires → matched frame found → drift
	// detector sees no line conflict + graph confirms symbol →
	// DriftStatusNone → cross-source factual. The pre-fix path
	// would have returned current_repo + factual (no perf signal
	// at all). Now perf is acknowledged via cross-source.
	if p.Origin != types.ClaimOriginCrossSource {
		t.Errorf("Origin = %q; want cross_source (symbol-axis confirmed)", p.Origin)
	}
	if p.Authority != types.AuthorityFactual {
		t.Errorf("Authority = %q; want factual", p.Authority)
	}
}
