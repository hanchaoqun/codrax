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
				FixDirection: "lock_priority", ChainRelevance: "on_chain", WithinRequestedWindow: &inside,
			},
			{
				EvidenceID: "supply", Subject: "target-100", Object: "running",
				StateKind: "running", ImpactMS: 26.946, EffectiveImpactMS: 10.331, Rank: 2,
				FixDirection: "frequency_thermal", SystemSupplement: true,
				ChainRelevance: "on_chain", WithinRequestedWindow: &inside,
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
		"compact_unknowns: evidence_absence_implication=`unknown_not_false`",
		"cross_direction_physical_relation=`unresolved_unless_an_exact_pair_row_says_otherwise`",
		"target_direct_blocking_not_established_does_not_prove_no_external_blocking=`true`",
		"absent_overlap_record_proves_independence=`false`",
		"cause_decomposition_status=`not_closed_by_state_partition_or_ranked_seat_roster`",
		"exhaustive_cause_wording=`requires_one_exact_typed_additive_cause_partition`",
		"fully explained/composed by listed Axis B seats only when one exact typed additive carrier publishes that same subtotal",
		"remaining on-chain work as unpriced or unresolved instead of treating the arithmetic remainder as zero",
		"Do not compute a residual by adding or subtracting overlapping rows unless a typed partition authorizes that arithmetic",
		"Causal evidence boundary: the selected evidence does not prove a dropped-frame or frame-deadline cause",
		"Frame evidence boundary: the selected evidence produced no frame or deadline observation bound to the target",
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
		"Confirmed wakeup dependency path: `ThreadPool-300 -> Network-200 -> Cookie-150 -> target-100`",
		"Reader meaning: an edge `A -> B` proves the recorded wakeup/dependency relation",
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
		"this seat does not establish a synchronous blocker, a lock or resource holder, or a holder/waiter relation",
		"priority_candidate_scope=`dependency_scheduler_supply_before_downstream_wakeup`",
		"post_wakeup_preemption_authority=`not_provided_by_this_seat`",
		"conclusion_caliber=`validation_candidate`",
		"rank=#2; subject=`target-100`; kind=`running`; effective_attribution=10.331ms; fix_direction=`frequency_thermal`",
		"source_lane=`deterministic_system_supplement`",
		"cross_row_additivity=`not_authorized_without_exact_pair_carrier`",
		"contextual_noncausal_rows",
		"lane=`background`; subject=`scheduler-demand`; kind=`supply_pressure`; value=3.500ms; reader_calibration=`window-level background measurement in the stated unit; not target-causal or recoverable time`",
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
	for _, forbidden := range []string{"causal_conclusion=", "frame_evidence_status=", "frame_evidence_status_semantics="} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("model-facing trace handoff leaked raw control metadata %q:\n%s", forbidden, got)
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
	for _, forbidden := range []string{"elected_wakeup_path=", "wakeup_path_semantics:"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("wakeup path handoff leaked machine-facing label %q:\n%s", forbidden, got)
		}
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
				ChainDepth: 1, ChainRelevance: "on_chain", WithinRequestedWindow: &inside,
			},
			{
				EvidenceID: "ordinary-runnable", Subject: "ordinary-worker-9",
				StateKind: "runnable", Rank: 2, EffectiveImpactMS: 3,
				FixDirection: "lock_priority", ChainDepth: 1, ChainRelevance: "on_chain", WithinRequestedWindow: &inside,
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
		"this seat does not establish a synchronous blocker, a lock or resource holder, or a holder/waiter relation",
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
	for _, forbidden := range []string{"synchronous_blocker_authority=", "holder_waiter_authority="} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("candidate handoff leaked retired model-facing authority key %q:\n%s", forbidden, got)
		}
	}
}

func TestTraceDecisionHandoffDoesNotPromoteBroadEnvelopesToPhysicalOverlap(t *testing.T) {
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
	got := renderAnswerDocTraceDecisionHandoffSet(
		types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}},
		runtimeTraceGuidanceView{},
	)
	for _, forbidden := range []string{
		"typed_relation_authority: relation_claim_copy=`{\"authority_id\":\"trace:overlapping_members:",
		"measured_envelope_overlap=90.000ms",
		"members_independent=`false`",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("broad envelope leaked as physical relation through %q:\n%s", forbidden, got)
		}
	}
	for _, want := range []string{"relation_authority=`typed_pair_only`", "cross_row_additivity=`not_authorized_without_exact_pair_carrier`"} {
		if !strings.Contains(got, want) {
			t.Fatalf("safe unresolved relation guidance missing %q:\n%s", want, got)
		}
	}
}

