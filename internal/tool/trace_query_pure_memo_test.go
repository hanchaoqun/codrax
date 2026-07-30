package tool

// trace_query_pure_memo_test.go — EVALFIX-2B 类2 pins (2026-07-30): the
// run-scoped pure-tool memo on trace_query's DET-1-pinned pure core.
//
// MUTATION self-check (in place of a strict red-first sequence for pin 1):
// the kill-switch pin below (TestTraceQueryRunMemo_ExclusionLanes, knob
// arm) IS the pre-change behavior — with the memo disabled the second
// identical call re-executes and carries ReusedFromRunMemo=false and no
// disclosure line, which is exactly what pin 1 rejects on the enabled
// path. Note the refs themselves cannot discriminate memo-vs-recompute
// here: blob names are content-addressed (StoreBlob/StoreBlobArtifact
// hash the payload), so a fresh identical recompute happens to mint the
// SAME ref paths today. The memo makes that byte-equality structural
// (verbatim copy) instead of coincidental, and the typed
// ReusedFromRunMemo + disclosure line are the precise discriminators.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

const pureMemoFixtureTrace = `# tracer: nop
    app-100   (  100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
 worker-200   (  200) [002] .... 5.000400: tracing_mark_write: B|200|VerifyClass com.example.Foo
 worker-200   (  200) [002] .... 5.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
 worker-200   (  200) [002] .... 5.005000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
 worker-200   (  200) [002] .... 5.005400: tracing_mark_write: E|200
 worker-200   (  200) [002] .... 5.005600: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
    app-100   (  100) [001] .... 5.005800: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
    app-100   (  100) [001] .... 5.007000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=R ==> next_comm=idle/1 next_pid=0 next_prio=120
`

