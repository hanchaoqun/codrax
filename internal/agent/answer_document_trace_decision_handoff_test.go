package agent

import (
	"fmt"
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
			SleepMS: 84.358, TotalMS: 114.940, WindowStartTs: 10, WindowEndTs: 10.11494,
		},
		SelfRunnableTwoRulerAccountings: []types.TraceCausalProjectionSelfRunnableTwoRuler{{
			Subject: "target-100", WallEffsMS: []float64{2.2, 1.1}, WallRanks: []int{4, 9},
			WallSubtotalMS: 3.3, EdgeEffsMS: []float64{0.336}, EdgeRanks: []int{10}, EdgeSubtotalMS: 0.336,
		}},
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
		BackgroundCauses: []types.TraceCausalProjectionNode{{
			EvidenceID: "scheduler-pressure", Subject: "scheduler-demand", Object: "supply_pressure",
			ImpactMS: 3.5, Unit: "ms", SubjectKind: types.TraceCausalSubjectKindAggregateMetric,
			SystemSupplement: true, WithinRequestedWindow: &inside,
		}},
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
		"two distinct decision axes that are actually available",
		"this distinction is not a claim of physical independence",
		"Exhaustive-decomposition ceiling",
		"fully explained/composed by listed Axis B seats only when one exact typed additive carrier publishes that same subtotal",
		"remaining on-chain work as unpriced or unresolved instead of treating the arithmetic remainder as zero",
		"Do not compute a residual by adding or subtracting overlapping rows unless a typed partition authorizes that arithmetic",
		"causal_conclusion=`unproven`",
		"frame_evidence_status=`absent`",
		"frame_evidence_status_semantics=`no target-bound frame/deadline evidence was produced in the selected evidence`",
		"proves neither that a frame drop occurred nor that no frame drop occurred",
		"phase_semantics: `pre_wakeup_dependency`",
		"upstream on-chain work overlapping the downstream consumer's pre-wakeup interval",
		"does not prove that the consumer waited for this work, waited until it completed, or was directly blocked by it",
		"Use direct-blocker or completion-dependency wording only when a separate typed holder/waiter or blocking relation provides that authority",
		"owns no post-wakeup runnable/dispatch delay",
		"candidate flag alone proves neither a lock holder/waiter relation nor post-wakeup preemption",
		"target_state_symptom: subject=`target-100`",
		"sleep_cause_authority=`not_provided_by_target_state_account`",
		"zero_d_state_or_iowait_does_not_classify_sleep_reason=`true`",
		"cannot classify S-sleep as normal frame pacing, lock/condition waiting, IPC, timer/event waiting, or another cause",
		"selected_window_value_authority:",
		"copy the typed `target_state_symptom` values above",
		"Whole-attachment extent, a switch-in after the selected-window end, and pre-triage navigation hypotheses are different calibers",
		"reasoning guidance only; you still own the conclusion and wording",
		"partition_relation=`mutually_exclusive_and_additive_to_total`",
		"partition_addition_authority=`these_five_members_only`",
		"authorized_relation_fact: family=`self_runnable_two_ruler`",
		"self_wall_clock_seats=`#4:2.200ms,#9:1.100ms`",
		"self_wall_clock_subtotal=3.300ms",
		"wakeup_edge_seats=`#10:0.336ms`",
		"same_ruler_addition=`authorized_to_published_subtotal`",
		"cross_ruler_addition=`forbidden`",
		"cross_ruler_physical_relation=`unresolved`",
		"typed_relation_authority: relation_claim_copy=`{\"authority_id\":\"trace:self_runnable_two_ruler:",
		"typed_relation_authority: relation_claim_copy=`{\"authority_id\":\"trace:target_state_partition:",
		"\"member_refs\":[\"running\",\"runnable\",\"sleep\",\"d_state\",\"io_wait\"],\"physical_relation\":\"mutually_exclusive\",\"addition\":\"authorized_to_published_subtotal\"",
		"final_relation_claim_carrier:",
		"already carried automatically",
		"Prefer omitting optional `blocks[i].relation_claims`",
		"omission does not trigger a retry",
		"elected_wakeup_path=`ThreadPool-300 -> Network-200 -> Cookie-150 -> target-100`",
		"wakeup_path_semantics:",
		"does not prove that B synchronously blocked waiting for A",
		"Use stronger blocked-wait/holder wording only when a separate typed blocking or holder relation provides that authority",
		"axis_A_actual_occupancy_candidates",
		"subject=`Cookie-150`; kind=`s_sleep`; window_projection=44.836ms",
		"span=`ParseCards`; total=18.500ms; occurrences=12; member_max=3.200ms",
		"axis_B_existing_rule_eliminable",
		"rank=#1; subject=`Cookie-150`; kind=`priority_inversion_candidate`; effective_attribution=23.994ms; validation_direction=`priority_or_dependency_supply`",
		"impact_phase=`pre_wakeup_dependency`",
		"mechanism_ceiling=`on_chain_prewakeup_work_candidate_only`",
		"target_wait_for_work_authority=`not_provided_by_this_seat`",
		"work_completion_dependency_authority=`not_provided_by_this_seat`",
		"direct_blocking_authority=`not_provided_by_this_seat`",
		"claim_envelope=`measured_lower_priority_dependency_supply_candidate`",
		"candidate_mechanism_authority=`lower_priority_dependency_only`",
		"synchronous_blocker_authority=`not_provided_by_candidate_seat`",
		"priority_candidate_scope=`dependency_scheduler_supply_before_downstream_wakeup`",
		"post_wakeup_preemption_authority=`not_provided_by_this_seat`",
		"holder_waiter_authority=`not_provided_by_candidate_seat`",
		"conclusion_caliber=`validation_candidate`",
		"rank=#2; subject=`target-100`; kind=`running`; effective_attribution=10.331ms; fix_direction=`frequency_thermal`",
		"source_lane=`deterministic_system_supplement`",
		"cross_row_additivity=`not_authorized_without_exact_pair_carrier`",
		"contextual_noncausal_rows",
		"lane=`background`; subject=`scheduler-demand`; kind=`supply_pressure`; value=3.500; unit=`ms`; caliber=`aggregate_context_non_target_wall_clock`",
		"target_causal_authority=`not_provided`; cross_axis_addition=`forbidden`; source_lane=`deterministic_system_supplement`",
		"relation_authority=`typed_pair_only`",
		"closed engine-state partition",
		"physical relationship is unresolved",
		"cross-row addition is not authorized",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("decision handoff missing %q:\n%s", want, got)
		}
	}
	axisA := got[strings.Index(got, "axis_A_actual_occupancy_candidates"):strings.Index(got, "axis_B_existing_rule_eliminable")]
	if strings.Contains(axisA, "kind=`priority_inversion_candidate`") {
		t.Fatalf("priced composite seat leaked into actual-time axis:\n%s", axisA)
	}
	if strings.Contains(got, "rank=#1; subject=`Cookie-150`; kind=`priority_inversion_candidate`; effective_attribution=23.994ms; fix_direction=`lock_priority`") {
		t.Fatalf("unconfirmed priority candidate leaked the registry lock bucket into model context:\n%s", got)
	}
	if strings.Contains(got, "closed four-state partition") {
		t.Fatalf("five engine lanes must not be taught as a four-state partition:\n%s", got)
	}
}