func TestTraceDecisionHandoffKeepsSemanticMembersOutsideGenericTopN(t *testing.T) {
	inside := true
	projection := types.TraceCausalProjection{WindowStartTs: 10, WindowEndTs: 10.2}
	for i := 0; i < 10; i++ {
		projection.OnChainCauses = append(projection.OnChainCauses, types.TraceCausalProjectionNode{
			EvidenceID: fmt.Sprintf("state-%d", i), Subject: "target-7", StateKind: "running",
			ImpactMS: float64(100 - i), WithinRequestedWindow: &inside,
		})
	}
	semantic := types.TraceCausalProjectionNode{
		EvidenceID: "semantic-jit", Subject: "Jit thread pool-12", SemanticClass: "jit_compile",
		SpanName: "JIT compilation", ImpactMS: 2.388, LineStart: 5969, LineEnd: 12664,
		FamilyMemberCount: 2, FamilyMemberMaxMS: 1.781,
		FamilyMemberRoster:     []string{"JIT compiling TextView.<init>()", "JIT compiling DecimalQuantity.readIntToBcd()"},
		FamilyMemberLineRanges: [][2]int{{5969, 6114}, {12611, 12664}},
		FamilyMemberWallMS:     []float64{1.781, 0.607},
		WithinRequestedWindow:  &inside,
	}
	semanticSupplement := semantic
	semanticSupplement.EvidenceID = "semantic-jit-supplement"
	semanticSupplement.SystemSupplement = true
	projection.SemanticSpans = []types.TraceCausalProjectionNode{semantic, semanticSupplement}

	got := renderAnswerDocTraceDecisionHandoffSet(
		types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}},
		runtimeTraceGuidanceView{},
	)
	for _, want := range []string{
		"deterministic_semantic_spans (typed in-window work inventory",
		"subject=`Jit thread pool-12`; semantic_class=`jit_compile`; span=`JIT compilation`; total=2.388ms; occurrences=2; member_max=1.781ms; lines=5969..12664",
		"member_1 span=`JIT compiling TextView.<init>()`; duration=1.781ms; lines=5969..6114",
		"member_2 span=`JIT compiling DecimalQuantity.readIntToBcd()`; duration=0.607ms; lines=12611..12664",
		"Its presence neither proves an effect on the target nor proves no effect",
		"Its absence must not be inferred from a target-thread-only keyword search",
		"Every member row is a distinct typed span",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("semantic member handoff missing %q:\n%s", want, got)
		}
	}
	if count := strings.Count(got, "subject=`Jit thread pool-12`; semantic_class=`jit_compile`"); count != 1 {
		t.Fatalf("same typed semantic family from exploration and supplement rendered %d times:\n%s", count, got)
	}
}

