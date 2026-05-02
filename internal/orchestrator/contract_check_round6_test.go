package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestIsPlumbingFailureViolation_ClassifiesAuthorityOverreach: the
// kind that fires only on renderer bypass is plumbing, not answer
// content. Must NOT pollute answer_taxonomy.
func TestIsPlumbingFailureViolation_ClassifiesAuthorityOverreach(t *testing.T) {
	if !isPlumbingFailureViolation(types.ViolAuthorityOverreach) {
		t.Errorf("ViolAuthorityOverreach should be plumbing-classified")
	}
}

// TestIsPlumbingFailureViolation_AnswerQualityKindsPassThrough:
// content-related violations must be eligible for taxonomy collection.
func TestIsPlumbingFailureViolation_AnswerQualityKindsPassThrough(t *testing.T) {
	contentKinds := []types.ViolationKind{
		types.ViolShape,
		types.ViolCitation,
		types.ViolMustInclude,
		types.ViolMustExclude,
		types.ViolShapeIntentMismatch,
		types.ViolSubTopicCountMismatch,
		types.ViolDeclaredCountDrift,
		types.ViolSelfContradiction,
		types.ViolExternalArtifactUnderdecoded,
	}
	for _, k := range contentKinds {
		if isPlumbingFailureViolation(k) {
			t.Errorf("ViolationKind %q misclassified as plumbing", k)
		}
	}
}

// TestRunAuthorityOverreachCheck_RepairIsLLMActionable: the repair
// hint must speak in terms the LLM has a schema for (tool name +
// shape + required fields), not internal plumbing names like
// "ApplyAuthorityHedging" or "render path".
func TestRunAuthorityOverreachCheck_RepairIsLLMActionable(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetAnswerDocument(&types.AnswerDocument{
		Shape:     types.ShapeExplanation,
		Citations: []types.Citation{{File: "x.go", Line: 10}},
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityConditional},
	})
	viols := runAuthorityOverreachCheck(mut, "Plain prose without disclosure.")
	if len(viols) != 1 {
		t.Fatalf("got %d violations; want 1", len(viols))
	}
	repair := viols[0].Repair
	// Must mention the user-actionable tool name.
	if !substringContainsRound6(repair, "emit_answer_document") {
		t.Errorf("repair missing tool name: %q", repair)
	}
	// Must NOT leak internal plumbing names.
	for _, leak := range []string{"ApplyAuthorityHedging", "render path", "doc.Caveats"} {
		if substringContainsRound6(repair, leak) {
			t.Errorf("repair leaks internal name %q: %q", leak, repair)
		}
	}
}

func substringContainsRound6(s, sub string) bool {
	if sub == "" {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
