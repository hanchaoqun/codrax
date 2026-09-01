package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAnswerDocObservationPromptRecords_MixedRuntimeSourceUsesCompactBudget(t *testing.T) {
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentRootCause,
		Predicates: types.SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		LogTriage: &types.LogBundle{
			Observations: []types.LogObservation{{Kind: types.LogObservationRetryCycle, Summary: "finalizer timeout"}},
		},
		CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes: []types.CurrentSourceExplanationMode{
				types.CurrentSourceExplanationExplainCurrentMechanism,
			},
			SourceQuotes: []string{"结合当前源码"},
			Confidence:   0.9,
		},
	}}}

	got := answerDocObservationPromptRecords(ctx, answerDocProjectionBudgetRecords(14), answerDocObservationLedgerPromptLimit)
	if len(got) != answerDocMixedRuntimeSourceObservationLedgerPromptLimit {
		t.Fatalf("mixed runtime/source observation prompt records=%d, want compact limit %d", len(got), answerDocMixedRuntimeSourceObservationLedgerPromptLimit)
	}
	for _, record := range got {
		if record.Origin == types.AnswerEvidenceOriginRuntimeArtifact &&
			record.Role == types.AnswerAggregateRoleSupportingCoverage &&
			len(record.Notes) > 3 {
			t.Fatalf("mixed runtime/source support notes should be compact, got %d notes for %+v", len(record.Notes), record)
		}
	}
}

func TestAnswerDocFiniteRuntimeScopeProjectsRankingRowsOutOfFinalizerPromptOnly(t *testing.T) {
	records := []types.ObservationRecord{
		{ID: "trace_query:t#root_cause_rank:1", ClaimKey: "root_cause_primary", Subject: "target"},
		{ID: "trace_query:t#root_evidence:1", ClaimKey: "root_cause_evidence", Subject: "target"},
		{ID: "trace_query:t#target_window_states:1", ClaimKey: "target_window_states", Subject: "target"},
		{ID: "trace_query:t#frequency_limit:1", ClaimKey: "frequency_limit", Subject: "cpu=4"},
	}
	ctx := &types.AgentContext{
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
				Scope: types.RuntimeQuestionScopeBoundedEffectVerdict,
				FactFamilies: []types.RuntimeQuestionFactFamily{
					types.RuntimeQuestionFactTargetSchedulerState,
					types.RuntimeQuestionFactFrequencyResidency,
				},
			},
		}},
	}
	projected, _ := answerDocFinalizerObservationRecords(ctx, records)
	if len(projected) != 2 || projected[0].ID != records[2].ID || projected[1].ID != records[3].ID {
		t.Fatalf("finite prompt projection should retain direct facts only: %+v", projected)
	}
	if len(records) != 4 {
		t.Fatalf("prompt projection mutated the lossless source ledger: %+v", records)
	}

	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeCausalDiagnosis
	causal, _ := answerDocFinalizerObservationRecords(ctx, records)
	if len(causal) != len(records) {
		t.Fatalf("causal diagnosis lost ranking rows: %+v", causal)
	}
}

