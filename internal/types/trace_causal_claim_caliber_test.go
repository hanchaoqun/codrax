package types

import "testing"

func traceCausalClaimTestRecord(id string) ObservationRecord {
	return ObservationRecord{
		ID:              id,
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRolePrincipalAnswer,
		GroundingPolicy: ClaimGroundingHard,
		SourceRef: ObservationSourceRef{
			Kind: ObservationSourceRuntimeArtifact, ArtifactID: "customer.systrace", ArtifactKind: "trace",
		},
		Span:      ObservationSpan{StartTs: 10, EndTs: 10.020, LineStart: 1, LineEnd: 2},
		ClaimKey:  "root_cause_primary:worker-200",
		Predicate: "root_cause_primary",
		Subject:   "worker-200",
		Object:    "runnable",
		Value:     "7.000",
		Unit:      "ms",
		RichNotes: []string{"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "impact_ms=7.000", "effective_impact_ms=6.000", "selected_window=10.000000..10.020000"},
	}
}

func traceCausalClaimWindowRequest() *RequestModel {
	start, end := 10.0, 10.020
	return &RequestModel{RuntimeArtifactScopeProfile: &RuntimeArtifactScopeProfile{
		RequestedScope: RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &start,
		TimeEnd:        &end,
		SourceQuote:    "10.000..10.020",
	}}
}

func TestBuildTraceCausalClaimContractFrameAbsentCapsAtBoundedCandidate(t *testing.T) {
	contract := BuildTraceCausalClaimContract(ObservationLedgerInput{
		RequestModel: traceCausalClaimWindowRequest(),
		ToolResults: []ToolResult{{
			ToolName: "trace_query", Success: true,
			TraceEvidenceAuthority: &TraceEvidenceAuthority{
				TypedCausalRowCount: 1, FrameEvidenceStatus: "absent", CausalConclusion: "unproven",
			},
			Observations: []ObservationRecord{traceCausalClaimTestRecord("lead")},
		}},
	})
	if contract == nil || contract.Ceiling != TraceCausalClaimBoundedWindow {
		t.Fatalf("absent frame evidence must cap the model claim at bounded candidate: %+v", contract)
	}
	if !contract.Allows(TraceCausalClaimNoConclusion) || !contract.Allows(TraceCausalClaimBoundedWindow) ||
		contract.Allows(TraceCausalClaimTypedChain) || contract.Allows(TraceCausalClaimTypedFrame) {
		t.Fatalf("unexpected absent-frame allowed set: %+v", contract)
	}
}

func TestBuildTraceCausalClaimContractIgnoresUnrelatedEarlierUnprovenProbe(t *testing.T) {
	contract := BuildTraceCausalClaimContract(ObservationLedgerInput{
		RequestModel: traceCausalClaimWindowRequest(),
		ToolResults: []ToolResult{
			{
				ToolName: "trace_query", Success: true,
				TraceEvidenceAuthority: &TraceEvidenceAuthority{CausalConclusion: "unproven"},
			},
			{
				ToolName: "trace_query", Success: true,
				TraceEvidenceAuthority: &TraceEvidenceAuthority{TypedCausalRowCount: 1, CausalConclusion: "bounded_by_typed_rows"},
				Observations:           []ObservationRecord{traceCausalClaimTestRecord("proven-lead")},
			},
		},
	})
	if contract == nil || contract.Ceiling != TraceCausalClaimTypedChain || !contract.Allows(TraceCausalClaimTypedChain) {
		t.Fatalf("unrelated earlier miss must not lower the elected typed chain seat: %+v", contract)
	}
}

func TestBuildTraceCausalClaimContractAllowsTypedFrameOnlyWithPresentFrameAuthority(t *testing.T) {
	contract := BuildTraceCausalClaimContract(ObservationLedgerInput{
		RequestModel: traceCausalClaimWindowRequest(),
		ToolResults: []ToolResult{{
			ToolName: "trace_query", Success: true,
			TraceEvidenceAuthority: &TraceEvidenceAuthority{
				TypedCausalRowCount: 1, FrameEvidenceStatus: "present", CausalConclusion: "bounded_by_typed_rows",
			},
			Observations: []ObservationRecord{traceCausalClaimTestRecord("frame-lead")},
		}},
	})
	if contract == nil || contract.Ceiling != TraceCausalClaimTypedFrame || !contract.Allows(TraceCausalClaimTypedFrame) {
		t.Fatalf("present frame authority should permit the model to declare typed frame causality: %+v", contract)
	}
}

func TestBuildTraceCausalClaimContractDoesNotActivateForBoundedFactQuery(t *testing.T) {
	rm := &RequestModel{RuntimeQuestionProfile: &RuntimeQuestionProfile{
		Scope:        RuntimeQuestionScopeBoundedFactSet,
		FactFamilies: []RuntimeQuestionFactFamily{RuntimeQuestionFactTargetSchedulerState},
	}}
	contract := BuildTraceCausalClaimContract(ObservationLedgerInput{
		RequestModel: rm,
		ToolResults: []ToolResult{{
			ToolName: "trace_query", Success: true,
			TraceEvidenceAuthority: &TraceEvidenceAuthority{TypedCausalRowCount: 1, CausalConclusion: "bounded_by_typed_rows"},
			Observations:           []ObservationRecord{traceCausalClaimTestRecord("status-query-row")},
		}},
	})
	if contract != nil {
		t.Fatalf("bounded status/fact query must not inherit the full causal-report contract: %+v", contract)
	}
}
