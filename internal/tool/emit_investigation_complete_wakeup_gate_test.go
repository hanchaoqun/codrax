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

// TestEmitInvestigationComplete_WakeupChainSiblingStateChurnReconciles pins
// CHAIN-RECONCILE (账本 §18.B B-2, P1): the SAME (thread, s_sleep) subject
// carries two sibling state_drilldown rows across different calls — a
// narrow-window top_sleep row (chain_required=true; FragmentCount<4 in that
// window, so it survived the fragmented filter) and a wide-window state_churn
// row (chain_required=false). The completion gate must NOT force a
// wakeup_chain drilldown for a subject that already has the
// chain_required=false state_churn evidence face; the sleep is covered
// without a chain. Pins:
//
//	①  a chain_required=true top_sleep row WITH a chain_required=false
//	   state_churn sibling for the same subject whose wide query window
//	   CONTAINS the narrow one (the q9 shape) → NO downgrade (false downgrade
//	   removed);
//	②  the reconcile is keyed on the SAME Subject — another thread's
//	   state_churn sibling reconciles nothing;
//	③  the downgrade text of a genuinely-required subject carries the query
//	   window and the source attribution so the model can locate the row;
//	④  a chain_required=false sibling WITHOUT source=state_churn reconciles
//	   nothing — the false verdict must come from the churn decomposition lane
//	   (its window overlaps here, so ONLY the source lane rejects it).
//
// The window-relation arms (disjoint sibling window / missing window notes)
// are pinned separately by
// TestEmitInvestigationComplete_WakeupChainSiblingWindowOverlap.
func TestEmitInvestigationComplete_WakeupChainSiblingStateChurnReconciles(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"目标线程滑动窗口内 sleep 主导,宽窗 state_churn 已覆盖",
		"confidence":"high",
		"result_kind":"resolved"
	}`)
	preflight := types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
		SourceNavigationOptional: true,
		Artifacts: []types.RuntimeArtifactPreflightArtifact{{
			Kind: "trace", Source: "berlin.systrace", Carrier: "request_path",
		}},
		RepoSourceCensus: types.RuntimeArtifactRepoSourceCensus{Completed: true, ArtifactFiles: 1},
	})
	ir := &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentRootCause}}

	// ① Reconciled: the narrow-window top_sleep row (chain_required=true) has a
	// wide-window state_churn sibling (chain_required=false) for the SAME
	// subject → the sleep already has a non-chain-required evidence face.
	mut := types.NewMutableState("分析 42591 滑动卡顿,不分析代码")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName:  "trace_query",
		Success:   true,
		Timestamp: time.Now(),
		Observations: []types.ObservationRecord{{
			// narrow window: top_sleep survives the fragmented filter
			// (FragmentCount<4 inside this window), chain_required=true
			ID:        "trace_query:narrow#state_drilldown:1",
			Producer:  "trace_query",
			Subject:   "oney.hmn.berlin-42591",
			Predicate: "state_drilldown",
			Object:    "s_sleep",
			RichNotes: []string{"source=top_sleep", "chain_required=true", "selected_window=1200.000000..1260.000000"},
		}},
	})
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName:  "trace_query",
		Success:   true,
		Timestamp: time.Now(),
		Observations: []types.ObservationRecord{{
			// wide window: state_churn, chain_required=false — the reconciling sibling
			ID:        "trace_query:wide#state_drilldown:1",
			Producer:  "trace_query",
			Subject:   "oney.hmn.berlin-42591",
			Predicate: "state_drilldown",
			Object:    "s_sleep",
			RichNotes: []string{"source=state_churn", "chain_required=false", "selected_window=1000.000000..6000.000000"},
		}},
	})
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir, RuntimeArtifactPreflight: preflight}
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(res.Summary, "wakeup-chain drilldown") || strings.Contains(res.Summary, "wakeup_chain view was run") {
		t.Fatalf("①: a sleep subject with a chain_required=false state_churn sibling must NOT be downgraded, got: %s", res.Summary)
	}
	if !res.Success || strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("①: reconciled completion must pass, got: %s", res.Summary)
	}

	// ② Same-Subject keying: ANOTHER thread's state_churn chain_required=false
	// row must not reconcile this thread's obligation — the downgrade still
	// fires and names the chain-required thread.
	mut2 := types.NewMutableState("分析 42591 滑动卡顿,另一线程宽窗碎片化")
	mut2.AppendDispatchToolResult(types.ToolResult{
		ToolName:  "trace_query",
		Success:   true,
		Timestamp: time.Now(),
		Observations: []types.ObservationRecord{
			{
				ID:        "trace_query:narrow#state_drilldown:1",
				Producer:  "trace_query",
				Subject:   "oney.hmn.berlin-42591",
				Predicate: "state_drilldown",
				Object:    "s_sleep",
				RichNotes: []string{"source=top_sleep", "chain_required=true", "selected_window=1200.000000..1260.000000"},
			},
			{
				// different Subject — must reconcile nothing
				ID:        "trace_query:wide#state_drilldown:2",
				Producer:  "trace_query",
				Subject:   "render_service-1522",
				Predicate: "state_drilldown",
				Object:    "s_sleep",
				RichNotes: []string{"source=state_churn", "chain_required=false", "selected_window=1000.000000..6000.000000"},
			},
		},
	})
	bus2 := &types.BusContext{Mutable: mut2, AnalysisIR: ir, RuntimeArtifactPreflight: preflight}
	res2, err := tool.Execute(bus2, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res2.Summary, "wakeup_chain") || !strings.Contains(res2.Summary, "oney.hmn.berlin-42591") {
		t.Fatalf("②: a state_churn sibling of a DIFFERENT subject must not reconcile the obligation, got: %s", res2.Summary)
	}
	if strings.TrimSpace(mut2.InvestigationCompleteReason()) != "" {
		t.Fatalf("②: cross-subject churn row must not let the first attempt pass, got: %s", res2.Summary)
	}

	// ③ Genuinely required, with attribution: a chain_required=true top_sleep
	// row and NO state_churn sibling still downgrades, and the message names
	// the query window and the top_sleep source.
	mut3 := types.NewMutableState("分析 77012 纯碎片睡眠")
	mut3.AppendDispatchToolResult(types.ToolResult{
		ToolName:  "trace_query",
		Success:   true,
		Timestamp: time.Now(),
		Observations: []types.ObservationRecord{{
			ID:        "trace_query:narrow#state_drilldown:1",
			Producer:  "trace_query",
			Subject:   "worker-77012",
			Predicate: "state_drilldown",
			Object:    "s_sleep",
			RichNotes: []string{"source=top_sleep", "chain_required=true", "selected_window=800.000000..840.000000"},
		}},
	})
	bus3 := &types.BusContext{Mutable: mut3, AnalysisIR: ir, RuntimeArtifactPreflight: preflight}
	res3, err := tool.Execute(bus3, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res3.Summary, "wakeup_chain") {
		t.Fatalf("③: pure fragmented top_sleep with no sibling must still downgrade, got: %s", res3.Summary)
	}
	if !strings.Contains(res3.Summary, "worker-77012") ||
		!strings.Contains(res3.Summary, "800.000000..840.000000") ||
		!strings.Contains(res3.Summary, "top_sleep") {
		t.Fatalf("③: downgrade text must carry subject, narrow window, and source attribution, got: %s", res3.Summary)
	}
	if strings.TrimSpace(mut3.InvestigationCompleteReason()) != "" {
		t.Fatalf("③: first attempt of a genuinely-required subject must be downgraded (not accepted), got: %s", res3.Summary)
	}

	// ④ Source-lane precision: a chain_required=false sibling whose source is
	// NOT state_churn (adversarial/legacy shape — today an s_sleep row is
	// chain_required=false ⟺ source=state_churn by construction) must not
	// reconcile; the semantic justification is the wide-window churn
	// decomposition, not the bare boolean.
	mut4 := types.NewMutableState("分析 42591 滑动卡顿,非 churn 假兄弟")
	mut4.AppendDispatchToolResult(types.ToolResult{
		ToolName:  "trace_query",
		Success:   true,
		Timestamp: time.Now(),
		Observations: []types.ObservationRecord{
			{
				ID:        "trace_query:narrow#state_drilldown:1",
				Producer:  "trace_query",
				Subject:   "oney.hmn.berlin-42591",
				Predicate: "state_drilldown",
				Object:    "s_sleep",
				RichNotes: []string{"source=top_sleep", "chain_required=true", "selected_window=1200.000000..1260.000000"},
			},
			{
				// same Subject, chain_required=false but NOT from the churn lane
				ID:        "trace_query:wide#state_drilldown:2",
				Producer:  "trace_query",
				Subject:   "oney.hmn.berlin-42591",
				Predicate: "state_drilldown",
				Object:    "s_sleep",
				RichNotes: []string{"source=top_sleep", "chain_required=false", "selected_window=1000.000000..6000.000000"},
			},
		},
	})
	bus4 := &types.BusContext{Mutable: mut4, AnalysisIR: ir, RuntimeArtifactPreflight: preflight}
	res4, err := tool.Execute(bus4, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res4.Summary, "wakeup_chain") || strings.TrimSpace(mut4.InvestigationCompleteReason()) != "" {
		t.Fatalf("④: a chain_required=false sibling without source=state_churn must not reconcile, got: %s", res4.Summary)
	}
}

// TestEmitInvestigationComplete_WakeupChainSiblingWindowOverlap pins the
// window-relation tightening of the CHAIN-RECONCILE rule (账本 §18.B B-2
// follow-up): a state_churn / chain_required=false sibling reconciles a
// chain_required=true row ONLY when its query window overlaps the
// chain-required row's window. A churn face from a disjoint window (thread
// churned in window A, fragmented-slept in window B) explains nothing about
// the sleep in B; and when the typed window note is missing on EITHER side,
// the gate conservatively keeps the obligation — precise-signal absence is
// never guessed over. Arms:
//
//	②  DISJOINT windows: same-Subject state_churn chain_required=false sibling
//	   from a non-overlapping window → NOT reconciled, downgrade still fires;
//	③a sibling row missing its window note → NOT reconciled, downgrade fires;
//	③b chain-required row missing its window note (sibling has one) → NOT
//	   reconciled, downgrade fires (conservative arm is two-sided).
//
// The overlapping/containing shape (q9) staying reconciled is pinned as arm ①
// of TestEmitInvestigationComplete_WakeupChainSiblingStateChurnReconciles.
func TestEmitInvestigationComplete_WakeupChainSiblingWindowOverlap(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"目标线程窄窗碎片睡眠,另窗 churn 证据不覆盖",
		"confidence":"high",
		"result_kind":"resolved"
	}`)
	preflight := types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
		SourceNavigationOptional: true,
		Artifacts: []types.RuntimeArtifactPreflightArtifact{{
			Kind: "trace", Source: "berlin.systrace", Carrier: "request_path",
		}},
		RepoSourceCensus: types.RuntimeArtifactRepoSourceCensus{Completed: true, ArtifactFiles: 1},
	})
	ir := &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentRootCause}}
	run := func(label string, chainRequiredNotes, siblingNotes []string) (types.ToolResult, *types.MutableState) {
		t.Helper()
		mut := types.NewMutableState("分析 42591 " + label)
		mut.AppendDispatchToolResult(types.ToolResult{
			ToolName:  "trace_query",
			Success:   true,
			Timestamp: time.Now(),
			Observations: []types.ObservationRecord{{
				ID:        "trace_query:narrow#state_drilldown:1",
				Producer:  "trace_query",
				Subject:   "oney.hmn.berlin-42591",
				Predicate: "state_drilldown",
				Object:    "s_sleep",
				RichNotes: chainRequiredNotes,
			}},
		})
		mut.AppendDispatchToolResult(types.ToolResult{
			ToolName:  "trace_query",
			Success:   true,
			Timestamp: time.Now(),
			Observations: []types.ObservationRecord{{
				ID:        "trace_query:other#state_drilldown:1",
				Producer:  "trace_query",
				Subject:   "oney.hmn.berlin-42591",
				Predicate: "state_drilldown",
				Object:    "s_sleep",
				RichNotes: siblingNotes,
			}},
		})
		bus := &types.BusContext{Mutable: mut, AnalysisIR: ir, RuntimeArtifactPreflight: preflight}
		res, err := tool.Execute(bus, params)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", label, err)
		}
		return res, mut
	}

	// ② Disjoint windows: churn evidence from 3000..5000 says nothing about
	// the fragmented sleep in 1200..1260 — the obligation stands.
	res, mut := run("不相交窗",
		[]string{"source=top_sleep", "chain_required=true", "selected_window=1200.000000..1260.000000"},
		[]string{"source=state_churn", "chain_required=false", "selected_window=3000.000000..5000.000000"})
	if !strings.Contains(res.Summary, "wakeup_chain") || strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("②: a state_churn sibling from a DISJOINT window must not reconcile — downgrade must still fire, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "1200.000000..1260.000000") {
		t.Fatalf("②: downgrade text must still carry the chain-required row's window attribution, got: %s", res.Summary)
	}

	// ③a Sibling missing its window note: cannot prove overlap → conservative,
	// no reconcile.
	res, mut = run("兄弟行缺窗口",
		[]string{"source=top_sleep", "chain_required=true", "selected_window=1200.000000..1260.000000"},
		[]string{"source=state_churn", "chain_required=false"})
	if !strings.Contains(res.Summary, "wakeup_chain") || strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("③a: a sibling without a window note must not reconcile (conservative), got: %s", res.Summary)
	}

	// ③b Chain-required row missing its window note: overlap is unprovable
	// from the other side too → conservative, no reconcile.
	res, mut = run("主行缺窗口",
		[]string{"source=top_sleep", "chain_required=true"},
		[]string{"source=state_churn", "chain_required=false", "selected_window=1000.000000..6000.000000"})
	if !strings.Contains(res.Summary, "wakeup_chain") || strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("③b: a chain-required row without a window note must not be reconciled (conservative), got: %s", res.Summary)
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
