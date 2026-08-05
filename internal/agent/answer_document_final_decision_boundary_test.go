package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestFinalCallChainEvidenceBoundaryIsLanguageAgnosticAndLast(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState("trace source to sink"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:        types.IntentTrace,
			Scenario:      types.ScenarioGeneric,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		}},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Final Call-Chain Evidence Boundary",
		"A call-site proves that edge, not the callee's body",
		"storage medium",
		"Class names, method names, comments, layer labels, and the wording of the request do not mint implementation authority",
		"say only that the chain reaches or invokes the endpoint",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("call-chain final boundary missing %q:\n%s", want, prompt)
		}
	}
	if strings.LastIndex(prompt, "## Final Call-Chain Evidence Boundary") < strings.LastIndex(prompt, "## Submission Checklist") {
		t.Fatalf("call-chain evidence boundary must follow generic structure guidance:\n%s", prompt)
	}
}

func TestFinalTraceDecisionBoundaryFollowsGenericGuidanceAndKeepsModelOwnership(t *testing.T) {
	ctx := answerDocCausalCeilingTestContext(true)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View: "frame_root_cause_bundle", FrameEvidenceStatus: "absent", CausalConclusion: "unproven",
		},
		Observations: []types.ObservationRecord{{
			ID:              "typed-seat",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef: types.ObservationSourceRef{
				Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "customer.systrace", ArtifactKind: "trace",
			},
			Span:      types.ObservationSpan{StartTs: 10, EndTs: 10.020, LineStart: 1, LineEnd: 2},
			ClaimKey:  "root_cause_primary:worker-200",
			Predicate: "root_cause_primary",
			Subject:   "worker-200",
			Object:    "runnable",
			Value:     "7.000",
			Unit:      "ms",
			RichNotes: []string{"rank=1", "tier=primary", "chain_relevance=on_chain", "impact_ms=7.000", "effective_impact_ms=6.000", "fix_direction=scheduling_priority", "selected_window=10.000000..10.020000"},
		}},
	}}})

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Final Trace Decision Boundary (Typed Facts; Model-Owned Conclusion)",
		"You own the diagnosis, prioritization, optimization direction, and wording",
		"causal_conclusion=`unproven`",
		"frame_evidence_status=`absent`",
		"cross_row_addition=`not_authorized_without_exact_typed_relation`",
		"does not prove physical independence",
		"does not prove synchronous blocking, lock ownership, post-wakeup preemption, or physical coupling",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("trace final boundary missing %q:\n%s", want, prompt)
		}
	}
	if strings.LastIndex(prompt, "## Final Trace Decision Boundary") < strings.LastIndex(prompt, "## Submission Checklist") {
		t.Fatalf("trace decision boundary must follow generic structure guidance:\n%s", prompt)
	}
	for _, forbidden := range []string{"the root cause is", "the primary cause is", "system-authored conclusion"} {
		if strings.Contains(strings.ToLower(renderAnswerDocTraceFinalDecisionBoundary(ctx)), forbidden) {
			t.Fatalf("typed boundary authored a conclusion phrase %q", forbidden)
		}
	}

	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet}
	bounded := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(bounded, "## Final Trace Decision Boundary") {
		t.Fatalf("bounded fact request was widened into trace synthesis:\n%s", bounded)
	}
}

func TestTypedTraceProjectionDoesNotReplayUnboundExplorerAggregateAsFact(t *testing.T) {
	ctx := answerDocCausalCeilingTestContext(true)
	ctx.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateGroupedCount,
		Label: "explorer-composed-root-ranking",
		Value: "4",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Dimensions: []types.AnswerAggregateDimension{{
			Name: "origin", Value: "runtime_artifact",
		}},
		Members: []string{"model-derived subtotal"},
	}})
	ctx.Mutable.RetainInvestigationAggregateFacts()
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View: "frame_root_cause_bundle", FrameEvidenceStatus: "absent", CausalConclusion: "unproven",
		},
		Observations: []types.ObservationRecord{{
			ID:              "typed-seat",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef: types.ObservationSourceRef{
				Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "customer.systrace", ArtifactKind: "trace",
			},
			Span:      types.ObservationSpan{StartTs: 10, EndTs: 10.020, LineStart: 1, LineEnd: 2},
			ClaimKey:  "root_cause_primary:worker-200",
			Predicate: "root_cause_primary",
			Subject:   "worker-200",
			Object:    "runnable",
			Value:     "7.000",
			Unit:      "ms",
			RichNotes: []string{"rank=1", "tier=primary", "chain_relevance=on_chain", "impact_ms=7.000", "effective_impact_ms=6.000", "fix_direction=scheduling_priority", "selected_window=10.000000..10.020000"},
		}},
	}}})

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "## Final Trace Decision Boundary") {
		t.Fatalf("typed trace decision authority disappeared:\n%s", prompt)
	}
	for _, forbidden := range []string{"explorer-composed-root-ranking", "model-derived subtotal", "aggregate_facts[0]#runtime_artifact"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("unbound explorer aggregate leaked into factual finalizer handoff via %q:\n%s", forbidden, prompt)
		}
	}
	if got := ctx.Mutable.StableInvestigationAggregateFacts(); len(got) != 1 {
		t.Fatalf("raw model aggregate must remain available for audit, got %+v", got)
	}
}
