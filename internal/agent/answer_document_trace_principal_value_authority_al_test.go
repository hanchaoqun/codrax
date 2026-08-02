package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderAnswerDocTracePrincipalValueAuthorityCarriesCompleteElevenRowWaitSetAL(t *testing.T) {
	const (
		subject = "CompThread_0-2955"
		start   = 13762.791708
		end     = 13763.024898
	)
	count := 11
	ref := types.ObservationSourceRef{
		Kind:       types.ObservationSourceRuntimeArtifact,
		Path:       "/tmp/attached_trace.txt",
		ArtifactID: "attached_trace",
		PayloadRef: "trace-query-result.json",
	}
	aggregate := types.ObservationRecord{
		ID:              "trace_query:window#target_window_wait_occurrences",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef:       ref,
		ObservedAt:      "result-1",
		Span:            types.ObservationSpan{StartTs: start, EndTs: end},
		Subject:         subject,
		Predicate:       "target_window_wait_occurrences",
		Object:          "complete",
		Value:           "11",
		Unit:            "occurrences",
		ResultCount:     &count,
	}
	observations := []types.ObservationRecord{aggregate}
	for i := 1; i <= count; i++ {
		rowStart := start + float64(i)*0.002
		observations = append(observations, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:window#target_window_wait_occurrence:%d", i),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       ref,
			ObservedAt:      "result-1",
			Span:            types.ObservationSpan{StartTs: rowStart, EndTs: rowStart + 0.001},
			Subject:         subject,
			Predicate:       "target_window_wait_occurrence",
			Object:          "state=d_sleep;iowait=0;caller=dma_fence_default_w",
			Value:           "1.000",
			Unit:            "ms",
		})
	}
	ctx := tracePrincipalValueAuthorityTestContext(subject, 2955, observations)
	ctx.Language = "zh-CN"

	got := renderAnswerDocTracePrincipalValueAuthority(ctx)
	for _, want := range []string{
		"## Runtime Trace Principal Values — Final Typed Recap",
		"permission=`exact_complete_rowset`",
		"occurrence_count=11",
		"d_state_occurrences=11",
		"wall_clock_sum=11.000ms",
		"blocked_reason_callers=`dma_fence_default_w`",
		"caller_role=`kernel_reported_wait_callsite`",
		"holder_authority=`not_provided_by_caller`",
		"内核报告的等待调用点/符号为 dma_fence_default_w（不据此推断资源持有者）",
		"rather than blocked_reason record count or aggregate-group count",
		"principal_conclusion_zh=`CompThread_0-2955",
		"确切发生 11 次目标等待",
		"目标等待墙钟合计 11.000ms",
		"数值差本身不是关系证据",
		"不得把 record/occurrence/partition 的差值解释成窗口边界",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("principal-value recap missing %q:\n%s", want, got)
		}
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	recapAt := strings.Index(prompt, "## Runtime Trace Principal Values — Final Typed Recap")
	checklistAt := strings.Index(prompt, "## Submission Checklist")
	if recapAt < 0 || checklistAt < 0 || recapAt > checklistAt {
		t.Fatalf("principal-value recap must be wired immediately before the submission tail: recap=%d checklist=%d", recapAt, checklistAt)
	}
}

