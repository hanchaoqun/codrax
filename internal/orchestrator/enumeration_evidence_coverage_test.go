package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// 修 B (post_v2_runtime_gap_remediation, 2026-05-04). Unit tests for
// validateEnumerationEvidenceCoverage. The validator fires only on
// (a) explicit DeclaredCount, (b) enumeration-shaped family,
// (c) len(unique anchor_symbol) < DeclaredCount.

func mutWithEvidenceAnchors(anchors ...string) *types.MutableState {
	mut := &types.MutableState{}
	items := make([]types.EvidenceItem, 0, len(anchors))
	for _, a := range anchors {
		items = append(items, types.EvidenceItem{
			Kind: types.EvidenceDirect, Source: "x.go", LineStart: 1, LineEnd: 1,
			AnchorSymbol: a,
		})
	}
	mut.AppendEvidence(items)
	return mut
}

func setRequestModelWithBoundary(mut *types.MutableState, n int, intent types.Intent) {
	rm := types.RequestModel{Intent: intent}
	if n > 0 {
		rm.EnumerationBoundary = &types.RequestedEnumerationBoundary{DeclaredCount: n}
	}
	mut.SetRequestModel(rm)
}

// Trigger condition 1: declared count vs unique anchor count.
func TestValidateEnumerationEvidenceCoverage_FiresWhenAnchorCountBelowDeclared(t *testing.T) {
	mut := mutWithEvidenceAnchors("checks", "checkPendingFieldsWellformed") // 2 unique
	setRequestModelWithBoundary(mut, 9, types.IntentEnumerate)
	view := &types.AnswerSemanticView{Family: types.QFEnumeration}

	vs := validateEnumerationEvidenceCoverage(mut, view)
	if len(vs) != 1 {
		t.Fatalf("want 1 violation; got %d (%+v)", len(vs), vs)
	}
	if vs[0].Kind != types.ViolEnumerationEvidenceUnderspecified {
		t.Errorf("kind = %q, want ViolEnumerationEvidenceUnderspecified", vs[0].Kind)
	}
	if got := vs[0].ClusterKey; got != "family:enumeration|root:evidence_pool_named_anchors" {
		t.Errorf("ClusterKey = %q, want family-based typed cluster key", got)
	}
	if !strings.Contains(vs[0].Detail, "9") || !strings.Contains(vs[0].Detail, "2") {
		t.Errorf("Detail must name declared (9) and actual (2) counts: %q", vs[0].Detail)
	}
}

// Trigger 2: when enough unique anchors, gate skips.
func TestValidateEnumerationEvidenceCoverage_SkipsWhenAnchorCountAtOrAboveDeclared(t *testing.T) {
	mut := mutWithEvidenceAnchors("a", "b", "c", "d", "e", "f", "g", "h", "i") // 9 unique
	setRequestModelWithBoundary(mut, 9, types.IntentEnumerate)
	view := &types.AnswerSemanticView{Family: types.QFEnumeration}

	if vs := validateEnumerationEvidenceCoverage(mut, view); len(vs) != 0 {
		t.Errorf("9 anchors >= 9 declared: must skip; got %+v", vs)
	}
}

func TestValidateEnumerationEvidenceCoverage_SkipsWithAggregateMemberSetSourceLocations(t *testing.T) {
	mut := mutWithEvidenceAnchors("CitationReq.Required")
	setRequestModelWithBoundary(mut, 4, types.IntentEnumerate)
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "verified source locations",
		Value: "4",
		Members: []string{
			"internal/agent/analyzer.go:1927",
			"internal/orchestrator/contract_check.go:63",
			"internal/orchestrator/orchestrator.go:6397",
			"internal/orchestrator/orchestrator.go:6529",
		},
	}})
	mut.SetInvestigationComplete("resolved through aggregate_facts.member_set")
	view := &types.AnswerSemanticView{Family: types.QFEnumeration}

	if vs := validateEnumerationEvidenceCoverage(mut, view); len(vs) != 0 {
		t.Fatalf("source-location member_set is the principal anchor set; got %+v", vs)
	}
}

func TestValidateEnumerationEvidenceCoverage_FiresWhenAggregateMemberSetBelowDeclared(t *testing.T) {
	mut := mutWithEvidenceAnchors("CitationReq.Required")
	setRequestModelWithBoundary(mut, 4, types.IntentEnumerate)
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "partial source locations",
		Value: "2",
		Members: []string{
			"internal/agent/analyzer.go:1927",
			"internal/orchestrator/contract_check.go:63",
		},
	}})
	mut.SetInvestigationComplete("partial set")
	view := &types.AnswerSemanticView{Family: types.QFEnumeration}

	if vs := validateEnumerationEvidenceCoverage(mut, view); len(vs) == 0 {
		t.Fatal("partial aggregate member_set must not mask an evidence coverage gap")
	}
}

// Trigger 3: no DeclaredCount → skip (修 B is declared-count-only;
// implicit completeness is by design out of scope to preserve R3).
func TestValidateEnumerationEvidenceCoverage_SkipsWhenNoDeclaredCount(t *testing.T) {
	mut := mutWithEvidenceAnchors("checks") // 1 anchor
	setRequestModelWithBoundary(mut, 0, types.IntentEnumerate)
	view := &types.AnswerSemanticView{Family: types.QFEnumeration}

	if vs := validateEnumerationEvidenceCoverage(mut, view); len(vs) != 0 {
		t.Errorf("no DeclaredCount: must skip (R3 — no fuzzy hard gate); got %+v", vs)
	}
}