func pureMemoFixtureCtx(t *testing.T) (*types.BusContext, string) {
	t.Helper()
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "pure-memo.systrace")
	if err := os.WriteFile(tracePath, []byte(pureMemoFixtureTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	return &types.BusContext{
		RepoRoot: dir,
		WorkDir:  dir,
		Mutable:  types.NewMutableState("pure memo pin"),
	}, tracePath
}

func pureMemoFixtureParams(t *testing.T, overrides map[string]any) json.RawMessage {
	t.Helper()
	base := map[string]any{
		"source": "path", "path": "pure-memo.systrace",
		"view": "root_cause_rank", "pid": 100, "thread": "app-100",
		"time_start": 5.000, "time_end": 5.007,
	}
	for k, v := range overrides {
		base[k] = v
	}
	raw, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// Pin 1 (spec 类2 §7.1): an identical second call is served from the run
// memo — typed ReusedFromRunMemo, verbatim refs/Observations, and the
// Summary-tail disclosure line.
func TestTraceQueryRunMemo_IdenticalCallReusesVerbatimRefs(t *testing.T) {
	ctx, _ := pureMemoFixtureCtx(t)
	params := pureMemoFixtureParams(t, nil)

	res1, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !res1.Success {
		t.Fatalf("first call: err=%v success=%v summary=%s", err, res1.Success, res1.Summary)
	}
	if res1.ReusedFromRunMemo {
		t.Fatal("first call must be a real execution, not a memo hit")
	}
	res2, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !res2.Success {
		t.Fatalf("second call: err=%v success=%v summary=%s", err, res2.Success, res2.Summary)
	}
	if !res2.ReusedFromRunMemo {
		t.Fatal("identical second call must be served from the run memo (ReusedFromRunMemo=true)")
	}
	if res2.RawRef != res1.RawRef {
		t.Fatalf("memo hit must carry the verbatim RawRef: first=%q second=%q", res1.RawRef, res2.RawRef)
	}
	if len(res1.Observations) == 0 {
		t.Fatal("fixture published no typed observations — pin would be vacuous")
	}
	if !reflect.DeepEqual(res1.Observations, res2.Observations) {
		t.Fatalf("memo hit must carry the Observations verbatim (same SourceRefs, same ObservedAt):\nfirst:  %+v\nsecond: %+v", res1.Observations, res2.Observations)
	}
	note := PureToolMemoReuseDisclosure("trace_query")
	if !strings.Contains(res2.Summary, note) {
		t.Fatalf("memo hit must append the disclosure line %q, got:\n%s", note, res2.Summary)
	}
	if strings.Contains(res1.Summary, note) {
		t.Fatal("first (real) execution must not carry the reuse disclosure")
	}
	if !strings.HasPrefix(res2.Summary, res1.Summary) {
		t.Fatalf("memo hit Summary must be the first Summary + disclosure tail:\nfirst:\n%s\nsecond:\n%s", res1.Summary, res2.Summary)
	}
}

// Pin 2 (spec 类2 §7.2): any param variation or a different artifact is a
// new key — no memo hit, real execution.
func TestTraceQueryRunMemo_ParamVariationExecutesFresh(t *testing.T) {
	ctx, _ := pureMemoFixtureCtx(t)
	if res, err := (&TraceQuery{}).Execute(ctx, pureMemoFixtureParams(t, nil)); err != nil || !res.Success {
		t.Fatalf("seed call: err=%v success=%v", err, res.Success)
	}

	varied, err := (&TraceQuery{}).Execute(ctx, pureMemoFixtureParams(t, map[string]any{"time_end": 5.006}))
	if err != nil || !varied.Success {
		t.Fatalf("varied call: err=%v success=%v summary=%s", err, varied.Success, varied.Summary)
	}
	if varied.ReusedFromRunMemo {
		t.Fatal("a different time_end must not hit the memo")
	}
	if strings.Contains(varied.Summary, PureToolMemoReuseDisclosure("trace_query")) {
		t.Fatal("fresh execution must not carry the reuse disclosure")
	}

	// Different artifact, same params: append one line so the stat
	// fingerprint (size) and content change.
	otherDir := ctx.RepoRoot
	otherPath := filepath.Join(otherDir, "pure-memo-2.systrace")
	if err := os.WriteFile(otherPath, []byte(pureMemoFixtureTrace+
		"    app-100   (  100) [001] .... 5.008000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	other, err := (&TraceQuery{}).Execute(ctx, pureMemoFixtureParams(t, map[string]any{"path": "pure-memo-2.systrace"}))
	if err != nil || !other.Success {
		t.Fatalf("other-artifact call: err=%v success=%v summary=%s", err, other.Success, other.Summary)
	}
	if other.ReusedFromRunMemo {
		t.Fatal("a different artifact must not hit the memo")
	}
}

// Pin 3 (spec 类2 §7.3): a memo hit is byte-equivalent to a fresh run for
// the run-scoped side-effect registries — the SUPP-CORE call-window
// registry keeps one entry per call ("duplicates kept, call order is the
// deterministic tiebreak"), hit or miss.
func TestTraceQueryRunMemo_SideEffectRegistriesIdentical(t *testing.T) {
	ctx, _ := pureMemoFixtureCtx(t)
	params := pureMemoFixtureParams(t, nil)
	for i := 0; i < 2; i++ {
		if res, err := (&TraceQuery{}).Execute(ctx, params); err != nil || !res.Success {
			t.Fatalf("call %d: err=%v success=%v", i+1, err, res.Success)
		}
	}
	windows := ctx.Mutable.TraceQueryCallWindows()
	if len(windows) != 2 {
		t.Fatalf("call-window registry must record BOTH calls (memo hit included), got %d entries: %+v", len(windows), windows)
	}
}

// Pin 4 (spec 类2 §7.4): every exclusion lane executes directly — no memo
// participation. Each lane is a precise typed signal.
func TestTraceQueryRunMemo_ExclusionLanes(t *testing.T) {
	t.Run("kill_switch", func(t *testing.T) {
		prev := PureToolMemoEnabled()
		defer SetPureToolMemoEnabled(prev)
		SetPureToolMemoEnabled(false)

		ctx, _ := pureMemoFixtureCtx(t)
		params := pureMemoFixtureParams(t, nil)
		if res, err := (&TraceQuery{}).Execute(ctx, params); err != nil || !res.Success {
			t.Fatalf("seed: err=%v success=%v", err, res.Success)
		}
		res2, err := (&TraceQuery{}).Execute(ctx, params)
		if err != nil || !res2.Success {
			t.Fatalf("second: err=%v success=%v", err, res2.Success)
		}
		// This is the pre-change behavior verbatim (the pin-1 MUTATION
		// self-check): recompute, no typed reuse marker, no disclosure.
		if res2.ReusedFromRunMemo {
			t.Fatal("kill switch off must restore always-recompute behavior")
		}
		if strings.Contains(res2.Summary, PureToolMemoReuseDisclosure("trace_query")) {
			t.Fatal("kill switch off must not append the reuse disclosure")
		}
	})

	t.Run("supplement_in_progress", func(t *testing.T) {
		ctx, _ := pureMemoFixtureCtx(t)
		ctx.Mutable.BeginSystemTraceSupplementExecution()
		defer ctx.Mutable.EndSystemTraceSupplementExecution()
		params := pureMemoFixtureParams(t, nil)
		if res, err := (&TraceQuery{}).Execute(ctx, params); err != nil || !res.Success {
			t.Fatalf("seed: err=%v success=%v", err, res.Success)
		}
		res2, err := (&TraceQuery{}).Execute(ctx, params)
		if err != nil || !res2.Success {
			t.Fatalf("second: err=%v success=%v", err, res2.Success)
		}
		if res2.ReusedFromRunMemo {
			t.Fatal("the system supplement lane must stay byte-independent — no memo participation")
		}
	})

	t.Run("dead_run_context", func(t *testing.T) {
		// SUPP-CANCEL contract: a warm repeat under a canceled context
		// must flow through the existing cancellation lanes (typed
		// partial, no faces) — the memo must not resurrect a success.
		// The full-path pin is TestTraceQueryCancelModelLaneWarmIndex-
		// TypedPartial; this arm pins the key-side refusal directly.
		ctx, tracePath := pureMemoFixtureCtx(t)
		dead, cancel := context.WithCancel(context.Background())
		cancel()
		ctx.Ctx = dead
		var p traceQueryParams
		if _, ok := traceQueryMemoKey(ctx, p, tracePath, "path", ""); ok {
			t.Fatal("a dead run context must refuse memo participation")
		}
	})

	t.Run("stat_failure_and_nil_mutable", func(t *testing.T) {
		ctx, tracePath := pureMemoFixtureCtx(t)
		var p traceQueryParams
		if _, ok := traceQueryMemoKey(ctx, p, filepath.Join(ctx.RepoRoot, "no-such-file.systrace"), "path", ""); ok {
			t.Fatal("stat failure must refuse to mint a purity claim")
		}
		bare := &types.BusContext{RepoRoot: ctx.RepoRoot, WorkDir: ctx.WorkDir}
		if _, ok := traceQueryMemoKey(bare, p, tracePath, "path", ""); ok {
			t.Fatal("a context without MutableState has no memo — must refuse")
		}
		if _, ok := traceQueryMemoKey(ctx, p, tracePath, "path", ""); !ok {
			t.Fatal("healthy lane must mint a key (otherwise the arms above are vacuous)")
		}
	})
}

// Pin 5 (spec 类2 §7.5): the memo dies at the per-task boundary — after
// ResetTurnAArtifacts the same call executes for real.
func TestTraceQueryRunMemo_ClearedAtTurnBoundary(t *testing.T) {
	ctx, _ := pureMemoFixtureCtx(t)
	params := pureMemoFixtureParams(t, nil)
	if res, err := (&TraceQuery{}).Execute(ctx, params); err != nil || !res.Success {
		t.Fatalf("seed: err=%v success=%v", err, res.Success)
	}
	ctx.Mutable.ResetTurnAArtifacts()
	res2, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !res2.Success {
		t.Fatalf("post-reset: err=%v success=%v", err, res2.Success)
	}
	if res2.ReusedFromRunMemo {
		t.Fatal("ResetTurnAArtifacts must clear the memo — cross-task reuse is out of the purity boundary")
	}
}

// Pin 7 (R6-0, round-6 sweep, eval/sweep_round6_findings_20260730.md):
// target provenance is a memo-key input. An explicit-target call and a
// target-INHERITING call converge to the same post-inheritance params,
// but the memoized pure core bakes the inheritance caveat
// (trace_query_target_inherited=…) into the published result — serving
// the inherited call from the explicit call's memo entry would publish
// the wrong target-provenance face. The two must never collide onto one
// key. Red-first: pre-fix the second call returned ReusedFromRunMemo=
// true.
func TestTraceQueryRunMemo_TargetProvenanceSplitsKeys(t *testing.T) {
	ctx, _ := pureMemoFixtureCtx(t)
	ctx.Mutable.SetRequestModel(types.RequestModel{RuntimeTargets: []types.RuntimeTarget{{
		PID: 100, Thread: "app-100", Source: "analyzer", Confidence: 1,
	}}})

	explicit, err := (&TraceQuery{}).Execute(ctx, pureMemoFixtureParams(t, nil))
	if err != nil || !explicit.Success {
		t.Fatalf("explicit-target call: err=%v success=%v summary=%s", err, explicit.Success, explicit.Summary)
	}
	if explicit.ReusedFromRunMemo {
		t.Fatal("explicit-target seed call must be a real execution")
	}

	// Same call with pid/thread OMITTED: the request-model target is
	// inherited, so the post-inheritance params are identical — but the
	// call's published caveat face differs (inheritance disclosure).
	inheritedParams, err := json.Marshal(map[string]any{
		"source": "path", "path": "pure-memo.systrace",
		"view":       "root_cause_rank",
		"time_start": 5.000, "time_end": 5.007,
	})
	if err != nil {
		t.Fatal(err)
	}
	inherited, execErr := (&TraceQuery{}).Execute(ctx, json.RawMessage(inheritedParams))
	if execErr != nil || !inherited.Success {
		t.Fatalf("inherited-target call: err=%v success=%v summary=%s", execErr, inherited.Success, inherited.Summary)
	}
	if inherited.ReusedFromRunMemo {
		t.Fatal("a target-inheriting call must NOT be served from the explicit-target memo entry — its published target-provenance caveat differs")
	}

	// Key-level: the caveat is a key input — identical params with
	// different call caveats mint different keys.
	tracePath := filepath.Join(ctx.RepoRoot, "pure-memo.systrace")
	var p traceQueryParams
	k1, ok1 := traceQueryMemoKey(ctx, p, tracePath, "path", "")
	k2, ok2 := traceQueryMemoKey(ctx, p, tracePath, "path", "trace_query_target_inherited=true; pid=100")
	if !ok1 || !ok2 {
		t.Fatalf("healthy lane must mint keys (ok1=%v ok2=%v)", ok1, ok2)
	}
	if k1 == k2 {
		t.Fatal("different call caveats must never share one memo key")
	}
}

// Pin 6 (spec 类2 §7.6, 实例2 判决): with verbatim refs, the EXISTING
// observation-ledger dedupe collapses the duplicate query's records to a
// single set — zero new dedup code. Adding the memo-served second result
// to the compile input must not grow the ledger by even one record.
func TestObservationLedger_DuplicateQueryCollapsesToOneRecord(t *testing.T) {
	ctx, _ := pureMemoFixtureCtx(t)
	params := pureMemoFixtureParams(t, nil)
	res1, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !res1.Success {
		t.Fatalf("first: err=%v success=%v", err, res1.Success)
	}
	res2, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !res2.Success || !res2.ReusedFromRunMemo {
		t.Fatalf("second: err=%v success=%v reused=%v", err, res2.Success, res2.ReusedFromRunMemo)
	}

	baseline := types.CompileObservationLedger(types.ObservationLedgerInput{
		ToolResults: []types.ToolResult{res1},
	})
	if len(baseline.Records) == 0 {
		t.Fatal("baseline ledger empty — pin would be vacuous")
	}
	doubled := types.CompileObservationLedger(types.ObservationLedgerInput{
		ToolResults: []types.ToolResult{res1, res2},
	})
	if len(doubled.Records) != len(baseline.Records) {
		t.Fatalf("duplicate memo-served query must collapse in the existing ledger dedupe: baseline=%d doubled=%d",
			len(baseline.Records), len(doubled.Records))
	}
}
