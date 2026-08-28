package types

import "testing"

func TestProjectTypedTraceAnswerAuthorityWithholdsUnboundModelRestatements(t *testing.T) {
	rm := RequestModel{Intent: IntentRootCause, Scenario: ScenarioPerformanceBottleneck}
	plan := &AnswerSurfacePlan{StableAggregateFacts: []AnswerAggregateFact{
		{
			Kind: AnswerAggregateScalar, Label: "model trace total", Value: "17.819",
			Dimensions: []AnswerAggregateDimension{{Name: "origin", Value: "runtime_artifact"}},
		},
		{
			Kind: AnswerAggregateMemberSet, Label: "grounded source mechanism", Value: "1",
			Members: []string{"Run"}, SupportRefs: []string{"internal/run.go:42"},
		},
	}}
	ledger := typedTraceProjectionLedgerForAggregateAuthorityTest()

	projectTypedTraceAnswerAuthority(plan, &rm, ledger)

	if len(plan.StableAggregateFacts) != 1 {
		t.Fatalf("expected only the independently grounded non-runtime fact to remain, got %+v", plan.StableAggregateFacts)
	}
	if got := plan.StableAggregateFacts[0].Label; got != "grounded source mechanism" {
		t.Fatalf("wrong fact survived typed trace authority projection: %q", got)
	}
}

func TestProjectTypedTraceAnswerAuthorityWithholdsNarrowFactSetModelRestatements(t *testing.T) {
	rm := RequestModel{
		Intent: IntentTrace,
		RuntimeQuestionProfile: &RuntimeQuestionProfile{
			Scope:        RuntimeQuestionScopeBoundedFactSet,
			FactFamilies: []RuntimeQuestionFactFamily{RuntimeQuestionFactTargetSchedulerState},
		},
	}
	plan := &AnswerSurfacePlan{StableAggregateFacts: []AnswerAggregateFact{{
		Kind: AnswerAggregateScalar, Label: "requested scalar", Value: "3.636",
		Dimensions: []AnswerAggregateDimension{{Name: "origin", Value: "runtime_artifact"}},
	}}}

	projectTypedTraceAnswerAuthority(plan, &rm, typedTraceProjectionLedgerForAggregateAuthorityTest())

	if len(plan.StableAggregateFacts) != 0 {
		t.Fatalf("bounded fact-set prompt must use deterministic trace rows instead of model restatements, got %+v", plan.StableAggregateFacts)
	}
}

func TestProjectTypedTraceAnswerAuthorityDoesNotWithholdNonCausalAggregateWithoutBoundedProfile(t *testing.T) {
	rm := RequestModel{Intent: IntentExplain, Scenario: ScenarioGeneric}
	plan := &AnswerSurfacePlan{StableAggregateFacts: []AnswerAggregateFact{{
		Kind: AnswerAggregateScalar, Label: "requested scalar", Value: "3.636",
		Dimensions: []AnswerAggregateDimension{{Name: "origin", Value: "runtime_artifact"}},
	}}}
	ledger := ObservationLedger{Records: []ObservationRecord{{
		ID:        "trace_query:result#target_window_states",
		Origin:    AnswerEvidenceOriginRuntimeArtifact,
		Producer:  "trace_query",
		Predicate: "target_window_states",
		Subject:   "worker-200",
	}}}

	projectTypedTraceAnswerAuthority(plan, &rm, ledger)

	if len(plan.StableAggregateFacts) != 1 {
		t.Fatalf("legacy non-causal shape without a bounded profile must retain its compatibility handoff: %+v", plan.StableAggregateFacts)
	}
}

func TestProjectTypedTraceAnswerAuthorityNeedsDeterministicTraceRows(t *testing.T) {
	rm := RequestModel{Intent: IntentRootCause, Scenario: ScenarioPerformanceBottleneck}
	plan := &AnswerSurfacePlan{StableAggregateFacts: []AnswerAggregateFact{{
		Kind: AnswerAggregateScalar, Label: "log-only scalar", Value: "4",
		Dimensions: []AnswerAggregateDimension{{Name: "origin", Value: "runtime_artifact"}},
	}}}

	projectTypedTraceAnswerAuthority(plan, &rm, ObservationLedger{})

	if len(plan.StableAggregateFacts) != 1 {
		t.Fatalf("runtime artifacts without deterministic trace rows still need their aggregate handoff")
	}
}

