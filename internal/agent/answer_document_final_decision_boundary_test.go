package agent

import (
	"fmt"
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
		"Keep the requested conceptual destination separate from the current implementation",
		"selected_terminal_body_calls=`unproven`",
		"storage medium",
		"Class names, method names, comments, layer labels, and request wording do not prove what the endpoint currently does",
		"say only that the grounded chain reaches or invokes the endpoint",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("call-chain final boundary missing %q:\n%s", want, prompt)
		}
	}
	if strings.LastIndex(prompt, "## Final Call-Chain Evidence Boundary") < strings.LastIndex(prompt, "## Submission Checklist") {
		t.Fatalf("call-chain evidence boundary must follow generic structure guidance:\n%s", prompt)
	}
}

func TestFinalCallChainEvidenceBoundaryAcceptsTypedTerminalBodyOperationWithoutUpgradingItsSemantics(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState("opaque"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace, AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
			CallChainEndpointProfile: &types.CallChainEndpointProfile{
				Source: "VisitController.create", Sink: "AuditLog.record", SinkMode: types.CallChainSinkResolutionExact,
			},
		}},
		EvidenceItems: []types.EvidenceItem{{
			ID: "body", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
			Subject: "AuditLog.record", Object: "System.out.println", OwnerSymbol: "AuditLog.record",
			Source: "src/AuditLog.java", LineStart: 6, Scope: types.ScopeLine,
			GroundingStatus: types.GroundingGrounded, Producer: types.EvidenceProducerRepoMapTerminalBodyCall,
		}},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"body_call_facts=`AuditLog.record -> System.out.println @ src/AuditLog.java:6`",
		"selected_terminal_body_calls=`parser_grounded`",
		"caller=`AuditLog.record`; exact_operation=`System.out.println`; source=`src/AuditLog.java:6`; effect_scope=`exact_call_only`",
		"keep storage, durability, flushing, synchronization, and completion unproven unless separate typed evidence establishes them",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("terminal-body fact and final authority must agree on %q:\n%s", want, prompt)
		}
	}
}

func TestSelectedTerminalImplementationBoundaryConceptualDestinationKeepsOneOperationPerCandidateLeaf(t *testing.T) {
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		CallChainEndpointProfile: &types.CallChainEndpointProfile{SinkMode: types.CallChainSinkResolutionDiscoverPath},
	}}}
	for line := 1; line <= 8; line++ {
		ctx.EvidenceItems = append(ctx.EvidenceItems, types.EvidenceItem{
			ID: "config", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
			Subject: "Config.resolve", Object: "helper.call", OwnerSymbol: "Config.resolve",
			Source: "src/Config.java", LineStart: line, Scope: types.ScopeLine,
			GroundingStatus: types.GroundingGrounded, Producer: types.EvidenceProducerRepoMapTerminalBodyCall,
		})
	}
	ctx.EvidenceItems = append(ctx.EvidenceItems, types.EvidenceItem{
		ID: "audit", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
		Subject: "AuditLog.record", Object: "System.out.println", OwnerSymbol: "AuditLog.record",
		Source: "src/AuditLog.java", LineStart: 6, Scope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded, Producer: types.EvidenceProducerRepoMapTerminalBodyCall,
	})

	got := renderAnswerDocSelectedTerminalImplementationBoundary(ctx)
	for _, want := range []string{
		"terminal_body_candidates=`parser_grounded`; candidate_count=`2`",
		"Each caller below is a candidate endpoint, not automatically the requested business destination",
		"caller=`AuditLog.record`; exact_operation=`System.out.println`",
		"Investigator closure wording, names, comments, and layer labels do not upgrade these exact operations",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("conceptual destination boundary missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "caller=`Config.resolve`") != 1 {
		t.Fatalf("conceptual destination candidates should keep one operation per leaf, got:\n%s", got)
	}
}

