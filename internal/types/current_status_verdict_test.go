package types

import "testing"

func TestCurrentStatusVerdict_NormalizeAndValidate(t *testing.T) {
	for _, verdict := range AllCurrentStatusVerdicts() {
		got, ok := NormalizeCurrentStatusVerdict(string(verdict))
		if !ok || got != verdict {
			t.Fatalf("NormalizeCurrentStatusVerdict(%q) = %q,%v", verdict, got, ok)
		}
	}
	if got, ok := NormalizeCurrentStatusVerdict("not enough evidence"); ok || got != CurrentStatusUnknown {
		t.Fatalf("free-form verdict should be invalid, got %q,%v", got, ok)
	}
}

func TestMissingCurrentStatusVerdict_UsesTypedDecisionOnly(t *testing.T) {
	contract := &CurrentStatusDiagnosticContract{Required: true}
	doc := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID:          "d",
		Kind:        BlockDecision,
		SurfaceRole: SurfacePrincipal,
		Text:        "fixed: prose alone is not the carrier",
	}}}
	if !MissingCurrentStatusVerdict(doc, contract) {
		t.Fatal("prose-only decision must not satisfy typed current-status verdict")
	}
	doc.Blocks[0].CurrentStatusVerdict = CurrentStatusFixed
	if MissingCurrentStatusVerdict(doc, contract) {
		t.Fatal("typed current-status verdict should satisfy the contract")
	}
	doc.Blocks[0].CurrentStatusVerdict = CurrentStatusVerdict("maybe_fixed")
	if !MissingCurrentStatusVerdict(doc, contract) {
		t.Fatal("invalid typed current-status verdict should be rejected")
	}
}

func TestCurrentStatusAllowedVerdicts_DedupesAndDefaults(t *testing.T) {
	contract := &CurrentStatusDiagnosticContract{
		AllowedVerdicts: []CurrentStatusVerdict{
			CurrentStatusFixed,
			CurrentStatusFixed,
			CurrentStatusVerdict("bad"),
			CurrentStatusUnknown,
		},
	}
	got := CurrentStatusAllowedVerdicts(contract)
	if len(got) != 1 || got[0] != CurrentStatusFixed {
		t.Fatalf("allowed verdicts should dedupe valid values, got %+v", got)
	}
	if !CurrentStatusVerdictAllowed(CurrentStatusStillPresent, nil) {
		t.Fatal("nil contract should allow default verdict set")
	}
}

func TestAnswerBlockHasActiveTypedDecisionCarrier(t *testing.T) {
	block := AnswerBlock{
		Kind:                 BlockDecision,
		CurrentStatusVerdict: CurrentStatusStillPresent,
	}
	currentView := &AnswerSemanticView{
		CurrentStatusDiagnostic: &CurrentStatusDiagnosticContract{Required: true},
	}
	if !AnswerBlockHasActiveTypedDecisionCarrier(block, currentView) {
		t.Fatal("active current-status verdict should satisfy typed decision carrier")
	}
	if AnswerBlockHasActiveTypedDecisionCarrier(block, &AnswerSemanticView{}) {
		t.Fatal("inactive current-status lane must not satisfy typed decision carrier")
	}

	block.CurrentStatusVerdict = CurrentStatusVerdict("maybe")
	if AnswerBlockHasActiveTypedDecisionCarrier(block, currentView) {
		t.Fatal("invalid current-status verdict must not satisfy typed decision carrier")
	}

	block.CurrentStatusVerdict = CurrentStatusUnknown
	block.ErrorGranularityVerdict = ErrorGranularityPartialSuccess
	errorView := &AnswerSemanticView{
		ErrorGranularityProfile: &ErrorGranularityProfile{IsGranularityQuestion: true},
	}
	if !AnswerBlockHasActiveTypedDecisionCarrier(block, errorView) {
		t.Fatal("active error-granularity verdict should satisfy typed decision carrier")
	}
}
