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
		"causal_conclusion=`unproven`",
		"frame_evidence_status=`absent`",
		"phase_semantics: `pre_wakeup_dependency`",
		"not the consumer's post-wakeup runnable/dispatch delay",
		"never infer that a CFS dependency preempted an RT consumer after wake",
		"candidate flag alone proves neither a lock holder/waiter relation nor post-wakeup preemption",
		"target_state_symptom: subject=`target-100`",
		"partition_relation=`mutually_exclusive_and_additive_to_total`",
		"partition_addition_authority=`these_five_members_only`",
		"authorized_relation_fact: family=`self_runnable_two_ruler`",
		"self_wall_clock_seats=`#4:2.200ms,#9:1.100ms`",
		"self_wall_clock_subtotal=3.300ms",
		"wakeup_edge_seats=`#10:0.336ms`",
		"same_ruler_addition=`authorized_to_published_subtotal`",
		"cross_ruler_addition=`forbidden`",
		"cross_ruler_physical_relation=`unresolved`",
		"typed_relation_authority: authority_id=`trace:self_runnable_two_ruler:",
		"typed_relation_authority: authority_id=`trace:target_state_partition:",
		"member_refs=`running,runnable,sleep,d_state,io_wait`; physical_relation=`mutually_exclusive`; addition=`authorized_to_published_subtotal`",
		"final_relation_claim_obligation:",
		"authority added by deterministic supplement after investigation closure",
		"elected_wakeup_path=`ThreadPool-300 -> Network-200 -> Cookie-150 -> target-100`",
		"wakeup_path_semantics:",
		"does not prove that B synchronously blocked waiting for A",
		"Use stronger blocked-wait/holder wording only when a separate typed blocking or holder relation provides that authority",
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
		"cross_row_additivity=`not_authorized_without_exact_pair_carrier`",
		"relation_authority=`typed_pair_only`",
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
		"Preserve them on the model-authored answer block(s) via `relation_claims`",
		"system will reject a mismatch but will not rewrite your prose",
		"physical_relation=`unresolved`; addition=`forbidden`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("accepted relation handoff missing %q:\n%s", want, got)
		}
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