func TestSelectedTerminalImplementationBoundaryExactSinkRetainsBoundedBodyDetail(t *testing.T) {
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		CallChainEndpointProfile: &types.CallChainEndpointProfile{
			Source: "Source.run", Sink: "Config.resolve", SinkMode: types.CallChainSinkResolutionExact,
		},
	}}}
	for line := 1; line <= 9; line++ {
		ctx.EvidenceItems = append(ctx.EvidenceItems, types.EvidenceItem{
			ID: fmt.Sprintf("config-%d", line), Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
			Subject: "Config.resolve", Object: fmt.Sprintf("helper.call%d", line), OwnerSymbol: "Config.resolve",
			Source: "src/Config.java", LineStart: line, Scope: types.ScopeLine,
			GroundingStatus: types.GroundingGrounded, Producer: types.EvidenceProducerRepoMapTerminalBodyCall,
		})
	}
	got := renderAnswerDocSelectedTerminalImplementationBoundary(ctx)
	if !strings.Contains(got, "selected_terminal_body_calls=`parser_grounded`") ||
		strings.Contains(got, "terminal_body_candidates=") || strings.Count(got, "caller=`Config.resolve`") != 8 {
		t.Fatalf("exact typed sink should retain the bounded selected-body detail:\n%s", got)
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
		"kind: \"summary\"",
		"surface_role: \"principal\"",
		"invalid on every other block kind, including `section`",
		"`trace_causal_claim_caliber`",
		"`no_causal_conclusion|bounded_window_candidate`",
		"trace_causal_claim_caliber_mapping",
		"`bounded_window_candidate` when the lead names or ranks selected-window candidates",
		"Evidence-status values such as `unproven` are not JSON enum values",
		"No conclusion is inferred from prose or written for you",
		"causal_conclusion=`unproven`",
		"frame_evidence_status=`absent`",
		"frame_evidence_status_semantics=`no target-bound frame/deadline evidence was produced in the selected evidence`",
		"proves neither that a frame drop occurred nor that no frame drop occurred",
		"selected_window_authority artifact=`customer.systrace`; selected_window=`10.000000..10.020000`",
		"time_role_authority artifact=`customer.systrace`; selected_query_window=`10.000000..10.020000`; selected_query_window_duration=20.000ms",
		"attachment_extent_role=`artifact_navigation_only_not_selected_window_duration`",
		"out_of_window_artifact_preview=`navigation_only_not_selected_window_evidence`",
		"frame_boundary_authority=`not_provided`",
		"scheduler_state_interval_authority=`typed_state_segments`",
		"time from wakeup until the next sched-in is runnable_wait",
		"trace_value_caliber_authority=`measured_occupancy_vs_effective_attribution`",
		"never call it an actual wait/state duration",
		"target_direct_blocking_authority=`unavailable_without_typed_target`",
		"direct_blocking_decision=`not_established`",
		"fix_direction_summary_authority=`exact_typed_subtotal_when_published_else_single_leader`",
		"cross_direction_joint_total_authority=`not_provided`",
		"compact_unknowns: evidence_absence_implication=`unknown_not_false`",
		"cross_direction_physical_relation=`unresolved_unless_an_exact_pair_row_says_otherwise`",
		"target_direct_blocking_not_established_does_not_prove_no_external_blocking=`true`",
		"absent_overlap_record_proves_independence=`false`",
		"cause_decomposition_status=`not_closed_by_state_partition_or_ranked_seat_roster`",
		"exhaustive_cause_wording=`requires_one_exact_typed_additive_cause_partition`",
		"direction_subtotal_authority=`not_provided_without_exact_fold`",
		"leader_subject=`worker-200`",
		"cross_row_addition=`not_authorized_without_exact_typed_relation`",
		"final_synthesis_scope: principal_root_cause_population=`typed_on_chain_only`",
		"adjacent_and_background_role=`supporting_context_and_additional_investigation_only`",
		"actual_occupancy_and_existing_rule_eliminable=`separate_decision_axes`",
		"frame_claim_scope=`selected_window_observations_only`",
		"out_of_window_marker_role=`navigation_only`",
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

func TestFinalLogPeerDecisionBoundarySuppressesNoisyRelationSynthesisButKeepsModelOwnership(t *testing.T) {
	mu := types.NewMutableState("compare the two runtime errors")
	mu.SetLogTriage(&types.LogBundle{Errors: []types.LogError{
		{Type: "cangjie_panic", Message: "native panic"},
		{Type: "arkts_error", Message: "bridge invocation failed"},
	}})
	mu.SetInvestigationComplete("the Cangjie panic propagated through the bridge into ArkTS")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{InvestigationNotes: []string{
		"Cangjie is the caller and ArkTS is the callee",
	}})
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentRootCause, Scenario: types.ScenarioRootCause,
		}},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Final Runtime Error Relation Boundary (Typed Facts; Model-Owned Conclusion)",
		"cross_error_relation=`unproven`",
		"no validated explicit artifact marker connects one top-level error",
		"Answer the requested per-error/frame dimensions from the typed occurrences",
		"The final wording and conclusion remain model-authored",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("peer-log final boundary missing %q:\n%s", want, prompt)
		}
	}
	if strings.LastIndex(prompt, "## Final Runtime Error Relation Boundary") < strings.LastIndex(prompt, "## Submission Checklist") {
		t.Fatalf("peer-log final boundary must follow generic guidance:\n%s", prompt)
	}
	for _, forbidden := range []string{
		"## Investigation Narrative Handoff",
		"Cangjie is the caller and ArkTS is the callee",
		"the Cangjie panic propagated through the bridge into ArkTS",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("untyped peer relation synthesis leaked into finalizer context %q:\n%s", forbidden, prompt)
		}
	}
	if got := mu.StableInvestigationCompleteReason(); got == "" {
		t.Fatal("context projection must not delete the model-authored audit history")
	}
	if turnA := mu.TurnAArtifacts(); turnA == nil || len(turnA.InvestigationNotes) != 1 {
		t.Fatalf("context projection must preserve raw investigation notes for audit: %+v", turnA)
	}
}

func TestFinalLogPeerDecisionBoundaryDoesNotApplyToOneError(t *testing.T) {
	mu := types.NewMutableState("inspect one runtime error")
	mu.SetLogTriage(&types.LogBundle{Errors: []types.LogError{{Type: "cangjie_panic"}}})
	mu.SetTurnAArtifacts(types.TurnAArtifacts{InvestigationNotes: []string{"single-error context"}})
	ctx := &types.AgentContext{Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentRootCause, Scenario: types.ScenarioRootCause,
	}}}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(prompt, "## Final Runtime Error Relation Boundary") {
		t.Fatalf("single error was misclassified as a peer-relation boundary:\n%s", prompt)
	}
	if !strings.Contains(prompt, "## Investigation Narrative Handoff") || !strings.Contains(prompt, "single-error context") {
		t.Fatalf("single-error advisory handoff was over-suppressed:\n%s", prompt)
	}
}

