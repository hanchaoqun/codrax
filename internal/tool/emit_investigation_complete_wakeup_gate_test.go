package tool

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestEmitInvestigationComplete_WakeupChainDrilldownOneShot pins §7.30 裁定3:
// when the run's own drilldown plan marks the dominant sleep as
// chain_required=true and no wakeup-chain-family observation exists, the
// FIRST completion attempt is downgraded with a bounded wakeup_chain
// directive; the SECOND attempt passes regardless (one-shot, never a loop).
func TestEmitInvestigationComplete_WakeupChainDrilldownOneShot(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("分析 42591 滑动卡顿,不分析代码")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName:  "trace_query",
		Success:   true,
		Timestamp: time.Now(),
		Observations: []types.ObservationRecord{{
			ID:        "trace_query:w#state_drilldown:1",
			Origin:    types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:  "trace_query",
			Subject:   "oney.hmn.berlin-42591",
			Predicate: "state_drilldown",
			Object:    "s_sleep",
			RichNotes: []string{"impact=1280.602ms", "chain_required=true", "recommended_views=wakeup_chain,root_cause_rank"},
		}},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentRootCause},
		},
		RuntimeArtifactPreflight: types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
			SourceNavigationOptional: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind: "trace", Source: "berlin.systrace", Carrier: "request_path",
			}},
			RepoSourceCensus: types.RuntimeArtifactRepoSourceCensus{Completed: true, ArtifactFiles: 1},
		}),
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"主线程滑动窗口内 sleep 1280ms,状态碎片化",
		"confidence":"high",
		"result_kind":"resolved"
	}`)
	first, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(first.Summary, "wakeup_chain") || strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("first attempt must be downgraded with a wakeup_chain directive, got: %s", first.Summary)
	}
	second, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !second.Success || strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("second attempt must pass (one-shot gate), got: %s", second.Summary)
	}

	// Control: a run that DID produce a wakeup-chain observation is never
	// gated.
	mut2 := types.NewMutableState("同上")
	mut2.AppendDispatchToolResult(types.ToolResult{
		ToolName:  "trace_query",
		Success:   true,
		Timestamp: time.Now(),
		Observations: []types.ObservationRecord{
			{Producer: "trace_query", Predicate: "state_drilldown", Subject: "t-1", Object: "s_sleep", RichNotes: []string{"chain_required=true"}},
			{Producer: "trace_query", Predicate: "wakeup_causal_impact", Subject: "dep-2", Object: "t-1"},
		},
	})
	bus2 := &types.BusContext{Mutable: mut2, AnalysisIR: bus.AnalysisIR, RuntimeArtifactPreflight: bus.RuntimeArtifactPreflight}
	res2, err := tool.Execute(bus2, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res2.Success || strings.TrimSpace(mut2.InvestigationCompleteReason()) == "" {
		t.Fatalf("runs with wakeup-chain observations must not be gated, got: %s", res2.Summary)
	}
}
