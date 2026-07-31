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
		"### Trace Target-State Scope Authority",
		"scope=`target_thread_only`",
		"running=26.946ms; runnable=3.636ms",
		"cpu_wide_saturation_authority=`not_provided_by_target_window_states`",
		"low runnable share can bound that target's scheduler queueing",
		"Never rename target-thread running share as CPU utilization",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed target-state scope authority missing %q:\n%s", want, got)
		}
	}
}
