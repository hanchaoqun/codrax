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

	// Interleave regression: even when OTHER completion-gate denials (e.g.
	// the citation floor's streak fingerprints) fire between attempts, the
	// wakeup gate must never re-arm — the sticky per-run marker is immune to
	// the single-slot streak resets that made the first implementation
	// mutually destructive with the citation-denial breaker.
	mut.RecordCompletionDenialStreak("citation_floor min=2 eligible=0 reads=1")
	mut.RecordCompletionDenialStreak("citation_floor min=2 eligible=0 reads=2")
	third, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(third.Summary, "wakeup_chain view was run") {
		t.Fatalf("one-shot gate must stay disarmed across interleaved denials, got: %s", third.Summary)
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

// TestEmitInvestigationComplete_RunnableDrilldownNeverForcesWakeupChain pins
// RN-11 (§7.9, cust_runnable 2026-07-04): a runnable-dominant state_drilldown
// row must NEVER draw the wakeup-chain completion downgrade — runnable
// starvation is CPU competition (occupancy / scheduler_latency surfaces), not
// a wakeup dependency; the customer's exploration round 10 was pushed toward
// view=wakeup_chain for a thread with sleep=0 and no wakeup edge. The gate is
// keyed on the typed Object=="s_sleep", so even a legacy chain_required=true
// note on a runnable row (pre-RN-11 producers) must not trigger it. The
// sleep-row behavior is pinned unchanged by
// TestEmitInvestigationComplete_WakeupChainDrilldownOneShot above.
func TestEmitInvestigationComplete_RunnableDrilldownNeverForcesWakeupChain(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("分析 49706 调度延迟")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName:  "trace_query",
		Success:   true,
		Timestamp: time.Now(),
		Observations: []types.ObservationRecord{{
			ID:        "trace_query:w#state_drilldown:1",
			Origin:    types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:  "trace_query",
			Subject:   "OS_FFRT_2_3-49706",
			Predicate: "state_drilldown",
			Object:    "runnable",
			RichNotes: []string{
				"impact=2528.000ms",
				// Adversarial legacy shape: even if a producer still stamps
				// chain_required=true on a runnable row, the completion gate
				// must not force a wakeup chain for it.
				"chain_required=true",
				"recommended_views=scheduler_latency_stats,root_cause_rank,window_stats",
			},
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
				Kind: "trace", Source: "cust_runnable.systrace", Carrier: "request_path",
			}},
			RepoSourceCensus: types.RuntimeArtifactRepoSourceCensus{Completed: true, ArtifactFiles: 1},
		}),
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"FFRT 线程窗口内 runnable 2528ms,同窗占用者已定位",
		"confidence":"high",
		"result_kind":"resolved"
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(res.Summary, "wakeup_chain view was run") || strings.Contains(res.Summary, "wakeup-chain drilldown") {
		t.Fatalf("RN-11: runnable-dominant drilldown row must never draw the wakeup-chain downgrade, got: %s", res.Summary)
	}
	if !res.Success || strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("runnable-dominant completion must pass without a wakeup-chain attempt, got: %s", res.Summary)
	}
}
