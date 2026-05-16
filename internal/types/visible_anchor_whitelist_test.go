package types

import (
	"strings"
	"testing"
)

// TestBuildVisibleAnchorWhitelist_NilSafe pins the T1 nil-safety
// contract. Pre-AnalysisIR paths, single-shot tests, and any caller
// without a populated support plan / semantic view must get an empty
// whitelist back so the downstream renderer can short-circuit
// without panicking.
func TestBuildVisibleAnchorWhitelist_NilSafe(t *testing.T) {
	if got := BuildVisibleAnchorWhitelist(nil, nil); !got.IsEmpty() {
		t.Errorf("nil/nil should yield empty whitelist; got %+v", got)
	}
	if got := BuildVisibleAnchorWhitelist(&AnswerSupportPlan{}, nil); !got.IsEmpty() {
		t.Errorf("empty plan / nil view should yield empty whitelist; got %+v", got)
	}
	if got := BuildVisibleAnchorWhitelist(nil, &AnswerSemanticView{}); !got.IsEmpty() {
		t.Errorf("nil plan / empty view should yield empty whitelist; got %+v", got)
	}
}

// TestBuildVisibleAnchorWhitelist_RequiredPreservesOrder pins user-
// mention sequence: AnswerRequiredAnchor[] order is sourced from
// mechanismAnchorMentionedSet which preserves the order the entities
// appeared in the raw request. The whitelist must keep that order.
func TestBuildVisibleAnchorWhitelist_RequiredPreservesOrder(t *testing.T) {
	view := &AnswerSemanticView{
		RequiredMechanismAnchors: []AnswerRequiredAnchor{
			{Text: "Orchestrator", Kind: ContractTermSymbol},
			{Text: "Gate.Run", Kind: ContractTermSymbol},
			{Text: "Ground", Kind: ContractTermSymbol},
		},
	}
	got := BuildVisibleAnchorWhitelist(nil, view)
	if len(got.Required) != 3 {
		t.Fatalf("expected 3 required entries; got %d (%+v)", len(got.Required), got.Required)
	}
	if got.Required[0].Symbol != "Orchestrator" ||
		got.Required[1].Symbol != "Gate.Run" ||
		got.Required[2].Symbol != "Ground" {
		t.Errorf("required order broken: %+v", got.Required)
	}
}

// TestBuildVisibleAnchorWhitelist_GroundablePopulatedFromSupportLanes
// pins the support-lane projection: AnchorSymbol / Subject / Object
// from each AnswerSupportEntry surface in the Groundable bucket,
// deduped by (Symbol, SourceFile, SourceLine).
func TestBuildVisibleAnchorWhitelist_GroundablePopulatedFromSupportLanes(t *testing.T) {
	plan := &AnswerSupportPlan{
		Lanes: []AnswerSupportLane{
			{
				Kind:  SupportLaneCurrentCodePath,
				Title: "Current code path",
				Entries: []AnswerSupportEntry{
					{
						AnchorSymbol:  "runContractCheck",
						Subject:       "Orchestrator",
						Source:        "internal/orchestrator/contract_check.go",
						LineStart:     115,
						GroundingTier: TierLineText,
						AnchorKind:    AnchorDefinition,
						SurfaceTerms:  []string{"runContractCheck", "run_contract_check"},
					},
				},
			},
		},
	}
	got := BuildVisibleAnchorWhitelist(plan, nil)
	if len(got.Groundable) < 2 {
		t.Fatalf("expected ≥2 groundable entries (anchor + subject); got %d (%+v)", len(got.Groundable), got.Groundable)
	}
	foundAnchor, foundSubject := false, false
	for _, e := range got.Groundable {
		if e.Symbol == "runContractCheck" {
			foundAnchor = true
			if e.SourceFile != "internal/orchestrator/contract_check.go" || e.SourceLine != 115 {
				t.Errorf("anchor entry source mismatch: %+v", e)
			}
		}
		if e.Symbol == "Orchestrator" {
			foundSubject = true
		}
	}
	if !foundAnchor || !foundSubject {
		t.Errorf("expected both anchor and subject in groundable; got %+v", got.Groundable)
	}
}