func TestAnswerDocBoundedNamedTargetPromptProjectsUnrelatedRuntimeRows(t *testing.T) {
	count := 50
	record := func(id, subject, predicate string) types.ObservationRecord {
		return types.ObservationRecord{
			ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Subject: subject, Predicate: predicate,
		}
	}
	records := []types.ObservationRecord{
		record("target-state", ".ugc.aweme.lite-17267", "target_window_states"),
		record("target-blocked-census", ".ugc.aweme.lite-17267", "blocked_reason_census"),
		record("other-state", "tui thread-13629", "state_churn"),
		record("other-semantic", "Jit thread pool-17284", "trace_semantic_span"),
		record("frequency", "cpu=4", "cpu_frequency_limit"),
		record("other-cpuset", "logd.writer-9163", "cpu_constraint"),
		record("pressure", "cpu=4", "background_pressure"),
		record("gap", "", "trace_gap"),
		{ID: "repo", Origin: types.AnswerEvidenceOriginCurrentSource, Producer: "emit_evidence"},
	}
	records[1].ResultCount = &count
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeTargets: []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 17267, Thread: ".ugc.aweme.lite-17267", Source: "user_explicit"}},
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
			Scope: types.RuntimeQuestionScopeBoundedEffectVerdict,
			FactFamilies: []types.RuntimeQuestionFactFamily{
				types.RuntimeQuestionFactTargetSchedulerState,
				types.RuntimeQuestionFactCountOrDuration,
				types.RuntimeQuestionFactFrequencyResidency,
			},
		},
	}}}

	got := answerDocScopeProjectedObservationRecords(ctx, records)
	if gotIDs := answerDocObservationRecordIDs(got); strings.Join(gotIDs, ",") != "target-state,frequency,gap,repo" {
		t.Fatalf("bounded prompt retained unrelated or unauthorized rows: %v", gotIDs)
	}
	if len(records) != 9 {
		t.Fatalf("prompt projection mutated the lossless source ledger: %+v", records)
	}

	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.FactFamilies = append(
		ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.FactFamilies,
		types.RuntimeQuestionFactRecordedReason,
		types.RuntimeQuestionFactResourcePressure,
	)
	got = answerDocScopeProjectedObservationRecords(ctx, records)
	gotIDs := strings.Join(answerDocObservationRecordIDs(got), ",")
	for _, want := range []string{"target-blocked-census", "frequency", "pressure"} {
		if !strings.Contains(gotIDs, want) {
			t.Fatalf("requested bounded fact family lost %q: %s", want, gotIDs)
		}
	}
}

func TestAnswerDocBoundedNamedTargetIOLatencyUsesDedicatedTypedFamily(t *testing.T) {
	record := func(id, subject, predicate string) types.ObservationRecord {
		return types.ObservationRecord{
			ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Subject: subject, Predicate: predicate,
		}
	}
	records := []types.ObservationRecord{
		record("target-io", ".ugc.aweme.lite-17267", "io_latency"),
		record("io-coverage", "block_request_pairs", "io_latency_coverage"),
		record("storage", "block", "storage_latency_by_layer"),
		record("inode", "inode=123", "block_io_by_inode"),
		record("target-state", ".ugc.aweme.lite-17267", "target_window_states"),
		record("pressure", "cpu=4", "io_pressure"),
	}
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeTargets: []types.RuntimeTarget{{
			Kind: types.RuntimeTargetKindThread, PID: 17267, Thread: ".ugc.aweme.lite-17267", Source: "user_explicit",
		}},
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
			Scope:        types.RuntimeQuestionScopeBoundedFactSet,
			FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactIOLatency},
		},
	}}}

	got := answerDocScopeProjectedObservationRecords(ctx, records)
	if ids := strings.Join(answerDocObservationRecordIDs(got), ","); ids != "target-io,io-coverage,storage,inode,target-state" {
		t.Fatalf("typed IO latency projection drifted: %s", ids)
	}
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.FactFamilies = []types.RuntimeQuestionFactFamily{
		types.RuntimeQuestionFactTargetSchedulerState,
	}
	got = answerDocScopeProjectedObservationRecords(ctx, records)
	if ids := strings.Join(answerDocObservationRecordIDs(got), ","); ids != "target-state" {
		t.Fatalf("target ownership leaked unrequested IO latency rows: %s", ids)
	}
}

