package types

import "testing"

func TestSleepPopulationFactFamiliesDoNotRequireWakerOrMintCause(t *testing.T) {
	for _, predicate := range []string{"target_sleep_inventory", "target_sleep_state_summary", "target_sleep_interval"} {
		families := RuntimeObservationRecordFactFamilies(ObservationRecord{Predicate: predicate})
		has := map[RuntimeQuestionFactFamily]bool{}
		for _, family := range families {
			has[family] = true
		}
		for _, want := range []RuntimeQuestionFactFamily{RuntimeQuestionFactTargetSchedulerState, RuntimeQuestionFactTargetWaitOccurrences, RuntimeQuestionFactCountOrDuration} {
			if !has[want] {
				t.Errorf("%s lost %s", predicate, want)
			}
		}
		if has[RuntimeQuestionFactDirectWaker] || has[RuntimeQuestionFactRecordedReason] || has[RuntimeQuestionFactIOLatency] {
			t.Errorf("%s gained an unproved mechanism/relation: %v", predicate, families)
		}
		if has[RuntimeQuestionFactOccurrenceTime] != (predicate == "target_sleep_interval") {
			t.Errorf("summary envelope became an occurrence: %s", predicate)
		}
	}
}

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

func TestRuntimeQuestionProfileRuntimeWorkRelationDemandIsExplicitTypedState(t *testing.T) {
	profile := &RuntimeQuestionProfile{
		Scope:                        RuntimeQuestionScopeCausalDiagnosis,
		RuntimeWorkRelationRequested: true,
	}
	if !profile.RequestsRuntimeWorkRelation() {
		t.Fatal("explicit runtime-work relation demand must survive on the typed profile")
	}
	profile.RuntimeWorkRelationRequested = false
	if profile.RequestsRuntimeWorkRelation() {
		t.Fatal("false typed demand must not be inferred back from scope")
	}
	var absent *RuntimeQuestionProfile
	if absent.RequestsRuntimeWorkRelation() {
		t.Fatal("nil profile must not mint a runtime-work relation demand")
	}
}