// Trigger 4: family must be enumeration-shaped.
func TestValidateEnumerationEvidenceCoverage_SkipsOnNonEnumerationFamily(t *testing.T) {
	mut := mutWithEvidenceAnchors("only-one") // 1 anchor
	setRequestModelWithBoundary(mut, 9, types.IntentExplain)
	view := &types.AnswerSemanticView{Family: types.QFGeneric}

	if vs := validateEnumerationEvidenceCoverage(mut, view); len(vs) != 0 {
		t.Errorf("non-enumeration family: must skip; got %+v", vs)
	}
}

// Family eligibility: QFRootCauseTrace + QFCallChain also fire when
// declared count is set. These two families ship ordered_list slates
// where each item should carry a verbatim identifier.
func TestValidateEnumerationEvidenceCoverage_FamiliesEligible(t *testing.T) {
	for _, fam := range []types.QuestionFamily{
		types.QFEnumeration, types.QFRootCauseTrace, types.QFCallChain,
	} {
		t.Run(string(fam), func(t *testing.T) {
			mut := mutWithEvidenceAnchors("only-one")
			setRequestModelWithBoundary(mut, 5, types.IntentEnumerate)
			view := &types.AnswerSemanticView{Family: fam}
			if vs := validateEnumerationEvidenceCoverage(mut, view); len(vs) == 0 {
				t.Errorf("family %s must fire when DeclaredCount=5 > unique=1", fam)
			}
		})
	}
}

// Permanently-soft severity until isStrict promotes to Medium.
func TestViolEnumerationEvidenceUnderspecified_DefaultSoft(t *testing.T) {
	if got := types.DeriveSeverity(types.ViolEnumerationEvidenceUnderspecified, false); got != types.SeveritySoft {
		t.Errorf("default Severity = %v, want SeveritySoft", got)
	}
	if got := types.DeriveSeverity(types.ViolEnumerationEvidenceUnderspecified, true); got != types.SeverityMedium {
		t.Errorf("isStrict Severity = %v, want SeverityMedium (operator-promotable)", got)
	}
}

// Repair text must surface explicit guidance + count gap (LLM-facing).
func TestValidateEnumerationEvidenceCoverage_RepairNamesGap(t *testing.T) {
	mut := mutWithEvidenceAnchors("solo")
	setRequestModelWithBoundary(mut, 7, types.IntentEnumerate)
	view := &types.AnswerSemanticView{Family: types.QFEnumeration}

	vs := validateEnumerationEvidenceCoverage(mut, view)
	if len(vs) != 1 {
		t.Fatalf("want 1 violation; got %+v", vs)
	}
	repair := vs[0].Repair
	if !strings.Contains(repair, "scope=line") {
		t.Errorf("Repair must guide explorer to use scope=line per identifier; got %q", repair)
	}
	if !strings.Contains(repair, "anchor_symbol") {
		t.Errorf("Repair must reference anchor_symbol contract; got %q", repair)
	}
	if !strings.Contains(repair, "7") {
		t.Errorf("Repair must surface the declared count; got %q", repair)
	}
}

// R4 lock: Detail/Repair text MUST NOT leak Go-internal type names.
func TestValidateEnumerationEvidenceCoverage_NoLLMFacingJargon(t *testing.T) {
	mut := mutWithEvidenceAnchors("a", "b") // 2 anchors
	setRequestModelWithBoundary(mut, 9, types.IntentEnumerate)
	view := &types.AnswerSemanticView{Family: types.QFEnumeration}

	vs := validateEnumerationEvidenceCoverage(mut, view)
	if len(vs) != 1 {
		t.Fatalf("want 1 violation; got %+v", vs)
	}
	corpus := vs[0].Detail + " " + vs[0].Repair
	for _, banned := range []string{
		"AnalysisIR", "EnumerationBoundary", "RequestedEnumerationBoundary",
		"EvidenceItem", "AnswerSemanticView", "QFEnumeration", "QFRootCauseTrace",
		"FacetCoverageContract", "ViolationKind",
		"finalizer", "extractor", "explorer", "analyzer",
		"Phase ", "Layer ", "Tier-",
	} {
		if strings.Contains(corpus, banned) {
			t.Errorf("LLM-facing text contains banned token %q: %q", banned, corpus)
		}
	}
}

// Nil guards: validator must not panic on missing receivers.
func TestValidateEnumerationEvidenceCoverage_NilGuards(t *testing.T) {
	if vs := validateEnumerationEvidenceCoverage(nil, &types.AnswerSemanticView{Family: types.QFEnumeration}); vs != nil {
		t.Errorf("nil mut: must return nil; got %+v", vs)
	}
	mut := &types.MutableState{}
	if vs := validateEnumerationEvidenceCoverage(mut, nil); vs != nil {
		t.Errorf("nil view: must return nil; got %+v", vs)
	}
	if vs := validateEnumerationEvidenceCoverage(mut, &types.AnswerSemanticView{}); vs != nil {
		t.Errorf("empty mut + view: must return nil; got %+v", vs)
	}
}
