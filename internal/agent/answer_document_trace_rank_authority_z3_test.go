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
	mut := types.NewMutableState("分析显式窗口内需求侧与供给侧")
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
		"### Trace Rank Arithmetic And Supply Authority",
		"cross_row_additivity=`forbidden`",
		"Never add several seats into a new total",
		"roster_status=`complete`",
		"ordered_ranked_roster",
		"`#1`; type=`priority_inversion_candidate`; subject=`CookieMonsterCl-59843`; effective=23.994ms",
		"`#2`; type=`d_state_or_io_wait`; subject=`ThreadPoolForeg-60555`; effective=10.433ms",
		"`#3`; type=`runnable_wait`; subject=`RenderThread-60666`; effective=10.400ms",
		"`#4`; type=`running`; subject=`com.baidu.tieba-59566`; effective=10.331ms",
		"compute_delivery_positive=true",
		"relation_to_top=`secondary_bounded_candidate`",
		"#4 running com.baidu.tieba-59566 10.331ms",
		"never as absent, eliminated, or disproven",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed trace-rank authority missing %q:\n%s", want, got)
		}
	}
	authorityStart := strings.Index(got, "### Trace Rank Arithmetic And Supply Authority")
	if authorityStart < 0 {
		t.Fatal("trace rank authority block missing")
	}
	authority := got[authorityStart:]
	if strings.Contains(authority, "`#0`") || strings.Contains(authority, "type=`binder_wait`; subject=`binder:496_9-10961`") {
		t.Fatalf("unranked binder context must not acquire an ordinal in the rank authority:\n%s", authority)
	}
}
