package tool

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// wakeupAdvisoryNoteMarker is the stable head of the R5 advisory note
// (§7.30 裁定3, softened per §29.60 2026-07-13). Tests pin on it so the
// wording can evolve without re-pinning every sentence.
const wakeupAdvisoryNoteMarker = "wakeup-chain coverage note:"

// TestEmitInvestigationComplete_WakeupChainDrilldownOneShot pins the §29.60
// softened shape of §7.30 裁定3: when the run's own drilldown plan marks the
// dominant sleep as chain_required=true and no wakeup-chain-family
// observation exists, the FIRST completion attempt is ACCEPTED (behavior
// flip: it used to be downgraded — witness codrax-20260713-061452 iter=4)
// and carries a one-shot advisory note plus a typed caveat; a SECOND
// completion never repeats the note (per-artifact one-shot preserved).
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
	// §29.60 flip pin: the first attempt COMPLETES — the arm no longer blocks.
	if !first.Success || strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("first attempt must be accepted (§29.60 soften), got: %s", first.Summary)
	}
	if !strings.Contains(first.Summary, wakeupAdvisoryNoteMarker) || !strings.Contains(first.Summary, "oney.hmn.berlin-42591") {
		t.Fatalf("accepted completion must carry the wakeup-chain advisory note naming the subject, got: %s", first.Summary)
	}
	// Honesty contract: the note attributes the subject to the window census,
	// never claims it is the question's target thread.
	if !strings.Contains(first.Summary, "census") {
		t.Fatalf("advisory note must attribute the subject to the window census, got: %s", first.Summary)
	}
	// Typed caveat lane recorded for downstream consumers.
	caveats := mut.EvidenceClosure().CompletionCaveats()
	foundCaveat := false
	for _, c := range caveats {
		if c.Lane == types.DowngradeLaneWakeupChainDrilldown {
			foundCaveat = true
		}
	}
	if !foundCaveat {
		t.Fatalf("accepted completion must record the wakeup_chain_drilldown caveat, got %+v", caveats)
	}

	// One-shot: a second completion must not repeat the note.
	second, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !second.Success {
		t.Fatalf("second attempt must pass, got: %s", second.Summary)
	}
	if strings.Contains(second.Summary, wakeupAdvisoryNoteMarker) {
		t.Fatalf("one-shot advisory must not repeat on the second attempt, got: %s", second.Summary)
	}

	// Control: a run that DID produce a wakeup-chain observation never gets
	// the note (reconcile-first evaluation preserved).
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
		t.Fatalf("runs with wakeup-chain observations must pass, got: %s", res2.Summary)
	}
	if strings.Contains(res2.Summary, wakeupAdvisoryNoteMarker) {
		t.Fatalf("covered runs must not carry the advisory note, got: %s", res2.Summary)
	}
}

// TestEmitInvestigationComplete_WakeupChainSiblingStateChurnReconciles pins
// CHAIN-RECONCILE (账本 §18.B B-2, P1) on the §29.60 advisory surface: the
// reconcile-first evaluation is unchanged — a chain_required=true row with a
// same-subject overlapping state_churn chain_required=false sibling emits NO
// note; an unreconciled row emits the note WITH attribution. Completion now
// passes in every arm (§29.60 flip: the arm never blocks).
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
	// subject → no note at all.
	mut := types.NewMutableState("分析 42591 滑动卡顿,不分析代码")
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
			RichNotes: []string{"source=top_sleep", "chain_required=true", "selected_window=1200.000000..1260.000000"},
		}},
	})
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName:  "trace_query",
		Success:   true,
		Timestamp: time.Now(),
		Observations: []types.ObservationRecord{{
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
	if strings.Contains(res.Summary, wakeupAdvisoryNoteMarker) {
		t.Fatalf("①: a sleep subject with a chain_required=false state_churn sibling must NOT draw the advisory note, got: %s", res.Summary)
	}
	if !res.Success || strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("①: reconciled completion must pass, got: %s", res.Summary)
	}

	// ② Same-Subject keying: ANOTHER thread's state_churn chain_required=false
	// row must not reconcile — the note still fires (naming the chain-required
	// thread) and the completion still passes (§29.60 flip).
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
	if !strings.Contains(res2.Summary, wakeupAdvisoryNoteMarker) || !strings.Contains(res2.Summary, "oney.hmn.berlin-42591") {
		t.Fatalf("②: a state_churn sibling of a DIFFERENT subject must not reconcile — note must fire, got: %s", res2.Summary)
	}
	if !res2.Success || strings.TrimSpace(mut2.InvestigationCompleteReason()) == "" {
		t.Fatalf("②: completion must still pass (§29.60 — the arm never blocks), got: %s", res2.Summary)
	}

	// ③ Attribution: a genuinely-unreconciled subject's note carries the query
	// window and the top_sleep source so the row is locatable.
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
	if !strings.Contains(res3.Summary, wakeupAdvisoryNoteMarker) {
		t.Fatalf("③: pure fragmented top_sleep with no sibling must draw the note, got: %s", res3.Summary)
	}
	if !strings.Contains(res3.Summary, "worker-77012") ||
		!strings.Contains(res3.Summary, "800.000000..840.000000") ||
		!strings.Contains(res3.Summary, "top_sleep") {
		t.Fatalf("③: note must carry subject, narrow window, and source attribution, got: %s", res3.Summary)
	}
	if !res3.Success || strings.TrimSpace(mut3.InvestigationCompleteReason()) == "" {
		t.Fatalf("③: completion must pass with the note (§29.60), got: %s", res3.Summary)
	}

	// ④ Source-lane precision: a chain_required=false sibling whose source is
	// NOT state_churn must not reconcile — the note fires; completion passes.
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
	if !strings.Contains(res4.Summary, wakeupAdvisoryNoteMarker) {
		t.Fatalf("④: a chain_required=false sibling without source=state_churn must not reconcile — note must fire, got: %s", res4.Summary)
	}
	if !res4.Success || strings.TrimSpace(mut4.InvestigationCompleteReason()) == "" {
		t.Fatalf("④: completion must pass with the note (§29.60), got: %s", res4.Summary)
	}
}

