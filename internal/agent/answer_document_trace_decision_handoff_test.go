package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceDecisionHandoffLeavesConclusionToModelAndCarriesBothAxes(t *testing.T) {
	inside := true
	projection := types.TraceCausalProjection{
		ArtifactLabel: "customer.systrace",
		WindowStartTs: 10,
		WindowEndTs:   10.11494,
		TargetStateAccount: &types.TraceCausalProjectionTargetStateAccount{
			Subject: "target-100", RunningMS: 26.946, RunnableMS: 3.636,
			SleepMS: 84.358, TotalMS: 114.940,
		},
		WakeupPath: []string{"ThreadPool-300", "Network-200", "Cookie-150", "target-100"},
		OnChainCauses: []types.TraceCausalProjectionNode{
			{
				EvidenceID: "sleep", Subject: "Cookie-150", Object: "s_sleep",
				StateKind: "s_sleep", ImpactMS: 44.836, CumulativeImpactMS: 47.282,
				ChainDepth: 2, MergedCount: 7, MergedMaxMS: 10.976, WithinRequestedWindow: &inside,
			},
			{
				EvidenceID: "priced-only", Subject: "Cookie-150", Object: "priority_inversion_candidate",
				ImpactMS: 23.994, EffectiveImpactMS: 23.994, Rank: 1,
				ChainDepth: 2, PriorityInversionCandidate: true,
				FixDirection: "lock_priority", WithinRequestedWindow: &inside,
			},
		},
		RankedSeats: []types.TraceCausalProjectionNode{
			{
				EvidenceID: "priced-only", Subject: "Cookie-150", Object: "priority_inversion_candidate",
				ImpactMS: 23.994, EffectiveImpactMS: 23.994, Rank: 1,
				ChainDepth: 2, PriorityInversionCandidate: true,
				FixDirection: "lock_priority", WithinRequestedWindow: &inside,
			},
			{
				EvidenceID: "supply", Subject: "target-100", Object: "running",
				StateKind: "running", ImpactMS: 26.946, EffectiveImpactMS: 10.331, Rank: 2,
				FixDirection: "frequency_thermal", SystemSupplement: true,
				WithinRequestedWindow: &inside,
			},
		},
		BusinessSpanMentions: []types.TraceCausalProjectionBusinessSpanMention{{
			Subject: "worker-400", Name: "ParseCards", Count: 12,
			TotalMS: 18.5, MaxMS: 3.2, StartLine: 50, EndLine: 90, Basis: "chain_member",
		}},
	}
	got := renderAnswerDocTraceDecisionHandoffSet(
		types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}},
		runtimeTraceGuidanceView{CausalUnproven: true, FrameEvidenceStatus: "absent"},
	)
	for _, want := range []string{
		"Model Owns The Conclusion",
		"You own the diagnosis and final recommendation",
		"do not merely repeat the rows",
		"two independent decision axes that are actually available",
		"causal_conclusion=`unproven`",
		"frame_evidence_status=`absent`",
		"phase_semantics: `pre_wakeup_dependency`",
		"not the consumer's post-wakeup runnable/dispatch delay",
		"never infer that a CFS dependency preempted an RT consumer after wake",
		"candidate flag alone proves neither a lock holder/waiter relation nor post-wakeup preemption",
		"target_state_symptom: subject=`target-100`",
		"elected_wakeup_path=`ThreadPool-300 -> Network-200 -> Cookie-150 -> target-100`",
		"axis_A_actual_occupancy_candidates",
		"subject=`Cookie-150`; kind=`s_sleep`; window_projection=44.836ms",
		"span=`ParseCards`; total=18.500ms; occurrences=12; member_max=3.200ms",
		"axis_B_existing_rule_eliminable",
		"rank=#1; subject=`Cookie-150`; kind=`priority_inversion_candidate`; effective_attribution=23.994ms; fix_direction=`lock_priority`",
		"impact_phase=`pre_wakeup_dependency`",
		"priority_candidate_scope=`dependency_scheduler_supply`",
		"post_wakeup_preemption_authority=`not_provided_by_this_seat`",
		"holder_waiter_authority=`not_provided_by_candidate_flag`",
		"rank=#2; subject=`target-100`; kind=`running`; effective_attribution=10.331ms; fix_direction=`frequency_thermal`",
		"source_lane=`deterministic_system_supplement`",
		"cross_row_additivity=`forbidden`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("decision handoff missing %q:\n%s", want, got)
		}
	}
	axisA := got[strings.Index(got, "axis_A_actual_occupancy_candidates"):strings.Index(got, "axis_B_existing_rule_eliminable")]
	if strings.Contains(axisA, "kind=`priority_inversion_candidate`") {
		t.Fatalf("priced composite seat leaked into actual-time axis:\n%s", axisA)
	}
}

