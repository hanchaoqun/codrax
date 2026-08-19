package agent

// NW-05 软臂 (P3, 2026-07-24) — the composition prompt gains ONE causal-ceiling
// directive when a typed evidence authority published causal_conclusion=
// unproven. Precise trigger (typed authority field), soft effect (prompt hint
// only); absence of the authority keeps the prompt byte-identical.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func answerDocCausalCeilingTestContext(withUnproven bool) *types.AgentContext {
	mut := types.NewMutableState("分析鸿蒙 trace 丢帧")
	mut.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace"},
		Observations: []types.PerfObservation{{
			Kind:    "priority_semantics",
			Subject: "HarmonyOS priority semantics",
			Summary: "Harmony priority semantics: prio=120/ohos_rt observed in attached trace",
			Tags:    []string{"harmony_priority", "prio=120/ohos_rt"},
		}},
	})
	if withUnproven {
		mut.AppendDispatchToolResult(types.ToolResult{
			ToolName: "trace_query",
			Success:  true,
			TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
				View:                "frame_root_cause_bundle",
				FrameEvidenceStatus: "absent",
				CausalConclusion:    "unproven",
			},
		})
	}
	return &types.AgentContext{
		Objective:             "分析鸿蒙 trace 丢帧",
		AttachedHitraceSource: "harmony_hitrace",
		Mutable:               mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioRootCause,
				Intent:   types.IntentRootCause,
			},
			AnswerContract: types.AnswerContract{},
		},
	}
}

