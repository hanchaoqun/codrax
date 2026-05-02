package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRunAuthorityOverreachCheck_NilMutableSafe defends nil-input
// safety on the producer.
func TestRunAuthorityOverreachCheck_NilMutableSafe(t *testing.T) {
	if got := runAuthorityOverreachCheck(nil, "anything"); got != nil {
		t.Errorf("nil mut: got %d violations; want nil", len(got))
	}
}

// TestRunAuthorityOverreachCheck_EmptyEvidenceNoEmit asserts the
// producer is silent when there is no evidence to ground a verdict.
func TestRunAuthorityOverreachCheck_EmptyEvidenceNoEmit(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetAnswerDocument(&types.AnswerDocument{
		Shape:     types.ShapeExplanation,
		Citations: []types.Citation{{File: "x.go", Line: 10}},
	})
	if got := runAuthorityOverreachCheck(mut, "anything"); got != nil {
		t.Errorf("empty evidence: got %d violations; want nil", len(got))
	}
}

// TestRunAuthorityOverreachCheck_FactualOnlyNoEmit: cited anchors
// all factual → producer silent.
func TestRunAuthorityOverreachCheck_FactualOnlyNoEmit(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetAnswerDocument(&types.AnswerDocument{
		Shape:     types.ShapeExplanation,
		Citations: []types.Citation{{File: "x.go", Line: 10}},
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityFactual},
	})
	if got := runAuthorityOverreachCheck(mut, "X is at x.go:10."); got != nil {
		t.Errorf("factual-only: got %d violations; want nil", len(got))
	}
}

// TestRunAuthorityOverreachCheck_ConditionalCitedButNoHedgeFires:
// the load-bearing case — a cited anchor's underlying evidence is
// conditional, but the rendered prose lacks [hedged]. Producer
// must surface a single ViolAuthorityOverreach.
func TestRunAuthorityOverreachCheck_ConditionalCitedButNoHedgeFires(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetAnswerDocument(&types.AnswerDocument{
		Shape:     types.ShapeExplanation,
		Citations: []types.Citation{{File: "x.go", Line: 10}},
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityConditional},
	})
	viols := runAuthorityOverreachCheck(mut, "X directly causes Y; see x.go:10.")
	if len(viols) != 1 {
		t.Fatalf("got %d violations; want 1", len(viols))
	}
	if viols[0].Kind != types.ViolAuthorityOverreach {
		t.Errorf("Kind = %q; want authority_overreach", viols[0].Kind)
	}
	if !strings.Contains(viols[0].Detail, "conditional") {
		t.Errorf("Detail missing 'conditional': %q", viols[0].Detail)
	}
}

// TestRunAuthorityOverreachCheck_ConditionalCitedHedgePresentNoEmit:
// when the rendered prose DOES contain the hedge sentinel for a
// conditional anchor, the check passes.
func TestRunAuthorityOverreachCheck_ConditionalCitedHedgePresentNoEmit(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetAnswerDocument(&types.AnswerDocument{
		Shape:     types.ShapeExplanation,
		Citations: []types.Citation{{File: "x.go", Line: 10}},
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityConditional},
	})
	hedged := "Based on log observation, " + render.HedgeMarkerConditional + " current code at x.go:10 reads X."
	if got := runAuthorityOverreachCheck(mut, hedged); got != nil {
		t.Errorf("hedged prose: got %d violations; want nil", len(got))
	}
}

// TestRunAuthorityOverreachCheck_HistoricalRequiresHistoricalMarker:
// each ceiling needs ITS sentinel — a [hedged] marker doesn't
// satisfy a historical anchor's requirement.
func TestRunAuthorityOverreachCheck_HistoricalRequiresHistoricalMarker(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetAnswerDocument(&types.AnswerDocument{
		Shape:     types.ShapeExplanation,
		Citations: []types.Citation{{File: "x.go", Line: 10}},
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityHistorical},
	})
	text := render.HedgeMarkerConditional + " X happened in legacy code."
	viols := runAuthorityOverreachCheck(mut, text)
	if len(viols) != 1 {
		t.Fatalf("got %d violations; want 1", len(viols))
	}
	if !strings.Contains(viols[0].Detail, "historical") {
		t.Errorf("Detail missing 'historical': %q", viols[0].Detail)
	}
}