func TestRenderAnswerDocBoundedRuntimeFactAuthorityKeepsExactIOCalibersOutsideGenericBudget(t *testing.T) {
	rm := types.RequestModel{
		RuntimeTargets: []types.RuntimeTarget{{PID: 17267, Thread: ".ugc.aweme.lite-17267", Source: "user_explicit"}},
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
			Scope: types.RuntimeQuestionScopeBoundedFactSet,
			FactFamilies: []types.RuntimeQuestionFactFamily{
				types.RuntimeQuestionFactTargetWaitOccurrences,
				types.RuntimeQuestionFactIOLatency,
				types.RuntimeQuestionFactResourcePressure,
			},
		},
	}
	record := func(id, subject, predicate, value, unit string, notes ...string) types.ObservationRecord {
		return types.ObservationRecord{
			ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Subject: subject, Predicate: predicate,
			Value: value, Unit: unit, RichNotes: notes,
		}
	}
	pair := record("trace#io_latency:1", ".ugc.aweme.lite-17267", "io_latency", "1.347", "ms",
		"io_endpoint_family=block_rq",
		"dev=12,80",
		"io_sector=923339752",
		"io_len=64",
		"io_issue_ts=13762.872568",
		"io_complete_ts=13762.873915",
		"io_issue_thread=.ugc.aweme.lite-17267",
		"request_residence=1.347",
		"request_residence_caliber=block_rq_issue_to_complete",
		"request_residence_clock_scope=single_request_elapsed_wall_clock_not_target_blocking",
		"complete_thread=udk-irq-12-92",
		"completion_woke_issuer=true",
		"issuer_blocked_state=s_sleep",
		"issuer_blocked_start=13762.872578",
		"issuer_blocked_end=13762.873915",
		"issuer_blocked=1.337",
		"causal_wait_caliber=completion_closed_issuer_blocked",
		"issuer_blocked_clock_scope=target_blocking_elapsed_wall_clock",
		"selected_window=13762.791708..13763.024898",
		"capacity_truncated=true",
	)
	pair.SourceRef.ArtifactID = "runtime_artifact:donghu"
	pair.Span = types.ObservationSpan{StartTs: 13762.872568, EndTs: 13762.873915}
	duplicatePair := pair
	duplicatePair.ID = "trace-second-call#io_latency:1"
	waitTotal := 0
	waitRoster := record("wait", ".ugc.aweme.lite-17267", "target_window_wait_occurrences", "0", "occurrences",
		"target_wait_occurrence_prompt=status=complete,emitted=0,total=0",
		"target_wait_occurrence_prompt_sum_ms=0.000",
	)
	waitRoster.Object = "complete"
	waitRoster.ResultCount = &waitTotal
	records := []types.ObservationRecord{
		waitRoster,
		pair,
		duplicatePair,
		record("trace#io_latency_coverage", "block_request_pairs", "io_latency_coverage", "198", "requests",
			"io_latency_emitted=8", "total=198", "io_latency_coverage_status=capacity_truncated",
			"io_latency_overflow_pairs=190", "io_latency_overflow_request_ms=41.329",
			"overflow_sum_caliber=request_ms_non_wall_clock_non_additive"),
		record("storage", "block", "storage_latency_by_layer", "1.347", "ms", "dev=12,80"),
		record("inode", "inode=0x14088d", "block_io_by_inode", "2.694", types.TraceObservationUnitCompositeScore, "inode=0x14088d", "dev=12,80"),
		record("trace#io_pressure:1", "", "scheduler_iowait_with_storage_latency", "4340", types.TraceObservationUnitCompositeScore,
			"type=io_pressure", "io_pressure_signal=scheduler_iowait_with_storage_latency", "io_pressure_score_caliber=cross_unit_activity_index"),
	}
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	got := renderAnswerDocBoundedRuntimeFactAuthority(ctx, types.ObservationLedger{Records: records})
	for _, want := range []string{
		"### Requested Runtime Fact Authority",
		"requested_family=`io_latency`",
		"target_owned_request_rows_rendered=`1`",
		"global `io_latency_coverage` row's emitted/total/overflow values MUST NOT be attributed to the named target",
		"request_residence=`1.347`",
		"request_clock_scope=`single_request_elapsed_wall_clock_not_target_blocking`",
		"issuer_blocked=`1.337`",
		"issuer_blocked_clock_scope=`target_blocking_elapsed_wall_clock`",
		"overflow_request_ms=`41.329`",
		"IO measurement relation bridge",
		"Single-request device elapsed time: 1 target-owned request witness(es), largest visible 1.347ms",
		"IS elapsed wall clock for that one request",
		"MUST NOT be labelled non-wall-clock",
		"Target completion-closed blocking time: 1 proven interval(s), union=1.337ms, including interruptible S-state waits",
		"Scheduler-marked IO-wait roster: 0 occurrence(s), sum=0.000ms",
		"does NOT include or refute separately proven completion-closed waits",
		"hidden-request duration sum is 41.329 request·ms; this aggregate IS non-wall-clock and non-additive",
		"owner_scope=`selected_window_context`; subject=`block`",
		"requested_family=`resource_pressure`",
		"value=`4340composite_score`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("requested finite IO authority missing %q:\n%s", want, got)
		}
	}
	if count := strings.Count(got, "request_residence=`1.347`"); count != 1 {
		t.Fatalf("same physical request from two deterministic calls must render once, got %d:\n%s", count, got)
	}
}