func TestFinalPerfSampleStatisticalBoundaryUsesTypedCaliberAndStaysLast(t *testing.T) {
	mu := types.NewMutableState("inspect perf samples")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{{
			ID: "perf-caliber", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
			SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace"},
			ClaimKey:  "perf_sample_statistical_caliber", Subject: "perf_sample_population",
			Predicate: "perf_sample_statistical_caliber", Object: "single_observation_no_comparison",
			Value: "1", Unit: "samples",
			RichNotes: []string{types.TraceNoteKeyPerfStatisticalCaliber + "=observed_sample_count=1,observed_rank_scope=single_observation_no_comparison,weight_share_scope=within_observed_same_event_unit_cohort_only_not_elapsed_time_or_cpu_utilization,workload_hotspot_inference=not_established_by_perf_context_alone,temporal_coverage_fraction=unavailable,sampling_design_receipt=not_carried_by_perf_context"},
		}},
	}}})
	ctx := &types.AgentContext{Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain, Scenario: types.ScenarioPerformanceBottleneck,
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet},
	}}}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Final Perf-Sample Statistical Boundary (Typed Facts; Model-Owned Conclusion)",
		"observed_sample_count=`1`",
		"observed_rank_scope=`single_observation_no_comparison`",
		"not CPU utilization",
		"rather than a comparative workload hotspot",
		"workload-hotspot confidence and temporal coverage remain unavailable",
		"The final explanation remains model-authored",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("perf statistical boundary missing %q:\n%s", want, prompt)
		}
	}
	if strings.LastIndex(prompt, "## Final Perf-Sample Statistical Boundary") < strings.LastIndex(prompt, "## Submission Checklist") {
		t.Fatalf("perf statistical boundary must follow generic guidance:\n%s", prompt)
	}
}

func TestTraceFinalStateValueAuthoritySeparatesMeasuredOccupancyFromEffectiveAttribution(t *testing.T) {
	projection := types.TraceCausalProjection{
		ArtifactLabel: "customer.systrace",
		BackgroundCauses: []types.TraceCausalProjectionNode{{
			EvidenceID: "logger-bg", Subject: "logger-900", StateKind: "d_sleep",
			ImpactMS: 7, CumulativeImpactMS: 19.5, EffectiveImpactMS: 7,
			StartTs: 2.0005, EndTs: 2.020,
		}},
	}
	got := renderTraceFinalStateValueAuthority(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}})
	for _, want := range []string{
		"subject=`logger-900`",
		"state_kind=`d_sleep`",
		"measured_state_occupancy=19.500ms",
		"effective_attribution=7.000ms",
		"relation=`distinct_do_not_substitute`",
		"occurrence_interval=`2.000500..2.020000`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("state-value authority missing %q:\n%s", want, got)
		}
	}

	projection.BackgroundCauses[0].EffectiveImpactMS = 19.5
	if got := renderTraceFinalStateValueAuthority(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}}); got != "" {
		t.Fatalf("equal measured/effective values need no distinction row: %s", got)
	}
}

func TestTraceFinalTimeRoleAuthorityKeepsSelectedWindowStateSeparateFromAttachmentExtent(t *testing.T) {
	projection := types.TraceCausalProjection{
		ArtifactLabel: "customer.systrace", WindowStartTs: 2, WindowEndTs: 2.020,
		TargetStateAccount: &types.TraceCausalProjectionTargetStateAccount{
			Subject: "app-100", SleepMS: 20, TotalMS: 20, WindowStartTs: 2, WindowEndTs: 2.020,
		},
	}
	got := renderTraceFinalTimeRoleAuthority(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}})
	for _, want := range []string{
		"selected_query_window=`2.000000..2.020000`",
		"selected_query_window_duration=20.000ms",
		"attachment_extent_role=`artifact_navigation_only_not_selected_window_duration`",
		"out_of_window_switch_in_role=`separate_event_not_selected_window_state_duration`",
		"selected_window_target_state subject=`app-100`",
		"sleep=20.000ms",
		"accounted_total=20.000ms",
		"partition_members=`five_engine_lanes`",
		"sleep_state_semantics=`state_only_mechanism_unproven`",
		"not that it was normal pacing, downstream-response waiting, lock/condition waiting, IPC, timer/event waiting, or the root cause",
		"duration_selection_rule=`use_the_value_whose_typed_role_matches_the_sentence`",
		"A post-wakeup runnable/dispatch duration requires its own typed interval",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("final time-role authority missing %q:\n%s", want, got)
		}
	}
}

