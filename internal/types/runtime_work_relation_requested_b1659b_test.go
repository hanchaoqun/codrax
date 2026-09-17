package types

import (
	"reflect"
	"testing"
)

func b1659bSemanticWorkResult() ToolResult {
	return ToolResult{
		ToolName: "trace_query", Success: true,
		Observations: []ObservationRecord{{
			ID:     "trace_query:b1659b#trace_semantic_span:1",
			Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			Role: AnswerAggregateRoleSupportingCoverage, GroundingPolicy: ClaimGroundingHard,
			SourceRef: ObservationSourceRef{
				Kind: ObservationSourceRuntimeArtifact, Path: "/traces/work.ftrace",
				ArtifactID: "attached_trace", ArtifactKind: "trace",
			},
			Span:      ObservationSpan{LineStart: 10, LineEnd: 11, StartTs: 5, EndTs: 5.002},
			Predicate: "trace_semantic_span", ClaimKey: "trace_semantic_span:class_verification",
			Subject: "worker-7", Object: "class_verification", Value: "2.000", Unit: "ms",
			RichNotes: []string{
				"span_name=VerifyClass Example", "semantic_class=class_verification",
				"chain_relevance=on_chain", "causality=on_wakeup_chain",
				"on_chain_basis=" + TraceCausalOnChainBasisHostWakeupEdgeSpan,
			},
		}},
	}
}

func b1659bLegacyWorkRequest(active, required bool) RequestModel {
	return RequestModel{RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: active,
		Dimensions: []RequestedAnswerDimension{{
			Index: 1, Role: RequestedAnswerDimensionRuntimeWorkRelation, Required: required,
		}},
	}}
}

func TestB1659bRuntimeWorkRelationRequestedUsesOnlyTypedCarriers(t *testing.T) {
	legacyWithFalseProfile := b1659bLegacyWorkRequest(true, true)
	legacyWithFalseProfile.RuntimeQuestionProfile = &RuntimeQuestionProfile{}
	otherRole := b1659bLegacyWorkRequest(true, true)
	otherRole.RequestedAnswerDimensions.Dimensions[0].Role = RequestedAnswerDimensionRelationPath
	otherRole.RequestedAnswerDimensions.Dimensions[0].Label = "runtime_work_relation work-to-target relation"
	otherRole.RequestedAnswerDimensions.Dimensions[0].SourceQuote = "runtime_work_relation"
	for _, tc := range []struct {
		name string
		rm   RequestModel
		want bool
	}{
		{"none", RequestModel{}, false},
		{"profile_true", RequestModel{RuntimeQuestionProfile: &RuntimeQuestionProfile{RuntimeWorkRelationRequested: true}}, true},
		{"profile_false", RequestModel{RuntimeQuestionProfile: &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeRelationAnalysis}}, false},
		{"profile_prose", RequestModel{RuntimeQuestionProfile: &RuntimeQuestionProfile{
			SourceQuote: "runtime_work_relation", Rationale: "work-to-target relation requested",
		}}, false},
		{"legacy_required", b1659bLegacyWorkRequest(true, true), true},
		{"legacy_even_with_false_profile", legacyWithFalseProfile, true},
		{"legacy_inactive", b1659bLegacyWorkRequest(false, true), false},
		{"legacy_optional", b1659bLegacyWorkRequest(true, false), false},
		{"legacy_empty", RequestModel{RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{IsDimensionedAnswer: true}}, false},
		{"other_role_with_matching_prose", otherRole, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := RuntimeWorkRelationRequested(tc.rm); got != tc.want {
				t.Fatalf("requested=%t want=%t", got, tc.want)
			}
		})
	}
}

// Exercise both public context compilers, including the existing observation
// ledger and semantic projection. The test does not inject a pre-bound receipt
// or manufacture a contract to bypass the evidence lane.
func TestB1659bRuntimeWorkRequestCompilesIdenticalAgentBusContract(t *testing.T) {
	result := b1659bSemanticWorkResult()
	want := BuildRuntimeWorkRelationContract(ObservationLedgerInput{ToolResults: []ToolResult{result}}, true)
	if want == nil || len(want.Rows) != 1 || want.Rows[0].WorkLabel != "VerifyClass Example" {
		t.Fatalf("typed semantic-work fixture must compile one real row, got %+v", want)
	}
	for _, tc := range []struct {
		name      string
		rm        RequestModel
		requested bool
	}{
		{"profile", RequestModel{RuntimeQuestionProfile: &RuntimeQuestionProfile{
			Scope: RuntimeQuestionScopeRelationAnalysis, RuntimeWorkRelationRequested: true,
		}}, true},
		{"legacy_required", b1659bLegacyWorkRequest(true, true), true},
		{"legacy_inactive", b1659bLegacyWorkRequest(false, true), false},
		{"legacy_optional", b1659bLegacyWorkRequest(true, false), false},
		{"none", RequestModel{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := NewMutableState("unrelated model prose")
			state.AppendDispatchToolResult(result)
			ir := &AnalysisIR{RequestModel: tc.rm}
			agentView := BuildAnswerSemanticViewForAgentContext(&AgentContext{Mutable: state, AnalysisIR: ir})
			busView := BuildAnswerSemanticViewForBusContext(&BusContext{Mutable: state, AnalysisIR: ir})
			for surface, view := range map[string]*AnswerSemanticView{"agent": agentView, "bus": busView} {
				if view == nil {
					t.Fatalf("%s view missing", surface)
				}
				if !tc.requested {
					if view.RuntimeWorkRelationContract != nil {
						t.Errorf("%s unrequested work relation acquired a contract: %+v", surface, view.RuntimeWorkRelationContract)
					}
					continue
				}
				if !reflect.DeepEqual(view.RuntimeWorkRelationContract, want) {
					t.Errorf("%s requested semantic-work contract = %+v, want %+v", surface, view.RuntimeWorkRelationContract, want)
				}
			}
		})
	}
}

func TestB1659bRuntimeWorkRequestWithoutSemanticRowsDoesNotMintEvidence(t *testing.T) {
	scheduler := b1659bSemanticWorkResult()
	scheduler.Observations[0].Predicate = "root_cause_primary"
	scheduler.Observations[0].ClaimKey = "root_cause_primary"
	scheduler.Observations[0].Object = "runnable_wait"
	scheduler.Observations[0].RichNotes = []string{"rank=1", "type=runnable_wait", "chain_relevance=on_chain"}
	for _, request := range []RequestModel{
		{RuntimeQuestionProfile: &RuntimeQuestionProfile{RuntimeWorkRelationRequested: true}},
		b1659bLegacyWorkRequest(true, true),
	} {
		for _, tc := range []struct {
			name    string
			results []ToolResult
		}{
			{"no_observations", nil},
			{"scheduler_only", []ToolResult{scheduler}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				state := NewMutableState("runtime_work_relation")
				for _, result := range tc.results {
					state.AppendDispatchToolResult(result)
				}
				ir := &AnalysisIR{RequestModel: request}
				views := []*AnswerSemanticView{
					BuildAnswerSemanticViewForAgentContext(&AgentContext{AnalysisIR: ir, Mutable: state}),
					BuildAnswerSemanticViewForBusContext(&BusContext{AnalysisIR: ir, Mutable: state}),
				}
				for _, view := range views {
					if view == nil || view.RuntimeWorkRelationContract != nil {
						t.Fatalf("request alone must not invent semantic-work rows: %+v", view)
					}
				}
			})
		}
	}
}