func TestAnswerDocBoundedNamedTargetPromptAcceptsTypedDiagnosticPIDSuffix(t *testing.T) {
	record := func(id, subject, predicate string) types.ObservationRecord {
		return types.ObservationRecord{
			ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Subject: subject, Predicate: predicate,
		}
	}
	records := []types.ObservationRecord{
		record("target-state", ".ugc.aweme.lite-17267", "target_window_states"),
		record("target-running-cpu", ".ugc.aweme.lite-17267", "top_running"),
		record("other-state", "tui thread-13629", "state_churn"),
	}
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeTargets: []types.RuntimeTarget{{
			Kind: types.RuntimeTargetKindThread, Thread: ".ugc.aweme.lite-17267 [17267]", Source: "user_explicit",
		}},
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
			Scope: types.RuntimeQuestionScopeBoundedEffectVerdict,
			FactFamilies: []types.RuntimeQuestionFactFamily{
				types.RuntimeQuestionFactTargetSchedulerState,
				types.RuntimeQuestionFactFrequencyResidency,
			},
		},
	}}}

	got := answerDocScopeProjectedObservationRecords(ctx, records)
	if gotIDs := strings.Join(answerDocObservationRecordIDs(got), ","); gotIDs != "target-state,target-running-cpu" {
		t.Fatalf("typed diagnostic pid display filtered genuine target rows: %s", gotIDs)
	}
}

func TestAnswerDocBoundedPromptProjectionFailsOpenWithoutUserTargetAndSkipsCausalScope(t *testing.T) {
	records := []types.ObservationRecord{{
		ID: "other-state", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", Subject: "other-7", Predicate: "state_churn",
	}}
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
			Scope:        types.RuntimeQuestionScopeBoundedFactSet,
			FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState},
		},
	}}}
	if got := answerDocScopeProjectedObservationRecords(ctx, records); len(got) != 1 {
		t.Fatalf("bounded prompt without a user target must fail open: %+v", got)
	}
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{PID: 7, Thread: "target-7", Source: "user_explicit"}}
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeCausalDiagnosis
	if got := answerDocScopeProjectedObservationRecords(ctx, records); len(got) != 1 {
		t.Fatalf("causal diagnosis must keep full runtime context: %+v", got)
	}
}

func answerDocObservationRecordIDs(records []types.ObservationRecord) []string {
	out := make([]string, 0, len(records))
	for _, record := range records {
		out = append(out, record.ID)
	}
	return out
}

func TestAnswerDocFiniteRuntimeScopeUsesTypedTraceRowsInsteadOfModelAggregateRestatements(t *testing.T) {
	modelFacts := []types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "model frequency verdict",
		Value:       "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"CPU 4 min was presented as the maximum"},
		MemberNotes: []string{"model-only interpretation"},
	}}
	mut := types.NewMutableState("bounded typed authority")
	mut.SetInvestigationAggregateFacts(modelFacts)
	mut.SetInvestigationComplete("model-only conclusion retained for audit")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{{
			ID:        "trace_query:result#target_window_states",
			Origin:    types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:  "trace_query",
			Role:      types.AnswerAggregateRolePrincipalAnswer,
			Predicate: "target_window_states",
			Subject:   "target-7",
			Value:     "12.000",
			Unit:      "ms",
		}},
	}}})
	ctx := &types.AgentContext{
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		Mutable:   mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace,
			RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
				Scope:        types.RuntimeQuestionScopeBoundedEffectVerdict,
				FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState},
			},
		}},
	}

	if got := answerDocStableAggregateFacts(ctx); len(got) != 0 {
		t.Fatalf("finite finalizer prompt retained model trace restatements: %+v", got)
	}
	if got := renderAnswerDocAggregateFacts(ctx); strings.Contains(got, "model frequency verdict") || strings.Contains(got, "presented as the maximum") {
		t.Fatalf("finite aggregate prompt leaked model-only runtime interpretation:\n%s", got)
	}
	if got := renderAnswerDocObservationLedger(ctx); !strings.Contains(got, "target_window_states") || strings.Contains(got, "model frequency verdict") {
		t.Fatalf("finite observation prompt must retain typed rows and omit model restatements:\n%s", got)
	}
	if raw := mut.StableInvestigationAggregateFacts(); len(raw) != 1 || raw[0].Label != modelFacts[0].Label {
		t.Fatalf("prompt projection mutated the lossless exploration handoff: %+v", raw)
	}
}