func TestTraceFinalBlockedReasonStateRelationKeepsRecordCensusSeparateFromStateIntervals(t *testing.T) {
	count := 12
	record := types.ObservationRecord{
		ID: "blocked-census", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
		SourceRef: types.ObservationSourceRef{
			Kind: types.ObservationSourceRuntimeArtifact, Path: "/captures/donghu.ftrace",
		},
		Predicate: "blocked_reason_census", Subject: "CompThread_0-2955",
		Value: "12", ResultCount: &count,
		RichNotes: []string{
			types.TraceNoteKeySelectedWindow + "=13762.791708..13763.024898",
			types.TraceNoteKeyBlockedReasonCensus + "=dma_fence_default_w×12(Σ39.157ms)",
		},
	}
	projection := types.TraceCausalProjection{
		ArtifactPath: "/captures/donghu.ftrace", ArtifactLabel: "donghu.ftrace",
		WindowStartTs: 13762.791708, WindowEndTs: 13763.024898,
		TargetStateAccount: &types.TraceCausalProjectionTargetStateAccount{
			Subject: "CompThread_0-2955", DStateMS: 36.757, IOWaitMS: 0,
			TotalMS: 36.757, WindowStartTs: 13762.791708, WindowEndTs: 13763.024898,
		},
	}
	got := renderTraceFinalBlockedReasonStateRelation(
		types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}},
		types.ObservationLedger{Records: []types.ObservationRecord{record}},
	)
	for _, want := range []string{
		"subject=`CompThread_0-2955`",
		"scheduler_state_caliber=`sched_switch_interval_wall_clock`",
		"d_state=36.757ms",
		"blocked_reason_records=12",
		"blocked_reason_census=`dma_fence_default_w×12(Σ39.157ms)`",
		"blocked_reason_caliber=`kernel_record_count_and_vendor_reported_delay_sum`",
		"relation=`unjoined_distinct_observation_domains`",
		"record_to_state_occurrence_mapping=`not_provided`",
		"count_or_delay_difference_interpretation=`forbidden`",
		"Do not pair records with state segments",
		"unless a separate typed interval join provides that mapping",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("blocked-reason/state relation missing %q:\n%s", want, got)
		}
	}

	wrongCapture := record
	wrongCapture.SourceRef.Path = "/captures/other.ftrace"
	wrongWindow := record
	wrongWindow.RichNotes = []string{
		types.TraceNoteKeySelectedWindow + "=13762.000000..13762.100000",
		types.TraceNoteKeyBlockedReasonCensus + "=dma_fence_default_w×12(Σ39.157ms)",
	}
	wrongSubject := record
	wrongSubject.Subject = "another-2956"
	for name, candidate := range map[string]types.ObservationRecord{
		"capture": wrongCapture, "window": wrongWindow, "subject": wrongSubject,
	} {
		if got := renderTraceFinalBlockedReasonStateRelation(
			types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}},
			types.ObservationLedger{Records: []types.ObservationRecord{candidate}},
		); got != "" {
			t.Fatalf("cross-%s census must not bind to the selected state account: %s", name, got)
		}
	}
}

func TestTraceFinalTargetWaitEnumerationAuthorityOverridesCandidateDisplayCaps(t *testing.T) {
	const subject = "CompThread_0-2955"
	count := 3
	ref := types.ObservationSourceRef{
		Kind: types.ObservationSourceRuntimeArtifact, Path: "/captures/donghu.ftrace", ArtifactID: "donghu.ftrace",
	}
	records := []types.ObservationRecord{{
		ID: "trace_query:window#target_window_wait_occurrences", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
		Span: types.ObservationSpan{StartTs: 10, EndTs: 10.020}, Predicate: "target_window_wait_occurrences",
		Subject: subject, Object: "complete", Value: "3", ResultCount: &count,
	}}
	for i := 1; i <= count; i++ {
		start := 10 + float64(i)*0.002
		records = append(records, types.ObservationRecord{
			ID:     fmt.Sprintf("trace_query:window#target_window_wait_occurrence:%d", i),
			Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
			Span:      types.ObservationSpan{StartTs: start, EndTs: start + 0.001},
			Predicate: "target_window_wait_occurrence", Subject: subject,
			Object: "state=d_sleep;iowait=0;caller=dma_fence_default_w", Value: "1.000", Unit: "ms",
		})
	}
	rm := &types.RequestModel{RuntimeTargets: []types.RuntimeTarget{{
		Kind: types.RuntimeTargetKindThread, PID: 2955, Thread: subject, Source: "user_explicit",
	}}}
	got := renderTraceFinalTargetWaitEnumerationAuthority(types.ObservationLedger{Records: records}, rm)
	for _, want := range []string{
		"rowset_permission=`exact_complete_same_result`",
		"occurrence_count=3",
		"complete_occurrence_ordinals=`1..3`",
		"wall_clock_sum=3.000ms",
		"candidate_view_compaction_role=`does_not_downgrade_this_rowset`",
		"missing_occurrence_inference=`forbidden`",
		"residual_count_or_duration_estimation=`forbidden`",
		"does not prove that any occurrence in this complete target-wait rowset is missing",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("complete wait enumeration authority missing %q:\n%s", want, got)
		}
	}

	records = records[:len(records)-1]
	if got := renderTraceFinalTargetWaitEnumerationAuthority(types.ObservationLedger{Records: records}, rm); got != "" {
		t.Fatalf("incomplete same-result rowset must fail closed: %s", got)
	}
}