func TestAnswerDocumentEvaluatorRendersCausalCeilingHintOnUnprovenAuthority(t *testing.T) {
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(answerDocCausalCeilingTestContext(true), nil)
	for _, want := range []string{
		"Runtime causal ceiling hint",
		"`causal_conclusion=unproven`",
		"bounded window facts and candidates",
		"`导致丢帧`/`caused the dropped frame`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluatorOmitsCausalCeilingHintWithoutUnprovenAuthority(t *testing.T) {
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(answerDocCausalCeilingTestContext(false), nil)
	if strings.Contains(prompt, "Runtime causal ceiling hint") {
		t.Fatalf("hint must stay absent without the typed unproven authority:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluatorRendersTemporalFrameEdgeAuthorityHint(t *testing.T) {
	ctx := answerDocCausalCeilingTestContext(false)
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View:                       "frame_flow",
			FrameFlowEdgeCount:         3,
			FrameFlowRelationAuthority: "temporal_sequence",
			FrameFlowCausalConclusion:  "unproven",
			CausalConclusion:           "unproven",
		},
	})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"Runtime frame-edge authority hint",
		"`frame_flow_causality=unproven`",
		"`relation=temporal_sequence`",
		"`edges=3`",
		"Note over <participant>",
		"relation_kind=temporal",
		"temporal adjacency (unproven)",
		"Every visible arrow occurrence needs one matching edge_anchors[] row",
		"including a self-arrow",
		"unless a separate typed causal row proves that exact relation",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("frame-edge prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestTypedTraceAuthoritySelectsCompactExactGuidance(t *testing.T) {
	ctx := answerDocCausalCeilingTestContext(true)
	got := renderAnswerDocRuntimeTraceAnswerGuidance(ctx)
	for _, want := range []string{
		"Typed trace context precedence",
		"on-chain/adjacent/background population",
		"Scheduler transition interval hint",
		"Never call t_run the wakeup timestamp",
		"switch-in beyond that window",
		"Scheduler count caliber hint",
		"chain-associated instances or folded members",
		"not the target's total wakeup count",
		"Wakeup CPU-topology hint",
		"`cpu_relation=same_cpu|cross_cpu`",
		"do not describe it as same-CPU occupancy, preemption, or direct CPU competition",
		"every typed on-chain candidate eligible",
		"lower-priority on-chain dependency with typed runnable effective impact and/or a typed running compute-supply deficit",
		"runnable/scheduler-supply delay",
		"deterministic semantic work",
		"shared IRQ/waker label",
		"same IO/pressure family",
		"Never call an adjacent/background row a direct or indirect contributor",
		"exact typed relation/fold carrier",
		"concurrent support or an additional investigation direction",
		"Thread and span semantic authority",
		"span/marker label proves its label, measured interval",
		"Numeric substrings/suffixes in a span or marker name are opaque identifiers, never durations",
		"without a pairable E/F endpoint proves no duration or target-span causal window",
		"does not by itself prove the internal work",
		"not the owning thread",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("compact typed trace guidance missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"Runtime Binder direction hint",
		"Runtime IO/supply hint",
		"Runtime perf support hint",
		"Runtime direct-blocking hint",
		"Runtime root-cause layering hint",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("typed trace guidance retained unrelated generic recipe %q:\n%s", forbidden, got)
		}
	}
	if !strings.Contains(got, "Runtime causal ceiling hint") {
		t.Fatalf("context dedupe removed exact causal ceiling:\n%s", got)
	}
}

func TestTypedFiniteTraceScopeDoesNotPromoteExploratoryRootCausePopulation(t *testing.T) {
	for _, scope := range []types.RuntimeQuestionScope{
		types.RuntimeQuestionScopeBoundedFactSet,
		types.RuntimeQuestionScopeBoundedEffectVerdict,
	} {
		t.Run(string(scope), func(t *testing.T) {
			ctx := answerDocCausalCeilingTestContext(true)
			ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
				Scope: scope,
				FactFamilies: []types.RuntimeQuestionFactFamily{
					types.RuntimeQuestionFactTargetSchedulerState,
					types.RuntimeQuestionFactFrequencyResidency,
				},
			}
			got := renderAnswerDocRuntimeTraceAnswerGuidance(ctx)
			for _, want := range []string{
				"Runtime finite-scope presentation hint",
				"not a root-cause roster",
				"not thereby part of the requested principal root-cause population",
				"without root-cause/rank/seat language",
				"does not suppress any requested scheduler-state",
				"does not decide yes/no/mixed/unproven",
				"Runtime user-facing language hint",
				"evidence metadata, not primary customer-facing prose",
				"answer's language",
				"without copying the English enum token",
				"display guidance only",
				"Runtime finite target-state caliber hint",
				"selected `target_window_states` account is the principal authority",
				"Copy its published running/runnable/sleep/D-state/IO-wait/total values",
				"a blocked-reason caller/census is a separately typed record inventory",
				"unless an explicit typed interval join is published",
				"target-running-slice-to-CPU/frequency overlap",
				"bounded conclusion remains model-owned",
			} {
				if !strings.Contains(got, want) {
					t.Fatalf("finite typed trace guidance missing %q:\n%s", want, got)
				}
			}
			if strings.Contains(got, "Keep every typed on-chain candidate eligible for the principal root-cause population") {
				t.Fatalf("finite typed scope retained full causal-population instruction:\n%s", got)
			}
		})
	}
}