func TestTraceDecisionWritePhaseDoesNotOverrideTypedBlockingAuthority(t *testing.T) {
	plain := types.TraceCausalProjectionNode{ChainDepth: 1}
	var plainOut strings.Builder
	traceDecisionWritePhase(&plainOut, plain)
	if !strings.Contains(plainOut.String(), "mechanism_ceiling=`on_chain_prewakeup_work_candidate_only`") ||
		!strings.Contains(plainOut.String(), "direct_blocking_authority=`not_provided_by_this_seat`") {
		t.Fatalf("plain pre-wakeup work must carry its typed mechanism ceiling: %s", plainOut.String())
	}

	blocked := types.TraceCausalProjectionNode{ChainDepth: 1, BlockingKind: "lock_contention", BlockingPeer: "holder-2"}
	var blockedOut strings.Builder
	traceDecisionWritePhase(&blockedOut, blocked)
	if strings.Contains(blockedOut.String(), "direct_blocking_authority=`not_provided_by_this_seat`") ||
		strings.Contains(blockedOut.String(), "mechanism_ceiling=`on_chain_prewakeup_work_candidate_only`") {
		t.Fatalf("phase guidance must not erase a separate typed blocking relation: %s", blockedOut.String())
	}
	if !strings.Contains(blockedOut.String(), "post_wakeup_delay_authority=`not_provided_by_this_seat`") {
		t.Fatalf("a pre-wakeup blocking relation still owns no post-wakeup scheduling delay: %s", blockedOut.String())
	}
}

