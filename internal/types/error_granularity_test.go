package types

import "testing"

func TestErrorGranularityVerdict_NormalizeAndValidate(t *testing.T) {
	for _, verdict := range AllErrorGranularityVerdicts() {
		got, ok := NormalizeErrorGranularityVerdict(string(verdict))
		if !ok || got != verdict {
			t.Fatalf("NormalizeErrorGranularityVerdict(%q) = %q,%v", verdict, got, ok)
		}
	}
	if got, ok := NormalizeErrorGranularityVerdict("whole batch failure"); ok || got != ErrorGranularityUnknown {
		t.Fatalf("invalid verdict should reject, got %q,%v", got, ok)
	}
}

func TestMissingErrorGranularityVerdict_UsesTypedPrincipalDecisionOnly(t *testing.T) {
	profile := &ErrorGranularityProfile{IsGranularityQuestion: true, Confidence: 0.9}
	doc := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID:          "summary",
		Kind:        BlockSummary,
		SurfaceRole: SurfacePrincipal,
		Text:        "per_item_rejection",
	}}}
	if !MissingErrorGranularityVerdict(doc, profile) {
		t.Fatal("prose-only summary must not satisfy error granularity verdict")
	}
	doc.Blocks = append(doc.Blocks, AnswerBlock{
		ID:                      "supporting",
		Kind:                    BlockDecision,
		ErrorGranularityVerdict: ErrorGranularityPerItemRejection,
	})
	if !MissingErrorGranularityVerdict(doc, profile) {
		t.Fatal("supporting decision must not satisfy principal verdict")
	}
	doc.Blocks = append(doc.Blocks, AnswerBlock{
		ID:                      "decision",
		Kind:                    BlockDecision,
		SurfaceRole:             SurfacePrincipal,
		ErrorGranularityVerdict: ErrorGranularityPerItemRejection,
	})
	if MissingErrorGranularityVerdict(doc, profile) {
		t.Fatal("principal typed decision verdict should satisfy profile")
	}
}

func TestErrorGranularityVerdictOptionMismatch_UsesRequestedOptions(t *testing.T) {
	profile := &ErrorGranularityProfile{
		IsGranularityQuestion: true,
		RequestedVerdictOptions: []ErrorGranularityVerdict{
			ErrorGranularityPerItemRejection,
			ErrorGranularityWholeBatch,
		},
		Confidence: 0.9,
	}
	doc := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID:                      "decision",
		Kind:                    BlockDecision,
		SurfaceRole:             SurfacePrincipal,
		ErrorGranularityVerdict: ErrorGranularityPartialSuccess,
	}}}
	if got, mismatch := ErrorGranularityVerdictOptionMismatch(doc, profile); !mismatch || got != ErrorGranularityPartialSuccess {
		t.Fatalf("expected partial_success option mismatch, got %q,%v", got, mismatch)
	}
	doc.Blocks[0].ErrorGranularityVerdict = ErrorGranularityPerItemRejection
	if got, mismatch := ErrorGranularityVerdictOptionMismatch(doc, profile); mismatch || got != ErrorGranularityUnknown {
		t.Fatalf("requested option should pass, got %q,%v", got, mismatch)
	}
	doc.Blocks[0].ErrorGranularityVerdict = ErrorGranularityNotEnoughEvidence
	if got, mismatch := ErrorGranularityVerdictOptionMismatch(doc, profile); mismatch || got != ErrorGranularityUnknown {
		t.Fatalf("not_enough_evidence fallback should pass, got %q,%v", got, mismatch)
	}
}
