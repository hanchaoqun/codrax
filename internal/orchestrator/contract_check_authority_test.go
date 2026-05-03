package orchestrator

import (
	"testing"

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