func TestTraceDecisionHandoffFallsBackToLeaderWithoutExactDirectionFold(t *testing.T) {
	inside := true
	seat := func(rank int, subject, direction, typeToken string, value float64) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			EvidenceID: fmt.Sprintf("seat-%d", rank), Rank: rank, Subject: subject,
			FixDirection: direction, TypeToken: typeToken, Object: typeToken,
			EffectiveImpactMS: value, EffectiveImpactPublished: true,
			ChainRelevance: "on_chain", WithinRequestedWindow: &inside,
		}
	}
	projection := types.TraceCausalProjection{
		ArtifactLabel: "customer.trace",
		RankedSeats: []types.TraceCausalProjectionNode{
			seat(1, "target", "frequency_thermal", "running", 58.320),
			seat(2, "worker-a", "lock_priority", "priority_inversion_candidate", 7.405),
			seat(3, "worker-b", "lock_priority", "priority_inversion_candidate", 4.710),
			seat(4, "target", "scheduling_supply", "runnable_wait", 3.956),
			seat(5, "target", "io_dependency", "io_latency", 3.670),
			seat(6, "worker-a", "io_dependency", "d_state_or_io_wait", 3.598),
		},
	}
	got := renderAnswerDocTraceDecisionHandoffSet(
		types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}},
		runtimeTraceGuidanceView{},
	)
	for _, want := range []string{
		"repair_direction_authority: artifact=`customer.trace`",
		"value_role=`exact_typed_direction_subtotal_when_published_else_single_leader`",
		"joint_total_authority=`not_provided`",
		"direction_independence_authority=`not_provided`",
		"instruction=`do_not_sum_across_directions_or_unlisted_members`",
		"fix_direction=`frequency_thermal`; member_count=1; leader_rank=#1; leader_subject=`target`; leader_value=58.320ms",
		"validation_direction=`priority_or_dependency_supply`; member_count=2; leader_rank=#2; leader_subject=`worker-a`; leader_value=7.405ms",
		"fix_direction=`io_dependency`; member_count=2; leader_rank=#5; leader_subject=`target`; leader_value=3.670ms",
		"same_direction_subtotal_authority=`not_provided`; published_direction_value=`leader_only`",
		"compute_supply_value_role=`frequency_relative_headroom_against_published_ideal_basis`",
		"compute_supply_value_proves_lower_frequency_cause=`false`",
		"compute_supply_value_proves_governance_binding=`false`",
		"policy_ceiling_proves_thermal_throttling_or_actual_binding=`false`",
		"lock_holder_or_priority_inheritance_need=`unproven_without_typed_relation`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("repair direction authority missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"18.853", "7.268", "31.4"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("direction authority fabricated subtotal %q:\n%s", forbidden, got)
		}
	}
}