func TestTraceFinalSupplyFoldValueAuthorityPublishesTypedEquation(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		EvidenceID: "rank-1", Subject: "CompThread_0-2955", StateKind: "running",
		CumulativeImpactMS: 74.915, EffectiveImpactMS: 65.912,
		SupplyFoldComputed: true, SupplyFoldDeficitMS: 65.912, SupplyFoldIdealMS: 9.003,
		SupplyFoldKnownMS: 74.915,
	}
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{
		ArtifactLabel: "donghu.ftrace", RankedSeats: []types.TraceCausalProjectionNode{node},
	}}}
	got := renderTraceFinalSupplyFoldValueAuthority(set)
	for _, want := range []string{
		"folded_running_total=74.915ms",
		"ideal_equivalent_running=9.003ms",
		"low_frequency_supply_deficit=65.912ms",
		"equation=`ideal_equivalent_running + low_frequency_supply_deficit = folded_running_total`",
		"effective_to_deficit_relation=`same_numeric_value_as_supply_deficit_for_this_seat`",
		"occupancy_minus_effective_role=`not_a_supply_deficit_formula`",
		"Do not derive or rename measured occupancy minus effective attribution as another supply deficit",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("supply-fold value authority missing %q:\n%s", want, got)
		}
	}

	conflicted := node
	conflicted.SupplyFoldKnownMS = 70
	set.Projections[0].RankedSeats = []types.TraceCausalProjectionNode{conflicted}
	if got := renderTraceFinalSupplyFoldValueAuthority(set); got != "" {
		t.Fatalf("inconsistent fold coverage must fail closed: %s", got)
	}
}

func TestFinalizerPromptCarriesBlockedReasonStateRelationAtProductionBoundary(t *testing.T) {
	count := 12
	ref := types.ObservationSourceRef{
		Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "customer.systrace", ArtifactKind: "trace",
	}
	root := types.ObservationRecord{
		ID: "root", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
		Predicate: "root_cause_primary", ClaimKey: "root_cause_primary:CompThread_0-2955",
		Subject: "CompThread_0-2955", Object: "d_state_or_io_wait", Value: "36.757", Unit: "ms",
		RichNotes: []string{
			"rank=1", "tier=primary", "chain_relevance=on_chain", "effective_impact_ms=36.757",
			types.TraceNoteKeySelectedWindow + "=13762.791708..13763.024898",
		},
	}
	state := types.ObservationRecord{
		ID: "state", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
		Predicate: "target_window_states", ClaimKey: "target_window_states:CompThread_0-2955",
		Subject: "CompThread_0-2955", Object: "state_partition", Value: "233.190", Unit: "ms",
		RichNotes: []string{
			types.TraceNoteKeySelectedWindow + "=13762.791708..13763.024898",
			types.TraceNoteKeyRunning + "=74.915", types.TraceNoteKeyRunnable + "=1.536",
			types.TraceNoteKeySleep + "=118.586", types.TraceNoteKeyDState + "=36.757",
			types.TraceNoteKeyIOWait + "=0.000", types.TraceNoteKeyTotal + "=231.794",
		},
	}
	census := types.ObservationRecord{
		ID: "census", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
		Predicate: "blocked_reason_census", ClaimKey: "blocked_reason_census:CompThread_0-2955",
		Subject: "CompThread_0-2955", Object: "blocked_reason", Value: "12", ResultCount: &count,
		RichNotes: []string{
			types.TraceNoteKeySelectedWindow + "=13762.791708..13763.024898",
			types.TraceNoteKeyBlockedReasonCensus + "=dma_fence_default_w×12(Σ39.157ms)",
		},
	}
	ctx := answerDocCausalCeilingTestContext(false)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{View: "root_cause_rank"},
		Observations:           []types.ObservationRecord{root, state, census},
	}}})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "blocked_reason_state_relation subject=`CompThread_0-2955`") ||
		!strings.Contains(prompt, "relation=`unjoined_distinct_observation_domains`") ||
		!strings.Contains(prompt, "count_or_delay_difference_interpretation=`forbidden`") {
		t.Fatalf("production Finalizer prompt lost the typed census/state relation:\n%s", prompt)
	}
	if strings.LastIndex(prompt, "blocked_reason_state_relation") < strings.LastIndex(prompt, "## Submission Checklist") {
		t.Fatalf("census/state relation must stay in the final typed decision boundary:\n%s", prompt)
	}
}

