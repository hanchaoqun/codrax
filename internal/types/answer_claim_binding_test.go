package types

import "testing"

func TestCompileAnswerClaimBindingsFromAggregateFacts_HistoryCountKeepsVCSAndMeasurement(t *testing.T) {
	rm := RequestModel{
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
			IsCountQuestion: true,
			IsScalarAnswer:  true,
		},
	}
	facts := []AnswerAggregateFact{{
		Kind:  AnswerAggregateScalar,
		Label: "system verified history count",
		Value: "3",
		Role:  AnswerAggregateRolePrincipalAnswer,
		Dimensions: []AnswerAggregateDimension{
			{Name: "origin", Value: "vcs_metadata"},
			{Name: "measurement_origin", Value: "command_measurement"},
		},
	}}

	got := CompileAnswerClaimBindingsFromAggregateFacts(facts, &rm, nil)
	assertClaimBinding(t, got, AnswerEvidenceOriginVCSMetadata, ClaimGroundingRepairable, AnswerRequestedOutputCount)
	assertClaimBinding(t, got, AnswerEvidenceOriginCommandMeasurement, ClaimGroundingHard, AnswerRequestedOutputCount)
}

func TestCompileAnswerClaimBindingsFromAggregateFacts_CurrentSourcePrincipalIsHard(t *testing.T) {
	rm := RequestModel{Intent: IntentExplain}
	facts := []AnswerAggregateFact{{
		Kind:        AnswerAggregateMemberSet,
		Label:       "current code symbols",
		Value:       "1",
		Role:        AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"Gate.Run"},
		SupportRefs: []string{"Gate.Run: internal/analysis/gate.go:42"},
	}}

	got := CompileAnswerClaimBindingsFromAggregateFacts(facts, &rm, nil)
	b := assertClaimBinding(t, got, AnswerEvidenceOriginCurrentSource, ClaimGroundingHard, AnswerRequestedOutputSummary)
	if b.TargetRef != "current code symbols" {
		t.Fatalf("target ref = %q", b.TargetRef)
	}
	if len(b.SupportRefs) != 1 {
		t.Fatalf("support refs were not preserved: %+v", b)
	}
}

func TestCompileAnswerClaimBindingsFromAggregateFacts_RuntimeArtifactDoesNotBecomeCurrentSource(t *testing.T) {
	rm := RequestModel{
		Intent: IntentRootCause,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		LogTriage: &LogBundle{Errors: []LogError{{Type: "panic"}}},
	}
	facts := []AnswerAggregateFact{{
		Kind:       AnswerAggregateScalar,
		Label:      "observed panic",
		Value:      "panic",
		Role:       AnswerAggregateRolePrincipalAnswer,
		Dimensions: []AnswerAggregateDimension{{Name: "origin", Value: "runtime_artifact"}},
	}}

	got := CompileAnswerClaimBindingsFromAggregateFacts(facts, &rm, nil)
	assertClaimBinding(t, got, AnswerEvidenceOriginRuntimeArtifact, ClaimGroundingRepairable, AnswerRequestedOutputDiagnostic)
	for _, binding := range got {
		if binding.Origin == AnswerEvidenceOriginCurrentSource {
			t.Fatalf("runtime artifact binding should not synthesize current_source: %+v", got)
		}
	}
}

func assertClaimBinding(t *testing.T, bindings []AnswerClaimBinding, origin AnswerEvidenceOrigin, policy ClaimGroundingPolicy, output AnswerRequestedOutput) AnswerClaimBinding {
	t.Helper()
	for _, binding := range bindings {
		if binding.Origin != origin {
			continue
		}
		if binding.GroundingPolicy != policy {
			t.Fatalf("binding policy for %s = %s, want %s: %+v", origin, binding.GroundingPolicy, policy, binding)
		}
		if !claimBindingHasOutput(binding, output) {
			t.Fatalf("binding for %s missing output %s: %+v", origin, output, binding.RequestedOutputs)
		}
		return binding
	}
	t.Fatalf("missing binding for origin %s in %+v", origin, bindings)
	return AnswerClaimBinding{}
}

func claimBindingHasOutput(binding AnswerClaimBinding, want AnswerRequestedOutput) bool {
	for _, got := range binding.RequestedOutputs {
		if got == want {
			return true
		}
	}
	return false
}