// TestBuildVisibleAnchorWhitelist_RequiredWinsOverGroundable: a
// symbol present in BOTH RequiredMechanismAnchors AND a support-lane
// entry lands in Required only. The Groundable bucket is the strict
// complement so the prompt-side rendering does not double-list the
// same identifier.
func TestBuildVisibleAnchorWhitelist_RequiredWinsOverGroundable(t *testing.T) {
	view := &AnswerSemanticView{
		RequiredMechanismAnchors: []AnswerRequiredAnchor{
			{Text: "Orchestrator", Kind: ContractTermSymbol},
		},
	}
	plan := &AnswerSupportPlan{
		Lanes: []AnswerSupportLane{
			{
				Kind: SupportLaneCurrentCodePath,
				Entries: []AnswerSupportEntry{
					{
						AnchorSymbol:  "Orchestrator",
						GroundingTier: TierLineText,
					},
				},
			},
		},
	}
	got := BuildVisibleAnchorWhitelist(plan, view)
	if len(got.Required) != 1 || got.Required[0].Symbol != "Orchestrator" {
		t.Fatalf("Required must hold Orchestrator: %+v", got.Required)
	}
	for _, e := range got.Groundable {
		if e.Symbol == "Orchestrator" {
			t.Errorf("Orchestrator must not appear in Groundable when also Required: %+v", got.Groundable)
		}
	}
}

// TestBuildVisibleAnchorWhitelist_GroundableTierOrder pins the
// confidence-first sort: high-confidence tiers (line_text /
// symbol_table) precede recovery tiers (snippet_fuzzy /
// package_symbol). Same-tier entries sort alphabetically by Symbol.
func TestBuildVisibleAnchorWhitelist_GroundableTierOrder(t *testing.T) {
	plan := &AnswerSupportPlan{
		Lanes: []AnswerSupportLane{
			{
				Entries: []AnswerSupportEntry{
					{AnchorSymbol: "ZebraRecovered", GroundingTier: TierSnippetFuzzy, Source: "a.go", LineStart: 1},
					{AnchorSymbol: "AlphaSolid", GroundingTier: TierLineText, Source: "b.go", LineStart: 2},
					{AnchorSymbol: "BetaSolid", GroundingTier: TierLineText, Source: "c.go", LineStart: 3},
				},
			},
		},
	}
	got := BuildVisibleAnchorWhitelist(plan, nil)
	if len(got.Groundable) < 3 {
		t.Fatalf("expected ≥3 entries; got %d", len(got.Groundable))
	}
	if got.Groundable[0].Symbol != "AlphaSolid" ||
		got.Groundable[1].Symbol != "BetaSolid" {
		t.Errorf("expected tier-1 entries first alphabetically; got %+v", got.Groundable[:2])
	}
	if got.Groundable[2].Symbol != "ZebraRecovered" {
		t.Errorf("expected recovered tier after high-confidence; got %+v", got.Groundable)
	}
}

// TestBuildVisibleAnchorWhitelist_CapAtMaximum pins the size bound
// — a support plan with 200 entries must yield at most
// MaxVisibleAnchorWhitelistEntries in the rendered slate. Required
// entries are counted against the cap first, so an over-large
// Required slice will leave the Groundable bucket empty rather than
// silently dropping Required entries.
func TestBuildVisibleAnchorWhitelist_CapAtMaximum(t *testing.T) {
	entries := make([]AnswerSupportEntry, 0, 200)
	for i := 0; i < 200; i++ {
		entries = append(entries, AnswerSupportEntry{
			AnchorSymbol:  "Sym" + intToString(i),
			GroundingTier: TierLineText,
			Source:        "a.go",
			LineStart:     i + 1,
		})
	}
	plan := &AnswerSupportPlan{
		Lanes: []AnswerSupportLane{{Entries: entries}},
	}
	got := BuildVisibleAnchorWhitelist(plan, nil)
	total := len(got.Required) + len(got.Groundable)
	if total > MaxVisibleAnchorWhitelistEntries {
		t.Errorf("total entries %d exceed cap %d", total, MaxVisibleAnchorWhitelistEntries)
	}
}