func TestTypedFiniteTracePublishesUnjoinedBlockedReasonAttributionBoundaryBeforeAuthoring(t *testing.T) {
	ctx := answerDocCausalCeilingTestContext(false)
	start, end := 10.0, 10.1
	ctx.AnalysisIR.RequestModel.Intent = types.IntentTrace
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
		Scope:        types.RuntimeQuestionScopeBoundedFactSet,
		FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState},
	}
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{
		Kind: types.RuntimeTargetKindThread, PID: 100, Thread: "main-100", Source: "user_explicit",
	}}
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &start, TimeEnd: &end, SourceQuote: "10.0..10.1",
	}
	ref := types.ObservationSourceRef{
		Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "trace-a", Path: "/tmp/a.ftrace",
	}
	count := 12
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query", Success: true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{View: "window_stats"},
		Observations: []types.ObservationRecord{
			{
				ID: "state-requested", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
				Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
				Predicate: "target_window_states", Subject: "main-100", Object: "state_partition",
				Value: "100.000", Unit: "ms", RichNotes: []string{
					"selected_window=10.000000..10.100000", "running=20.000", "runnable=10.000",
					"sleep=70.000", "d_state=0.000", "io_wait=0.000", "total=100.000",
				},
			},
			{
				ID: "blocked-census", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
				Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
				Predicate: "blocked_reason_census", Subject: "main-100", Object: "blocked_reason",
				Value: "12", ResultCount: &count, RichNotes: []string{
					"selected_window=10.000000..10.100000", "blocked_reason_census=fscache_page_wait_o×12(Σ7.000ms)",
				},
			},
		},
	})

	got := renderAnswerDocRuntimeTraceAnswerGuidance(ctx)
	for _, want := range []string{
		"Runtime finite blocked-reason attribution authority",
		"subject=`main-100`",
		"blocked_reason_records=12",
		"relation=`unjoined_distinct_observation_domains`",
		"record_to_state_occurrence_mapping=`not_provided`",
		"state_source_attribution=`unproven`",
		"does not establish the source, main source, explanation, or mechanism",
		"diagnosis and wording remain model-owned",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("finite blocked-reason attribution boundary missing %q:\n%s", want, got)
		}
	}
}

func TestTypedFiniteTraceBlockedReasonBoundaryFailsClosedAcrossArtifacts(t *testing.T) {
	ctx := answerDocCausalCeilingTestContext(false)
	start, end := 10.0, 10.1
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
		Scope:        types.RuntimeQuestionScopeBoundedFactSet,
		FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState},
	}
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{
		Kind: types.RuntimeTargetKindThread, PID: 100, Thread: "main-100", Source: "user_explicit",
	}}
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &start, TimeEnd: &end, SourceQuote: "10.0..10.1",
	}
	count := 1
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query", Success: true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{View: "window_stats"},
		Observations: []types.ObservationRecord{
			{
				ID: "state-requested", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
				Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
				SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "trace-a"},
				Predicate: "target_window_states", Subject: "main-100", Value: "100.000", RichNotes: []string{
					"selected_window=10.000000..10.100000", "running=20.000", "sleep=80.000", "total=100.000",
				},
			},
			{
				ID: "blocked-other", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
				Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
				SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "trace-b"},
				Predicate: "blocked_reason_census", Subject: "main-100", Value: "1", ResultCount: &count,
				RichNotes: []string{"selected_window=10.000000..10.100000", "blocked_reason_census=caller×1(Σ1.000ms)"},
			},
		},
	})
	got := renderAnswerDocRuntimeTraceAnswerGuidance(ctx)
	if strings.Contains(got, "Runtime finite blocked-reason attribution authority") {
		t.Fatalf("cross-artifact census must not bind to the target state account:\n%s", got)
	}
}

func TestTypedRelationAndOverviewTraceScopesDoNotInheritRootCausePopulation(t *testing.T) {
	tests := []struct {
		scope types.RuntimeQuestionScope
		want  string
	}{
		{scope: types.RuntimeQuestionScopeRelationAnalysis, want: "Runtime relation-scope presentation hint"},
		{scope: types.RuntimeQuestionScopeSystemOverview, want: "Runtime overview presentation hint"},
	}
	for _, tt := range tests {
		t.Run(string(tt.scope), func(t *testing.T) {
			ctx := answerDocCausalCeilingTestContext(true)
			ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: tt.scope}
			got := renderAnswerDocRuntimeTraceAnswerGuidance(ctx)
			if !strings.Contains(got, tt.want) {
				t.Fatalf("typed trace guidance missing %q:\n%s", tt.want, got)
			}
			if strings.Contains(got, "Keep every typed on-chain candidate eligible for the principal root-cause population") {
				t.Fatalf("%s scope inherited causal-diagnosis population:\n%s", tt.scope, got)
			}
			if strings.Contains(got, "Runtime finite-scope presentation hint") {
				t.Fatalf("%s scope was collapsed into finite fact/effect guidance:\n%s", tt.scope, got)
			}
		})
	}
}

