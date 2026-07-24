package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryEvidenceAuthorityWithdrawsFrameCausality(t *testing.T) {
	result := tracequery.Result{
		View:              "frame_root_cause_bundle",
		PrioritySemantics: "HarmonyOS/hitrace user-space priority: larger numeric value means higher priority; 1-40=CFS, 41-159=RT.",
		FrameRootCauseBundle: &tracequery.FrameRootCauseBundle{
			FrameTimeline: &tracequery.FrameTimelineResult{},
			RootCauseRank: &tracequery.RootCauseRankResult{
				Items: []tracequery.RootCauseRankItem{{Rank: 0, Tier: tracequery.RootCauseTierContextOnly}},
			},
			Caveats: []string{"thread_identity_fail_closed=true; thread_incarnation_conflict"},
		},
	}
	authority := traceQueryEvidenceAuthority(result)
	if authority == nil ||
		authority.FrameEvidenceStatus != "unavailable" ||
		authority.FrameItemCount != 0 ||
		authority.TypedCausalRowCount != 0 ||
		authority.CausalConclusion != "unproven" {
		t.Fatalf("authority boundary drifted: %+v", authority)
	}
}

func TestTraceCausalCoverageBlockPublishesAuthorityCeiling(t *testing.T) {
	input := types.ObservationLedgerInput{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View:                "frame_root_cause_bundle",
			FrameEvidenceStatus: "unavailable",
			CausalConclusion:    "unproven",
		},
	}}}
	block := runtimeTraceCausalProjectionCoverageBlock(input, "zh")
	if block == nil {
		t.Fatal("typed authority boundary must create a deterministic coverage block")
	}
	for _, want := range []string{
		"frame_causality=unproven",
		"frame_evidence_status=unavailable",
		"不能证明具体丢帧因果",
	} {
		if !strings.Contains(block.Text, want) {
			t.Fatalf("coverage block missing %q:\n%s", want, block.Text)
		}
	}
}

func TestTraceQuerySummaryDistinguishesUnavailableAndMeasuredZeroCPU(t *testing.T) {
	result := tracequery.Result{
		View: "window_stats",
		WindowStats: &tracequery.WindowStats{
			SchedulerHeadCoverage: &tracequery.SchedulerHeadCoverage{
				Status: "unknown", Reason: "thread_incarnation_conflict",
				SubjectCensusStatus: "not_evaluated",
			},
			CPU: []tracequery.CPUStats{
				{CPU: 0, BusyIdleStatus: tracequery.CPUBusyIdleStatusUnavailable, BusyIdleReason: "no_sched_switch_observation", Frequency: 1000},
				{CPU: 1, BusyMs: 0, IdleMs: 10, BusyIdleStatus: tracequery.CPUBusyIdleStatusMeasured},
			},
			CoreTopology: []tracequery.CoreClassStats{
				{
					Class: "big", CPUs: []int{0},
					BusyIdleStatus: tracequery.CPUBusyIdleStatusUnavailable,
					BusyIdleReason: "no_measured_cpu_busy_idle",
					MaxFrequency:   1000, ComputeSupplySignal: "class_frequency_observed",
				},
				{
					Class: "small", CPUs: []int{1}, BusyMs: 0, IdleMs: 10,
					BusyIdleStatus: tracequery.CPUBusyIdleStatusMeasured,
				},
			},
		},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "window_stats"}, "path", "")
	for _, want := range []string{
		"subject_census=not_evaluated missing_cpus=not_evaluated missing_threads=not_evaluated",
		"cpu=0 core_class= busy=unavailable idle=unavailable busy_idle_status=unavailable",
		"cpu=1 core_class= busy=0.000ms idle=10.000ms busy_idle_status=measured",
		"core_class=big cpus=[0] busy=unavailable idle=unavailable busy_idle_status=unavailable busy_idle_reason=no_measured_cpu_busy_idle",
		"core_class=small cpus=[1] busy=0.000ms idle=10.000ms busy_idle_status=measured",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "cpu=0 core_class= busy=0.000ms") {
		t.Fatalf("unavailable CPU was rendered as measured zero:\n%s", summary)
	}
	if strings.Contains(summary, "core_class=big cpus=[0] busy=0.000ms") {
		t.Fatalf("unavailable core class was rendered as measured zero:\n%s", summary)
	}
}
