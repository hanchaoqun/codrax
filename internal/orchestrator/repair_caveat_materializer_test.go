package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestMaterializeCaveats_GroupsByFamily — multiple violations in the
// same CaveatFamily collapse to one user-visible caveat. Mirrors the
// qfa-mr3 forensic case: 3 violations, 2 share answer_coverage, 1 is
// diagram_fidelity → 2 caveats output.
func TestMaterializeCaveats_GroupsByFamily(t *testing.T) {
	violations := []types.Violation{
		{Kind: types.ViolRichnessRegression},
		{Kind: types.ViolFacetUncovered},
		{Kind: types.ViolDiagramEdgeUnsupported},
	}
	caveats := MaterializeUnresolvedViolationsAsCaveats(violations, "zh")
	if len(caveats) != 2 {
		t.Fatalf("expected 2 caveats (1 per family), got %d: %v", len(caveats), caveats)
	}
}

// TestMaterializeCaveats_NoInternalJargon — output strings must not
// contain ViolKind names, IR field names, confidence numbers, or
// orchestration tokens.
func TestMaterializeCaveats_NoInternalJargon(t *testing.T) {
	violations := []types.Violation{
		{Kind: types.ViolRichnessRegression},
		{Kind: types.ViolDiagramEdgeUnsupported},
		{Kind: types.ViolEnumerationLabelUngrounded},
		{Kind: types.ViolAuthorityOverreach},
	}
	for _, lang := range []string{"zh", "en"} {
		caveats := MaterializeUnresolvedViolationsAsCaveats(violations, lang)
		for _, c := range caveats {
			for _, token := range []string{
				"yield kill", "Pipeline terminated", "retryUsed",
				"ViolKind", "IRField", "conf=", "event(s)",
				"block_items_label", "diagram_edges", "facet_uncovered",
				"answer_authority", "answer_richness_facet_coverage",
			} {
				if strings.Contains(c, token) {
					t.Errorf("[%s] caveat contains forbidden token %q: %q", lang, token, c)
				}
			}
		}
	}
}

// TestMaterializeCaveats_SkipsOperatorOnly — violations whose
// ViolKind is reviewer / frequency-bridge produce NO caveat.
func TestMaterializeCaveats_SkipsOperatorOnly(t *testing.T) {
	violations := []types.Violation{
		{Kind: types.ViolPlanCritic},
		{Kind: types.ViolReflectorObservation},
		{Kind: types.ViolDemotionStorm},
	}
	caveats := MaterializeUnresolvedViolationsAsCaveats(violations, "zh")
	if len(caveats) != 0 {
		t.Errorf("operator-only violations must produce zero caveats; got %d: %v", len(caveats), caveats)
	}
}

// TestMaterializeCaveats_LangFallback — empty/unknown lang defaults
// to ZH (project default). Explicit "en" returns English template.
func TestMaterializeCaveats_LangFallback(t *testing.T) {
	violations := []types.Violation{{Kind: types.ViolRichnessRegression}}
	zh := MaterializeUnresolvedViolationsAsCaveats(violations, "")
	en := MaterializeUnresolvedViolationsAsCaveats(violations, "en")
	if len(zh) != 1 || len(en) != 1 {
		t.Fatalf("expected 1 caveat each; got zh=%d en=%d", len(zh), len(en))
	}
	if zh[0] == en[0] {
		t.Errorf("ZH and EN templates rendered identically — language switch broken")
	}
	if !strings.ContainsAny(zh[0], "答案在某些维度") {
		t.Errorf("ZH caveat does not look Chinese: %q", zh[0])
	}
	if strings.ContainsAny(en[0], "答案") {
		t.Errorf("EN caveat unexpectedly contains Chinese: %q", en[0])
	}
}

// TestMaterializeCaveats_CapAt3 — output capped at MaxMaterializedCaveats
// to avoid drowning the user. Test fires 5 distinct families.
func TestMaterializeCaveats_CapAt3(t *testing.T) {
	violations := []types.Violation{
		{Kind: types.ViolRichnessRegression},        // answer_coverage
		{Kind: types.ViolDiagramEdgeUnsupported},    // diagram_fidelity
		{Kind: types.ViolEnumerationLabelUngrounded}, // enumeration_depth
		{Kind: types.ViolGhostAnchor},               // citation_grounding
		{Kind: types.ViolAuthorityOverreach},        // authority_hedging
	}
	caveats := MaterializeUnresolvedViolationsAsCaveats(violations, "zh")
	if len(caveats) > MaxMaterializedCaveats {
		t.Errorf("cap broken: got %d caveats, max %d", len(caveats), MaxMaterializedCaveats)
	}
}

// TestMaterializeCaveats_EmptyInput — nil/empty input → nil output.
func TestMaterializeCaveats_EmptyInput(t *testing.T) {
	if got := MaterializeUnresolvedViolationsAsCaveats(nil, "zh"); got != nil {
		t.Errorf("nil input should give nil output, got %v", got)
	}
	if got := MaterializeUnresolvedViolationsAsCaveats([]types.Violation{}, "zh"); got != nil {
		t.Errorf("empty input should give nil output, got %v", got)
	}
}

// TestMaterializeCaveats_StableOrder — same violations in different
// input order produce same output. Important for retry parity (so
// repeated runs don't see caveats shuffle order).
func TestMaterializeCaveats_StableOrder(t *testing.T) {
	a := []types.Violation{
		{Kind: types.ViolRichnessRegression},
		{Kind: types.ViolDiagramEdgeUnsupported},
	}
	b := []types.Violation{
		{Kind: types.ViolDiagramEdgeUnsupported},
		{Kind: types.ViolRichnessRegression},
	}
	out1 := MaterializeUnresolvedViolationsAsCaveats(a, "zh")
	out2 := MaterializeUnresolvedViolationsAsCaveats(b, "zh")
	if strings.Join(out1, "|") != strings.Join(out2, "|") {
		t.Errorf("input order changed output: %v vs %v", out1, out2)
	}
}
