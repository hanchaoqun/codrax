package types

import "testing"

func TestRuntimeQuestionProfilePromptBreadthIsTyped(t *testing.T) {
	finite := &RuntimeQuestionProfile{
		Scope: RuntimeQuestionScopeBoundedEffectVerdict,
		FactFamilies: []RuntimeQuestionFactFamily{
			RuntimeQuestionFactTargetSchedulerState,
			RuntimeQuestionFactCountOrDuration,
			RuntimeQuestionFactFrequencyResidency,
		},
	}
	if !finite.SuppressesRootCauseRankingPrompt() {
		t.Fatal("bounded effect verdict must suppress a root-cause roster prompt")
	}
	if finite.RequestsTraceWaitEvidencePrompt() {
		t.Fatal("state/duration/frequency families must not inherit the wait+wakeup appendix")
	}
	if !finite.RequestsFactFamily(RuntimeQuestionFactFrequencyResidency) ||
		finite.RequestsFactFamily(RuntimeQuestionFactRecordedReason) {
		t.Fatal("bounded fact-family membership must come only from the typed family list")
	}

	finite.FactFamilies = append(finite.FactFamilies, RuntimeQuestionFactDirectWaker)
	if !finite.RequestsTraceWaitEvidencePrompt() {
		t.Fatal("direct-waker family must retain exact wait/wakeup evidence")
	}

	causal := &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeCausalDiagnosis}
	if causal.SuppressesRootCauseRankingPrompt() || !causal.RequestsTraceWaitEvidencePrompt() {
		t.Fatal("causal diagnosis must retain the full ranking and wait-evidence surfaces")
	}
}