func TestTraceDecisionHandoffKeepsTypedPriorityCandidateCaliberOnProductionShapedRankSeat(t *testing.T) {
	inside := true
	projection := types.TraceCausalProjection{
		RankedSeats: []types.TraceCausalProjectionNode{
			{
				EvidenceID: "rank-production", Subject: "CookieMonsterCl-59843",
				TypeToken: "priority_inversion_candidate", StateKind: "runnable",
				Rank: 1, EffectiveImpactMS: 23.994, FixDirection: "lock_priority",
				ChainDepth: 1, WithinRequestedWindow: &inside,
			},
			{
				EvidenceID: "ordinary-runnable", Subject: "ordinary-worker-9",
				StateKind: "runnable", Rank: 2, EffectiveImpactMS: 3,
				FixDirection: "lock_priority", ChainDepth: 1, WithinRequestedWindow: &inside,
			},
		},
	}
	got := renderAnswerDocTraceDecisionHandoffSet(
		types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}},
		runtimeTraceGuidanceView{},
	)
	for _, want := range []string{
		"rank=#1; subject=`CookieMonsterCl-59843`; kind=`priority_inversion_candidate`; effective_attribution=23.994ms",
		"validation_direction=`priority_or_dependency_supply`",
		"claim_envelope=`measured_lower_priority_dependency_supply_candidate`",
		"candidate_mechanism_authority=`lower_priority_dependency_only`",
		"synchronous_blocker_authority=`not_provided_by_candidate_seat`",
		"holder_waiter_authority=`not_provided_by_candidate_seat`",
		"conclusion_caliber=`validation_candidate`",
		"priority_candidate_scope=`dependency_scheduler_supply_before_downstream_wakeup`",
		"post_wakeup_preemption_authority=`not_provided_by_this_seat`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("production-shaped priority candidate lost local caliber %q:\n%s", want, got)
		}
	}
	ordinary := got[strings.Index(got, "rank=#2; subject=`ordinary-worker-9`"):]
	if strings.Contains(ordinary, "candidate_mechanism_authority=") ||
		strings.Contains(ordinary, "holder_waiter_authority=") {
		t.Fatalf("ordinary runnable lock-priority row was inferred into a typed candidate:\n%s", ordinary)
	}
	if !strings.Contains(ordinary, "fix_direction=`lock_priority`") {
		t.Fatalf("ordinary non-candidate direction must remain verbatim:\n%s", ordinary)
	}
}