func TestFinalizerPromptCarriesCompleteWaitAndSupplyFoldRelationsAtProductionBoundary(t *testing.T) {
	const (
		subject = "CompThread_0-2955"
		start   = 13762.791708
		end     = 13763.024898
	)
	ref := types.ObservationSourceRef{
		Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "donghu.ftrace", ArtifactKind: "trace",
	}
	root := types.ObservationRecord{
		ID: "root-running", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
		Span:      types.ObservationSpan{StartTs: start, EndTs: end, LineStart: 140, LineEnd: 26117},
		Predicate: "root_cause_primary", ClaimKey: "root_cause_primary:" + subject,
		Subject: subject, Object: "running", Value: "74.915", Unit: "ms",
		RichNotes: []string{
			"rank=1", "tier=primary", "chain_relevance=on_chain", "dominant_state=running",
			"cumulative_impact_ms=74.915", "effective_impact_ms=65.912",
			types.TraceNoteKeyFoldBasis + "=known=74.915ms,unknown=0.000ms",
			types.TraceNoteKeySupplyFoldDeficitMS + "=65.912",
			types.TraceNoteKeySupplyFoldIdealMS + "=9.003",
			types.TraceNoteKeySelectedWindow + "=13762.791708..13763.024898",
		},
	}
	state := types.ObservationRecord{
		ID: "state", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
		Predicate: "target_window_states", ClaimKey: "target_window_states:" + subject,
		Subject: subject, Object: "state_partition", Value: "233.190", Unit: "ms",
		RichNotes: []string{
			types.TraceNoteKeySelectedWindow + "=13762.791708..13763.024898",
			types.TraceNoteKeyRunning + "=74.915", types.TraceNoteKeyRunnable + "=1.536",
			types.TraceNoteKeySleep + "=118.586", types.TraceNoteKeyDState + "=36.757",
			types.TraceNoteKeyIOWait + "=0.000", types.TraceNoteKeyTotal + "=231.794",
		},
	}
	count := 3
	waitAggregate := types.ObservationRecord{
		ID: "trace_query:window#target_window_wait_occurrences", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
		Span: types.ObservationSpan{StartTs: start, EndTs: end}, Predicate: "target_window_wait_occurrences",
		Subject: subject, Object: "complete", Value: "3", ResultCount: &count,
	}
	records := []types.ObservationRecord{root, state, waitAggregate}
	for i := 1; i <= count; i++ {
		rowStart := start + float64(i)*0.002
		records = append(records, types.ObservationRecord{
			ID:     fmt.Sprintf("trace_query:window#target_window_wait_occurrence:%d", i),
			Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
			Span:      types.ObservationSpan{StartTs: rowStart, EndTs: rowStart + 0.001},
			Predicate: "target_window_wait_occurrence", Subject: subject,
			Object: "state=d_sleep;iowait=0;caller=dma_fence_default_w", Value: "1.000", Unit: "ms",
		})
	}
	ctx := answerDocCausalCeilingTestContext(false)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{
		Kind: types.RuntimeTargetKindThread, PID: 2955, Thread: subject, Source: "user_explicit",
	}}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{View: "root_cause_rank"},
		Observations:           records,
	}}})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"target_wait_enumeration_authority artifact=`donghu.ftrace`",
		"candidate_view_compaction_role=`does_not_downgrade_this_rowset`",
		"missing_occurrence_inference=`forbidden`",
		"supply_fold_value_authority artifact=`donghu.ftrace`",
		"low_frequency_supply_deficit=65.912ms",
		"ideal_equivalent_running=9.003ms",
		"occupancy_minus_effective_role=`not_a_supply_deficit_formula`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("production Finalizer prompt lost %q:\n%s", want, prompt)
		}
	}
	for _, marker := range []string{"target_wait_enumeration_authority", "supply_fold_value_authority"} {
		if strings.LastIndex(prompt, marker) < strings.LastIndex(prompt, "## Submission Checklist") {
			t.Fatalf("%s must remain in the final typed decision boundary:\n%s", marker, prompt)
		}
	}
}

func TestTraceFinalSelectedWindowAuthorityKeepsPreviewOutsideTypedWindow(t *testing.T) {
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{
		{ArtifactLabel: "customer.systrace", WindowStartTs: 10, WindowEndTs: 10.1},
		{ArtifactLabel: "unbounded.systrace"},
	}}
	got := renderTraceFinalSelectedWindowAuthority(set, "absent")
	for _, want := range []string{
		"selected_window=`10.000000..10.100000`",
		"out_of_window_artifact_preview=`navigation_only_not_selected_window_evidence`",
		"cannot establish selected-window state, event order, duration, frame boundary, completion, or deadline",
		"unless a separate typed relation explicitly binds it into this projection",
		"frame_boundary_authority=`not_provided`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("selected-window final authority missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "unbounded.systrace") {
		t.Fatalf("a projection without a typed selected window must not mint a window boundary:\n%s", got)
	}
}