func TestTraceDecisionHandoffPublishesExactDisjointDirectionSubtotal(t *testing.T) {
	inside := true
	seat := func(rank int, subject, direction string, value, start, end float64) types.TraceCausalProjectionNode {
		token := "priority_inversion_candidate"
		switch direction {
		case "frequency_thermal":
			token = "running"
		case "scheduling_supply":
			token = "runnable_wait"
		case "io_dependency":
			token = "io_latency"
		}
		return types.TraceCausalProjectionNode{
			EvidenceID: fmt.Sprintf("seat-%d", rank), Rank: rank, Subject: subject,
			Object: token, TypeToken: token,
			FixDirection: direction, EffectiveImpactMS: value, EffectiveImpactPublished: true,
			ChainRelevance: "on_chain", WithinRequestedWindow: &inside,
			StartTs: start, EndTs: end, RankBoardTarget: "target-100",
			RankBoardParamsFingerprint: "board-a", RankQueryWindowStartTs: 10, RankQueryWindowEndTs: 10.1,
		}
	}
	seats := []types.TraceCausalProjectionNode{
		seat(1, "target-100", "frequency_thermal", 58.320, 10, 10.055),
		seat(2, "worker-a", "lock_priority", 7.405, 10.051, 10.06),
		seat(3, "worker-b", "lock_priority", 4.710, 10.061, 10.07),
		seat(4, "target-100", "scheduling_supply", 3.956, 10.071, 10.075),
		seat(5, "target-100", "io_dependency", 3.670, 10.076, 10.08),
		seat(6, "worker-c", "lock_priority", 3.429, 10.081, 10.085),
		seat(7, "worker-d", "lock_priority", 3.309, 10.086, 10.09),
	}
	seats[0].Subject = "shared-thread"
	seats[1].Subject = "shared-thread"
	seats[0].LineStart, seats[0].LineEnd = 100, 110
	seats[1].LineStart, seats[1].LineEnd = 200, 210
	seats[2].LineStart, seats[2].LineEnd = 300, 310
	seats[0].CrossDirectionOverlaps = []types.TraceCausalProjectionCrossDirectionOverlap{{
		OverlapMS: 4, LineStart: 200, LineEnd: 210, Direction: "lock_priority", Basis: "interval_intersection",
	}}
	seats[1].CrossDirectionOverlaps = []types.TraceCausalProjectionCrossDirectionOverlap{{
		OverlapMS: 4, LineStart: 100, LineEnd: 110, Direction: "frequency_thermal", Basis: "interval_intersection",
	}}
	projection := types.TraceCausalProjection{
		ArtifactLabel: "customer.trace", WindowStartTs: 10, WindowEndTs: 10.1,
		WakeupPath: []string{"worker-a", "target-100"}, RankedSeats: seats, OnChainCauses: seats,
	}
	got := renderAnswerDocTraceDecisionHandoffSet(
		types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}},
		runtimeTraceGuidanceView{},
	)
	for _, want := range []string{
		"validation_direction=`priority_or_dependency_supply`; member_count=4; leader_rank=#2",
		"same_direction_subtotal_authority=`typed_pairwise_disjoint_section`",
		"published_direction_value=`exact_subtotal`; direction_subtotal=12.115ms; subtotal_member_count=2",
		"repair_direction_relation_roster: artifact=`customer.trace`; emitted=2; total=2; complete=`true`",
		"relation_scope=`same_direction`; direction_a=`lock_priority`",
		"physical_relation=`mutually_exclusive`; addition=`authorized_to_published_subtotal`; published_direction_subtotal=12.115ms",
		"reasoning_boundary=`separate_nonoverlapping_contributions_not_overlap_or_competition`",
		"relation_scope=`cross_direction`; direction_a=`frequency_thermal`; direction_b=`lock_priority`",
		"physical_relation=`overlap`; addition=`forbidden`; measured_physical_overlap=4.000ms",
		"reasoning_boundary=`shared_physical_time_only_at_the_published_overlap`",
		"relation_scope=`unlisted_pairs`; physical_relation=`unresolved`; addition=`forbidden_without_exact_typed_carrier`",
		"repair_direction_presentation_plan: artifact=`customer.trace`; emitted=4; total=4; complete=`true`",
		"direction=`lock_priority`; validation_direction=`priority_or_dependency_supply`; member_count=4; headline_value_role=`exact_typed_subtotal`; headline_value=12.115ms",
		"additional_members_emitted=2; additional_members_total=2; additional_members_complete=`true`",
		"display_contract=`headline_arithmetic_applies_only_to_headline_member_refs|list_additional_members_as_separate_values|never_plus_join_additional_members|pairwise_relation_unresolved_unless_relation_roster_lists_it|independence_not_authorized`",
		"direction=`io_dependency`; fix_direction=`io_dependency`; member_count=1; headline_value_role=`single_leader`; headline_value=3.670ms",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("exact direction subtotal authority missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "joint_total_authority=`provided`") {
		t.Fatalf("one direction subtotal must not mint a cross-direction total:\n%s", got)
	}
	compact := renderTraceFinalCompactAuthorityLedger(types.TraceCausalProjectionSet{
		Projections: []types.TraceCausalProjection{projection},
	})
	for _, want := range []string{
		"repair_direction_relation_roster: artifact=`customer.trace`; emitted=2; total=2; complete=`true`",
		"relation_scope=`same_direction`; direction_a=`lock_priority`",
		"physical_relation=`mutually_exclusive`; addition=`authorized_to_published_subtotal`; published_direction_subtotal=12.115ms",
		"relation_scope=`cross_direction`; direction_a=`frequency_thermal`; direction_b=`lock_priority`",
		"physical_relation=`overlap`; addition=`forbidden`; measured_physical_overlap=4.000ms",
		"repair_direction_presentation_plan: artifact=`customer.trace`; emitted=4; total=4; complete=`true`",
		"direction=`lock_priority`; validation_direction=`priority_or_dependency_supply`; member_count=4; headline_value_role=`exact_typed_subtotal`; headline_value=12.115ms",
		"additional_members_emitted=2; additional_members_total=2; additional_members_complete=`true`",
	} {
		if !strings.Contains(compact, want) {
			t.Fatalf("final compact boundary drifted from relation roster %q:\n%s", want, compact)
		}
	}
}