func TestTraceDecisionHandoffCarriesExactOverlapBeforeModelConclusion(t *testing.T) {
	inside := true
	board := func(node types.TraceCausalProjectionNode) types.TraceCausalProjectionNode {
		node.Object = "runnable_wait"
		node.EffectiveImpactPublished = true
		node.QueryWindowStartTs, node.QueryWindowEndTs = 10, 10.2
		node.RankQueryWindowStartTs, node.RankQueryWindowEndTs = 10, 10.2
		node.RankBoardTarget = "target-7"
		node.RankBoardParamsFingerprint = "board-a"
		return node
	}
	projection := types.TraceCausalProjection{
		WindowStartTs: 10, WindowEndTs: 10.2,
		RankedSeats: []types.TraceCausalProjectionNode{
			board(types.TraceCausalProjectionNode{EvidenceID: "rank-1", Subject: "worker-a", Rank: 1, EffectiveImpactMS: 23.994,
				FixDirection: "lock_priority", ChainRelevance: "on_chain",
				StartTs: 10.010, EndTs: 10.110, WithinRequestedWindow: &inside}),
			board(types.TraceCausalProjectionNode{EvidenceID: "rank-2", Subject: "worker-b", Rank: 2, EffectiveImpactMS: 19.041,
				FixDirection: "lock_priority", ChainRelevance: "on_chain",
				StartTs: 10.020, EndTs: 10.120, WithinRequestedWindow: &inside}),
			board(types.TraceCausalProjectionNode{EvidenceID: "rank-3", Subject: "worker-c", Rank: 3, EffectiveImpactMS: 7,
				FixDirection: "io_dependency", ChainRelevance: "on_chain",
				StartTs: 10.030, EndTs: 10.090, WithinRequestedWindow: &inside}),
			board(types.TraceCausalProjectionNode{EvidenceID: "rank-4", Subject: "worker-d", Rank: 4, EffectiveImpactMS: 6,
				FixDirection: "lock_priority", ChainRelevance: "on_chain",
				StartTs: 10.130, EndTs: 10.150, WithinRequestedWindow: &inside}),
			board(types.TraceCausalProjectionNode{EvidenceID: "rank-5", Subject: "worker-e", Rank: 5, EffectiveImpactMS: 5,
				FixDirection: "lock_priority", ChainRelevance: "adjacent",
				StartTs: 10.030, EndTs: 10.080, WithinRequestedWindow: &inside}),
		},
	}
	leftRef := types.TraceAnswerRelationMemberRef(projection.RankedSeats[0])
	rightRef := types.TraceAnswerRelationMemberRef(projection.RankedSeats[1])
	got := renderAnswerDocTraceDecisionHandoffSet(
		types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}},
		runtimeTraceGuidanceView{},
	)
	for _, want := range []string{
		"typed_relation_authority: relation_claim_copy=`{\"authority_id\":\"trace:overlapping_members:",
		fmt.Sprintf("\"member_refs\":[\"%s\",\"%s\"]", leftRef, rightRef),
		fmt.Sprintf("member_values=`%s:23.994ms,%s:19.041ms`", leftRef, rightRef),
		"fix_direction=`lock_priority`",
		"chain_lane=`on_chain`",
		"\"physical_relation\":\"overlap\"",
		"measured_envelope_overlap=90.000ms",
		"members_independent=`false`",
		"\"addition\":\"forbidden\"",
		"comparison_rule=`max_member_only_no_subtotal`",
		"comparison_value=23.994ms",
		"arithmetic_instruction=`preserve_each_member_but_never_publish_their_sum_as_a_total_or_eliminable_amount`",
		fmt.Sprintf("relation_member_ref=`%s`", leftRef),
		fmt.Sprintf("relation_member_ref=`%s`", rightRef),
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed pre-final overlap relation missing %q:\n%s", want, got)
		}
	}
	authorityStart := strings.Index(got, "typed_relation_authority: relation_claim_copy=`{\"authority_id\":\"trace:overlapping_members:")
	authorityEnd := strings.Index(got[authorityStart:], "final_relation_claim_carrier:")
	authoritySection := got[authorityStart : authorityStart+authorityEnd]
	if strings.Contains(authoritySection, types.TraceAnswerRelationMemberRef(projection.RankedSeats[2])) ||
		strings.Contains(authoritySection, types.TraceAnswerRelationMemberRef(projection.RankedSeats[3])) ||
		strings.Contains(authoritySection, types.TraceAnswerRelationMemberRef(projection.RankedSeats[4])) {
		t.Fatalf("cross-direction pair must not add prompt noise:\n%s", got)
	}
	if strings.Index(got, "typed_relation_authority: relation_claim_copy=`{\"authority_id\":\"trace:overlapping_members:") > strings.Index(got, "axis_B_existing_rule_eliminable") {
		t.Fatalf("typed overlap authority must precede the detailed seats:\n%s", got)
	}
}

