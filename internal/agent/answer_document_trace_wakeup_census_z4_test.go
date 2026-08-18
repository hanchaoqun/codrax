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
		Language: "zh",
		Mutable:  mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentRootCause},
		},
	}

	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"### 面向答案的目标线程唤醒统计",
		"目标线程 com.baidu.tieba-59566；窗口 34579.472865..34579.587805；共记录 36 次唤醒（覆盖完整）",
		"唤醒发生前离开的状态：睡眠 36 次、D/IO 等待 0 次、其他或未分类 0 次",
		"唤醒方 CookieMonsterCl-59843 → 目标线程：34 次",
		"状态计数描述唤醒发生前目标线程离开的状态",
		"何时真正切入 CPU、是否随后被抢占或再次睡眠，需要独立且完整的调度切换证据",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("reader-ready target wakeup authority missing %q:\n%s", want, got)
		}
	}
	authority := got[strings.Index(got, "### 面向答案的目标线程唤醒统计"):]
	if end := strings.Index(authority, "*(showing "); end >= 0 {
		authority = authority[:end]
	}
	if end := strings.Index(authority, "\n- `trace_query:"); end >= 0 {
		authority = authority[:end]
	}
	for _, forbidden := range []string{"wakeup_edge_census", "target=`", "status=`complete`", "total_wakeups=", "pre_wakeup_exit_split=", "waker=`"} {
		if strings.Contains(authority, forbidden) {
			t.Fatalf("reader-ready target wakeup authority leaked control token %q:\n%s", forbidden, got)
		}
	}

	ctx.Language = "en"
	english := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"### Reader-ready target-thread wakeup counts",
		"Target com.baidu.tieba-59566; window 34579.472865..34579.587805; 36 wakeup(s) recorded (complete)",
		"state left before wakeup: sleep 36, D/IO wait 0, other or unclassified 0",
		"Waker CookieMonsterCl-59843 → target: 34 time(s)",
	} {
		if !strings.Contains(english, want) {
			t.Fatalf("English reader-ready wakeup authority missing %q:\n%s", want, english)
		}
	}
}