func TestRenderAnswerDocTracePrincipalValueAuthorityKeepsRequestedScopePrincipalAL(t *testing.T) {
	const subject = "com.baidu.tieba-59566"
	ref := types.ObservationSourceRef{
		Kind: types.ObservationSourceRuntimeArtifact,
		Path: "/tmp/attached_trace.txt", ArtifactID: "attached_trace",
	}
	makeRoster := func(scope string, start, end float64, durations []float64, supplement bool) []types.ObservationRecord {
		count := len(durations)
		records := []types.ObservationRecord{{
			ID:     "trace_query:" + scope + "#target_window_wait_occurrences",
			Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
			Span:      types.ObservationSpan{StartTs: start, EndTs: end},
			Predicate: "target_window_wait_occurrences", Subject: subject,
			Object: "complete", Value: fmt.Sprintf("%d", count), ResultCount: &count,
			SystemSupplement: supplement,
		}}
		cursor := start + 0.001
		for i, duration := range durations {
			rowEnd := cursor + duration/1000
			records = append(records, types.ObservationRecord{
				ID:     fmt.Sprintf("trace_query:%s#target_window_wait_occurrence:%d", scope, i+1),
				Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
				GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
				Span:      types.ObservationSpan{StartTs: cursor, EndTs: rowEnd},
				Predicate: "target_window_wait_occurrence", Subject: subject,
				Object: "state=io_wait;iowait=1;caller=sync_buffer_read_wi",
				Value:  fmt.Sprintf("%.3f", duration), Unit: "ms",
				SystemSupplement: supplement,
			})
			cursor = rowEnd + 0.001
		}
		return records
	}
	narrow := makeRoster("narrow", 10, 10.02, []float64{0.2, 0.3}, false)
	full := makeRoster("full", 10, 10.1, []float64{0.2, 0.3, 0.4}, false)
	ctx := tracePrincipalValueAuthorityTestContext(subject, 59566, narrow)
	ctx.Language = "zh"
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeFullArtifact,
		SourceQuote:    "这份 trace",
	}
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	ctx.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{
		Views:                  []string{"window_stats"},
		RequestedArtifactScope: types.RuntimeArtifactScopeFullArtifact,
	}, []types.ToolResult{{
		ToolName: "trace_query", Success: true, Observations: full,
	}})

	got := renderAnswerDocTracePrincipalValueAuthority(ctx)
	fullAt := strings.Index(got, "window=`10.000000..10.100000`")
	narrowAt := strings.Index(got, "window=`10.000000..10.020000`")
	if fullAt < 0 || narrowAt < 0 || fullAt > narrowAt {
		t.Fatalf("requested-scope row must lead supporting exploration: full=%d narrow=%d\n%s", fullAt, narrowAt, got)
	}
	for _, want := range []string{
		"scope_role=`requested_scope_principal`",
		"occurrence_count=3",
		"d_state_occurrences=0",
		"io_wait_occurrences=3",
		"principal_occurrence=`#3 state=io_wait",
		"scope_role=`supporting_exploration`",
		"supporting exploration window only",
		"Do not rename an `io_wait` row to D-state",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("requested-scope recap missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "principal_occurrence=`") != 3 {
		t.Fatalf("supporting roster leaked into principal occurrence list:\n%s", got)
	}
	if strings.Count(got, "principal_conclusion_zh=") != 1 ||
		!strings.Contains(got, "确切发生 3 次目标等待") {
		t.Fatalf("only the requested-scope row may mint a principal conclusion:\n%s", got)
	}
}

func TestRenderAnswerDocTracePrincipalValueAuthorityKeepsTruncatedBlockingAsLowerBoundAL(t *testing.T) {
	const subject = ".ugc.aweme.lite-17267"
	record := types.ObservationRecord{
		ID:              "blocking:binder",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef: types.ObservationSourceRef{
			Kind:       types.ObservationSourceRuntimeArtifact,
			Path:       "/tmp/attached_trace.txt",
			ArtifactID: "attached_trace",
		},
		Span:      types.ObservationSpan{StartTs: 13762.835861, EndTs: 13762.837270},
		ClaimKey:  "critical_blocking:binder_wait",
		Predicate: "critical_blocking",
		Subject:   subject,
		Object:    "binder:496_9-10961",
		Value:     "1.409",
		Unit:      "ms",
		RichNotes: []string{
			"type=binder_wait",
			"peer=binder:496_9-10961",
			"blocking_candidate=true",
			"selected_window=13762.791708..13763.024898",
			types.TraceNoteKeyCapacityTruncated + "=true",
		},
	}
	ctx := tracePrincipalValueAuthorityTestContext(subject, 17267, []types.ObservationRecord{record})
	ctx.Language = "zh"

	got := renderAnswerDocTracePrincipalValueAuthority(ctx)
	for _, want := range []string{
		"blocking_type=`binder_wait`",
		"permission=`lower_bound_only`",
		"observed_occurrences=>=1",
		"observed_wall_clock=>=1.409ms",
		"coverage_status=`lower_bound_capacity_truncated`",
		"never turn it into an exact total",
		"principal_conclusion_zh=`关于 binder_wait",
		"至少 1 次、至少 1.409ms",
		"全窗总次数和总量未知",
		"不能表述为只有、唯一或总计",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("lower-bound principal recap missing %q:\n%s", want, got)
		}
	}
}

func TestRenderAnswerDocTracePrincipalValueAuthorityRejectsEntitySupplementConsensusAL(t *testing.T) {
	const subject = "CompThread_0-2955"
	record := types.ObservationRecord{
		ID:              "blocking:dma",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef: types.ObservationSourceRef{
			Kind:       types.ObservationSourceRuntimeArtifact,
			Path:       "/tmp/attached_trace.txt",
			ArtifactID: "attached_trace",
		},
		Span:      types.ObservationSpan{StartTs: 1, EndTs: 1.004},
		ClaimKey:  "critical_blocking:d_state",
		Predicate: "critical_blocking",
		Subject:   subject,
		Object:    "dma_fence_default_w",
		Value:     "4.000",
		Unit:      "ms",
		RichNotes: []string{
			"type=d_state_or_io_wait",
			"selected_window=1.000000..2.000000",
			types.TraceNoteKeyCapacityTruncated + "=true",
		},
	}
	ctx := tracePrincipalValueAuthorityTestContext(subject, 2955, []types.ObservationRecord{record})
	ctx.AnalysisIR.RequestModel.RuntimeTargets[0].Source = types.RuntimeTargetSourceExplicitToolCall
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{subject}
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	ctx.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{
		Views:        []string{"root_cause_rank"},
		TargetPID:    2955,
		TargetThread: subject,
		TargetSource: "cursor",
	}, []types.ToolResult{{ToolName: "trace_query", Success: true}})

	got := renderAnswerDocTracePrincipalValueAuthority(ctx)
	if got != "" {
		t.Fatalf("entity + model cursor + supplement consensus must not mint finalizer user-target authority:\n%s", got)
	}
	if !types.RuntimeTargetIsExplorationCursorSource(ctx.AnalysisIR.RequestModel.RuntimeTargets[0].Source) {
		t.Fatalf("prompt-time consensus mutated persistent request model: %+v", ctx.AnalysisIR.RequestModel.RuntimeTargets)
	}
}