func TestTraceDecisionContextCapKeepsTypedBackgroundLane(t *testing.T) {
	inside := true
	projection := types.TraceCausalProjection{WindowStartTs: 10, WindowEndTs: 10.11494}
	for i := 0; i < 6; i++ {
		projection.AdjacentCauses = append(projection.AdjacentCauses, types.TraceCausalProjectionNode{
			EvidenceID: fmt.Sprintf("adjacent-%d", i), Subject: fmt.Sprintf("worker-%d", i),
			Object: "runnable_wait", ImpactMS: float64(20 - i), Unit: "ms",
			WithinRequestedWindow: &inside,
		})
	}
	projection.BackgroundCauses = []types.TraceCausalProjectionNode{{
		EvidenceID: "cpu-pressure", Subject: "unknown-thread", Object: "supply_pressure",
		TypeToken: "supply_pressure", ImpactMS: 604.528, Unit: "cpu·ms",
		SubjectKind:           types.TraceCausalSubjectKindAggregateMetric,
		WithinRequestedWindow: &inside,
	}}

	rows := traceDecisionNonCausalContextRows(projection, 6)
	if len(rows) != 6 || rows[len(rows)-1].lane != "background" || rows[len(rows)-1].node.EvidenceID != "cpu-pressure" {
		t.Fatalf("global cap must retain one typed background row: %+v", rows)
	}
	got := renderAnswerDocTraceDecisionHandoffSet(
		types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}},
		runtimeTraceGuidanceView{},
	)
	for _, want := range []string{
		"lane=`background`; subject=`supply_pressure`; kind=`supply_pressure`; value=604.528; unit=`cpu·ms`",
		"caliber=`aggregate_context_non_target_wall_clock`",
		"target_causal_authority=`not_provided`",
		"cross_axis_addition=`forbidden`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed background handoff missing %q:\n%s", want, got)
		}
	}
}

func TestTraceDecisionHandoffCarriesIndependentAggregateFactsWithoutProjectionAuthority(t *testing.T) {
	records := []types.ObservationRecord{
		{
			ID: "trace_query:q1#supply_pressure:1", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", Role: types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Path: "donghu.ftrace", ArtifactKind: "trace"},
			Predicate:       "cpu_pressure", Value: "604.528", Unit: "cpu·ms",
			RichNotes: []string{
				"type=supply_pressure", "subject_kind=aggregate_metric", "chain_relevance=background",
				"selected_window=10.000000..10.114940", "pressure_density=5.259", "window_ms=114.940",
			},
		},
		{
			ID: "trace_query:q1#io_pressure:1", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query:run2", Role: types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Path: "donghu.ftrace", ArtifactKind: "trace"},
			Predicate:       "scheduler_iowait", Value: "4340", Unit: "score",
			RichNotes: []string{
				"type=io_pressure", "subject_kind=aggregate_metric", "chain_relevance=background",
				"selected_window=10.000000..10.114940", "absolute_level=high",
				"comparison_scope=same_caliber_only", "io_pressure_score_caliber=count_weighted_composite",
			},
		},
	}
	facts := traceDecisionTypedAggregateFacts(records)
	if len(facts) != 2 {
		t.Fatalf("typed aggregate extraction=%d, want 2: %+v", len(facts), facts)
	}
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{
		ArtifactLabel: "donghu.ftrace", WindowStartTs: 10, WindowEndTs: 10.11494,
	}}}
	got := renderAnswerDocTraceDecisionHandoffSetWithAggregateFacts(set, runtimeTraceGuidanceView{}, facts)
	for _, want := range []string{
		"typed_window_aggregate_context",
		"kind=`supply_pressure`; signal=`cpu_pressure`; value=604.528; unit=`cpu·ms`",
		"pressure_density=`5.259`",
		"kind=`io_pressure`; signal=`scheduler_iowait`; value=4340.000; unit=`score`",
		"absolute_level=`high`",
		"target_causal_authority=`not_provided`",
		"cross_axis_addition=`forbidden`",
		"do not infer severity from the raw number alone when no calibration field is present",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("aggregate handoff missing %q:\n%s", want, got)
		}
	}
	projection := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(projection.Projections) != 1 || len(projection.Projections[0].PrimaryRootCauses) != 0 ||
		len(projection.Projections[0].OnChainCauses) != 0 || len(projection.Projections[0].AdjacentCauses) != 0 ||
		len(projection.Projections[0].BackgroundCauses) != 0 || len(projection.Projections[0].SemanticSpans) != 0 ||
		projection.Projections[0].WindowStartTs != 0 || projection.Projections[0].WindowEndTs != 0 {
		t.Fatalf("prompt-only aggregate facts must not create or mutate deterministic projections: %+v", projection)
	}
}

