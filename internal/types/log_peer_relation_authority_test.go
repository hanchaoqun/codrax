package types

import "testing"

func peerErrorLogBundleForAuthorityTest() *LogBundle {
	return &LogBundle{
		Errors: []LogError{
			{Type: "cangjie_panic", Message: "native panic"},
			{Type: "arkts_error", Message: "bridge invocation failed"},
		},
		Observations: []LogObservation{{
			Kind:       LogObservationRuntimeEvent,
			Subject:    "bridge dispatch",
			Summary:    "the native panic propagated into ArkTS",
			Evidence:   "ArkTS bridge invocation failed",
			LineStart:  17,
			LineEnd:    17,
			Diagnostic: true,
			Confidence: 0.9,
		}},
	}
}

func peerRelationAggregateFactsForAuthorityTest() []AnswerAggregateFact {
	return []AnswerAggregateFact{
		{
			Kind:  AnswerAggregateBehaviorOutcome,
			Label: "unsupported propagation",
			Value: "panic -> bridge -> ArkTS",
			Role:  AnswerAggregateRoleSupportingCoverage,
		},
		{
			Kind:  AnswerAggregateErrorGranularity,
			Label: "unsupported relation verdict",
			Value: "one propagated failure",
			Role:  AnswerAggregateRoleSupportingCoverage,
		},
		{
			Kind:        AnswerAggregateBehaviorOutcome,
			Label:       "producer-bound runtime outcome",
			Value:       "explicitly witnessed outcome",
			Role:        AnswerAggregateRoleSupportingCoverage,
			SupportRefs: []string{"runtime_artifact:attached_log"},
		},
		{
			Kind:        AnswerAggregateMemberSet,
			Label:       "peer error inventory",
			Value:       "2",
			Role:        AnswerAggregateRoleSupportingCoverage,
			Members:     []string{"cangjie_panic", "arkts_error"},
			MemberNotes: []string{"producer panic", "received propagated panic"},
		},
	}
}

func TestProjectLogPeerRelationAnswerAuthorityFiltersOnlyUnsupportedSynthesis(t *testing.T) {
	facts := peerRelationAggregateFactsForAuthorityTest()
	got := ProjectLogPeerRelationAnswerAuthority(facts, peerErrorLogBundleForAuthorityTest())
	if len(got) != 2 || got[0].Label != "producer-bound runtime outcome" || got[1].Label != "peer error inventory" {
		t.Fatalf("peer relation projection drifted: %+v", got)
	}
	if got[1].MemberNotes != nil {
		t.Fatalf("model-authored peer member relation notes leaked into answer authority: %+v", got[1])
	}
	if len(facts) != 4 {
		t.Fatalf("raw model facts must remain untouched for audit: %+v", facts)
	}
	if len(facts[3].MemberNotes) != 2 || facts[3].MemberNotes[1] != "received propagated panic" {
		t.Fatalf("answer projection mutated raw member-note audit history: %+v", facts[3])
	}
	negative := ProjectLogPeerRelationAnswerAuthority(facts, &LogBundle{Errors: []LogError{{
		Type:  "outer",
		Cause: &LogError{Type: "inner"},
		CauseRelation: &LogCauseRelation{
			Authority: LogCauseAuthorityExplicitArtifactMarker,
			Marker:    "Caused by: inner",
		},
	}}})
	if len(negative) != len(facts) {
		t.Fatalf("one explicit cause tree is not a peer-error shape: %+v", negative)
	}
}

func TestBuildAnswerSurfacePlanFiltersUnsupportedPeerRelationFactsButKeepsMutableAudit(t *testing.T) {
	facts := peerRelationAggregateFactsForAuthorityTest()
	mutable := NewMutableState("mixed runtime errors")
	mutable.SetInvestigationAggregateFacts(facts)
	mutable.SetInvestigationComplete("done")
	mutable.RetainInvestigationAggregateFacts()
	bundle := peerErrorLogBundleForAuthorityTest()
	mutable.SetLogTriage(bundle)

	plan := BuildAnswerSurfacePlan(&AnalysisIR{RequestModel: RequestModel{}}, mutable, bundle, nil, nil, nil)
	if plan == nil || len(plan.StableAggregateFacts) != 2 {
		t.Fatalf("final answer surface retained unsupported peer synthesis: %+v", plan)
	}
	if raw := mutable.StableInvestigationAggregateFacts(); len(raw) != len(facts) {
		t.Fatalf("surface projection mutated raw audit facts: %+v", raw)
	}
}

func TestCompileObservationLedgerFiltersUnsupportedPeerRelationFacts(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		AggregateFacts: peerRelationAggregateFactsForAuthorityTest(),
		LogBundle:      peerErrorLogBundleForAuthorityTest(),
	})
	foundBoundOutcome := false
	foundInventory := false
	for _, record := range ledger.Records {
		switch record.Summary {
		case "unsupported propagation", "unsupported relation verdict":
			t.Fatalf("unsupported peer relation synthesis leaked into ledger: %+v", record)
		case "producer-bound runtime outcome":
			foundBoundOutcome = true
		case "peer error inventory":
			foundInventory = true
		}
	}
	if !foundBoundOutcome || !foundInventory {
		t.Fatalf("projection removed supported/non-relational runtime facts: %+v", ledger.Records)
	}
	for _, id := range []string{"log:error:0", "log:error:1", "log:cross_error_relation"} {
		findObservationRecord(t, ledger, id)
	}
	observation := findObservationRecord(t, ledger, "log:observation:0")
	if observation.Summary != "ArkTS bridge invocation failed" || observation.RawExcerpt != "ArkTS bridge invocation failed" {
		t.Fatalf("peer projection lost the exact artifact observation: %+v", observation)
	}
	for _, note := range observation.RichNotes {
		if note == "triager_interpretation_advisory=the native panic propagated into ArkTS" {
			t.Fatalf("model-authored peer relation interpretation leaked into ledger: %+v", observation)
		}
	}
}

func TestCompileObservationLedgerKeepsSingleErrorInterpretationAdvisory(t *testing.T) {
	bundle := peerErrorLogBundleForAuthorityTest()
	bundle.Errors = bundle.Errors[:1]
	ledger := CompileObservationLedger(ObservationLedgerInput{LogBundle: bundle})
	observation := findObservationRecord(t, ledger, "log:observation:0")
	if !containsObservationString(observation.RichNotes, "triager_interpretation_advisory=the native panic propagated into ArkTS") {
		t.Fatalf("non-peer observation interpretation was over-suppressed: %+v", observation)
	}
}