func TestExplorerRuntimeQuestionScopeWorkflowKeepsFiniteAndCausalLanesSeparate(t *testing.T) {
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
			Scope: types.RuntimeQuestionScopeBoundedEffectVerdict,
			FactFamilies: []types.RuntimeQuestionFactFamily{
				types.RuntimeQuestionFactTargetSchedulerState,
				types.RuntimeQuestionFactFrequencyResidency,
			},
		},
	}}}
	finite := renderExplorerRuntimeQuestionScopeWorkflow(ctx)
	for _, want := range []string{
		"bounded_effect_verdict",
		"target_scheduler_state",
		"frequency_residency",
		"condition presence and condition-to-target binding as separate typed axes",
		"are not the default for this scope",
	} {
		if !strings.Contains(finite, want) {
			t.Fatalf("finite explorer scope guidance missing %q:\n%s", want, finite)
		}
	}
	if explorerRuntimeQuestionAllowsCausalRoster(ctx) {
		t.Fatal("finite scope unexpectedly authorizes causal roster workflow")
	}

	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeCausalDiagnosis
	if got := renderExplorerRuntimeQuestionScopeWorkflow(ctx); got != "" {
		t.Fatalf("causal scope should keep the established workflow without duplicate preface:\n%s", got)
	}
	if !explorerRuntimeQuestionAllowsCausalRoster(ctx) {
		t.Fatal("causal diagnosis lost broad drill workflow")
	}
}

func TestExplorerRuntimeQuestionScopeWorkflowSeparatesSchedulerAndSemanticWork(t *testing.T) {
	profile := &types.RuntimeQuestionProfile{
		Scope:                        types.RuntimeQuestionScopeCausalDiagnosis,
		RuntimeWorkRelationRequested: true,
	}
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeQuestionProfile: profile,
	}}}

	got := renderExplorerRuntimeQuestionScopeWorkflow(ctx)
	for _, want := range []string{
		"keep the scheduler and semantic-work inventories distinct",
		"explicitly typed as semantic span/work",
		"A scheduler-state row",
		"A is the recorded waker/source and B is the wakee/target",
		"`runnable` means eligible but waiting for CPU",
		"chain-ranked runnable row alone does not prove same-CPU competition",
		"compatible typed running/runnable overlap or competitor carrier",
		"does not pre-decide the model-owned relation or root-cause conclusion",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("runtime work-relation explorer guidance missing %q:\n%s", want, got)
		}
	}

	profile.RuntimeWorkRelationRequested = false
	if got := renderExplorerRuntimeQuestionScopeWorkflow(ctx); got != "" {
		t.Fatalf("causal trace without a work-relation subquestion gained extra prompt load:\n%s", got)
	}
}

func TestAnswerDocObservationPromptRecords_RouteBackedTypedRuntimeUsesAuthorityCompactBudget(t *testing.T) {
	ctx := &types.AgentContext{
		TurnRouteHint: types.TurnRouteHint{Source: "mixed", NeedsRepoAccess: true},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentRootCause,
			CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
				IsCurrentSourceExplanationRequested: true,
				Modes: []types.CurrentSourceExplanationMode{
					types.CurrentSourceExplanationExplainCurrentMechanism,
				},
				SourceQuotes: []string{"current parser mechanism"},
				Confidence:   0.9,
			},
		}},
	}

	got := answerDocObservationPromptRecords(ctx, answerDocProjectionBudgetRecords(14), answerDocObservationLedgerPromptLimit)
	if len(got) != answerDocMixedRuntimeSourceObservationLedgerPromptLimit {
		t.Fatalf("route-backed typed runtime/source observation prompt records=%d, want compact limit %d", len(got), answerDocMixedRuntimeSourceObservationLedgerPromptLimit)
	}
}