func TestTraceDecisionHandoffCarriesAcceptedModelRelationClaimsWithoutAuthoringConclusion(t *testing.T) {
	record := types.TraceCausalProjectionSelfRunnableTwoRuler{
		Subject: "target-100", WallEffsMS: []float64{2.2, 1.1}, WallRanks: []int{4, 9}, WallSubtotalMS: 3.3,
		EdgeEffsMS: []float64{0.336}, EdgeRanks: []int{10}, EdgeSubtotalMS: 0.336,
	}
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{SelfRunnableTwoRulerAccountings: []types.TraceCausalProjectionSelfRunnableTwoRuler{record}}}}
	authorities := types.CompileTraceAnswerRelationAuthorities(set)
	var claims []types.AnswerRelationClaim
	for _, authority := range authorities {
		members := authority.MemberRefs
		if authority.Kind == types.AnswerRelationAuthorityCrossRulerBoundary {
			members = append(append([]string(nil), authority.LeftMemberRefs...), authority.RightMemberRefs...)
		}
		claims = append(claims, types.AnswerRelationClaim{
			AuthorityID: authority.ID, MemberRefs: members, PhysicalRelation: authority.PhysicalRelation,
			Addition: authority.Addition, SubtotalValue: authority.SubtotalValue, SubtotalUnit: authority.SubtotalUnit,
		})
	}
	got := renderAnswerDocTraceDecisionHandoffSet(set, runtimeTraceGuidanceView{}, claims)
	for _, want := range []string{
		"accepted_model_relation_claims: these declarations were authored by the investigation model",
		"already accepted against the typed authorities",
		"you do not need to duplicate them in the final document",
		"invalid submitted claim but will not rewrite your prose",
		"physical_relation=`unresolved`; addition=`forbidden`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("accepted relation handoff missing %q:\n%s", want, got)
		}
	}
}

func TestTraceDecisionHandoffWithdrawsAcceptedClaimSupersededByFinalAuthority(t *testing.T) {
	account := types.TraceCausalProjectionTargetStateAccount{
		Subject: "target-100", RunningMS: 1, RunnableMS: 2, SleepMS: 7,
		TotalMS: 10, WindowStartTs: 1, WindowEndTs: 1.010,
	}
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{
		WindowStartTs: 1, WindowEndTs: 1.010, TargetStateAccount: &account,
	}}}
	oldSubtotal := 15.0
	stale := types.AnswerRelationClaim{
		AuthorityID:      "trace:target_state_partition:explore-window",
		MemberRefs:       []string{"running", "runnable", "sleep", "d_state", "io_wait"},
		PhysicalRelation: types.AnswerPhysicalRelationMutuallyExclusive,
		Addition:         types.AnswerRelationAdditionAuthorized, SubtotalValue: &oldSubtotal, SubtotalUnit: "ms",
	}
	got := renderAnswerDocTraceDecisionHandoffSet(set, runtimeTraceGuidanceView{}, []types.AnswerRelationClaim{stale})
	for _, want := range []string{
		"typed_relation_authority: relation_claim_copy=`{\"authority_id\":\"trace:target_state_partition:",
		"accepted_model_relation_claims_superseded: count=1",
		"do not copy them into the final document",
		"revise your own visible conclusion",
		"system does not rewrite your prose",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("superseded claim handoff missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, stale.AuthorityID) || strings.Contains(got, "- accepted_model_relation_claims: these declarations") {
		t.Fatalf("stale investigation claim remained a final obligation:\n%s", got)
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
		"phase_semantics:", "impact_phase=", "priority_candidate_scope=", "typed_relation_semantics:",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("zero/absent chain depth guessed a phase from non-phase fields %q:\n%s", forbidden, got)
		}
	}
}

