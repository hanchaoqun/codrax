package types

import (
	"testing"
)

// EVALFIX-2E (CLASS 5) — typed degradation ledger unit pins.

func TestDegradationLedgerAppendAggregatesPerLane(t *testing.T) {
	m := NewMutableState("q")
	m.AppendDegradation(DegradeLaneCitationQuoteRewrite, 3)
	m.AppendDegradation(DegradeLaneCitationQuoteRewrite, 14)
	m.AppendDegradation(DegradeLaneCompletenessDowngraded, 1)
	got := m.DegradationLedger()
	want := []DegradationEntry{
		{Lane: DegradeLaneCitationQuoteRewrite, Count: 17},
		{Lane: DegradeLaneCompletenessDowngraded, Count: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("ledger entries = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ledger[%d] = %+v, want %+v (registry declaration order + per-lane aggregation)", i, got[i], want[i])
		}
	}
}

func TestDegradationLedgerNonPositiveAndEmptyLaneNoOp(t *testing.T) {
	m := NewMutableState("q")
	m.AppendDegradation(DegradeLaneCitationQuoteRewrite, 0)
	m.AppendDegradation(DegradeLaneCitationQuoteRewrite, -5)
	m.AppendDegradation(DegradationLaneID("  "), 3)
	if got := m.DegradationLedger(); got != nil {
		t.Fatalf("non-positive / empty-lane appends must be no-ops, got %+v", got)
	}
	// nil receiver is safe too (fail-open discipline).
	var nilState *MutableState
	nilState.AppendDegradation(DegradeLaneCitationQuoteRewrite, 1)
	if got := nilState.DegradationLedger(); got != nil {
		t.Fatalf("nil MutableState must be inert, got %+v", got)
	}
	if got := BuildDegradationLedgerView(nil); got != nil {
		t.Fatalf("BuildDegradationLedgerView(nil) must be nil, got %+v", got)
	}
}

func TestDegradationLedgerResetClears(t *testing.T) {
	m := NewMutableState("q")
	m.AppendDegradation(DegradeLaneCitationQuoteRewrite, 17)
	if got := m.DegradationLedger(); len(got) != 1 {
		t.Fatalf("precondition: 1 entry expected, got %+v", got)
	}
	m.ResetDegradationLedger()
	if got := m.DegradationLedger(); got != nil {
		t.Fatalf("ResetDegradationLedger must clear the account, got %+v", got)
	}
	var nilState *MutableState
	nilState.ResetDegradationLedger() // must not panic
}

// TestBuildDegradationLedgerViewProjectsRichnessFacetSoftened pins the
// A1 one-way projection arm: facet_softened signals on the EXISTING
// richness-telemetry channel surface as DegradeLaneRichnessFacetSoftened
// counts, with zero writer changes on the channel; other kinds
// (family_underrepresented) must NOT project.
func TestBuildDegradationLedgerViewProjectsRichnessFacetSoftened(t *testing.T) {
	m := NewMutableState("q")
	m.AppendRichnessTelemetry(RichnessTelemetrySignal{Kind: "facet_softened", FacetID: "f1", Reason: "r1"})
	m.AppendRichnessTelemetry(RichnessTelemetrySignal{Kind: "facet_softened", FacetID: "f2", Reason: "r2"})
	m.AppendRichnessTelemetry(RichnessTelemetrySignal{Kind: "family_underrepresented", Family: "qf_generic", Reason: "r3"})
	got := BuildDegradationLedgerView(m)
	if len(got) != 1 || got[0].Lane != DegradeLaneRichnessFacetSoftened || got[0].Count != 2 {
		t.Fatalf("view = %+v, want exactly [{%s 2}] (facet_softened projects, family_underrepresented must not)",
			got, DegradeLaneRichnessFacetSoftened)
	}
	// Negative arm: a channel with ONLY the non-projected kind yields an
	// empty view (0 entries → the footer emits 0 bytes).
	m2 := NewMutableState("q")
	m2.AppendRichnessTelemetry(RichnessTelemetrySignal{Kind: "family_underrepresented", Family: "qf_generic", Reason: "r"})
	if got := BuildDegradationLedgerView(m2); got != nil {
		t.Fatalf("family_underrepresented alone must not mint a ledger entry, got %+v", got)
	}
}