func TestRenderAnswerDocTracePrincipalValueAuthoritySkipsBoundedRelationAL(t *testing.T) {
	subject := "client-20"
	observations := []types.ObservationRecord{{
		ID: "state", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
		SourceRef: types.ObservationSourceRef{
			Kind: types.ObservationSourceRuntimeArtifact, Path: "/tmp/attached_trace.txt", ArtifactID: "attached_trace",
		},
		Predicate: "target_window_state", Subject: subject,
		Span:      types.ObservationSpan{StartTs: 3.0, EndTs: 3.05},
		RichNotes: []string{"running_ms=35", "runnable_ms=10", "sleep_ms=5", "d_state_ms=0", "io_wait_ms=0"},
	}}
	ctx := tracePrincipalValueAuthorityTestContext(subject, 20, observations)
	rm := &ctx.AnalysisIR.RequestModel
	rm.Intent = types.IntentExplain
	rm.AnalyzerHints.Kind = string(types.ReqCallChain)
	rm.PredicateAxis = types.AxisCall
	rm.Predicates.IsRelationalLookup = true
	rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet}
	if got := renderAnswerDocTracePrincipalValueAuthority(ctx); got != "" {
		t.Fatalf("bounded relation must not receive an unrelated state recap:\n%s", got)
	}
}

func TestRenderAnswerDocTracePrincipalValueAuthoritySeparatesWaitAndStateFamiliesAL(t *testing.T) {
	subject := "client-20"
	ref := types.ObservationSourceRef{
		Kind: types.ObservationSourceRuntimeArtifact, Path: "/tmp/attached_trace.txt", ArtifactID: "attached_trace",
	}
	observations := []types.ObservationRecord{{
		ID: "root", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
		SourceRef: ref, Predicate: "root_cause_primary", ClaimKey: "root_cause_primary:" + subject,
		Subject: subject, Object: "runnable", Value: "10", Unit: "ms",
		RichNotes: []string{
			"rank=1", "tier=primary", "chain_relevance=on_chain",
			"effective_impact_ms=10", "selected_window=3.000000..3.050000",
		},
	}, {
		ID: "state", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
		SourceRef: ref,
		Predicate: "target_window_states", ClaimKey: "target_window_states:" + subject,
		Subject: subject, Object: "state_partition", Value: "50", Unit: "ms",
		Span: types.ObservationSpan{StartTs: 3.0, EndTs: 3.05},
		RichNotes: []string{
			"selected_window=3.000000..3.050000",
			"running=35",
			"runnable=10",
			"sleep=5",
			"d_state=0",
			"io_wait=0",
			"total=50",
		},
	}}
	ctx := tracePrincipalValueAuthorityTestContext(subject, 20, observations)
	rm := &ctx.AnalysisIR.RequestModel
	rm.Intent = types.IntentExplain
	rm.Scenario = types.ScenarioGeneric
	rm.AnalyzerHints.Kind = string(types.ReqMechanism)
	rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
		Scope: types.RuntimeQuestionScopeBoundedFactSet,
		FactFamilies: []types.RuntimeQuestionFactFamily{
			types.RuntimeQuestionFactTargetWaitOccurrences,
			types.RuntimeQuestionFactDirectWaker,
		},
	}
	if got := renderAnswerDocTracePrincipalValueAuthority(ctx); got != "" {
		t.Fatalf("a wait family without a typed wait roster must not expose the unrelated state account:\n%s", got)
	}

	rm.RuntimeQuestionProfile.FactFamilies = append(
		rm.RuntimeQuestionProfile.FactFamilies,
		types.RuntimeQuestionFactTargetSchedulerState,
	)
	if got := renderAnswerDocTracePrincipalValueAuthority(ctx); !strings.Contains(got, "principal_state:") {
		t.Fatalf("an explicitly requested scheduler-state family must restore its recap:\n%s", got)
	}
}

func tracePrincipalValueAuthorityTestContext(subject string, pid int, observations []types.ObservationRecord) *types.AgentContext {
	mut := types.NewMutableState("typed trace authority test")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName:     "trace_query",
		Success:      true,
		Observations: observations,
	}}})
	return &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentRootCause,
				RuntimeTargets: []types.RuntimeTarget{{
					Kind:   types.RuntimeTargetKindThread,
					PID:    pid,
					Thread: subject,
					Source: "user_explicit",
				}},
			},
			AnswerContract: types.AnswerContract{},
		},
	}
}