func TestTraceDecisionHandoffKeepsMissingWakeupAsEvidenceBoundary(t *testing.T) {
	inside := true
	projection := types.TraceCausalProjection{
		ArtifactLabel: "customer.systrace",
		WindowStartTs: 10,
		WindowEndTs:   10.050,
		SupportingHops: []types.TraceCausalProjectionNode{{
			EvidenceID: "missing", Subject: "target-100", Predicate: "missing_wakeup",
			Object: "missing_wakeup", TypeToken: "missing_wakeup",
			UndrillableReason: "missing_wakeup", StartTs: 10.046416, EndTs: 10.050,
			ImpactMS: 3.584, WithinRequestedWindow: &inside,
		}},
	}
	got := renderAnswerDocTraceDecisionHandoffSet(
		types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}},
		runtimeTraceGuidanceView{},
	)
	for _, want := range []string{
		"evidence_boundary_semantics:",
		"contains no matching `sched_wakeup` row",
		"does not prove that a physical wakeup was absent",
		"owns no positive causal/eliminable amount",
		"evidence_boundary: subject=`target-100`; kind=`missing_wakeup`",
		"status=`no_matching_sched_wakeup_row_in_selected_window`",
		"positive_blocker_authority=`not_provided`",
		"causal_identity=`unresolved`",
		"observed_sleep_interval=`10.046416..10.050000`; interval_ms=3.584",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing-wakeup evidence boundary handoff missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"axis_A_actual_occupancy_candidates", "axis_B_existing_rule_eliminable",
		"proven blocking", "physical wakeup was missing",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("absence boundary was promoted through %q:\n%s", forbidden, got)
		}
	}
}