func TestTraceDecisionRepairDirectionRelationRosterRejectsOneSidedOrCrossBoardOverlap(t *testing.T) {
	inside := true
	seat := func(rank int, direction, board string, lineStart int) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			EvidenceID: fmt.Sprintf("seat-%d", rank), Rank: rank, Subject: "shared-thread",
			Object: "ranked_cause", TypeToken: "ranked_cause", FixDirection: direction,
			EffectiveImpactMS: 4, EffectiveImpactPublished: true, ChainRelevance: "on_chain",
			WithinRequestedWindow: &inside, StartTs: 10, EndTs: 10.004,
			LineStart: lineStart, LineEnd: lineStart + 5, RankBoardTarget: "target-100",
			RankBoardParamsFingerprint: board, RankQueryWindowStartTs: 10, RankQueryWindowEndTs: 10.1,
		}
	}
	left := seat(1, "frequency_thermal", "board-a", 100)
	right := seat(2, "io_dependency", "board-a", 200)
	left.CrossDirectionOverlaps = []types.TraceCausalProjectionCrossDirectionOverlap{{
		OverlapMS: 3, LineStart: 200, LineEnd: 205, Direction: "io_dependency", Basis: "interval_intersection",
	}}
	projection := types.TraceCausalProjection{RankedSeats: []types.TraceCausalProjectionNode{left, right}}
	got := renderAnswerDocTraceDecisionHandoffSet(
		types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}}, runtimeTraceGuidanceView{})
	if strings.Contains(got, "measured_physical_overlap=3.000ms") {
		t.Fatalf("one-sided overlap carrier must fail closed:\n%s", got)
	}

	right.CrossDirectionOverlaps = []types.TraceCausalProjectionCrossDirectionOverlap{{
		OverlapMS: 3, LineStart: 100, LineEnd: 105, Direction: "frequency_thermal", Basis: "interval_intersection",
	}}
	right.RankBoardParamsFingerprint = "board-b"
	projection.RankedSeats = []types.TraceCausalProjectionNode{left, right}
	got = renderAnswerDocTraceDecisionHandoffSet(
		types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}}, runtimeTraceGuidanceView{})
	if strings.Contains(got, "measured_physical_overlap=3.000ms") {
		t.Fatalf("cross-board overlap carrier must fail closed:\n%s", got)
	}
	if !strings.Contains(got, "relation_scope=`unlisted_pairs`; physical_relation=`unresolved`") {
		t.Fatalf("failed-closed pair must retain the explicit unresolved boundary:\n%s", got)
	}
}

func TestTraceDecisionDirectionPresentationPlanKeepsUnfoldedMembersSeparate(t *testing.T) {
	inside := true
	seat := func(rank int, subject string, value, start, end float64) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			EvidenceID: fmt.Sprintf("seat-%d", rank), Rank: rank, Subject: subject,
			Object: "io_latency", TypeToken: "io_latency", FixDirection: "io_dependency",
			EffectiveImpactMS: value, EffectiveImpactPublished: true, ChainRelevance: "on_chain",
			WithinRequestedWindow: &inside, StartTs: start, EndTs: end, LineStart: 100 * rank, LineEnd: 100*rank + 10,
			RankBoardTarget: "target-100", RankBoardParamsFingerprint: "board-a",
			RankQueryWindowStartTs: 10, RankQueryWindowEndTs: 10.1,
		}
	}
	// The two seats overlap, so sharing one direction label cannot mint a
	// subtotal or an independence claim.
	first := seat(1, "target-100", 3.670, 10, 10.006)
	second := seat(2, "worker-200", 3.598, 10.004, 10.009)
	projection := types.TraceCausalProjection{
		ArtifactLabel: "customer.trace", WindowStartTs: 10, WindowEndTs: 10.1,
		RankedSeats:   []types.TraceCausalProjectionNode{first, second},
		OnChainCauses: []types.TraceCausalProjectionNode{first, second},
	}
	got := renderAnswerDocTraceDecisionHandoffSet(
		types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}}, runtimeTraceGuidanceView{})
	for _, want := range []string{
		"direction=`io_dependency`; fix_direction=`io_dependency`; member_count=2; headline_value_role=`single_leader`; headline_value=3.670ms",
		"additional_members_emitted=1; additional_members_total=1; additional_members_complete=`true`",
		"never_plus_join_additional_members",
		"pairwise_relation_unresolved_unless_relation_roster_lists_it",
		"independence_not_authorized",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("unfolded direction member boundary missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "direction=`io_dependency`; fix_direction=`io_dependency`; member_count=2; headline_value_role=`exact_typed_subtotal`") {
		t.Fatalf("overlapping same-label seats must not acquire a subtotal:\n%s", got)
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
		"lane=`background`; subject=`supply_pressure`; kind=`supply_pressure`; value=604.528cpu·ms",
		"reader_calibration=`window-level background measurement in the stated unit; not target-causal or recoverable time`",
		"target_causal_authority=`not_provided`",
		"cross_axis_addition=`forbidden`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed background handoff missing %q:\n%s", want, got)
		}
	}
}

