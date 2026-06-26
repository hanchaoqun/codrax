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

func TestErrorGranularityCountsAreContextual(t *testing.T) {
	profile := &ErrorGranularityProfile{IsGranularityQuestion: true}
	if !ErrorGranularityCountsAreContextual(IntentExplain, SemanticPredicates{}, profile) {
		t.Fatal("active failure-scope profile with no count/category/relation lane should make counts contextual")
	}
	for name, preds := range map[string]SemanticPredicates{
		"count":    {IsCountQuestion: true},
		"category": {IsCategoryEnumeration: true},
		"relation": {IsRelationalLookup: true},
	} {
		if ErrorGranularityCountsAreContextual(IntentExplain, preds, profile) {
			t.Fatalf("%s answer lane should keep counts structural", name)
		}
	}
	if ErrorGranularityCountsAreContextual(IntentEnumerate, SemanticPredicates{}, profile) {
		t.Fatal("explicit enumerate intent should keep count/category handling available")
	}
}

func TestIsFailureScopeDecisionAnswer(t *testing.T) {
	rm := RequestModel{
		Intent: IntentExplain,
		ErrorGranularityProfile: &ErrorGranularityProfile{
			IsGranularityQuestion: true,
		},
	}
	if !IsFailureScopeDecisionAnswer(rm) {
		t.Fatal("plain active failure-scope profile should resolve as a decision answer")
	}
	rm.Predicates.IsCategoryEnumeration = true
	if IsFailureScopeDecisionAnswer(rm) {
		t.Fatal("explicit category answer predicate should win over the decision helper")
	}
}

func TestShouldCarryErrorGranularityHardContract_DiagnosticMechanismSoftens(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioRootCause,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		AnalyzerHints: AnalyzerHints{Kind: string(ReqMechanism)},
		ErrorGranularityProfile: &ErrorGranularityProfile{
			IsGranularityQuestion: true,
			RequestedVerdictOptions: []ErrorGranularityVerdict{
				ErrorGranularityPerItemRejection,
				ErrorGranularityWholeBatch,
			},
			SourceQuotes: []string{"distinguish the two failure classes"},
			Confidence:   0.85,
		},
	}

	if ShouldCarryErrorGranularityHardContract(rm) {
		t.Fatal("diagnostic mechanism explanation must not force a failure-scope verdict")
	}
	if !ErrorGranularityConflictsWithDiagnosticMechanism(rm) {
		t.Fatal("diagnostic mechanism conflict should be visible for softening/audit")
	}
}

func TestShouldCarryErrorGranularityHardContract_ReturnValueFailureScopeCarries(t *testing.T) {
	rm := RequestModel{
		Intent: IntentReturnValue,
		Predicates: SemanticPredicates{
			IsScalarAnswer: true,
		},
		AnalyzerHints: AnalyzerHints{Kind: string(ReqReturnValue)},
		ErrorGranularityProfile: &ErrorGranularityProfile{
			IsGranularityQuestion: true,
			RequestedVerdictOptions: []ErrorGranularityVerdict{
				ErrorGranularityPerItemRejection,
				ErrorGranularityWholeBatch,
			},
			SourceQuotes: []string{"reject just that record or fail the whole batch"},
			Confidence:   0.95,
		},
	}

	if !ShouldCarryErrorGranularityHardContract(rm) {
		t.Fatal("scalar return-value failure-scope question should keep the hard verdict contract")
	}
	if ErrorGranularityConflictsWithDiagnosticMechanism(rm) {
		t.Fatal("non-diagnostic failure-scope question should not be reported as a diagnostic conflict")
	}
}
