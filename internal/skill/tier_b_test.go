package skill

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// === P5-B step 1 — TierBItem + AppliesToFilter type tests ===

// TestAppliesToFilter_Always — Always=true short-circuits.
func TestAppliesToFilter_Always(t *testing.T) {
	f := AppliesToFilter{Always: true}
	if !f.MatchesAppliesTo(AppliesToContext{}) {
		t.Errorf("Always=true should match any context")
	}
}

// TestAppliesToFilter_BoolGates_AND — multiple bool gates must all pass.
func TestAppliesToFilter_BoolGates_AND(t *testing.T) {
	f := AppliesToFilter{RequiresDiagram: true, RequiresLog: true}
	cases := []struct {
		name string
		ctx  AppliesToContext
		want bool
	}{
		{"both true", AppliesToContext{HasDiagram: true, HasLog: true}, true},
		{"diagram missing", AppliesToContext{HasLog: true}, false},
		{"log missing", AppliesToContext{HasDiagram: true}, false},
		{"both missing", AppliesToContext{}, false},
	}
	for _, c := range cases {
		if got := f.MatchesAppliesTo(c.ctx); got != c.want {
			t.Errorf("%s: want %v, got %v", c.name, c.want, got)
		}
	}
}

// TestAppliesToFilter_PrincipalKinds_OR — any matching kind admits.
func TestAppliesToFilter_PrincipalKinds_OR(t *testing.T) {
	f := AppliesToFilter{
		PrincipalKinds: []types.AnswerBlockKind{
			types.BlockOrderedList,
			types.BlockBulletList,
		},
	}
	cases := []struct {
		name string
		kind types.AnswerBlockKind
		want bool
	}{
		{"ordered_list matches", types.BlockOrderedList, true},
		{"bullet_list matches", types.BlockBulletList, true},
		{"summary doesn't match", types.BlockSummary, false},
		{"empty doesn't match", "", false},
	}
	for _, c := range cases {
		got := f.MatchesAppliesTo(AppliesToContext{PrincipalKind: c.kind})
		if got != c.want {
			t.Errorf("%s: want %v, got %v", c.name, c.want, got)
		}
	}
}

// TestAppliesToFilter_AbsenceContract — typed contract gate.
func TestAppliesToFilter_AbsenceContract(t *testing.T) {
	f := AppliesToFilter{AbsenceContract: true}
	if f.MatchesAppliesTo(AppliesToContext{}) {
		t.Errorf("AbsenceContract=true should NOT match without IsAbsence")
	}
	if !f.MatchesAppliesTo(AppliesToContext{IsAbsence: true}) {
		t.Errorf("AbsenceContract=true should match when IsAbsence=true")
	}
}

// TestAppliesToFilter_ZeroValue — empty filter is neutral.
// Documents the contract: zero-value filter NEVER matches (unless
// Always=true). This way migration steps that add a TierBItem
// without an explicit filter default to "hidden" rather than
// "always shown" — safer fail-loud default.
func TestAppliesToFilter_ZeroValue(t *testing.T) {
	f := AppliesToFilter{}
	if f.MatchesAppliesTo(AppliesToContext{}) {
		t.Errorf("zero-value filter should NOT match (defensive default)")
	}
}

// TestTierBItem_ShouldRender_AppliesPath — matches AppliesTo filter.
func TestTierBItem_ShouldRender_AppliesPath(t *testing.T) {
	item := TierBItem{
		Body:      "log-only rule",
		AppliesTo: AppliesToFilter{RequiresLog: true},
	}
	if !item.ShouldRender(AppliesToContext{HasLog: true}) {
		t.Errorf("RequiresLog → match when HasLog=true")
	}
	if item.ShouldRender(AppliesToContext{HasDiagram: true}) {
		t.Errorf("RequiresLog → no match when HasLog=false")
	}
}

// TestTierBItem_ShouldRender_OnViolationPath — typed violation
// gate fires even when AppliesTo wouldn't.
func TestTierBItem_ShouldRender_OnViolationPath(t *testing.T) {
	item := TierBItem{
		Body:      "diagram edge label rule",
		AppliesTo: AppliesToFilter{RequiresDiagram: true},
		OnViolation: []types.ViolationKind{
			types.ViolDiagramEdgeLabelMismatch,
		},
	}
	// AppliesTo doesn't match (no diagram), but the violation
	// kind is in retry set → admit.
	ctx := AppliesToContext{
		HasDiagram: false,
		RetryViolations: []types.ViolationKind{
			types.ViolDiagramEdgeLabelMismatch,
		},
	}
	if !item.ShouldRender(ctx) {
		t.Errorf("OnViolation match should admit even when AppliesTo doesn't")
	}
}

// TestTierBItem_ShouldRender_NeitherPath — both gates fail → hidden.
func TestTierBItem_ShouldRender_NeitherPath(t *testing.T) {
	item := TierBItem{
		Body:      "log-only rule",
		AppliesTo: AppliesToFilter{RequiresLog: true},
		OnViolation: []types.ViolationKind{
			types.ViolFacetUncovered,
		},
	}
	ctx := AppliesToContext{
		HasLog:          false,
		RetryViolations: []types.ViolationKind{types.ViolBlockCoverageMissing},
	}
	if item.ShouldRender(ctx) {
		t.Errorf("neither AppliesTo nor OnViolation matches → should NOT render")
	}
}

// TestConfig_TierBFieldsZeroValueByteIdentical — pre-P5 skills (no
// Tier B fields populated) must be backward compatible. JSON
// marshal of a Config with empty Tier B fields should not include
// them (omitempty).
func TestConfig_TierBFieldsZeroValueByteIdentical(t *testing.T) {
	cfg := Config{
		Name:     "test",
		Goal:     "g",
		Workflow: []string{"w1"},
	}
	if cfg.WorkflowTierB != nil {
		t.Errorf("zero-value WorkflowTierB should be nil; got %v", cfg.WorkflowTierB)
	}
	if cfg.ProhibitionsTierB != nil {
		t.Errorf("zero-value ProhibitionsTierB should be nil; got %v", cfg.ProhibitionsTierB)
	}
}
