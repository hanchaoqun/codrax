package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderAnswerDocObservationLedgerSeparatesTargetStateFromCPUWideScopeAA1(t *testing.T) {
	const window = "selected_window=34579.472865..34579.587805"
	observations := []types.ObservationRecord{
		{
			ID:              "rank:1",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane:  types.ObservationProvenanceObservedDirectCause,
			SourceRef: types.ObservationSourceRef{
				Kind:       types.ObservationSourceRuntimeArtifact,
				Path:       "/tmp/tieba.systrace",
				ArtifactID: "attached_trace",
			},
			ClaimKey:  "root_cause_primary",
			Subject:   "CookieMonsterCl-59843",
			Predicate: "root_cause_primary",
			Object:    "priority_inversion_candidate",
			Value:     "23.994",
			Unit:      "ms",
			RichNotes: []string{
				"rank=1",
				"tier=primary",
				"effective_impact_ms=23.994",
				"fix_direction=lock_priority",
				"chain_relevance=on_chain",
				window,
			},
		},
		{
			ID:              "target-state",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef: types.ObservationSourceRef{
				Kind:       types.ObservationSourceRuntimeArtifact,
				Path:       "/tmp/tieba.systrace",
				ArtifactID: "attached_trace",
			},
			ClaimKey:  "target_window_states:com.baidu.tieba-59566",
			Subject:   "com.baidu.tieba-59566",
			Predicate: "target_window_states",
			Object:    "state_partition",
			Value:     "114.940",
			Unit:      "ms",
			RichNotes: []string{
				"running=26.946",
				"runnable=3.636",
				"sleep=84.358",
				"d_state=0",
				"total=114.940",
				window,
			},
		},
	}
	mut := types.NewMutableState("分析显式窗口内目标线程状态")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName:     "trace_query",
		Success:      true,
		Observations: observations,
	}}})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentRootCause},
		},
	}

	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"### Target-thread scheduler-state accounting",
		"They describe only that thread",
		"running 26.946 ms, runnable but not yet scheduled 3.636 ms",
		"does not establish system-wide CPU utilization, idle capacity, or saturation",
		"Preserve the published millisecond values and window precision",
		"A scheduler state says where the target spent time, not why",
		"complete coverage; 0.000 ms unaccounted",
		"narrow scheduler-marked definition",
		"Target blocking closed by IO issue-to-completion evidence is a separate ruler",
		"unassessed rather than zero",
		"does not prove yielding, idleness, preemption, IO, a lock",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed target-state scope authority missing %q:\n%s", want, got)
		}
	}
	for _, internal := range []string{"state_partition_coverage", "io_wait_caliber", "sleep_mechanism", "cpu_wide_saturation_authority"} {
		if strings.Contains(got, internal) {
			t.Fatalf("target-state reader authority leaked machine token %q:\n%s", internal, got)
		}
	}
}

func TestRenderAnswerDocObservationLedgerPublishesFiniteExplicitWindowStateWithoutCausalBoard(t *testing.T) {
	start, end := 10.0, 10.1
	mut := types.NewMutableState("查询目标线程四态")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{{
			ID: "state-requested", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
			SourceRef: types.ObservationSourceRef{
				Kind: types.ObservationSourceRuntimeArtifact, Path: "/tmp/customer.systrace",
			},
			ClaimKey: "target_window_states:main-100", Predicate: "target_window_states",
			Subject: "main-100", Object: "state_partition", Value: "100.000", Unit: "ms",
			RichNotes: []string{
				"selected_window=10.000000..10.100000", "running=20.000", "runnable=10.000",
				"sleep=70.000", "d_state=0.000", "io_wait=0.000", "sleep_io_wait=3.000", "total=100.000",
			},
		}},
	}}})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RuntimeTargets: []types.RuntimeTarget{{Thread: "ui-100", Source: "user_explicit"}},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
				RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
				TimeStart:      &start, TimeEnd: &end, SourceQuote: "10.0 到 10.1",
			},
		}},
	}
	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"### Target-thread scheduler-state accounting",
		"target thread main-100",
		// EVOLUTION RECORD (V3-1, §40.20): the account sentence is now the
		// types-level formatter (types.FormatTargetStateAccount): the sleep-
		// side IO marker is disclosed INSIDE the sleep term, the
		// uninterruptible figure is the D+IO fold with its IO share disclosed.
		"running 20.000 ms, runnable but not yet scheduled 10.000 ms, interruptible sleep 70.000 ms (including 3.000 ms of interruptible sleep carrying an IO-wait marker, already inside the sleep term), uninterruptible wait 0.000 ms",
		"uninterruptible wait 0.000 ms (including 0.000 ms of scheduler-marked IO wait)",
		"complete coverage",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("finite state authority missing %q:\n%s", want, got)
		}
	}
}

func TestRenderAnswerDocObservationLedgerPublishesFiniteStateForTypedParenthesizedTarget(t *testing.T) {
	start, end := 13762.791708, 13763.024898
	mut := types.NewMutableState("查询目标线程四态")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{{
			ID: "state-requested", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
			SourceRef: types.ObservationSourceRef{
				Kind: types.ObservationSourceRuntimeArtifact, Path: "/tmp/donghu.ftrace",
			},
			ClaimKey: "target_window_states:.ugc.aweme.lite-17267", Predicate: "target_window_states",
			Subject: ".ugc.aweme.lite-17267", Object: "state_partition", Value: "233.190", Unit: "ms",
			RichNotes: []string{
				"selected_window=13762.791708..13763.024898", "running=157.248", "runnable=5.604",
				"sleep=70.338", "d_state=0.000", "io_wait=0.000", "sleep_io_wait=0.000", "total=233.190",
			},
		}},
	}}})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RuntimeTargets: []types.RuntimeTarget{{
				Kind: types.RuntimeTargetKindThread, Thread: ".ugc.aweme.lite-17267 (17267)", Source: "user_explicit",
			}},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
				RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
				TimeStart:      &start, TimeEnd: &end, SourceQuote: "13762.791708 到 13763.024898",
			},
		}},
	}
	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"### Target-thread scheduler-state accounting",
		"target thread .ugc.aweme.lite-17267",
		"running 157.248 ms, runnable but not yet scheduled 5.604 ms, interruptible sleep 70.338 ms, uninterruptible wait 0.000 ms",
		"total 233.190 ms; complete coverage",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("parenthesized typed target lost finite state authority %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Trace Causal Projection") {
		t.Fatalf("finite target-state authority must not manufacture a causal projection:\n%s", got)
	}
}