// TestBuildVisibleAnchorWhitelist_QualifiedSymbolSplit ensures
// "Type.Method" surface text splits into Symbol="Method" and
// QualifiedSymbol="Type.Method", matching what the post-emit oracle's
// flat-form index resolves against.
func TestBuildVisibleAnchorWhitelist_QualifiedSymbolSplit(t *testing.T) {
	plan := &AnswerSupportPlan{
		Lanes: []AnswerSupportLane{
			{
				Entries: []AnswerSupportEntry{
					{AnchorSymbol: "Orchestrator.runContractCheck", GroundingTier: TierLineText},
				},
			},
		},
	}
	got := BuildVisibleAnchorWhitelist(plan, nil)
	if len(got.Groundable) < 1 {
		t.Fatalf("no groundable entry produced")
	}
	e := got.Groundable[0]
	if e.Symbol != "runContractCheck" {
		t.Errorf("bare Symbol should drop the qualifier; got %q", e.Symbol)
	}
	if e.QualifiedSymbol != "Orchestrator.runContractCheck" {
		t.Errorf("QualifiedSymbol should retain the full form; got %q", e.QualifiedSymbol)
	}
}

// TestBuildVisibleAnchorWhitelist_SurfaceFormsFilteredToIdentifiers
// — prose phrases in SurfaceTerms are dropped (only identifier-shaped
// forms reach the SymbolOracle's flat index, so listing prose in the
// prompt would mis-direct the model).
func TestBuildVisibleAnchorWhitelist_SurfaceFormsFilteredToIdentifiers(t *testing.T) {
	plan := &AnswerSupportPlan{
		Lanes: []AnswerSupportLane{
			{
				Entries: []AnswerSupportEntry{
					{
						AnchorSymbol:  "runContractCheck",
						GroundingTier: TierLineText,
						SurfaceTerms: []string{
							"runContractCheck",      // identifier — keep
							"run_contract_check",    // identifier — keep
							"runs the contract",     // prose — drop
							"contract check oracle", // prose — drop
						},
					},
				},
			},
		},
	}
	got := BuildVisibleAnchorWhitelist(plan, nil)
	if len(got.Groundable) < 1 {
		t.Fatalf("no groundable entry produced")
	}
	forms := got.Groundable[0].SurfaceForms
	for _, sf := range forms {
		if strings.Contains(sf, " ") {
			t.Errorf("prose surface leaked into SurfaceForms: %q", sf)
		}
	}
	hasUnderscored := false
	for _, sf := range forms {
		if sf == "run_contract_check" {
			hasUnderscored = true
		}
	}
	if !hasUnderscored {
		t.Errorf("identifier-shaped alternative spelling dropped; got %+v", forms)
	}
}

// TestVisibleAnchorEntry_IsHighConfidenceTier_TierBoundary verifies
// the boundary between "safe to cite" (rank 0-1) and "cite with
// caveat" (rank 2+) tracks the documented tier vocabulary.
func TestVisibleAnchorEntry_IsHighConfidenceTier_TierBoundary(t *testing.T) {
	cases := []struct {
		tier GroundingTier
		want bool
	}{
		{TierLineText, true},
		{TierFileLayer, true},
		{TierLineRange, true},
		{TierSymbolTable, true},
		{TierFQNameSameFile, true},
		{TierCrossfileVerified, true},
		{TierSnippetFuzzy, false},
		{TierPackageSymbol, false},
		{TierNearestCall, false},
		{TierNearestCondition, false},
		{GroundingTier(""), false},
	}
	for _, c := range cases {
		entry := VisibleAnchorEntry{GroundingTier: c.tier}
		if got := entry.IsHighConfidenceTier(); got != c.want {
			t.Errorf("tier %q: high-confidence=%v want %v", c.tier, got, c.want)
		}
	}
}
