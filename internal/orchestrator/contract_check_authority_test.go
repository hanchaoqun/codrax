package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/authority"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func withAuthGate(t *testing.T, on bool, run func()) {
	t.Helper()
	prev := authority.Enabled()
	authority.Enable(on)
	defer authority.Enable(prev)
	run()
}

// TestRunAuthorityOverreachCheck_GateOffNoEmit defends the legacy-
// safety contract: when the master gate is off, the producer is a
// no-op even when the doc would otherwise warrant a violation.
func TestRunAuthorityOverreachCheck_GateOffNoEmit(t *testing.T) {
	withAuthGate(t, false, func() {
		mut := types.NewMutableState("test")
		mut.SetAnswerDocument(&types.AnswerDocument{
			Shape:     types.ShapeStepList,
			Citations: []types.Citation{{File: "x.go", Line: 10}},
		})
		mut.AppendEvidence([]types.EvidenceItem{
			{Source: "x.go", LineStart: 10, Authority: types.AuthorityHistorical},
		})
		// Prose explicitly missing the [historical] hedge sentinel.
		viols := runAuthorityOverreachCheck(mut, "X directly causes Y; see x.go:10")
		if len(viols) != 0 {
			t.Errorf("gate off: emitted %d violations; want 0", len(viols))
		}
	})
}

// TestRunAuthorityOverreachCheck_NilMutableSafe defends nil-input
// safety on the producer.
func TestRunAuthorityOverreachCheck_NilMutableSafe(t *testing.T) {
	withAuthGate(t, true, func() {
		if got := runAuthorityOverreachCheck(nil, "anything"); got != nil {
			t.Errorf("nil mut: got %d violations; want nil", len(got))
		}
	})
}

// TestRunAuthorityOverreachCheck_EmptyEvidenceNoEmit asserts the
// producer is silent when there is no evidence to ground a verdict.
func TestRunAuthorityOverreachCheck_EmptyEvidenceNoEmit(t *testing.T) {
	withAuthGate(t, true, func() {
		mut := types.NewMutableState("test")
		mut.SetAnswerDocument(&types.AnswerDocument{
			Shape:     types.ShapeExplanation,
			Citations: []types.Citation{{File: "x.go", Line: 10}},
		})
		// No evidence appended.
		if got := runAuthorityOverreachCheck(mut, "anything"); got != nil {
			t.Errorf("empty evidence: got %d violations; want nil", len(got))
		}
	})
}

// TestRunAuthorityOverreachCheck_FactualOnlyNoEmit: cited anchors
// all factual → producer silent.
func TestRunAuthorityOverreachCheck_FactualOnlyNoEmit(t *testing.T) {
	withAuthGate(t, true, func() {
		mut := types.NewMutableState("test")
		mut.SetAnswerDocument(&types.AnswerDocument{
			Shape:     types.ShapeExplanation,
			Citations: []types.Citation{{File: "x.go", Line: 10}},
		})
		mut.AppendEvidence([]types.EvidenceItem{
			{Source: "x.go", LineStart: 10, Authority: types.AuthorityFactual},
		})
		// Plain prose, no hedge — that's fine for factual evidence.
		if got := runAuthorityOverreachCheck(mut, "X is at x.go:10."); got != nil {
			t.Errorf("factual-only: got %d violations; want nil", len(got))
		}
	})
}

// TestRunAuthorityOverreachCheck_ConditionalCitedButNoHedgeFires:
// the load-bearing case — a cited anchor's underlying evidence is
// conditional, but the rendered prose lacks [hedged]. Producer
// must surface a single ViolAuthorityOverreach.
func TestRunAuthorityOverreachCheck_ConditionalCitedButNoHedgeFires(t *testing.T) {
	withAuthGate(t, true, func() {
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
	})
}

// TestRunAuthorityOverreachCheck_ConditionalCitedHedgePresentNoEmit:
// when the rendered prose DOES contain the hedge sentinel for a
// conditional anchor, the check passes.
func TestRunAuthorityOverreachCheck_ConditionalCitedHedgePresentNoEmit(t *testing.T) {
	withAuthGate(t, true, func() {
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
	})
}

// TestRunAuthorityOverreachCheck_HistoricalRequiresHistoricalMarker:
// each ceiling needs ITS sentinel — a [hedged] marker doesn't
// satisfy a historical anchor's requirement.
func TestRunAuthorityOverreachCheck_HistoricalRequiresHistoricalMarker(t *testing.T) {
	withAuthGate(t, true, func() {
		mut := types.NewMutableState("test")
		mut.SetAnswerDocument(&types.AnswerDocument{
			Shape:     types.ShapeExplanation,
			Citations: []types.Citation{{File: "x.go", Line: 10}},
		})
		mut.AppendEvidence([]types.EvidenceItem{
			{Source: "x.go", LineStart: 10, Authority: types.AuthorityHistorical},
		})
		// Prose has [hedged] but not [historical] — should still fire.
		text := render.HedgeMarkerConditional + " X happened in legacy code."
		viols := runAuthorityOverreachCheck(mut, text)
		if len(viols) != 1 {
			t.Fatalf("got %d violations; want 1", len(viols))
		}
		if !strings.Contains(viols[0].Detail, "historical") {
			t.Errorf("Detail missing 'historical': %q", viols[0].Detail)
		}
	})
}