// TestEmitInvestigationComplete_WakeupChainSiblingWindowOverlap pins the
// window-relation rules of CHAIN-RECONCILE (账本 §18.B B-2 follow-up) on the
// §29.60 advisory surface: a state_churn / chain_required=false sibling
// reconciles ONLY from an overlapping window with both window notes present;
// disjoint or note-missing shapes keep the advisory note. Completion always
// passes (§29.60 flip — the arm never blocks).
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
		if !res.Success || strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
			t.Fatalf("%s: completion must pass (§29.60 — the arm never blocks), got: %s", label, res.Summary)
		}
		return res, mut
	}

	// ② Disjoint windows: churn evidence from 3000..5000 says nothing about
	// the fragmented sleep in 1200..1260 — the note fires.
	res, _ := run("不相交窗",
		[]string{"source=top_sleep", "chain_required=true", "selected_window=1200.000000..1260.000000"},
		[]string{"source=state_churn", "chain_required=false", "selected_window=3000.000000..5000.000000"})
	if !strings.Contains(res.Summary, wakeupAdvisoryNoteMarker) {
		t.Fatalf("②: a state_churn sibling from a DISJOINT window must not reconcile — note must fire, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "1200.000000..1260.000000") {
		t.Fatalf("②: note must carry the chain-required row's window attribution, got: %s", res.Summary)
	}

	// ③a Sibling missing its window note: cannot prove overlap → conservative,
	// no reconcile → note fires.
	res, _ = run("兄弟行缺窗口",
		[]string{"source=top_sleep", "chain_required=true", "selected_window=1200.000000..1260.000000"},
		[]string{"source=state_churn", "chain_required=false"})
	if !strings.Contains(res.Summary, wakeupAdvisoryNoteMarker) {
		t.Fatalf("③a: a sibling without a window note must not reconcile (conservative), got: %s", res.Summary)
	}

	// ③b Chain-required row missing its window note: overlap unprovable from
	// the other side too → conservative, note fires.
	res, _ = run("主行缺窗口",
		[]string{"source=top_sleep", "chain_required=true"},
		[]string{"source=state_churn", "chain_required=false", "selected_window=1000.000000..6000.000000"})
	if !strings.Contains(res.Summary, wakeupAdvisoryNoteMarker) {
		t.Fatalf("③b: a chain-required row without a window note must not be reconciled (conservative), got: %s", res.Summary)
	}
}

// TestEmitInvestigationComplete_RunnableDrilldownNeverForcesWakeupChain pins
// RN-11 (§7.9, cust_runnable 2026-07-04) on the advisory surface: a
// runnable-dominant state_drilldown row must NEVER draw the wakeup-chain
// note — runnable starvation is CPU competition, not a wakeup dependency.
// The gate is keyed on the typed Object=="s_sleep", so even a legacy
// chain_required=true note on a runnable row must not trigger it.
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
				// chain_required=true on a runnable row, the advisory must
				// not fire for it.
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
	if strings.Contains(res.Summary, wakeupAdvisoryNoteMarker) {
		t.Fatalf("RN-11: runnable-dominant drilldown row must never draw the wakeup-chain note, got: %s", res.Summary)
	}
	if !res.Success || strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("runnable-dominant completion must pass without a wakeup-chain note, got: %s", res.Summary)
	}
}