// TestBuildDegradationLedgerViewProjectsCompletenessDowngraded pins the
// A3 projection arm over the analyzer-decision channel; every other
// decision kind stays off the ledger.
func TestBuildDegradationLedgerViewProjectsCompletenessDowngraded(t *testing.T) {
	m := NewMutableState("q")
	m.AppendAnalyzerDecision(AnalyzerDecisionSignal{Kind: "completeness_downgraded", Stage: "extract", Reason: "floor"})
	m.AppendAnalyzerDecision(AnalyzerDecisionSignal{Kind: "scenario_reconciled", Reason: "flip"})
	m.AppendAnalyzerDecision(AnalyzerDecisionSignal{Kind: "prescan_rejected", Reason: "budget"})
	got := BuildDegradationLedgerView(m)
	if len(got) != 1 || got[0].Lane != DegradeLaneCompletenessDowngraded || got[0].Count != 1 {
		t.Fatalf("view = %+v, want exactly [{%s 1}]", got, DegradeLaneCompletenessDowngraded)
	}
	m2 := NewMutableState("q")
	m2.AppendAnalyzerDecision(AnalyzerDecisionSignal{Kind: "scenario_reconciled", Reason: "flip"})
	if got := BuildDegradationLedgerView(m2); got != nil {
		t.Fatalf("non-projected decision kinds must not mint a ledger entry, got %+v", got)
	}
}

// TestBuildDegradationLedgerViewMergesDirectAndProjected pins the merged
// view + stable registry-declaration ordering regardless of append order
// (citation lane declared first must render first even when appended
// last), with unregistered lanes trailing lexically.
func TestBuildDegradationLedgerViewMergesDirectAndProjected(t *testing.T) {
	m := NewMutableState("q")
	m.AppendDegradation(DegradationLaneID("zz_unknown"), 2)
	m.AppendAnalyzerDecision(AnalyzerDecisionSignal{Kind: "completeness_downgraded", Reason: "floor"})
	m.AppendRichnessTelemetry(RichnessTelemetrySignal{Kind: "facet_softened", FacetID: "f", Reason: "r"})
	m.AppendDegradation(DegradeLaneCitationQuoteRewrite, 17)
	got := BuildDegradationLedgerView(m)
	want := []DegradationEntry{
		{Lane: DegradeLaneCitationQuoteRewrite, Count: 17},
		{Lane: DegradeLaneRichnessFacetSoftened, Count: 1},
		{Lane: DegradeLaneCompletenessDowngraded, Count: 1},
		{Lane: DegradationLaneID("zz_unknown"), Count: 2},
	}
	if len(got) != len(want) {
		t.Fatalf("view = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("view[%d] = %+v, want %+v (registry declaration order, unregistered last)", i, got[i], want[i])
		}
	}
}

// TestDegradationLaneRegistryClosedSetTripwire is the LEDGER-TRIPWIRE
// style closed-set invariant (equality, not count): every registered
// lane carries a valid Class and non-empty bilingual words, and the
// registry map key set equals the declaration-order slice in BOTH
// directions — a lane cannot exist in one without the other.
func TestDegradationLaneRegistryClosedSetTripwire(t *testing.T) {
	validClass := map[DegradationClass]bool{ClassAnswerSemantics: true, ClassPlumbing: true}
	for lane, spec := range DegradationLaneRegistry {
		if !validClass[spec.Class] {
			t.Errorf("lane %q has invalid Class %q — registry is a closed two-class set", lane, spec.Class)
		}
		if spec.ZH == "" || spec.EN == "" {
			t.Errorf("lane %q must carry non-empty ZH+EN display words (ZH=%q EN=%q) — even plumbing lanes, so a future reclassification never ships an empty label", lane, spec.ZH, spec.EN)
		}
		if lane == "" {
			t.Errorf("empty lane id registered")
		}
	}
	inOrder := map[DegradationLaneID]bool{}
	for _, lane := range DegradationLaneRegistryOrder {
		if inOrder[lane] {
			t.Errorf("lane %q duplicated in DegradationLaneRegistryOrder", lane)
		}
		inOrder[lane] = true
		if _, ok := DegradationLaneRegistry[lane]; !ok {
			t.Errorf("lane %q in order slice but missing from registry", lane)
		}
	}
	for lane := range DegradationLaneRegistry {
		if !inOrder[lane] {
			t.Errorf("lane %q in registry but missing from DegradationLaneRegistryOrder — declaration order would be undefined", lane)
		}
	}
}