func TestTraceDecisionHandoffKeepsRowStateBreakdownBelowPairRelationAuthority(t *testing.T) {
	inside := true
	wait := types.TraceCausalProjectionNode{
		EvidenceID: "wait-row", Subject: "ThreadPool-300", StateKind: "d_state_or_io_wait",
		ImpactMS: 10.433, EffectiveImpactMS: 7.386, Rank: 1,
		DStateSplitMS: 3.047, IOWaitSplitMS: 7.386,
		BlockedReasonCaller: "sync_buffer_read_wi",
		CrossDirectionOverlaps: []types.TraceCausalProjectionCrossDirectionOverlap{{
			OverlapMS: 6.673, LineStart: 420, LineEnd: 430,
			Direction: "io_path", Basis: "interval_intersection",
		}},
		WithinRequestedWindow: &inside,
	}
	completion := types.TraceCausalProjectionNode{
		EvidenceID: "completion-row", Subject: "udk-irq-78", SemanticClass: "io_latency",
		ImpactMS: 6.673, EffectiveImpactMS: 6.673, Rank: 2,
		ResourceCompletionClosure: true, WithinRequestedWindow: &inside,
	}
	holder := types.TraceCausalProjectionNode{
		EvidenceID: "lock-holder", Subject: "holder-11", SemanticClass: "lock_contention",
		ImpactMS: 2, EffectiveImpactMS: 2, Rank: 3, BlockingKind: "lock_contention",
		BlockingPeer: "waiter-22", BlockingSubjectIsHolder: true,
		WithinRequestedWindow: &inside,
	}
	projection := types.TraceCausalProjection{
		OnChainCauses: []types.TraceCausalProjectionNode{wait, completion, holder},
		RankedSeats:   []types.TraceCausalProjectionNode{wait, completion, holder},
	}
	got := renderAnswerDocTraceDecisionHandoffSet(
		types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}},
		runtimeTraceGuidanceView{},
	)
	for _, want := range []string{
		"typed_relation_semantics:",
		"A `row_state_breakdown` describes only that observation's own state accounting",
		"`sched_blocked_reason.caller` is a kernel-reported wait call-site/symbol, not a resource or lock owner",
		"row_identity=`wait-row`",
		"row_state_breakdown=`d_state:3.047ms,io_wait:7.386ms`",
		"state_breakdown_scope=`this_observation_only`",
		"cross_row_relation_authority=`not_provided_by_state_breakdown`",
		"physical_overlap_1=`6.673ms@lines:420..430`",
		"peer_fix_direction=`io_path`",
		"overlap_addition=`forbidden`",
		"blocked_reason_caller=`sync_buffer_read_wi`",
		"caller_role=`kernel_reported_wait_callsite`",
		"holder_authority=`not_provided_by_caller`",
		"row_identity=`completion-row`",
		"completion_relation=`resource_completion_closure_for_anchored_wait`",
		"completion_thread_holder_authority=`not_provided`",
		"row_identity=`lock-holder`",
		"subject_lock_role=`typed_holder`",
		"blocked_waiter=`waiter-22`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed relation handoff missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"embedded_components=", "component_relation=`already_inside_parent_row`",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("row-local state split minted unsupported cross-row relation %q:\n%s", forbidden, got)
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

func TestFinalizerInitialInstructionWiresRuntimeEnumerationAuthorityBeforeAnswer(t *testing.T) {
	ctx := answerDocCausalCeilingTestContext(true)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{
		{
			ToolName: "trace_query", Success: true,
			EnumerationAuthority: &types.ToolEnumerationAuthority{Status: "incomplete", Boundaries: []types.ToolEnumerationBoundary{
				{Scope: "root_cause_rank", Dimension: "candidates", Emitted: 12, Total: 61, TotalKnown: true, Reason: "capacity"},
				{Scope: "span_window", Dimension: "spans", Emitted: 40, Total: 1475, TotalKnown: true, Reason: "capacity"},
			}},
		},
		// Ordinary source pagination is not a runtime-artifact enumeration
		// boundary and must not pollute this trace authority section.
		{
			ToolName: "read_file", Success: true,
			EnumerationAuthority: &types.ToolEnumerationAuthority{Status: "incomplete", Boundaries: []types.ToolEnumerationBoundary{
				{Scope: "internal/source.go", Dimension: "lines", Emitted: 200, Total: 500, TotalKnown: true},
			}},
		},
	}})

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Runtime Enumeration Authority",
		"status=`incomplete`",
		"affected_scopes=`root_cause_rank,span_window`",
		"scope=`root_cause_rank`; dimension=`candidates`; emitted=12; total=61; total_known=true",
		"scope=`span_window`; dimension=`spans`; emitted=40; total=1475; total_known=true",
		"bounded samples or lower bounds, not an exhaustive census",
		"You still own the diagnosis, prioritization, summary, and recommendations",
		"this handoff supplies no conclusion and never replaces yours",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("finalizer prompt missed runtime enumeration authority %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "scope=`internal/source.go`") {
		t.Fatalf("ordinary source pagination polluted runtime enumeration authority:\n%s", prompt)
	}
	if strings.Index(prompt, "## Runtime Enumeration Authority") > strings.Index(prompt, "## Observation Ledger") {
		t.Fatalf("runtime enumeration authority must precede the larger observation ledger:\n%s", prompt)
	}
}

func TestFinalizerInitialInstructionOmitsCompleteRuntimeEnumerationAuthority(t *testing.T) {
	ctx := answerDocCausalCeilingTestContext(true)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		EnumerationAuthority: &types.ToolEnumerationAuthority{Status: "complete", Boundaries: []types.ToolEnumerationBoundary{{
			Scope: "window_stats", Dimension: "rows", Emitted: 1, Total: 1, TotalKnown: true,
		}},
		}}}})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(prompt, "## Runtime Enumeration Authority") {
		t.Fatalf("complete runtime enumeration should not add an incomplete-boundary section:\n%s", prompt)
	}
}