func TestTraceFinalAggregateScaleAuthorityRequiresTypedAbsoluteLevel(t *testing.T) {
	facts := []traceDecisionAggregateFact{
		{Kind: "supply_pressure", Signal: "cpu_pressure", Calibration: [][2]string{{"pressure_density", "5.26"}}},
		{Kind: "io_pressure", Signal: "io_pressure", Calibration: [][2]string{{"absolute_level", "high"}}},
	}
	got := renderTraceFinalAggregateScaleAuthority(facts)
	for _, want := range []string{
		"aggregate_absolute_level_authority=`not_provided`",
		"affected_signals=`cpu_pressure`",
		"does not by itself mean low/medium/high or serious/not-serious",
		"observed value/density; absolute level unavailable without calibration",
		"do not supply an absolute severity adjective",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("aggregate scale authority missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "io_pressure") {
		t.Fatalf("an aggregate with typed absolute_level must not be reported as uncalibrated: %s", got)
	}
	if got := renderTraceFinalAggregateScaleAuthority([]traceDecisionAggregateFact{{
		Kind: "io_pressure", Signal: "io_pressure", Calibration: [][2]string{{"absolute_level", "low"}},
	}}); got != "" {
		t.Fatalf("fully calibrated aggregate facts need no negative authority: %s", got)
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
			{EvidenceID: "rank-1", Subject: "worker-200", Rank: 1, EffectiveImpactMS: 8, FixDirection: "io_dependency", ChainRelevance: "on_chain", WithinRequestedWindow: &inWindow},
			{EvidenceID: "rank-2", Subject: "worker-201", Rank: 2, EffectiveImpactMS: 4, FixDirection: "io_dependency", ChainRelevance: "on_chain", WithinRequestedWindow: &inWindow},
			{EvidenceID: "rank-3", Subject: target, Rank: 3, EffectiveImpactMS: 3, FixDirection: "scheduling", ChainRelevance: "on_chain", BlockingKind: "lock_contention", BlockingPeer: "holder-300", WithinRequestedWindow: &inWindow},
		},
	}
	got := renderTraceFinalCompactAuthorityLedger(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}})
	for _, want := range []string{
		"target_direct_blocking_authority=`typed_waiter_holder`",
		"direct_blocking_decision=`established_by_typed_relation`",
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
		!strings.Contains(got, "direct_blocking_decision=`not_established`") ||
		!strings.Contains(got, "say that no typed direct blocker was established for this target") ||
		!strings.Contains(got, "do not promote a wakeup peer, IRQ peer, kernel caller, adjacent row, or another thread's blocking interval") ||
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
				ChainRelevance:         "on_chain",
				RankQueryWindowStartTs: 10.02, RankQueryWindowEndTs: 10.07,
			},
			{
				EvidenceID: "full-io", Subject: "full-worker", Rank: 3,
				EffectiveImpactMS: 10.433, FixDirection: "io_dependency", ChainDepth: 2,
				ChainRelevance:         "on_chain",
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
		"mechanism_ceiling=`on_chain_prewakeup_work_candidate_only`",
		"target_wait_for_work_authority=`not_provided_by_this_seat`",
		"work_completion_dependency_authority=`not_provided_by_this_seat`",
		"direct_blocking_authority=`not_provided_by_this_seat`",
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

func TestTraceFinalDecisionLedgerKeepsBlockedReasonCallerOnExactPartitionSeat(t *testing.T) {
	inWindow := true
	unproven := types.TraceCausalProjectionNode{
		EvidenceID: "unproven-seat", Subject: "worker-60555", Rank: 3,
		StateKind: "d_sleep", EffectiveImpactMS: 10.433, FixDirection: "io_dependency",
		ChainRelevance:               "on_chain",
		DStateCauseUnprovenRemainder: true, BlockedReasonWindowCount: 17,
		WithinRequestedWindow: &inWindow,
	}
	proved := types.TraceCausalProjectionNode{
		EvidenceID: "proved-seat", Subject: "worker-60555", Rank: 5,
		StateKind: "io_wait", EffectiveImpactMS: 7.386, FixDirection: "io_dependency",
		ChainRelevance:      "on_chain",
		BlockedReasonCaller: "fscache_page_wait_o", WithinRequestedWindow: &inWindow,
	}
	projection := types.TraceCausalProjection{
		ArtifactLabel: "customer.systrace", WindowStartTs: 10, WindowEndTs: 10.1,
		RankedSeats:   []types.TraceCausalProjectionNode{unproven, proved},
		OnChainCauses: []types.TraceCausalProjectionNode{unproven, proved},
	}
	got := renderTraceFinalCompactAuthorityLedger(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}})
	for _, want := range []string{
		"leader_subject=`worker-60555`",
		"leader_effective_attribution=10.433ms",
		"row_identity=`unproven-seat`",
		"blocking_reason_authority=`not_provided_by_this_seat`",
		"blocked_reason_caller=`not_provided`",
		"sibling_caller_transfer=`forbidden`",
		"allowed_mechanism_scope=`measured_state_occupancy_with_unknown_blocking_reason`",
		"not_authorized_mechanisms=`sibling_caller,irq_or_storage_cause,resource_or_holder_identity,cross_row_delay`",
		"window_blocked_reason_records=17",
		"window_record_binding_to_this_seat=`not_provided`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("partition-seat caller boundary missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "fscache_page_wait_o") {
		t.Fatalf("sibling caller leaked into the unproven direction leader:\n%s", got)
	}

	var relation strings.Builder
	traceDecisionWriteNodeBlockingReasonAuthority(&relation, proved)
	for _, want := range []string{
		"blocked_reason_caller=`fscache_page_wait_o`",
		"caller_scope=`this_seat_only`",
		"sibling_caller_transfer=`forbidden`",
		"holder_authority=`not_provided_by_caller`",
		"allowed_mechanism_scope=`kernel_reported_wait_callsite_for_this_seat`",
		"not_authorized_mechanisms=`sibling_or_cross_row_cause,holder_identity,resource_identity`",
	} {
		if !strings.Contains(relation.String(), want) {
			t.Fatalf("proved seat caller scope missing %q: %s", want, relation.String())
		}
	}
}