func TestAnswerDocObservationPromptRecords_DefaultBudgetUnchanged(t *testing.T) {
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain,
	}}}

	got := answerDocObservationPromptRecords(ctx, answerDocProjectionBudgetRecords(14), answerDocObservationLedgerPromptLimit)
	if len(got) != 14 {
		t.Fatalf("ordinary observation prompt records=%d, want default full set", len(got))
	}
	runtimeSupportSeen := false
	for _, record := range got {
		if record.Origin == types.AnswerEvidenceOriginRuntimeArtifact &&
			record.Role == types.AnswerAggregateRoleSupportingCoverage {
			runtimeSupportSeen = true
			if len(record.Notes) != 5 {
				t.Fatalf("ordinary runtime support notes should keep default origin-specific budget, got %+v", record.Notes)
			}
		}
	}
	if !runtimeSupportSeen {
		t.Fatalf("test fixture did not include runtime support rows: %+v", got)
	}
}

func TestAnswerDocObservationPromptRecords_SameCaptureQuerySuppressesOnlyPreTriageCandidates(t *testing.T) {
	perf := &types.PerfBundle{
		Observations: []types.PerfObservation{
			// The tool schema cannot set Authority; zero is the production
			// pre-triage shape and IsNavigationOnly must own that fail-closed
			// classification consistently across ledger and Finalizer views.
			{Subject: "navigation"},
			{Authority: types.PerfObservationAuthorityDeterministicValidator, Subject: "validated"},
		},
		Stalls: []types.PerfStall{
			{Authority: types.PerfObservationAuthorityPreTriageModelExtraction, Symbol: "candidate stall"},
			{Authority: types.PerfObservationAuthorityDeterministicValidator, Symbol: "validated stall"},
		},
	}
	ctx := &types.AgentContext{
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			PerfTrace: perf,
		}},
	}
	const capture = "/captures/customer.systrace"
	records := []types.ObservationRecord{
		{
			ID: "perf:frame:0", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "perf_trace",
			SourceRef: types.ObservationSourceRef{CaptureIdentityPath: capture}, Subject: "measured frame",
		},
		{
			ID: "perf:observation:0", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "perf_trace",
			SourceRef: types.ObservationSourceRef{CaptureIdentityPath: capture}, Subject: "navigation",
		},
		{
			ID: "perf:observation:1", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "perf_trace",
			SourceRef: types.ObservationSourceRef{CaptureIdentityPath: capture}, Subject: "validated",
		},
		{
			ID: "perf:stall:0", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "perf_trace",
			SourceRef: types.ObservationSourceRef{CaptureIdentityPath: capture}, Subject: "candidate stall",
		},
		{
			ID: "perf:stall:1", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "perf_trace",
			SourceRef: types.ObservationSourceRef{CaptureIdentityPath: capture}, Subject: "validated stall",
		},
		{
			ID: "trace_query:q#evidence_fact:1", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			SourceRef: types.ObservationSourceRef{CaptureIdentityPath: capture}, Subject: "exact query row",
		},
	}
	got, omitted := answerDocFinalizerObservationRecords(ctx, records)
	if omitted != 2 || len(got) != 4 {
		t.Fatalf("same-capture filter omitted=%d records=%+v", omitted, got)
	}
	joined := ""
	for _, record := range got {
		joined += record.ID + "\n"
	}
	for _, want := range []string{"perf:frame:0", "perf:observation:1", "perf:stall:1", "trace_query:q#evidence_fact:1"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("required measured/deterministic row %q missing: %s", want, joined)
		}
	}
	if strings.Contains(joined, "perf:observation:0") || strings.Contains(joined, "perf:stall:0") {
		t.Fatalf("pre-triage narrative leaked after same-capture query: %s", joined)
	}
}

