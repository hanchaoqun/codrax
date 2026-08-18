package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderAnswerDocObservationLedgerCarriesTraceRankAuthorityZ3(t *testing.T) {
	rank := func(id, claim, tier, typ, subject, direction, value string, ordinal int) types.ObservationRecord {
		return types.ObservationRecord{
			ID:              id,
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
			ClaimKey:  claim,
			Subject:   subject,
			Predicate: claim,
			Object:    typ,
			Value:     value,
			Unit:      "ms",
			RichNotes: []string{
				"rank=" + string(rune('0'+ordinal)),
				"tier=" + tier,
				"type=" + typ,
				"effective_impact_ms=" + value,
				"fix_direction=" + direction,
				"chain_relevance=on_chain",
				"selected_window=34579.472865..34579.587805",
			},
			Confidence: 0.9,
		}
	}
	observations := []types.ObservationRecord{
		rank("rank:1", "root_cause_primary", "primary", "priority_inversion_candidate", "CookieMonsterCl-59843", "lock_priority", "23.994", 1),
		rank("rank:2", "root_cause_secondary", "secondary", "d_state_or_io_wait", "ThreadPoolForeg-60555", "io_dependency", "10.433", 2),
		rank("rank:3", "root_cause_tertiary", "tertiary", "runnable_wait", "RenderThread-60666", "scheduler_supply", "10.400", 3),
		rank("rank:4", "root_cause_tertiary", "tertiary", "running", "com.baidu.tieba-59566", "frequency_thermal", "10.331", 4),
		rank("context:binder", "root_cause_context_only", "context_only", "binder_wait", "binder:496_9-10961", "io_dependency", "1.409", 0),
	}
	adjacent := rank("adjacent:1", "root_cause_tertiary", "tertiary", "runnable_wait", "neighbor-1", "scheduler_supply", "2.000", 1)
	adjacent.RichNotes[len(adjacent.RichNotes)-2] = "chain_relevance=adjacent"
	observations = append(observations, adjacent)
	mut := types.NewMutableState("分析显式窗口内需求侧与供给侧")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName:     "trace_query",
		Success:      true,
		Observations: observations,
	}}})
	ctx := &types.AgentContext{
		Mutable:  mut,
		Language: "zh-CN",
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentRootCause},
		},
	}
	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"### 面向答案的 Trace 排名与算力供给口径",
		"清单完整",
		"排序范围：唤醒/依赖链上",
		"排序范围：邻近区域（仅支撑额外排查，不属于链上主因）",
		"按名次排列的原因清单",
		"#1 CookieMonsterCl-59843：优先级反转候选；按现有规则可消除影响 23.994ms",
		"#2 ThreadPoolForeg-60555：D-state/iowait；按现有规则可消除影响 10.433ms",
		"#3 RenderThread-60666：runnable；按现有规则可消除影响 10.400ms",
		"#4 com.baidu.tieba-59566：running；按现有规则可消除影响 10.331ms",
		"算力供给补充：#4 com.baidu.tieba-59566",
		"已测算力供给提升空间",
		"不单独证明热限频、调频策略限制或绑错核",
		"不能说它不存在或已被排除",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed trace-rank authority missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "清单完整") != 2 {
		t.Fatalf("on-chain and adjacent ordinals must publish as two complete boards:\n%s", got)
	}
	authorityStart := strings.Index(got, "### 面向答案的 Trace 排名与算力供给口径")
	if authorityStart < 0 {
		t.Fatal("trace rank authority block missing")
	}
	authority := got[authorityStart:]
	if end := strings.Index(authority, "\n- `rank:1`"); end > 0 {
		authority = authority[:end]
	}
	if strings.Contains(authority, "#0") || strings.Contains(authority, "binder:496_9-10961") {
		t.Fatalf("unranked binder context must not acquire an ordinal in the rank authority:\n%s", authority)
	}
	for _, forbidden := range []string{
		"priority_inversion_candidate", "d_state_or_io_wait", "runnable_wait",
		"type=`", "tier=`", "channel=`", "fix_direction", "roster_status", "board_channel",
	} {
		if strings.Contains(authority, forbidden) {
			t.Fatalf("reader-ready rank authority leaked internal token %q:\n%s", forbidden, authority)
		}
	}

	ctx.Language = "en"
	en := renderAnswerDocObservationLedger(ctx)
	enStart := strings.Index(en, "### Reader-ready Trace ranking and compute-supply basis")
	if enStart < 0 {
		t.Fatalf("english reader-ready rank authority missing:\n%s", en)
	}
	en = en[enStart:]
	if end := strings.Index(en, "\n- `rank:1`"); end > 0 {
		en = en[:end]
	}
	for _, want := range []string{
		"ranking scope: on the wakeup/dependency chain",
		"#1 CookieMonsterCl-59843: priority inversion (candidate); 23.994ms eliminable under existing rules",
		"#2 ThreadPoolForeg-60555: D-state/iowait; 10.433ms eliminable under existing rules",
		"Compute-supply note: #4 com.baidu.tieba-59566",
	} {
		if !strings.Contains(en, want) {
			t.Fatalf("english reader-ready rank authority missing %q:\n%s", want, en)
		}
	}
	for _, forbidden := range []string{"priority_inversion_candidate", "d_state_or_io_wait", "runnable_wait", "fix_direction", "roster_status", "board_channel"} {
		if strings.Contains(en, forbidden) {
			t.Fatalf("english reader-ready rank authority leaked internal token %q:\n%s", forbidden, en)
		}
	}
}