func TestTraceFinalSynthesisScopeCalibratesCandidateWithoutChangingPopulation(t *testing.T) {
	inWindow := true
	candidate := types.TraceCausalProjectionNode{
		EvidenceID: "candidate", Subject: "CookieMonsterCl-59843",
		TypeToken: "priority_inversion_candidate", StateKind: "runnable",
		Rank: 1, EffectiveImpactMS: 23.994, FixDirection: "lock_priority",
		ChainDepth: 1, ChainRelevance: "on_chain", WithinRequestedWindow: &inWindow,
	}
	projection := types.TraceCausalProjection{
		WindowStartTs: 10, WindowEndTs: 10.1,
		RankedSeats:   []types.TraceCausalProjectionNode{candidate},
		OnChainCauses: []types.TraceCausalProjectionNode{candidate},
		AdjacentCauses: []types.TraceCausalProjectionNode{{
			EvidenceID: "adjacent", Subject: "background-7", Rank: 2,
			EffectiveImpactMS: 99, ChainRelevance: "adjacent", WithinRequestedWindow: &inWindow,
		}},
	}
	got := renderTraceFinalSynthesisScope(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}}, "absent")
	for _, want := range []string{
		"principal_root_cause_population=`typed_on_chain_only`",
		"adjacent_and_background_role=`supporting_context_and_additional_investigation_only`",
		"candidate_subject=`CookieMonsterCl-59843`; effective_attribution=23.994ms",
		"claim_envelope=`measured_lower_priority_dependency_supply_candidate`",
		"allowed_mechanism_scope=`measured_dependency_scheduler_supply_before_downstream_wakeup`",
		"not_authorized_mechanisms=`priority_inversion_occurrence,post_wakeup_delay,lock_or_holder_waiter,synchronous_blocking`",
		"holder_waiter_authority=`not_provided_by_candidate_seat`",
		"priority_candidate_scope=`dependency_scheduler_supply_before_downstream_wakeup`",
		"post_wakeup_preemption_authority=`not_provided_by_this_seat`",
		"frame_evidence_status=`absent`",
		"out_of_window_marker_role=`navigation_only`",
		"frame_boundary_completion_deadline_authority=`not_provided`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("final synthesis scope missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "background-7") || strings.Contains(got, "99.000") || strings.Contains(got, "lock_priority") {
		t.Fatalf("adjacent seat or uncalibrated registry direction leaked into final scope:\n%s", got)
	}
}

func TestTraceFinalLeaderMechanismCeilingIsSalientWithoutTypedTargetBlocker(t *testing.T) {
	inWindow := true
	leader := types.TraceCausalProjectionNode{
		EvidenceID: "pre-wakeup-leader", Subject: "worker-200", Rank: 1,
		EffectiveImpactMS: 4.6, ImpactMS: 4.6, StateKind: "running",
		FixDirection: "self_workload", ChainDepth: 1, ChainRelevance: "on_chain",
		WithinRequestedWindow: &inWindow,
	}
	projection := types.TraceCausalProjection{
		ArtifactLabel:      "customer.systrace",
		TargetStateAccount: &types.TraceCausalProjectionTargetStateAccount{Subject: "app-100", TotalMS: 7},
		RankedSeats:        []types.TraceCausalProjectionNode{leader},
		OnChainCauses:      []types.TraceCausalProjectionNode{leader},
	}
	got := renderTraceFinalSynthesisScope(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}}, "absent")
	for _, want := range []string{
		"final_answer_mechanism_scope artifact=`customer.systrace`",
		"subject=`worker-200`; target=`app-100`",
		"only as on-chain work overlapping the interval before the target wakeup",
		"No typed target-blocking relation establishes that the target waited for this work, waited for its completion, or was directly blocked by it",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("final leader mechanism ceiling missing %q:\n%s", want, got)
		}
	}
}

func TestTraceFinalLeaderMechanismCeilingYieldsToTypedTargetBlocker(t *testing.T) {
	inWindow := true
	leader := types.TraceCausalProjectionNode{
		EvidenceID: "pre-wakeup-leader", Subject: "worker-200", Rank: 1,
		EffectiveImpactMS: 4.6, ImpactMS: 4.6, StateKind: "running",
		FixDirection: "self_workload", ChainDepth: 1, ChainRelevance: "on_chain",
		WithinRequestedWindow: &inWindow,
	}
	blocker := types.TraceCausalProjectionNode{
		EvidenceID: "typed-blocker", Subject: "app-100", BlockingKind: "monitor_contention",
		BlockingPeer: "holder-300", WithinRequestedWindow: &inWindow,
	}
	projection := types.TraceCausalProjection{
		ArtifactLabel:      "customer.systrace",
		TargetStateAccount: &types.TraceCausalProjectionTargetStateAccount{Subject: "app-100", TotalMS: 7},
		RankedSeats:        []types.TraceCausalProjectionNode{leader, blocker},
		OnChainCauses:      []types.TraceCausalProjectionNode{leader, blocker},
	}
	got := renderTraceFinalSynthesisScope(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}}, "absent")
	if strings.Contains(got, "final_answer_mechanism_scope") {
		t.Fatalf("negative mechanism ceiling must yield to exact typed target blocker:\n%s", got)
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