func TestAnswerDocObservationPromptRecords_SingleInlineAttachmentJoinsReservedMaterialization(t *testing.T) {
	ctx := &types.AgentContext{
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{
			Kind: "trace", Source: "(inline)", Carrier: "attachment",
		}}},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{PerfTrace: &types.PerfBundle{
			Observations: []types.PerfObservation{{Subject: "model dependency candidate"}},
		}}},
	}
	records := []types.ObservationRecord{
		{
			ID: "perf:observation:0", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "perf_trace",
			SourceRef: types.ObservationSourceRef{ArtifactID: "attached_trace"}, Subject: "model dependency candidate",
		},
		{
			ID: "trace_query:q#root_cause:1", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			SourceRef: types.ObservationSourceRef{
				Path: "/repo/.codrax/blob/run/attached_trace.txt", ArtifactID: "attached_trace",
			}, Subject: "typed on-chain cause",
		},
	}

	got, omitted := answerDocFinalizerObservationRecords(ctx, records)
	if omitted != 1 || len(got) != 1 || got[0].ID != "trace_query:q#root_cause:1" {
		t.Fatalf("single inline attachment should join its reserved query materialization, omitted=%d got=%+v", omitted, got)
	}
}

func TestAnswerDocObservationPromptRecords_MultipleAttachedTraceIDsFailClosed(t *testing.T) {
	ctx := &types.AgentContext{
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{
			{Kind: "trace", Source: "/captures/a.systrace", Carrier: "attachment"},
			{Kind: "trace", Source: "/captures/b.systrace", Carrier: "attachment"},
		}},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{PerfTrace: &types.PerfBundle{
			Observations: []types.PerfObservation{{Subject: "ambiguous navigation"}},
		}}},
	}
	records := []types.ObservationRecord{
		{
			ID: "perf:observation:0", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "perf_trace",
			SourceRef: types.ObservationSourceRef{ArtifactID: "attached_trace"}, Subject: "ambiguous navigation",
		},
		{
			ID: "trace_query:q#root_cause:1", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			SourceRef: types.ObservationSourceRef{Path: "/captures/a.systrace", ArtifactID: "attached_trace"},
		},
	}

	got, omitted := answerDocFinalizerObservationRecords(ctx, records)
	if omitted != 0 || len(got) != len(records) {
		t.Fatalf("generic attached_trace ID must not collapse multiple attachments, omitted=%d got=%+v", omitted, got)
	}
}

func TestAnswerDocObservationPromptRecords_DifferentCaptureFailsOpen(t *testing.T) {
	ctx := &types.AgentContext{
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{PerfTrace: &types.PerfBundle{
			Observations: []types.PerfObservation{{Authority: types.PerfObservationAuthorityPreTriageModelExtraction}},
		}}},
	}
	records := []types.ObservationRecord{
		{ID: "perf:observation:0", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "perf_trace", SourceRef: types.ObservationSourceRef{CaptureIdentityPath: "/captures/a.systrace"}},
		{ID: "trace_query:q#evidence_fact:1", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", SourceRef: types.ObservationSourceRef{CaptureIdentityPath: "/captures/b.systrace"}},
	}
	got, omitted := answerDocFinalizerObservationRecords(ctx, records)
	if omitted != 0 || len(got) != len(records) {
		t.Fatalf("different-capture observations must fail open, omitted=%d got=%+v", omitted, got)
	}
}

func TestRenderAnswerDocRuntimeSourceAuthority_SoftRequirementHandoff(t *testing.T) {
	ctx := &types.AgentContext{
		TurnRouteHint: types.TurnRouteHint{Source: "mixed", NeedsRepoAccess: true},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentRootCause,
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
			},
		}},
	}
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{answerDocRuntimeAuthorityRuntimeRecord()}}

	got := renderAnswerDocRuntimeSourceAuthority(ctx, ledger)
	for _, want := range []string{
		"##",
		"requirement_precision=`soft`",
		"hard_block=false",
		"caveat_only=true",
		"current_source_soft",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("soft runtime/source authority handoff missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "requirement_precision=`precise`") {
		t.Fatalf("soft authority handoff must not claim precise requirement:\n%s", got)
	}
}