func TestTraceDecisionHandoffKeepsAdjacentRankOutOfEliminableAxis(t *testing.T) {
	inside := true
	onChain := types.TraceCausalProjectionNode{
		EvidenceID: "chain-rank-1", Subject: "target-32788", Object: "running",
		StateKind: "running", Rank: 1, ImpactMS: 74.915, EffectiveImpactMS: 65.912,
		EffectiveImpactPublished: true, FixDirection: "frequency_thermal",
		ChainRelevance: "on_chain", WithinRequestedWindow: &inside,
	}
	adjacent := types.TraceCausalProjectionNode{
		EvidenceID: "adjacent-rank-1", Subject: "logd.writer-1913", Object: "runnable_wait",
		StateKind: "runnable", Rank: 1, ImpactMS: 49.623, EffectiveImpactMS: 49.623,
		EffectiveImpactPublished: true, FixDirection: "scheduling_supply",
		ChainRelevance: "adjacent", WithinRequestedWindow: &inside,
	}
	projection := types.TraceCausalProjection{
		ArtifactLabel: "customer.systrace", WindowStartTs: 10, WindowEndTs: 10.23319,
		RankedSeats:    []types.TraceCausalProjectionNode{onChain, adjacent},
		OnChainCauses:  []types.TraceCausalProjectionNode{onChain},
		AdjacentCauses: []types.TraceCausalProjectionNode{adjacent},
	}

	got := renderAnswerDocTraceDecisionHandoffSet(
		types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}},
		runtimeTraceGuidanceView{},
	)
	axisStart := strings.Index(got, "axis_B_existing_rule_eliminable")
	contextStart := strings.Index(got, "contextual_noncausal_rows")
	if axisStart < 0 || contextStart < 0 || contextStart <= axisStart {
		t.Fatalf("expected both ordered decision lanes:\n%s", got)
	}
	axis := got[axisStart:contextStart]
	if !strings.Contains(axis, "subject=`target-32788`") ||
		strings.Contains(axis, "logd.writer-1913") || strings.Contains(axis, "49.623") {
		t.Fatalf("adjacent rank leaked into on-chain eliminable axis:\n%s", got)
	}
	context := got[contextStart:]
	for _, want := range []string{
		"lane=`adjacent`", "subject=`logd.writer-1913`", "value=49.623",
		"target_causal_authority=`not_provided`", "cross_axis_addition=`forbidden`",
	} {
		if !strings.Contains(context, want) {
			t.Fatalf("adjacent evidence was not preserved as context %q:\n%s", want, got)
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
				"selected_window=10.000000..10.114940", "absolute_level=not_defined",
				"comparison_scope=same_caliber_only", "io_pressure_score_caliber=cross_unit_activity_index",
				"io_pressure_evidence_quality=wall_clock_or_latency_corroborated",
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
		"All available background rows are included in this bounded handoff",
		"kind=`supply_pressure`; signal=`cpu_pressure`; value=604.528; unit=`cpu·ms`",
		"pressure_density=`5.259`",
		"kind=`io_pressure`; signal=`scheduler_iowait`; value=4340.000; unit=`mixed-unit activity index (not wall-clock)`",
		"reader_calibration=`mixed-unit activity index, not elapsed time, target-causal time, or a recoverable amount; absolute high/low is undefined; compare only the same caliber, capture conditions, and window duration; separate latency evidence exists but does not convert this index into time or prove absolute severity`",
		"target_causal_authority=`not_provided`",
		"cross_axis_addition=`forbidden`",
		"do not infer severity from the raw number alone when no calibration field is present",
		"every machine-facing Trace token below",
		"use only its reader-language meaning in the visible answer",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("aggregate handoff missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "complete=`") {
		t.Fatalf("machine complete enum leaked into the aggregate handoff:\n%s", got)
	}
	for _, forbidden := range []string{"wall_clock_or_latency_corroborated", "cross_unit_activity_index", "absolute_level=`", "comparison_scope=`", "kind=`io_pressure`; signal=`scheduler_iowait`; value=4340.000; unit=`score`"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("aggregate handoff leaked raw calibration token %q:\n%s", forbidden, got)
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

func TestTraceDecisionContextUsesSharedNonWallClockCaliber(t *testing.T) {
	projection := types.TraceCausalProjection{
		ArtifactLabel: "customer.trace",
		BackgroundCauses: []types.TraceCausalProjectionNode{
			{Subject: "reclaim", TypeToken: "page_cache_churn", Object: "page_cache_churn", ImpactMS: 7.2, Unit: "ms"},
			{Subject: "io-window", TypeToken: "io_pressure", Object: "io_pressure", ImpactMS: 551.6, Unit: "score", SubjectKind: types.TraceCausalSubjectKindAggregateMetric},
			{Subject: "cpu-window", TypeToken: "supply_pressure", Object: "supply_pressure", ImpactMS: 3.5, Unit: "cpu·ms", SubjectKind: types.TraceCausalSubjectKindAggregateMetric},
		},
	}
	got := renderAnswerDocTraceDecisionHandoffSet(
		types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}},
		runtimeTraceGuidanceView{},
	)
	for _, want := range []string{
		"kind=`page_cache_churn`; value=7.200; reader_calibration=`count-derived observation (not elapsed time)`",
		"kind=`io_pressure`; value=551.600; reader_calibration=`mixed-unit activity index (not elapsed time or a recoverable amount; absolute high/low is undefined)`",
		"kind=`supply_pressure`; value=3.500cpu·ms; reader_calibration=`window-level background measurement in the stated unit; not target-causal or recoverable time`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("non-wall-clock context missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"kind=`page_cache_churn`; value=7.200; unit=`ms`",
		"kind=`io_pressure`; value=551.600; unit=`score`",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("non-wall-clock context leaked false unit %q:\n%s", forbidden, got)
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
	if strings.Contains(windowPrompt, "## Trace Decision Inputs (Model Owns The Conclusion)") {
		t.Fatalf("explicit typed window widened a bounded fact request into decision synthesis:\n%s", windowPrompt)
	}
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
		Scope: types.RuntimeQuestionScopeCausalDiagnosis,
	}
	causalWindowPrompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(causalWindowPrompt, "## Trace Decision Inputs (Model Owns The Conclusion)") {
		t.Fatalf("explicit-window causal diagnosis lost decision-input authority:\n%s", causalWindowPrompt)
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
	for _, want := range []string{
		"runtime_enumeration_final_authority status=`incomplete`",
		"affected_scopes=`root_cause_rank,span_window`",
		"exhaustive_claim_permission=`forbidden`",
		"exact_total_count_extrema_absence_permission=`requires_separate_complete_typed_authority`",
		"incomplete_boundary scope=`root_cause_rank`; dimension=`candidates`; emitted=12; total=61",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("final trace decision tail missed runtime enumeration authority %q:\n%s", want, prompt)
		}
	}
	if strings.LastIndex(prompt, "runtime_enumeration_final_authority") < strings.LastIndex(prompt, "## Submission Checklist") {
		t.Fatalf("runtime enumeration authority must be replayed at the final trace decision tail:\n%s", prompt)
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

func TestFinalizerRuntimeEnumerationAuthorityDoesNotTeachPrivateTraceQueryBlobPath(t *testing.T) {
	privatePath := "/work/.codrax/blob/session/trace_query-deadbeef.txt"
	ctx := answerDocCausalCeilingTestContext(true)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "read_file", Success: true,
		RuntimeArtifactRead: &types.ToolRuntimeArtifactRead{
			RequestedPath: privatePath, Kind: "blob", TraceQueryBlob: true,
		},
		EnumerationAuthority: &types.ToolEnumerationAuthority{
			Status: "incomplete",
			Boundaries: []types.ToolEnumerationBoundary{{
				Scope: privatePath, Dimension: "lines", Emitted: 66,
				Total: 332, TotalKnown: true, Reason: "inline_budget_clamped",
			}},
		},
	}}})

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "affected_scopes=`trace_query_result_page`") ||
		!strings.Contains(prompt, "scope=`trace_query_result_page`; dimension=`lines`; emitted=66; total=332") {
		t.Fatalf("finalizer missed the typed public trace-query page boundary:\n%s", prompt)
	}
	if strings.Contains(prompt, privatePath) || strings.Contains(prompt, ".codrax/blob") {
		t.Fatalf("finalizer was taught a private trace-query blob identity:\n%s", prompt)
	}
}
