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
		"principal_trace_summary_contract",
		"`trace_causal_claim_caliber`",
		"`no_causal_conclusion|bounded_window_candidate`",
		"No conclusion is inferred from prose or written for you",
		"causal_conclusion=`unproven`",
		"frame_evidence_status=`absent`",
		"target_direct_blocking_authority=`unavailable_without_typed_target`",
		"fix_direction_summary_authority=`single_published_leader_only`",
		"direction_subtotal_authority=`not_provided_without_exact_fold`",
		"leader_subject=`worker-200`",
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

func TestTraceFinalCompactAuthorityLedgerSeparatesWakeupFromTypedBlockingAndDirectionFolds(t *testing.T) {
	target := "ui-100"
	inWindow := true
	projection := types.TraceCausalProjection{
		ArtifactLabel:      "customer.systrace",
		TargetStateAccount: &types.TraceCausalProjectionTargetStateAccount{Subject: target, TotalMS: 20},
		WakeupPath:         []string{"worker-200", target},
		RankedSeats: []types.TraceCausalProjectionNode{
			{EvidenceID: "rank-1", Subject: "worker-200", Rank: 1, EffectiveImpactMS: 8, FixDirection: "io_dependency", WithinRequestedWindow: &inWindow},
			{EvidenceID: "rank-2", Subject: "worker-201", Rank: 2, EffectiveImpactMS: 4, FixDirection: "io_dependency", WithinRequestedWindow: &inWindow},
			{EvidenceID: "rank-3", Subject: target, Rank: 3, EffectiveImpactMS: 3, FixDirection: "scheduling", BlockingKind: "lock_contention", BlockingPeer: "holder-300", WithinRequestedWindow: &inWindow},
		},
	}
	got := renderTraceFinalCompactAuthorityLedger(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}})
	for _, want := range []string{
		"target_direct_blocking_authority=`typed_waiter_holder`",
		"waiter=`ui-100`",
		"holder=`holder-300`",
		"blocking_kind=`lock_contention`",
		"fix_direction=`io_dependency`; leader_rank=#1; leader_subject=`worker-200`; leader_effective_attribution=8.000ms",
		"fix_direction=`scheduling`; leader_rank=#3; leader_subject=`ui-100`; leader_effective_attribution=3.000ms",
		"direction_subtotal_authority=`not_provided_without_exact_fold`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("compact authority ledger missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "worker-201") {
		t.Fatalf("same-direction non-leading seat must not be presented as a direction subtotal member:\n%s", got)
	}

	projection.RankedSeats[2].BlockingKind = ""
	projection.RankedSeats[2].BlockingPeer = ""
	got = renderTraceFinalCompactAuthorityLedger(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}})
	if !strings.Contains(got, "target_direct_blocking_authority=`not_provided_by_projection`") ||
		!strings.Contains(got, "wakeup_path_blocking_authority=`not_implied`") {
		t.Fatalf("wakeup path without typed blocking row must stay below blocker authority:\n%s", got)
	}
}

func TestTraceFinalDecisionLedgerPrefersRequestedWindowBoardAndCarriesPreWakeupPhase(t *testing.T) {
	projection := types.TraceCausalProjection{
		ArtifactLabel: "customer.systrace",
		WindowStartTs: 10,
		WindowEndTs:   10.1,
		RankedSeats: []types.TraceCausalProjectionNode{
			{
				EvidenceID: "micro-io", Subject: "micro-worker", Rank: 1,
				EffectiveImpactMS: 2.202, FixDirection: "io_dependency",
				RankQueryWindowStartTs: 10.02, RankQueryWindowEndTs: 10.07,
			},
			{
				EvidenceID: "full-io", Subject: "full-worker", Rank: 3,
				EffectiveImpactMS: 10.433, FixDirection: "io_dependency", ChainDepth: 2,
				RankQueryWindowStartTs: 10, RankQueryWindowEndTs: 10.1,
			},
		},
	}
	got := renderTraceFinalCompactAuthorityLedger(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}})
	for _, want := range []string{
		"leader_subject=`full-worker`",
		"leader_effective_attribution=10.433ms",
		"query_window=`10.000000..10.100000`",
		"window_role=`requested_or_elected_window`",
		"impact_phase=`pre_wakeup_dependency`",
		"post_wakeup_delay_authority=`not_provided_by_this_seat`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("requested-window compact leader missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "micro-worker") || strings.Contains(got, "2.202") {
		t.Fatalf("interior drilldown seat displaced requested-window direction authority:\n%s", got)
	}
}

func TestTraceFinalDecisionLedgerDirectionLeaderPrefersPublishedOnChainMaximumOverAdjacentRank(t *testing.T) {
	chain := types.TraceCausalProjectionNode{
		EvidenceID: "full-chain-io", Subject: "chain-worker", Rank: 3,
		EffectiveImpactMS: 10.433, FixDirection: "io_dependency", ChainRelevance: "on_chain",
		RankQueryWindowStartTs: 10, RankQueryWindowEndTs: 10.1,
	}
	adjacent := types.TraceCausalProjectionNode{
		EvidenceID: "adjacent-io", Subject: "adjacent-worker", Rank: 1,
		EffectiveImpactMS: 0.171, FixDirection: "io_dependency", ChainRelevance: "adjacent",
		RankQueryWindowStartTs: 10, RankQueryWindowEndTs: 10.1,
	}
	projection := types.TraceCausalProjection{
		ArtifactLabel: "customer.systrace", WindowStartTs: 10, WindowEndTs: 10.1,
		RankedSeats:    []types.TraceCausalProjectionNode{adjacent, chain},
		OnChainCauses:  []types.TraceCausalProjectionNode{chain},
		AdjacentCauses: []types.TraceCausalProjectionNode{adjacent},
	}
	got := renderTraceFinalCompactAuthorityLedger(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}})
	for _, want := range []string{
		"leader_rank=#3", "leader_subject=`chain-worker`", "leader_effective_attribution=10.433ms",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("published on-chain direction maximum missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "adjacent-worker") || strings.Contains(got, "0.171") {
		t.Fatalf("adjacent local rank displaced the published on-chain direction leader:\n%s", got)
	}
}

func TestTraceFinalTargetBlockingRelationsRespectsHolderSubjectRole(t *testing.T) {
	target := "ui-100"
	projection := types.TraceCausalProjection{OnChainCauses: []types.TraceCausalProjectionNode{
		{
			EvidenceID:              "holder-row",
			Subject:                 "holder-400",
			BlockingKind:            "monitor_contention",
			BlockingPeer:            target,
			BlockingSubjectIsHolder: true,
		},
		{
			EvidenceID:              "other-holder-row",
			Subject:                 "holder-500",
			BlockingKind:            "monitor_contention",
			BlockingPeer:            "other-waiter",
			BlockingSubjectIsHolder: true,
		},
	}}
	relations := traceFinalTargetBlockingRelations(projection, target)
	if len(relations) != 1 {
		t.Fatalf("target should bind only its exact typed holder row: %+v", relations)
	}
	if relations[0].waiter != target || relations[0].holder != "holder-400" || relations[0].kind != "monitor_contention" {
		t.Fatalf("holder-subject relation reversed or widened: %+v", relations[0])
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