func TestRenderAnswerDocRuntimeSourceAuthority_PreciseRequirementHandoff(t *testing.T) {
	ctx := &types.AgentContext{
		TurnRouteHint: types.TurnRouteHint{Source: "mixed", NeedsRepoAccess: true},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentRootCause,
			CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
				IsCurrentSourceExplanationRequested: true,
				Modes: []types.CurrentSourceExplanationMode{
					types.CurrentSourceExplanationExplainCurrentMechanism,
				},
				SourceQuotes: []string{"internal/tracequery/parse.go:42"},
				Confidence:   0.9,
			},
		}},
	}
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{answerDocRuntimeAuthorityRuntimeRecord()}}

	got := renderAnswerDocRuntimeSourceAuthority(ctx, ledger)
	for _, want := range []string{
		"requirement_precision=`precise`",
		"hard_block=true",
		"caveat_only=false",
		"current_source_precise",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("precise runtime/source authority handoff missing %q:\n%s", want, got)
		}
	}
}

func TestRenderAnswerDocRuntimeSourceAuthority_RuntimeOnlyDoesNotClaimCombinedProof(t *testing.T) {
	ctx := &types.AgentContext{
		TurnRouteHint: types.TurnRouteHint{
			Route:                     "repo",
			Source:                    "artifact",
			NeedsRepoAccess:           true,
			CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional,
		},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentRootCause}},
	}
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{answerDocRuntimeAuthorityRuntimeRecord()}}

	got := renderAnswerDocRuntimeSourceAuthority(ctx, ledger)
	for _, want := range []string{
		"current_source_satisfied=false",
		"current_source_records=0",
		"runtime evidence is sufficient for the runtime artifact claim",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("runtime-only authority handoff missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "runtime and current-source proof are both present") {
		t.Fatalf("runtime-only authority must not claim absent current-source proof:\n%s", got)
	}
}

func TestRenderAnswerDocRuntimeSourceAuthority_InactiveOmitted(t *testing.T) {
	if got := renderAnswerDocRuntimeSourceAuthority(&types.AgentContext{}, types.ObservationLedger{}); got != "" {
		t.Fatalf("inactive runtime/source authority should not render, got:\n%s", got)
	}
}

func answerDocProjectionBudgetRecords(n int) []types.ObservationRecord {
	records := make([]types.ObservationRecord, 0, n+2)
	records = append(records,
		types.ObservationRecord{
			ID:              "runtime:principal",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "log_triage",
			Role:            types.AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_log", ArtifactKind: "log"},
			Span:            types.ObservationSpan{LineStart: 2, LineEnd: 3},
			Summary:         "attached log observed a finalizer timeout",
			RichNotes:       answerDocProjectionNotes("runtime-principal", 5),
		},
		types.ObservationRecord{
			ID:              "current:owner",
			Origin:          types.AnswerEvidenceOriginCurrentSource,
			Producer:        "emit_evidence",
			Role:            types.AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceCurrentSource, Path: "internal/orchestrator/orchestrator.go"},
			Span:            types.ObservationSpan{LineStart: 6714},
			AnchorKind:      types.AnchorDefinition,
			EvidenceScope:   types.ScopeLine,
			GroundingStatus: types.GroundingGrounded,
			Summary:         "current source owns the retry branch",
			RichNotes:       answerDocProjectionNotes("current-owner", 5),
		},
	)
	for i := 0; i < n-2; i++ {
		records = append(records, types.ObservationRecord{
			ID:              fmt.Sprintf("runtime:support:%02d", i),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "log_triage",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingSoft,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_log", ArtifactKind: "log"},
			Span:            types.ObservationSpan{LineStart: i + 4},
			Summary:         fmt.Sprintf("supporting runtime observation %02d", i),
			RichNotes:       answerDocProjectionNotes(fmt.Sprintf("runtime-support-%02d", i), 5),
		})
	}
	return records
}

func answerDocRuntimeAuthorityRuntimeRecord() types.ObservationRecord {
	return types.ObservationRecord{
		ID:       "runtime:trace",
		Origin:   types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query",
		Role:     types.AnswerAggregateRolePrincipalAnswer,
		SourceRef: types.ObservationSourceRef{
			Kind:         types.ObservationSourceRuntimeArtifact,
			ArtifactID:   "attached_trace",
			ArtifactKind: "trace",
			PayloadRef:   "blob://trace",
		},
		Span:    types.ObservationSpan{StartTsMs: 62380029.1},
		Summary: "trace_query observed the selected runtime span",
	}
}

func answerDocProjectionNotes(prefix string, n int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, strings.Repeat(prefix, 1)+fmt.Sprintf("-note-%02d", i))
	}
	return out
}
