package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderAnswerDocObservationLedgerCarriesTargetWakeupCensusSemanticsZ4(t *testing.T) {
	row := func(ordinal, waker, count string) types.ObservationRecord {
		return types.ObservationRecord{
			ID:              "trace_query:fixture#wakeup_edge_census:" + ordinal,
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
			SourceRef: types.ObservationSourceRef{
				Kind: types.ObservationSourceRuntimeArtifact,
				Path: "/tmp/tieba.systrace",
			},
			Predicate: "wakeup_edge_census",
			Subject:   waker,
			Object:    "com.baidu.tieba-59566",
			Value:     count,
			RichNotes: []string{
				types.TraceNoteKeyWakeupEdgeCensusTargetWakee + "=true",
				types.TraceNoteKeyWakeupEdgeCensusSleepExit + "=" + count,
				types.TraceNoteKeyWakeupEdgeCensusFirstTs + "=34579.475843",
				types.TraceNoteKeyWakeupEdgeCensusLastTs + "=34579.587805",
				types.TraceNoteKeySelectedWindow + "=34579.472865..34579.587805",
			},
		}
	}
	mut := types.NewMutableState("分析显式窗口内的唤醒关系")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{
			row("1", "CookieMonsterCl-59843", "34"),
			row("2", "Binder:43397_19-23088", "1"),
			row("3", "T7@ZeusThreadPo-61839", "1"),
		},
	}}})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentRootCause},
		},
	}

	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"### Trace Target Wakeup Census Authority",
		"target=`com.baidu.tieba-59566`",
		"status=`complete`; total_wakeups=36",
		"waker=`CookieMonsterCl-59843`; count=34",
		"pre_wakeup_exit_split=`sleep:36 d_or_io:0 other_or_unclassified:0`",
		"classifies the scheduler state the target LEFT",
		"cannot support “woke and immediately slept N times”",
		"requires a separately complete paired scheduler-transition census",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed target wakeup authority missing %q:\n%s", want, got)
		}
	}
}