func TestTraceDecisionHandoffDoesNotGuessPhaseFromStateOrPriorityWords(t *testing.T) {
	projection := types.TraceCausalProjection{
		RankedSeats: []types.TraceCausalProjectionNode{{
			EvidenceID: "legacy-depth-zero", Subject: "worker-20", StateKind: "runnable",
			PriorityInversionCandidate: true, Rank: 1, ImpactMS: 4, EffectiveImpactMS: 4,
		}},
	}
	got := renderAnswerDocTraceDecisionHandoffSet(
		types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}},
		runtimeTraceGuidanceView{},
	)
	for _, forbidden := range []string{
		"phase_semantics:", "impact_phase=", "priority_candidate_scope=",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("zero/absent chain depth guessed a phase from non-phase fields %q:\n%s", forbidden, got)
		}
	}
}

func TestTraceDecisionHandoffFiltersOutsideWindowAndDoesNotInventAnAnswer(t *testing.T) {
	outside := false
	projection := types.TraceCausalProjection{
		OnChainCauses: []types.TraceCausalProjectionNode{{
			EvidenceID: "outside", Subject: "outside-worker", StateKind: "runnable",
			ImpactMS: 99, WithinRequestedWindow: &outside,
		}},
		RankedSeats: []types.TraceCausalProjectionNode{{
			EvidenceID: "outside-seat", Subject: "outside-worker", Object: "running",
			Rank: 1, EffectiveImpactMS: 99, WithinRequestedWindow: &outside,
		}},
		WakeupPath: []string{"upstream", "target"},
	}
	got := renderAnswerDocTraceDecisionHandoffSet(
		types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}},
		runtimeTraceGuidanceView{},
	)
	if strings.Contains(got, "outside-worker") {
		t.Fatalf("outside-window candidate leaked into decision inputs:\n%s", got)
	}
	for _, forbidden := range []string{
		"the root cause is", "the primary cause is", "system conclusion",
	} {
		if strings.Contains(strings.ToLower(got), forbidden) {
			t.Fatalf("prompt handoff minted a conclusion phrase %q:\n%s", forbidden, got)
		}
	}
}

func TestFinalizerInitialInstructionWiresTraceDecisionHandoff(t *testing.T) {
	ctx := answerDocCausalCeilingTestContext(true)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		RawRef:   "[trace_query params: view=root_cause_rank source=path path=/tmp/customer.systrace origin=runtime_artifact artifact_kind=trace]",
		Observations: []types.ObservationRecord{{
			ID:              "root-seat",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef: types.ObservationSourceRef{
				Kind:         types.ObservationSourceRuntimeArtifact,
				ArtifactID:   "customer.systrace",
				ArtifactKind: "trace",
				Path:         "/tmp/customer.systrace",
			},
			Span:      types.ObservationSpan{StartTs: 10, EndTs: 10.020, LineStart: 100, LineEnd: 110},
			ClaimKey:  "root_cause_primary:worker-200",
			Predicate: "root_cause_primary",
			Subject:   "worker-200",
			Object:    "runnable",
			Value:     "7.000",
			Unit:      "ms",
			Summary:   "root_cause_rank rank=1 chain_relevance=on_chain impact=7.000ms",
			RichNotes: []string{
				"rank=1", "tier=primary", "chain_relevance=on_chain",
				"dominant_state=runnable", "impact_ms=7.000",
				"effective_impact_ms=6.000", "fix_direction=scheduling_priority",
				"selected_window=10.000000..10.020000",
			},
			SupportRefs: []string{"/tmp/customer.systrace:100-110"},
		}},
	}}})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Trace Decision Inputs (Model Owns The Conclusion)",
		"axis_A_actual_occupancy_candidates",
		"subject=`worker-200`; kind=`runnable`; window_projection=7.000ms",
		"axis_B_existing_rule_eliminable",
		"effective_attribution=6.000ms",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("finalizer production prompt missed decision handoff %q:\n%s", want, prompt)
		}
	}

	// The handoff shares the typed full-report authority with the system
	// projection. Merely having a causal row in exploration must never widen a
	// bounded fact request into a root-cause synthesis prompt.
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
		Scope: types.RuntimeQuestionScopeBoundedFactSet,
	}
	boundedPrompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(boundedPrompt, "## Trace Decision Inputs (Model Owns The Conclusion)") {
		t.Fatalf("bounded runtime fact request was widened by collected causal rows:\n%s", boundedPrompt)
	}
	start, end := 10.0, 10.020
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &start,
		TimeEnd:        &end,
		SourceQuote:    "10.000000..10.020000",
	}
	windowPrompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(windowPrompt, "## Trace Decision Inputs (Model Owns The Conclusion)") {
		t.Fatalf("explicit typed window did not retain decision-input authority:\n%s", windowPrompt)
	}
}
