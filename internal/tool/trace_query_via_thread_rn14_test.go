package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// RN-14a/RN-14b (§7.9, cust_runnable 2026-07-04) tool-surface pins: the
// via_thread schema parameter exists and decodes, and the runnable-anchor
// recovery hint fires exactly on the customer shape — runnable-dominant chain
// target + analyzer-pinned user-focus thread (pid 6565) that differs from the
// queried anchor — and stays silent otherwise.

// TestTraceQuerySchema_ViaThreadParameterPresentAndDecodes pins the schema
// lane: the LLM-facing parameter must exist (tool schema enum/params are
// load-bearing) and decode into the typed param the engine consumes.
func TestTraceQuerySchema_ViaThreadParameterPresentAndDecodes(t *testing.T) {
	schema := string((&TraceQuery{}).Parameters())
	if !strings.Contains(schema, `"via_thread"`) {
		t.Fatalf("trace_query schema must expose the via_thread parameter (RN-14a)")
	}
	if !strings.Contains(schema, "scheduling contention") {
		t.Fatalf("via_thread schema description must teach the contention-vs-dependency verdict, got schema without it")
	}
	var p traceQueryParams
	if err := json.Unmarshal([]byte(`{"view":"wakeup_chain","pid":6565,"via_thread":"49706"}`), &p); err != nil {
		t.Fatalf("via_thread must decode: %v", err)
	}
	if p.ViaThread != "49706" {
		t.Fatalf("decoded ViaThread = %q, want 49706", p.ViaThread)
	}
}

// TestTraceQuerySummary_ViaThreadVerdictDedicatedStanza pins that the via
// verdict rides the human-facing result summary as its own line, so the
// on-chain-root-cause vs scheduling-contention ruling survives paraphrase.
func TestTraceQuerySummary_ViaThreadVerdictDedicatedStanza(t *testing.T) {
	verdict := "via_thread OS_FFRT_2_3-49706 NOT on any wakeup path to main-6565 in this window; its influence is scheduling contention only (runnable queuing), not a wakeup dependency"
	result := tracequery.Result{
		View: "wakeup_chain",
		WakeupChain: &tracequery.ChainResult{
			Target: tracequery.ThreadRef{Comm: "main", PID: 6565},
			ViaThread: &tracequery.ChainViaThreadReport{
				Requested: "49706",
				OnChain:   false,
				Summary:   verdict,
			},
		},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "wakeup_chain"}, "path", "/tmp/payload.json")
	if !strings.Contains(summary, "- "+verdict) {
		t.Fatalf("via verdict must be a dedicated summary stanza, got:\n%s", summary)
	}
}

func rn14bChainResult(targetPID int, impact tracequery.WakeupCausalImpact) tracequery.Result {
	impact.ChainDepth = 0
	impact.Thread = tracequery.ThreadRef{Comm: "OS_FFRT_2_3", PID: targetPID}
	return tracequery.Result{
		View: "wakeup_chain",
		WakeupChain: &tracequery.ChainResult{
			Target:        tracequery.ThreadRef{Comm: "OS_FFRT_2_3", PID: targetPID},
			CausalImpacts: []tracequery.WakeupCausalImpact{impact},
		},
	}
}

func rn14bFocusCtx(targets ...types.RuntimeTarget) *types.BusContext {
	return &types.BusContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{RuntimeTargets: targets},
		},
	}
}

// TestTraceQueryRunnableAnchorRecoveryHint_CustomerShapeEmitsViaThreadNextStep
// pins the positive state: FFRT anchor 49706 runnable-dominant (2528.721ms,
// the customer's full-window runnable observation), user focus pinned on pid
// 6565 → one soft next-step line pointing wakeup_chain at the focus thread
// with via_thread on the anchor.
func TestTraceQueryRunnableAnchorRecoveryHint_CustomerShapeEmitsViaThreadNextStep(t *testing.T) {
	result := rn14bChainResult(49706, tracequery.WakeupCausalImpact{
		DominantState: "runnable",
		RunnableMs:    2528.721,
		RunningMs:     300,
	})
	ctx := rn14bFocusCtx(types.RuntimeTarget{Kind: types.RuntimeTargetKindProcess, PID: 6565, Source: "user_explicit", Confidence: 1})
	hint := traceQueryRunnableAnchorRecoveryHint(ctx, result)
	want := "next: view=wakeup_chain pid=6565 via_thread=49706 — connect the runnable anchor to the user-focus thread's wakeup chain"
	if hint != want {
		t.Fatalf("recovery hint = %q, want %q", hint, want)
	}
}

// TestTraceQueryRunnableAnchorRecoveryHint_SilentStates pins every
// suppression lane: no pinned focus, focus == target, non-runnable-dominant
// target, ambiguous multi-target pins, and non-chain views all stay silent —
// the hint is soft guidance and a guessed focus would be worse than none.
func TestTraceQueryRunnableAnchorRecoveryHint_SilentStates(t *testing.T) {
	runnable := tracequery.WakeupCausalImpact{DominantState: "runnable", RunnableMs: 2528.721, RunningMs: 300}
	focus := types.RuntimeTarget{Kind: types.RuntimeTargetKindProcess, PID: 6565, Source: "user_explicit", Confidence: 1}

	if hint := traceQueryRunnableAnchorRecoveryHint(rn14bFocusCtx(), rn14bChainResult(49706, runnable)); hint != "" {
		t.Fatalf("no pinned focus entity must be silent, got %q", hint)
	}
	if hint := traceQueryRunnableAnchorRecoveryHint(rn14bFocusCtx(focus), rn14bChainResult(6565, runnable)); hint != "" {
		t.Fatalf("focus == target must be silent, got %q", hint)
	}
	sleepDominant := tracequery.WakeupCausalImpact{DominantState: "s_sleep", SleepMs: 2000, RunnableMs: 100}
	if hint := traceQueryRunnableAnchorRecoveryHint(rn14bFocusCtx(focus), rn14bChainResult(49706, sleepDominant)); hint != "" {
		t.Fatalf("sleep-dominant target must be silent (typed dominant check), got %q", hint)
	}
	ambiguous := rn14bFocusCtx(
		types.RuntimeTarget{Kind: types.RuntimeTargetKindProcess, PID: 6565, Source: "analyzer", Confidence: 1},
		types.RuntimeTarget{Kind: types.RuntimeTargetKindProcess, PID: 7777, Source: "analyzer", Confidence: 1},
	)
	if hint := traceQueryRunnableAnchorRecoveryHint(ambiguous, rn14bChainResult(49706, runnable)); hint != "" {
		t.Fatalf("ambiguous multi-pid pins without user_explicit must be silent, got %q", hint)
	}
	nonChain := rn14bChainResult(49706, runnable)
	nonChain.View = "window_stats"
	if hint := traceQueryRunnableAnchorRecoveryHint(rn14bFocusCtx(focus), nonChain); hint != "" {
		t.Fatalf("non-chain views must be silent, got %q", hint)
	}
	// The model's own exploration cursor (explicit tool-call target lane)
	// must never masquerade as the user focus.
	cursorOnly := rn14bFocusCtx(types.RuntimeTarget{Kind: types.RuntimeTargetKindProcess, PID: 6565, Source: traceQueryExplicitToolCallTargetSource, Confidence: 1})
	if hint := traceQueryRunnableAnchorRecoveryHint(cursorOnly, rn14bChainResult(49706, runnable)); hint != "" {
		t.Fatalf("explicit tool-call cursor targets must not count as user focus, got %q", hint)
	}
}