func TestTypedCausalTraceScopeRetainsCompleteOnChainPopulation(t *testing.T) {
	ctx := answerDocCausalCeilingTestContext(true)
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
		Scope: types.RuntimeQuestionScopeCausalDiagnosis,
	}
	got := renderAnswerDocRuntimeTraceAnswerGuidance(ctx)
	if !strings.Contains(got, "Keep every typed on-chain candidate eligible for the principal root-cause population") {
		t.Fatalf("causal diagnosis lost complete typed on-chain population:\n%s", got)
	}
	if strings.Contains(got, "Runtime finite-scope presentation hint") {
		t.Fatalf("causal diagnosis was narrowed to finite-scope guidance:\n%s", got)
	}
	if strings.Contains(got, "Runtime finite target-state caliber hint") {
		t.Fatalf("causal diagnosis inherited the finite target-state hint:\n%s", got)
	}
}

func TestTypedFiniteTraceScopeOmitsTargetStateCaliberForOtherFactFamilies(t *testing.T) {
	ctx := answerDocCausalCeilingTestContext(true)
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
		Scope:        types.RuntimeQuestionScopeBoundedFactSet,
		FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactFrequencyResidency},
	}
	got := renderAnswerDocRuntimeTraceAnswerGuidance(ctx)
	if strings.Contains(got, "Runtime finite target-state caliber hint") {
		t.Fatalf("non-state finite question inherited target-state caliber guidance:\n%s", got)
	}
}

func TestTypedFrameMeasurementKeepsExtentUnionAndSpanSumSeparate(t *testing.T) {
	ctx := answerDocCausalCeilingTestContext(false)
	rows := make([]types.ObservationRecord, 0, 4)
	for i, interval := range [][2]float64{{7.000, 7.004}, {7.005, 7.016}, {7.017, 7.030}, {7.031, 7.040}} {
		rows = append(rows, types.ObservationRecord{
			ID: fmt.Sprintf("frame-%d", i+1), Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", ClaimKey: fmt.Sprintf("evidence_fact:frame_timeline_%d", i+1),
			Subject: fmt.Sprintf("tid=%d", i+1), Span: types.ObservationSpan{
				LineStart: i*2 + 1, LineEnd: i*2 + 2, StartTs: interval[0], EndTs: interval[1],
			},
		})
	}
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query", Success: true, Observations: rows,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{View: "frame_timeline", FrameItemCount: 4},
	})
	got := renderAnswerDocRuntimeTraceAnswerGuidance(ctx)
	for _, want := range []string{
		"Runtime frame measurement caliber",
		"items=4 first_start=7.000000 last_end=7.040000",
		"end_to_end_extent=40.000ms",
		"interval_union_coverage=37.000ms",
		"per_span_duration_sum=37.000ms",
		"uncovered_gap=3.000ms",
		"only the interval complement",
		"not scheduler latency, blocking time, efficiency, or proof that no blocking occurred",
		"without separate typed scheduler or causal evidence",
		"Never substitute one ruler for another",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed frame measurement guidance missing %q:\n%s", want, got)
		}
	}
}

func TestRequestedDimensionTokensSupportHanRuns(t *testing.T) {
	// NG-4 (§13.4): 中文维度标签此前 token 化为空,只有 ASCII 名维度能上
	// 指标摘录面。Han 连续段现自成 token,两侧仍精确等值匹配。
	tokens := requestedDimensionIdentifierTokens("丢帧阶段 CPU调度分析 vsync")
	want := map[string]bool{"丢帧阶段": false, "cpu": false, "调度分析": false, "vsync": false}
	for _, token := range tokens {
		if _, ok := want[token]; ok {
			want[token] = true
		}
	}
	for token, seen := range want {
		if !seen {
			t.Fatalf("token %q missing from %v", token, tokens)
		}
	}
}