func TestProjectDirectRuntimeObservationAuthorityWithholdsUnsupportedModelRestatement(t *testing.T) {
	rm := RequestModel{
		Intent: IntentRootCause,
		ExternalObservationPolicy: &ExternalObservationPolicy{
			CurrentSourceMode: ExternalObservationCurrentSourceExclude,
			ExclusionKind:     ExternalObservationSourceExclusionExplicitUserBoundary,
			SourceQuotes:      []string{"only analyze the log"},
			Confidence:        0.9,
		},
	}
	plan := &AnswerSurfacePlan{
		StableInvestigationReason: "caller supplied the bad receiver",
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind: AnswerAggregateBehaviorOutcome, Label: "caller ownership", Value: "nil receiver",
			Dimensions: []AnswerAggregateDimension{{Name: "origin", Value: "runtime_artifact"}},
		}},
	}
	ledger := ObservationLedger{Records: []ObservationRecord{{
		ID: "log:error:0", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "log_triage",
		ClaimAuthority: ObservationClaimAuthorityDirectObservation,
	}}}

	projectDirectRuntimeObservationAuthority(plan, &rm, ledger)

	if len(plan.StableAggregateFacts) != 0 {
		t.Fatalf("unsupported model restatement must leave the answer evidence pool: %+v", plan.StableAggregateFacts)
	}
	if plan.StableInvestigationReason == "" {
		t.Fatal("durable closure reason must remain for the transparent omission receipt and audit")
	}
}

func TestProjectDirectRuntimeObservationAuthorityPreservesTypedSupport(t *testing.T) {
	rm := RequestModel{
		Intent: IntentRootCause,
		ExternalObservationPolicy: &ExternalObservationPolicy{
			CurrentSourceMode: ExternalObservationCurrentSourceExclude,
			ExclusionKind:     ExternalObservationSourceExclusionExplicitUserBoundary,
			SourceQuotes:      []string{"only analyze the log"},
			Confidence:        0.9,
		},
	}
	plan := &AnswerSurfacePlan{StableAggregateFacts: []AnswerAggregateFact{{
		Kind: AnswerAggregateScalar, Label: "typed duration", Value: "7.000", Unit: "ms",
		SupportRefs: []string{"trace_query:window_stats:E7"},
	}}}
	ledger := ObservationLedger{Records: []ObservationRecord{{
		ID: "log:error:0", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "log_triage",
		ClaimAuthority: ObservationClaimAuthorityDirectObservation,
	}}}

	projectDirectRuntimeObservationAuthority(plan, &rm, ledger)

	if len(plan.StableAggregateFacts) != 1 || plan.StableAggregateFacts[0].Label != "typed duration" {
		t.Fatalf("independently typed aggregate support must survive: %+v", plan.StableAggregateFacts)
	}
}

func TestProjectDirectRuntimeObservationAuthorityNeedsDirectProducerRow(t *testing.T) {
	rm := RequestModel{
		Intent: IntentRootCause,
		ExternalObservationPolicy: &ExternalObservationPolicy{
			CurrentSourceMode: ExternalObservationCurrentSourceExclude,
			ExclusionKind:     ExternalObservationSourceExclusionExplicitUserBoundary,
			SourceQuotes:      []string{"only analyze the log"},
			Confidence:        0.9,
		},
	}
	plan := &AnswerSurfacePlan{StableAggregateFacts: []AnswerAggregateFact{{
		Kind: AnswerAggregateBehaviorOutcome, Label: "only retained hypothesis", Value: "possible",
	}}}
	ledger := ObservationLedger{Records: []ObservationRecord{{
		ID: "aggregate:0#runtime_artifact", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "aggregate_facts",
		ClaimAuthority: ObservationClaimAuthorityModelInference,
	}}}

	projectDirectRuntimeObservationAuthority(plan, &rm, ledger)

	if len(plan.StableAggregateFacts) != 1 {
		t.Fatalf("without direct producer rows, the model handoff remains the only available context: %+v", plan.StableAggregateFacts)
	}
}

func typedTraceProjectionLedgerForAggregateAuthorityTest() ObservationLedger {
	return ObservationLedger{Records: []ObservationRecord{{
		ID:              "trace_query:result#root_cause_rank:1",
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRolePrincipalAnswer,
		GroundingPolicy: ClaimGroundingHard,
		SourceRef: ObservationSourceRef{
			Kind: ObservationSourceRuntimeArtifact, ArtifactID: "customer.systrace", ArtifactKind: "trace",
		},
		Span:      ObservationSpan{StartTs: 10, EndTs: 10.02, LineStart: 1, LineEnd: 2},
		ClaimKey:  "root_cause_primary:worker-200",
		Predicate: "root_cause_primary",
		Subject:   "worker-200",
		Object:    "runnable",
		Value:     "7.000",
		Unit:      "ms",
		RichNotes: []string{"rank=1", "tier=primary", "chain_relevance=on_chain", "impact_ms=7.000", "effective_impact_ms=6.000", "fix_direction=scheduling_priority", "selected_window=10.000000..10.020000"},
	}}}
}
