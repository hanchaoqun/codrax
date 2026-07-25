package tool

// C8PROSE-1 收编 (2026-07-20): the SUPP disclosure caveat sentences adopted the
// C8 prose punctuation regime (§29.164/§29.170) — the pinned zh joints below
// evolved half→full width deliberately (EVOLUTION RECORD).

// trace_query_supplement_test.go — SUPP-CORE (DISPATCH-IND 批1, 2026-07-14)
// pins. FULL-CHAIN engine-minted fixtures (§28.7 复核纪律②: fixture 取引擎实
// 铸形): trace text → (&TraceQuery{}).Execute (the SAME runner model calls
// use) → bus/system lanes → ObservationLedgerInputFromBusContext →
// CompileObservationLedger → CompileTraceCausalProjectionSet → tree fence.
//
// Pin family (design §3.1 批1 件6):
//   ① fallback-form red→green — the h2-20260713-223753 dispatch shape
//     (event_search + window_stats + thread_timeline + critical, NO
//     rank/chain) plus the supplement mints the anchored tree + self
//     segment + rank seats + wait object.
//   ② no-op — all core families already present ⇒ zero engine execution and
//     a byte-identical tree fence.
//   ③ fail-open 禁猜 — missing typed target / inconsistent windows / no
//     windows ⇒ zero execution, no meta, no disclosure.
//   ④ determinism — two independent supplement runs on equal input render
//     byte-identical tree fences.
//   ⑦ explore zero displacement — supplement results never appear on
//     bus.ToolResults / the dispatch buffer / the explore-stage prompt feed;
//     the finalize feed sees them (R1 ruling a).
//   Plus: boolean family detector, typed target/window derivation, system-
//   lane provenance stamping (SystemSupplement=true), single-line disclosure
//   upsert, per-task latch, kill switch, cold budget.
//
// MUTATION self-checks:
//   - dropping the supplement execution (or the ledger-input merge) reds
//     TestTraceSupplementH2FlatShapeRedToGreen;
//   - relaxing fail-open (guessing a window/target) reds the FailOpen pins;
//   - stamping records without the structural lane boundary reds
//     TestTraceSupplementProvenanceStamp (model records must stay false);
//   - leaking results into bus.ToolResults / the explore feed reds
//     TestTraceSupplementExploreZeroDisplacement.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// suppCoreTrace: worker-200 (user focus) blocks in D twice (dma_fence-shaped
// sched_blocked_reason callers) and sleeps twice; peer-300 wakes it each
// time. Window 3.0..3.2 covers everything, so one root_cause_rank(pid,window)
// run mints the rank family + wakeup chain (+ edge census) + target window
// states (+ blocked_reason census); critical_blocking_calls mints the
// critical family.
const suppCoreTrace = `        app-100 (100) [001] .... 3.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 3.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [002] .... 3.010000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 3.010500: sched_blocked_reason: pid=200 iowait=0 caller=dma_fence_default_wait+0x1a8/0x2b8
       peer-300 (300) [003] .... 3.030000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=003
     worker-200 (200) [003] .... 3.031000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [003] .... 3.040000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/3 next_pid=0 next_prio=120
     worker-200 (200) [003] .... 3.040500: sched_blocked_reason: pid=200 iowait=0 caller=dma_fence_default_wait+0x1a8/0x2b8
       peer-300 (300) [003] .... 3.070000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=003
     worker-200 (200) [003] .... 3.071000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [003] .... 3.090000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.150000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=003
     worker-200 (200) [003] .... 3.151000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [003] .... 3.160000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
        app-100 (100) [001] .... 3.200000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

// suppCoreContext builds an attached-trace bus with the worker-200 user
// target on the typed RuntimeTargets lane.
func suppCoreContext(t *testing.T) *types.BusContext {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, types.AttachedTraceBlobBasename), []byte(suppCoreTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("为什么 worker 线程卡顿")}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeTargets: []types.RuntimeTarget{{
			Kind: types.RuntimeTargetKindThread, PID: 200, Thread: "worker",
			Source: "user_explicit", Confidence: 1,
		}},
	}}
	return ctx
}

// suppCoreModelCall executes one trace_query through the SAME runner a model
// dispatch uses and appends the result to the bus history (the model lane).
func suppCoreModelCall(t *testing.T, ctx *types.BusContext, params string) types.ToolResult {
	t.Helper()
	res, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(params))
	if err != nil {
		t.Fatalf("trace_query %s failed: %v", params, err)
	}
	if !res.Success {
		t.Fatalf("trace_query %s unsuccessful: %s", params, res.Summary)
	}
	ctx.ToolResults = append(ctx.ToolResults, res)
	return res
}

// suppCoreH2FlatShape reproduces the h2-20260713-223753 dispatch shape:
// event_search + window_stats + thread_timeline + critical_blocking_calls,
// zero root_cause_rank, zero wakeup_chain.
func suppCoreH2FlatShape(t *testing.T, ctx *types.BusContext) {
	t.Helper()
	suppCoreModelCall(t, ctx, `{"view":"event_search","pid":200,"time_start":3.0,"time_end":3.2}`)
	suppCoreModelCall(t, ctx, `{"view":"window_stats","pid":200,"time_start":3.0,"time_end":3.2}`)
	suppCoreModelCall(t, ctx, `{"view":"thread_timeline","pid":200,"time_start":3.0,"time_end":3.2}`)
	suppCoreModelCall(t, ctx, `{"view":"critical_blocking_calls","pid":200,"time_start":3.0,"time_end":3.2}`)
}

func suppCoreLedger(ctx *types.BusContext) types.ObservationLedger {
	return types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
}

func suppCoreTreeFence(t *testing.T, ctx *types.BusContext) string {
	t.Helper()
	set := types.CompileTraceCausalProjectionSet(suppCoreLedger(ctx))
	if len(set.Projections) == 0 {
		return ""
	}
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	model := buildRuntimeTraceProjTreeModel(set.Projections[0], evidence, true)
	return runtimeTraceProjTreeFence(model, true)
}

// --- pin ① + detector + provenance + disclosure -----------------------------

func TestTraceSupplementH2FlatShapeRedToGreen(t *testing.T) {
	ctx := suppCoreContext(t)
	suppCoreH2FlatShape(t, ctx)

	// 布尔探测判别 pin: the flat shape's engine-minted ledger has exactly the
	// window-stats/critical-side families, no rank/chain/edge-census.
	families := traceSupplementFamilies(suppCoreLedger(ctx))
	want := traceSupplementFamilyPresence{
		Rank: false, Chain: false, WindowStates: true,
		Critical: true, BlockedReasonCensus: true, WakeupEdgeCensus: false,
	}
	if families != want {
		t.Fatalf("flat-shape family detection = %+v, want %+v", families, want)
	}

	// RED: the h2 fallback tree — no anchor, no self segment, no seats.
	before := suppCoreTreeFence(t, ctx)
	for _, degraded := range []string{"⊘ 唤醒链未下钻", "窗口起止未采集", "回退尺度"} {
		if !strings.Contains(before, degraded) {
			t.Fatalf("flat-shape fence must carry the degraded form %q:\n%s", degraded, before)
		}
	}
	for _, anchored := range []string{"自身·", "‹用户关注线程›", "➊"} {
		if strings.Contains(before, anchored) {
			t.Fatalf("flat-shape fence must NOT already carry %q:\n%s", anchored, before)
		}
	}

	out := RunTraceQuerySystemSupplement(ctx)
	if !out.Attempted || out.SkipReason != "" {
		t.Fatalf("supplement must execute on the flat shape: %+v", out)
	}
	// Minimality: critical is already present, so exactly ONE engine view.
	if len(out.Executed) != 1 || out.Executed[0] != "root_cause_rank" {
		t.Fatalf("flat shape needs exactly [root_cause_rank], got %v", out.Executed)
	}

	// GREEN: anchored tree + self segment + rank seat + wait object.
	after := suppCoreTreeFence(t, ctx)
	for _, anchored := range []string{
		"⊚ worker-200 ‹用户关注线程›", "满格=窗口", "➊", "自身·D-state",
		"等待对象 dma_fence_default_wait",
	} {
		if !strings.Contains(after, anchored) {
			t.Fatalf("supplemented fence must carry %q:\n%s", anchored, after)
		}
	}
	for _, degraded := range []string{"⊘ 唤醒链未下钻", "窗口起止未采集", "回退尺度"} {
		if strings.Contains(after, degraded) {
			t.Fatalf("supplemented fence must drop the degraded form %q:\n%s", degraded, after)
		}
	}

	// typed 参数推导 pin: the meta carries the user-source target and the
	// unanimous model-call window.
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil {
		t.Fatal("executed supplement must store typed meta")
	}
	if meta.TargetPID != 200 || meta.TargetThread != "worker" || meta.TargetSource != "user" {
		t.Fatalf("meta target = %+v, want worker-200 from the user lane", meta)
	}
	if meta.WindowStart != 3.0 || meta.WindowEnd != 3.2 {
		t.Fatalf("meta window = %.6f..%.6f, want 3.0..3.2", meta.WindowStart, meta.WindowEnd)
	}
}

func TestTraceSupplementProvenanceStamp(t *testing.T) {
	ctx := suppCoreContext(t)
	suppCoreH2FlatShape(t, ctx)
	if out := RunTraceQuerySystemSupplement(ctx); len(out.Executed) == 0 {
		t.Fatalf("supplement must execute: %+v", out)
	}
	ledger := suppCoreLedger(ctx)
	supplementStamped := 0
	for _, record := range ledger.Records {
		fromRank := strings.HasPrefix(strings.TrimSpace(record.ClaimKey), "root_cause_") ||
			record.Predicate == "wakeup_chain" || record.Predicate == "wakeup_chain_edge" ||
			record.Predicate == "wakeup_edge_census"
		if fromRank {
			// These families exist ONLY via the supplement on this shape.
			if !record.SystemSupplement {
				t.Fatalf("supplement-minted record must carry SystemSupplement=true: %s %s", record.ID, record.Predicate)
			}
			supplementStamped++
		}
		if record.Predicate == "critical_blocking" && record.SystemSupplement {
			t.Fatalf("model-minted record must NOT carry the supplement stamp: %s", record.ID)
		}
	}
	if supplementStamped == 0 {
		t.Fatal("expected supplement-minted rank/chain records on the ledger")
	}
	// The supplement lane is dedicated: results are NOT on the bus history.
	if got := len(ctx.Mutable.SystemTraceSupplementResults()); got == 0 {
		t.Fatal("supplement results must ride the dedicated MutableState lane")
	}
}

func TestTraceSupplementDisclosureSingleLineUpsert(t *testing.T) {
	ctx := suppCoreContext(t)
	suppCoreH2FlatShape(t, ctx)

	// 触发前:零披露(fail-open/未执行时答案面逐字节同形)。
	doc := &types.AnswerDocumentV2{Caveats: []string{"模型自述 caveat 原样保留"}}
	if materializeRuntimeTraceSupplementDisclosureCaveat(doc, ctx) {
		t.Fatal("disclosure must not fire before the supplement executed")
	}
	if len(doc.Caveats) != 1 {
		t.Fatalf("caveat lane must be untouched before execution: %q", doc.Caveats)
	}

	if out := RunTraceQuerySystemSupplement(ctx); len(out.Executed) == 0 {
		t.Fatalf("supplement must execute: %+v", out)
	}
	if !materializeRuntimeTraceSupplementDisclosureCaveat(doc, ctx) {
		t.Fatal("disclosure must fire after execution")
	}
	// Idempotent upsert: a second mutation pass keeps exactly one line.
	if !materializeRuntimeTraceSupplementDisclosureCaveat(doc, ctx) {
		t.Fatal("disclosure upsert must stay active on later passes")
	}
	var lines []string
	for _, caveat := range doc.Caveats {
		if strings.HasPrefix(caveat, runtimeTraceSupplementDisclosurePrefixZH) {
			lines = append(lines, caveat)
		}
	}
	if len(lines) != 1 {
		t.Fatalf("exactly one disclosure line, got %d: %q", len(lines), doc.Caveats)
	}
	// EVOLUTION RECORD (WF-2 词面批 §29.71 残留3, 2026-07-14): 「装配期」→
	// 「成文前」(零内部管线词) + zh 视图名（token） per the D4 label（token）
	// precedent; EN "assembly-time" → "pre-report", tokens kept raw.
	wantZH := "系统补采: 成文前确定性补跑 根因排序（root_cause_rank）·值观测51条(窗 3.000000..3.200000, 目标 worker-200)"
	if lines[0] != wantZH {
		t.Fatalf("zh disclosure = %q, want %q", lines[0], wantZH)
	}
	if doc.Caveats[0] != "模型自述 caveat 原样保留" {
		t.Fatalf("model-authored caveats must survive the upsert: %q", doc.Caveats)
	}

	// EN wording form.
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	en := runtimeTraceSupplementDisclosureText(meta, false)
	wantEN := "System supplement: deterministic pre-report re-run of root_cause_rank [value observations: 51] (window 3.000000..3.200000, target worker-200)"
	if en != wantEN {
		t.Fatalf("en disclosure = %q, want %q", en, wantEN)
	}

	// h4-20260714-014124 witness: a thread label that already carries the
	// "-<tid>" tail is never doubled.
	tail := runtimeTraceSupplementDisclosureText(&types.SystemTraceSupplementMeta{
		Views: []string{"critical_blocking_calls"}, WindowStart: 1, WindowEnd: 2,
		TargetPID: 17267, TargetThread: ".ugc.aweme.lite-17267",
	}, true)
	if !strings.Contains(tail, "目标 .ugc.aweme.lite-17267)") || strings.Contains(tail, "17267-17267") {
		t.Fatalf("tid-tailed labels must not double the tid: %q", tail)
	}
}

// --- pin ② no-op -------------------------------------------------------------

func TestTraceSupplementNoOpWhenFamiliesPresent(t *testing.T) {
	ctx := suppCoreContext(t)
	// The healthy PASS shape: the model itself dispatched rank + critical.
	suppCoreModelCall(t, ctx, `{"view":"root_cause_rank","pid":200,"time_start":3.0,"time_end":3.2}`)
	suppCoreModelCall(t, ctx, `{"view":"critical_blocking_calls","pid":200,"time_start":3.0,"time_end":3.2}`)

	before := suppCoreTreeFence(t, ctx)
	busLen := len(ctx.ToolResults)
	out := RunTraceQuerySystemSupplement(ctx)
	if !out.Attempted {
		t.Fatal("first call must consume the latch")
	}
	if len(out.Executed) != 0 || out.SkipReason != "families_present" {
		t.Fatalf("family-present run must be a zero-execution no-op, got %+v", out)
	}
	if got := ctx.Mutable.SystemTraceSupplementResults(); len(got) != 0 {
		t.Fatalf("no-op must store nothing on the supplement lane, got %d results", len(got))
	}
	if ctx.Mutable.SystemTraceSupplementMeta() != nil {
		t.Fatal("no-op must not mint disclosure meta")
	}
	if len(ctx.ToolResults) != busLen {
		t.Fatal("no-op must leave the bus history untouched")
	}
	after := suppCoreTreeFence(t, ctx)
	if before != after {
		t.Fatalf("no-op must keep the tree fence byte-identical:\n--- before ---\n%s\n--- after ---\n%s", before, after)
	}
	doc := &types.AnswerDocumentV2{}
	if materializeRuntimeTraceSupplementDisclosureCaveat(doc, ctx) || len(doc.Caveats) != 0 {
		t.Fatalf("no-op must not disclose: %q", doc.Caveats)
	}
}

// --- pin ③ fail-open 禁猜 -----------------------------------------------------

func suppCoreAssertFailOpen(t *testing.T, ctx *types.BusContext, out TraceQuerySupplementOutcome, wantReason string) {
	t.Helper()
	if !out.Attempted {
		t.Fatalf("first call must consume the latch: %+v", out)
	}
	if len(out.Executed) != 0 || out.SkipReason != wantReason {
		t.Fatalf("fail-open skip reason = %q executed=%v, want skip %q with zero execution", out.SkipReason, out.Executed, wantReason)
	}
	if len(ctx.Mutable.SystemTraceSupplementResults()) != 0 || ctx.Mutable.SystemTraceSupplementMeta() != nil {
		t.Fatal("fail-open must store nothing")
	}
	doc := &types.AnswerDocumentV2{}
	if materializeRuntimeTraceSupplementDisclosureCaveat(doc, ctx) || len(doc.Caveats) != 0 {
		t.Fatalf("fail-open must not disclose: %q", doc.Caveats)
	}
}

func TestTraceSupplementFailOpenNoTypedTarget(t *testing.T) {
	ctx := suppCoreContext(t)
	// No user target and no model cursor: unscoped model calls only.
	ctx.AnalysisIR.RequestModel.RuntimeTargets = nil
	suppCoreModelCall(t, ctx, `{"view":"event_search","time_start":3.0,"time_end":3.2}`)
	suppCoreAssertFailOpen(t, ctx, RunTraceQuerySystemSupplement(ctx), "no_typed_target")
}

func TestTraceSupplementFailOpenAmbiguousUserTargets(t *testing.T) {
	ctx := suppCoreContext(t)
	// TWO distinct user-source targets: user intent is ambiguous — the
	// consistent model cursor must NOT resolve it (user intent red line).
	ctx.AnalysisIR.RequestModel.RuntimeTargets = append(ctx.AnalysisIR.RequestModel.RuntimeTargets,
		types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, PID: 300, Thread: "peer", Source: "user_explicit", Confidence: 1})
	suppCoreModelCall(t, ctx, `{"view":"event_search","pid":200,"time_start":3.0,"time_end":3.2}`)
	suppCoreAssertFailOpen(t, ctx, RunTraceQuerySystemSupplement(ctx), "no_typed_target")
}

func TestTraceSupplementFailOpenInconsistentWindows(t *testing.T) {
	ctx := suppCoreContext(t)
	// Two disjoint non-anchor call windows: never last-wins, never averaged.
	suppCoreModelCall(t, ctx, `{"view":"event_search","pid":200,"time_start":3.0,"time_end":3.05}`)
	suppCoreModelCall(t, ctx, `{"view":"event_search","pid":200,"time_start":3.1,"time_end":3.2}`)
	suppCoreAssertFailOpen(t, ctx, RunTraceQuerySystemSupplement(ctx), "window_inconsistent")
}

func TestTraceSupplementFailOpenNoWindow(t *testing.T) {
	// R3+R4: a zero-trace_query run has no typed window source ⇒ skip (the
	// engine never guesses a default window).
	ctx := suppCoreContext(t)
	suppCoreAssertFailOpen(t, ctx, RunTraceQuerySystemSupplement(ctx), "no_typed_window")
}

// --- pin ④ determinism --------------------------------------------------------

func TestTraceSupplementDeterministicAcrossRuns(t *testing.T) {
	run := func() (string, *types.SystemTraceSupplementMeta) {
		ctx := suppCoreContext(t)
		suppCoreH2FlatShape(t, ctx)
		out := RunTraceQuerySystemSupplement(ctx)
		if len(out.Executed) != 1 {
			t.Fatalf("supplement must execute exactly one view: %+v", out)
		}
		return suppCoreTreeFence(t, ctx), ctx.Mutable.SystemTraceSupplementMeta()
	}
	fence1, meta1 := run()
	fence2, meta2 := run()
	if fence1 != fence2 {
		t.Fatalf("two supplement runs must render byte-identical fences:\n--- run1 ---\n%s\n--- run2 ---\n%s", fence1, fence2)
	}
	d1 := runtimeTraceSupplementDisclosureText(meta1, true)
	d2 := runtimeTraceSupplementDisclosureText(meta2, true)
	if d1 != d2 {
		t.Fatalf("disclosure lines must be byte-identical: %q vs %q", d1, d2)
	}
}

// --- pin ⑦ explore zero displacement ------------------------------------------

func TestTraceSupplementExploreZeroDisplacement(t *testing.T) {
	ctx := suppCoreContext(t)
	suppCoreH2FlatShape(t, ctx)
	busLen := len(ctx.ToolResults)

	exploreBefore := promptctx.BuildAgentContext(ctx, types.AgentExplorer, types.StageExplore)
	if out := RunTraceQuerySystemSupplement(ctx); len(out.Executed) == 0 {
		t.Fatalf("supplement must execute: %+v", out)
	}
	exploreAfter := promptctx.BuildAgentContext(ctx, types.AgentExplorer, types.StageExplore)

	// The explore model face is byte-identical before/after the supplement.
	if exploreBefore.TraceRootCauseBoard != exploreAfter.TraceRootCauseBoard {
		t.Fatalf("explore TraceRootCauseBoard must not see the supplement:\n--- before ---\n%s\n--- after ---\n%s",
			exploreBefore.TraceRootCauseBoard, exploreAfter.TraceRootCauseBoard)
	}
	if exploreBefore.TraceWaitEvidence != exploreAfter.TraceWaitEvidence {
		t.Fatalf("explore TraceWaitEvidence must not see the supplement:\n--- before ---\n%s\n--- after ---\n%s",
			exploreBefore.TraceWaitEvidence, exploreAfter.TraceWaitEvidence)
	}
	// The bus history and the per-dispatch buffer stay untouched — nothing
	// the explore transcript renders can contain the supplement results.
	if len(ctx.ToolResults) != busLen {
		t.Fatalf("bus.ToolResults grew from %d to %d — supplement leaked onto the model lane", busLen, len(ctx.ToolResults))
	}
	if got := ctx.Mutable.DispatchToolResults(); len(got) != 0 {
		t.Fatalf("dispatch buffer must stay empty, got %d results", len(got))
	}

	// R1 ruling (a): the FINALIZE feed does consume the supplement.
	finalize := promptctx.BuildAgentContext(ctx, types.AgentFinalizer, types.StageFinalize)
	if finalize.TraceRootCauseBoard == exploreAfter.TraceRootCauseBoard {
		t.Fatal("finalize TraceRootCauseBoard must differ from the explore face once the supplement minted rank rows")
	}
	if !strings.Contains(finalize.TraceRootCauseBoard, "worker-200") {
		t.Fatalf("finalize board must carry the supplemented rank rows:\n%s", finalize.TraceRootCauseBoard)
	}
}

// --- latch / kill switch / budgets ---------------------------------------------

// suppCoreSetConfig sets the supplement knobs DIRECTLY (bypassing the
// injection setter's keep-on-non-positive semantics) and restores the
// previous values on cleanup, so budget pins cannot leak into later tests.
func suppCoreSetConfig(t *testing.T, enabled bool, maxCold int64, maxDur time.Duration, maxSpanS float64) {
	t.Helper()
	prevEnabled := traceSupplementEnabled
	prevCold := traceSupplementMaxColdBytes
	prevDur := traceSupplementMaxDuration
	prevSpan := traceSupplementMaxWindowSpanS
	traceSupplementEnabled = enabled
	traceSupplementMaxColdBytes = maxCold
	traceSupplementMaxDuration = maxDur
	traceSupplementMaxWindowSpanS = maxSpanS
	t.Cleanup(func() {
		traceSupplementEnabled = prevEnabled
		traceSupplementMaxColdBytes = prevCold
		traceSupplementMaxDuration = prevDur
		traceSupplementMaxWindowSpanS = prevSpan
	})
}

func TestTraceSupplementLatchSingleAttempt(t *testing.T) {
	ctx := suppCoreContext(t)
	suppCoreH2FlatShape(t, ctx)
	first := RunTraceQuerySystemSupplement(ctx)
	if !first.Attempted || len(first.Executed) != 1 {
		t.Fatalf("first attempt must execute: %+v", first)
	}
	results := ctx.Mutable.SystemTraceSupplementResults()
	second := RunTraceQuerySystemSupplement(ctx)
	if second.Attempted {
		t.Fatalf("second call must be latched out: %+v", second)
	}
	if got := ctx.Mutable.SystemTraceSupplementResults(); len(got) != len(results) {
		t.Fatal("latched call must not touch the supplement lane")
	}
}

func TestTraceSupplementKillSwitch(t *testing.T) {
	suppCoreSetConfig(t, false, 2<<30, 20*time.Second, 120)
	ctx := suppCoreContext(t)
	suppCoreH2FlatShape(t, ctx)
	suppCoreAssertFailOpen(t, ctx, RunTraceQuerySystemSupplement(ctx), "disabled")
}

func TestTraceSupplementColdBudget(t *testing.T) {
	suppCoreSetConfig(t, true, 1, 20*time.Second, 120) // 1-byte cold budget
	ctx := suppCoreContext(t)
	// Typed window recorded but ZERO successful trace_query on the bus: the
	// cold lane must respect the budget instead of paying a cold parse.
	ctx.Mutable.RecordTraceQueryCallWindow(types.TraceQueryCallWindow{View: "event_search", TimeStart: 3.0, TimeEnd: 3.2})
	suppCoreAssertFailOpen(t, ctx, RunTraceQuerySystemSupplement(ctx), "cold_budget_exceeded")
}

// --- P1 warm-lane budget fuses (2026-07-14 修复轮) ------------------------------

func TestTraceSupplementDurationBudgetKeepsCompletedViews(t *testing.T) {
	// EVOLUTION RECORD (SUPP-CANCEL, 2026-07-14): this pin originally used a
	// 1ns budget and relied on "view 1 always runs" — the between-view
	// deadline was the ONLY duration fuse. The same knob now also rides a
	// context deadline INTO the view (in-view cooperative cancellation), so
	// a pre-expired budget cancels view 1 instead of letting it run (that
	// lane's pin: TestTraceSupplementInViewCancellation*). The between-view
	// invariant itself is unchanged — completed views are kept, the
	// remainder skips with disclosure — and stays deterministically pinned
	// here through the test-only after-view seam (a wall-clock overrun
	// between the views would otherwise race the in-view deadline).
	suppCoreSetConfig(t, true, 2<<30, 30*time.Second, 120)
	traceSupplementAfterViewHook = func(string) {
		// Overrun the budget AFTER view 1 completes: the view-2 iteration's
		// between-view check must fire before any engine call.
		traceSupplementMaxDuration = -time.Nanosecond
	}
	t.Cleanup(func() { traceSupplementAfterViewHook = nil })
	ctx := suppCoreContext(t)
	// event_search-only shape: every core family missing ⇒ both views planned.
	suppCoreModelCall(t, ctx, `{"view":"event_search","pid":200,"time_start":3.0,"time_end":3.2}`)
	out := RunTraceQuerySystemSupplement(ctx)
	if !out.Attempted || len(out.Executed) != 1 || out.Executed[0] != "root_cause_rank" {
		t.Fatalf("view 1 must complete under the between-view deadline: %+v", out)
	}
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || meta.SkipReason != "duration_budget_exceeded" {
		t.Fatalf("partial run must carry the duration skip reason: %+v", meta)
	}
	if len(meta.SkippedViews) != 1 || meta.SkippedViews[0] != "critical_blocking_calls" {
		t.Fatalf("skipped views must name the remainder: %+v", meta.SkippedViews)
	}
	// Completed view's observations are kept on the ledger.
	families := traceSupplementFamilies(suppCoreLedger(ctx))
	if !families.Rank || !families.Chain {
		t.Fatalf("completed view's observations must be kept: %+v", families)
	}
	if families.Critical {
		t.Fatal("the skipped view must not have executed")
	}
	// Disclosure carries the honest partial tail (ATOMIC wording pin).
	doc := &types.AnswerDocumentV2{}
	if !materializeRuntimeTraceSupplementDisclosureCaveat(doc, ctx) || len(doc.Caveats) != 1 {
		t.Fatalf("partial run must disclose: %q", doc.Caveats)
	}
	wantZH := "系统补采: 成文前确定性补跑 根因排序（root_cause_rank）·值观测51条(窗 3.000000..3.200000, 目标 worker-200)；超时长预算未补跑 关键阻塞调用（critical_blocking_calls）"
	if doc.Caveats[0] != wantZH {
		t.Fatalf("zh partial disclosure = %q, want %q", doc.Caveats[0], wantZH)
	}
	en := runtimeTraceSupplementDisclosureText(meta, false)
	wantEN := "System supplement: deterministic pre-report re-run of root_cause_rank [value observations: 51] (window 3.000000..3.200000, target worker-200); not re-run over the duration budget: critical_blocking_calls"
	if en != wantEN {
		t.Fatalf("en partial disclosure = %q, want %q", en, wantEN)
	}
}

func TestTraceSupplementWindowSpanBudgetDisclosedSkip(t *testing.T) {
	// 0.1s span budget against the fixture's 0.2s window: the whole
	// supplement is skipped BEFORE any engine call, with an honest
	// answer-side disclosure — never a silent truncation, never a guessed
	// sub-window.
	suppCoreSetConfig(t, true, 2<<30, 5*time.Second, 0.1)
	ctx := suppCoreContext(t)
	suppCoreH2FlatShape(t, ctx)
	familiesBefore := traceSupplementFamilies(suppCoreLedger(ctx))
	out := RunTraceQuerySystemSupplement(ctx)
	if !out.Attempted || len(out.Executed) != 0 || out.SkipReason != "window_span_exceeded" {
		t.Fatalf("over-span window must skip whole: %+v", out)
	}
	if got := ctx.Mutable.SystemTraceSupplementResults(); len(got) != 0 {
		t.Fatalf("span skip must store zero results, got %d", len(got))
	}
	if families := traceSupplementFamilies(suppCoreLedger(ctx)); families != familiesBefore {
		t.Fatalf("span skip must leave the ledger unchanged: %+v vs %+v", families, familiesBefore)
	}
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || meta.SkipReason != "window_span_exceeded" || len(meta.Views) != 0 {
		t.Fatalf("span skip must mint disclosure meta with zero executed views: %+v", meta)
	}
	doc := &types.AnswerDocumentV2{}
	if !materializeRuntimeTraceSupplementDisclosureCaveat(doc, ctx) || len(doc.Caveats) != 1 {
		t.Fatalf("span skip must disclose: %q", doc.Caveats)
	}
	wantZH := "系统补采: 未补跑 根因排序（root_cause_rank）——窗 3.000000..3.200000 跨度 0.200 秒超出补跑窗长预算 0.1 秒；缩小时间窗后可补齐该窗结果"
	if doc.Caveats[0] != wantZH {
		t.Fatalf("zh span disclosure = %q, want %q", doc.Caveats[0], wantZH)
	}
	en := runtimeTraceSupplementDisclosureText(meta, false)
	wantEN := "System supplement: root_cause_rank not re-run — window 3.000000..3.200000 spans 0.200s, over the 0.1s span budget; narrow the time window to fill it in"
	if en != wantEN {
		t.Fatalf("en span disclosure = %q, want %q", en, wantEN)
	}
}

// --- typed derivation unit pins -------------------------------------------------

func TestTraceSupplementWindowDerivation(t *testing.T) {
	tol := types.TraceCausalProjectionSameWindowToleranceS
	// Anchor-capable lane (rank/chain family calls) outranks the general lane.
	windows := []types.TraceQueryCallWindow{
		{View: "event_search", TimeStart: 3.0, TimeEnd: 3.2},
		{View: "wakeup_chain", TimeStart: 5.0, TimeEnd: 5.1},
	}
	if w, ok := traceSupplementDeriveWindow(windows); !ok || w.TimeStart != 5.0 || w.TimeEnd != 5.1 {
		t.Fatalf("anchor-capable lane must win: %+v ok=%t", w, ok)
	}
	// Tolerance-equal general-lane windows agree; the FIRST recorded wins.
	windows = []types.TraceQueryCallWindow{
		{View: "event_search", TimeStart: 3.0, TimeEnd: 3.2},
		{View: "event_search", TimeStart: 3.0 + tol/2, TimeEnd: 3.2 - tol/2},
	}
	if w, ok := traceSupplementDeriveWindow(windows); !ok || w.TimeStart != 3.0 || w.TimeEnd != 3.2 {
		t.Fatalf("tolerance-equal windows must derive the first recorded: %+v ok=%t", w, ok)
	}
	// NW-01 customer shape: one explicit analysis window encloses the
	// lifecycle-split anchor windows. The unique recorded outer window is a
	// precise authority; do not discard it merely because the model also
	// investigated its sub-windows.
	windows = []types.TraceQueryCallWindow{
		{View: "root_cause_rank", TimeStart: 69326.832743749, TimeEnd: 69327.060110624},
		{View: "root_cause_rank", TimeStart: 69326.832743749, TimeEnd: 69326.875412},
		{View: "wakeup_chain", TimeStart: 69326.875412, TimeEnd: 69327.060110624},
	}
	if w, ok := traceSupplementDeriveWindow(windows); !ok ||
		w.TimeStart != 69326.832743749 || w.TimeEnd != 69327.060110624 {
		t.Fatalf("unique explicit enclosing anchor window must win: %+v ok=%t", w, ok)
	}
	// NW-01 残余记档(2026-07-24 核验席,如实冻结现语义):模型若显式发过
	// 近全 trace 界的 anchor 调用,它包住其余全部窗而用户窗不包住它——唯一
	// enclosing 候选=全 trace 窗,当选。该窗仍是模型显式调查过的精确参数窗
	// (被 guard 拒绝的调用结构性进不了登记表,见
	// TestTraceQueryCallWindowRegistrationRequiresBothExplicitBounds 两腿),
	// 但相对用户关注窗偏大;根修=RequestModel typed 用户时间窗 lane(已记档
	// 待立案),选举侧不做「近全长」启发式排除(嘈声信号禁进硬门)。
	windows = []types.TraceQueryCallWindow{
		{View: "root_cause_rank", TimeStart: 69326.012, TimeEnd: 69328.343},
		{View: "root_cause_rank", TimeStart: 69326.832743749, TimeEnd: 69327.060110624},
		{View: "root_cause_rank", TimeStart: 69326.832743749, TimeEnd: 69326.875412},
		{View: "wakeup_chain", TimeStart: 69326.875412, TimeEnd: 69327.060110624},
	}
	if w, ok := traceSupplementDeriveWindow(windows); !ok || w.TimeStart != 69326.012 || w.TimeEnd != 69328.343 {
		t.Fatalf("explicit whole-trace anchor window is the unique enclosing candidate and wins (frozen semantics): %+v ok=%t", w, ok)
	}
	// Inconsistent anchor-capable lane: skip — never fall through, never
	// last-wins (F1 precedent).
	windows = []types.TraceQueryCallWindow{
		{View: "root_cause_rank", TimeStart: 3.0, TimeEnd: 3.2},
		{View: "wakeup_chain", TimeStart: 4.0, TimeEnd: 4.2},
		{View: "event_search", TimeStart: 3.0, TimeEnd: 3.2},
	}
	if _, ok := traceSupplementDeriveWindow(windows); ok {
		t.Fatal("inconsistent anchor-lane windows must fail open")
	}
	// Two incomparable outer windows are still ambiguous. Their synthetic
	// union was never an explicit call and must not be invented.
	windows = []types.TraceQueryCallWindow{
		{View: "root_cause_rank", TimeStart: 3.0, TimeEnd: 4.0},
		{View: "root_cause_rank", TimeStart: 3.5, TimeEnd: 4.5},
		{View: "wakeup_chain", TimeStart: 3.6, TimeEnd: 3.8},
	}
	if _, ok := traceSupplementDeriveWindow(windows); ok {
		t.Fatal("incomparable anchor outer windows must remain inconsistent")
	}
	// h2-20260714-013012 run-1 witness: the scoped-stats lane (the analysis
	// window) outranks event_search micro-probe drill-downs.
	windows = []types.TraceQueryCallWindow{
		{View: "event_search", TimeStart: 13762.791708, TimeEnd: 13763.024898},
		{View: "event_search", TimeStart: 13762.8, TimeEnd: 13762.825},
		{View: "event_search", TimeStart: 13762.81126, TimeEnd: 13762.8113},
		{View: "window_stats", TimeStart: 13762.791708, TimeEnd: 13763.024898},
	}
	if w, ok := traceSupplementDeriveWindow(windows); !ok || w.TimeStart != 13762.791708 || w.TimeEnd != 13763.024898 {
		t.Fatalf("stats lane must outrank locator micro-probes: %+v ok=%t", w, ok)
	}
	// Disagreeing stats-lane windows (main + window_stats micro-probe): skip
	// — the F1 pathology shape must never be resolved by picking either.
	windows = []types.TraceQueryCallWindow{
		{View: "window_stats", TimeStart: 13762.791708, TimeEnd: 13763.024898},
		{View: "window_stats", TimeStart: 13762.8, TimeEnd: 13762.825},
	}
	if _, ok := traceSupplementDeriveWindow(windows); ok {
		t.Fatal("disagreeing stats-lane windows must fail open")
	}
	if _, ok := traceSupplementDeriveWindow(nil); ok {
		t.Fatal("no windows must fail open")
	}
}

func TestTraceSupplementFrameFamilySelectsFrameBundleWhenEvidenceMissing(t *testing.T) {
	missing := traceSupplementFamilyPresence{}
	if got := traceSupplementViews(missing, true, false); len(got) != 1 || got[0] != "frame_root_cause_bundle" {
		t.Fatalf("frame-family request without frame evidence must run the frame bundle, got %v", got)
	}
	if got := traceSupplementViews(missing, true, true); len(got) != 2 ||
		got[0] != "root_cause_rank" || got[1] != "critical_blocking_calls" {
		t.Fatalf("present frame evidence must keep ordinary missing-family fill, got %v", got)
	}
	complete := traceSupplementFamilyPresence{
		Rank: true, Chain: true, WindowStates: true, Critical: true,
		BlockedReasonCensus: true, WakeupEdgeCensus: true,
	}
	if got := traceSupplementViews(complete, true, false); len(got) != 1 || got[0] != "frame_root_cause_bundle" {
		t.Fatalf("generic families cannot substitute for missing frame evidence, got %v", got)
	}
}

func TestTraceSupplementFrameBundleCarriesTypedProcessScopeEndToEnd(t *testing.T) {
	ctx := suppCoreContext(t)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{
		Kind: types.RuntimeTargetKindProcess, PID: 200, Source: "user_explicit", Confidence: 1,
	}}
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Keywords = []string{"丢帧"}
	suppCoreModelCall(t, ctx, `{"view":"event_search","time_start":3.0,"time_end":3.2}`)

	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) == 0 || out.Executed[0] != "frame_root_cause_bundle" {
		t.Fatalf("frame-family supplement must execute the frame bundle first: %+v", out)
	}
	results := ctx.Mutable.SystemTraceSupplementResults()
	if len(results) == 0 {
		t.Fatal("frame supplement result missing")
	}
	if !strings.Contains(results[0].Summary, "view=frame_root_cause_bundle") ||
		!strings.Contains(results[0].Summary, "target_scope=process") {
		t.Fatalf("typed process scope did not reach the actual engine call:\n%s", results[0].Summary)
	}
	if authority := results[0].TraceEvidenceAuthority; authority == nil ||
		authority.View != "frame_root_cause_bundle" ||
		authority.FrameEvidenceStatus == "" {
		t.Fatalf("frame bundle did not publish typed frame authority: %+v", authority)
	}
}

func TestTraceQueryCallWindowRegistrationRequiresBothExplicitBounds(t *testing.T) {
	// GUARDREG 互斥不变式腿①:登记要求 time_start 与 time_end 双显式。
	// 腿②(heavy-view guard 只拦无界调用)见
	// TestTraceQueryBoundedScopeKeepsHeavyGuardOut。两腿合取 ⇒ 被 guard
	// 拒绝的调用结构性不可能把窗送进补采选举(guard 触发 ⇒ 零时间/行界
	// ⇒ 登记门不满足)。
	for _, blob := range []string{
		`{"view":"root_cause_rank"}`,
		`{"view":"root_cause_rank","time_start":3.0}`,
		`{"view":"root_cause_rank","time_end":3.2}`,
	} {
		ctx := &types.BusContext{Mutable: types.NewMutableState("q")}
		var p traceQueryParams
		if err := json.Unmarshal([]byte(blob), &p); err != nil {
			t.Fatalf("params %s: %v", blob, err)
		}
		traceQueryRecordCallWindow(ctx, p, normalizedTraceQueryWindow(p))
		if got := ctx.Mutable.TraceQueryCallWindows(); len(got) != 0 {
			t.Fatalf("params %s must not register a call window: %+v", blob, got)
		}
	}
	ctx := &types.BusContext{Mutable: types.NewMutableState("q")}
	var p traceQueryParams
	if err := json.Unmarshal([]byte(`{"view":"root_cause_rank","time_start":3.0,"time_end":3.2}`), &p); err != nil {
		t.Fatal(err)
	}
	traceQueryRecordCallWindow(ctx, p, normalizedTraceQueryWindow(p))
	if got := ctx.Mutable.TraceQueryCallWindows(); len(got) != 1 {
		t.Fatalf("both-bounds params must register exactly one window: %+v", got)
	}
}

func TestTraceQueryBoundedScopeKeepsHeavyGuardOut(t *testing.T) {
	// GUARDREG 互斥不变式腿②:任一显式时间/行界即 bounded,heavy-view
	// guard 放行(guard 只拦全无界调用)。
	for _, blob := range []string{
		`{"time_start":3.0}`,
		`{"time_end":3.2}`,
		`{"line_start":10}`,
		`{"line_end":20}`,
	} {
		var p traceQueryParams
		if err := json.Unmarshal([]byte(blob), &p); err != nil {
			t.Fatalf("params %s: %v", blob, err)
		}
		if !traceQueryHasBoundedTraceScope(p) {
			t.Fatalf("params %s must count as bounded scope", blob)
		}
	}
	var p traceQueryParams
	if err := json.Unmarshal([]byte(`{"view":"window_stats"}`), &p); err != nil {
		t.Fatal(err)
	}
	if traceQueryHasBoundedTraceScope(p) {
		t.Fatal("scope-free params must stay unbounded (heavy guard eligible)")
	}
}

func TestTraceSupplementFrameFamilyWindowFailureSkipsInsteadOfWindowlessRank(t *testing.T) {
	// NW-02 判词的窄形回归:frame 家族命中且帧证据缺席(views 已选帧
	// bundle)时,窗派生失败不得让 G4 D-state 无窗回退用通用
	// root_cause_rank 顶替帧调查——无窗的帧因果调查无意义,诚实出口
	// 是 typed skip(census-lite 车道不受影响)。
	ctx := suppCoreContext(t)
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Keywords = []string{"丢帧", "iowait"}
	// Two disjoint call windows: window derivation fails.
	suppCoreModelCall(t, ctx, `{"view":"event_search","pid":200,"time_start":3.0,"time_end":3.05}`)
	suppCoreModelCall(t, ctx, `{"view":"event_search","pid":200,"time_start":3.1,"time_end":3.2}`)
	out := RunTraceQuerySystemSupplement(ctx)
	for _, view := range out.Executed {
		if view == "root_cause_rank" || view == "frame_root_cause_bundle" {
			t.Fatalf("frame-family window failure must not run %q: %+v", view, out)
		}
	}
	if out.SkipReason != "window_inconsistent" {
		t.Fatalf("skip reason = %q, want window_inconsistent", out.SkipReason)
	}
}

func TestTraceSupplementLabelFormUserTargetParsesPid(t *testing.T) {
	// R3a (§13.2, no_touying 第四放实证): analyzer 把「name [pid]」标签原串
	// 放进 RuntimeTarget.Thread 且 PID=0 时,用户车道此前原样拷贝——补采以
	// TargetPID=0 原串形跑(披露「目标 ss.hm.ugc.aweme [32788]」)。标签形
	// 必须解析为 typed pid(bracket/hyphen 双形,precise 规则同 entities
	// fallback),供 meta/scope/披露消费;引擎锚(frame_target_resolution)
	// 同 pin 封口。
	ctx := suppCoreContext(t)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{
		Kind: types.RuntimeTargetKindThread, Thread: "worker [200]",
		Source: "user_explicit", Confidence: 1,
	}}
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Keywords = []string{"丢帧"}
	suppCoreModelCall(t, ctx, `{"view":"event_search","time_start":3.0,"time_end":3.2}`)
	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) == 0 || out.Executed[0] != "frame_root_cause_bundle" {
		t.Fatalf("frame bundle must run for the label-form user target: %+v", out)
	}
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || meta.TargetPID != 200 || meta.TargetThread != "worker" {
		t.Fatalf("label-form user target must parse into typed pid+name, got %+v", meta)
	}
	results := ctx.Mutable.SystemTraceSupplementResults()
	if len(results) == 0 {
		t.Fatal("supplement result missing")
	}
	anchors := 0
	for _, obs := range results[0].Observations {
		if strings.Contains(obs.Predicate, "frame_target_resolution") {
			anchors++
		}
	}
	if anchors == 0 {
		t.Fatalf("successful bundle must mint the frame_target_resolution anchor: %d observations", len(results[0].Observations))
	}
}

func TestTraceSupplementTargetDerivation(t *testing.T) {
	ctx := &types.BusContext{Mutable: types.NewMutableState("q")}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeTargets: []types.RuntimeTarget{
		{Kind: types.RuntimeTargetKindThread, PID: 200, Thread: "worker", Source: "user_explicit"},
		{Kind: types.RuntimeTargetKindThread, PID: 999, Thread: "elsewhere", Source: types.RuntimeTargetSourceExplicitToolCall},
	}}}
	// R2: the user-source target outranks the model's exploration cursor.
	target, source, ok := traceSupplementDeriveTarget(ctx)
	if !ok || target.PID != 200 || target.Thread != "worker" ||
		target.TargetScope != tracequery.TargetScopeThread || source != "user" {
		t.Fatalf("user lane must win: %+v source=%q ok=%t", target, source, ok)
	}
	// Cursor fallback: no user target, ONE consistent cursor target.
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{
		{Kind: types.RuntimeTargetKindThread, PID: 999, Thread: "elsewhere", Source: types.RuntimeTargetSourceExplicitToolCall},
	}
	target, source, ok = traceSupplementDeriveTarget(ctx)
	if !ok || target.PID != 999 || source != "cursor" {
		t.Fatalf("cursor fallback must engage when the user lane is empty: %+v source=%q ok=%t", target, source, ok)
	}
	// Ambiguous cursor lane with no user target: fail open.
	ctx.AnalysisIR.RequestModel.RuntimeTargets = append(ctx.AnalysisIR.RequestModel.RuntimeTargets,
		types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, PID: 1000, Thread: "other", Source: types.RuntimeTargetSourceExplicitToolCall})
	if _, _, ok = traceSupplementDeriveTarget(ctx); ok {
		t.Fatal("ambiguous cursor lane must fail open")
	}
	// h4 first-trip witness (2026-07-14): ONE pid under two thread SPELLINGS
	// unifies to a pid-only target — the integer pid is the precise signal.
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{
		{Kind: types.RuntimeTargetKindThread, PID: 17267, Thread: ".ugc.aweme.lite 17267", Source: types.RuntimeTargetSourceExplicitToolCall},
		{Kind: types.RuntimeTargetKindThread, PID: 17267, Thread: ".ugc.aweme.lite-17267", Source: types.RuntimeTargetSourceExplicitToolCall},
	}
	target, source, ok = traceSupplementDeriveTarget(ctx)
	if !ok || target.PID != 17267 || target.Thread != "" || source != "cursor" {
		t.Fatalf("one-pid mixed-spelling lane must unify to a pid-only target: %+v source=%q ok=%t", target, source, ok)
	}
	// Thread-only entries: exact-label uniqueness, mixed labels fail open.
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{
		{Kind: types.RuntimeTargetKindThread, Thread: "RenderThread", Source: "user_explicit"},
		{Kind: types.RuntimeTargetKindThread, Thread: "renderthread", Source: "user_explicit"},
	}
	target, source, ok = traceSupplementDeriveTarget(ctx)
	if !ok || target.Thread != "RenderThread" || target.PID != 0 || source != "user" {
		t.Fatalf("case-equal thread-only labels must unify: %+v source=%q ok=%t", target, source, ok)
	}
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{
		{Kind: types.RuntimeTargetKindThread, Thread: "RenderThread", Source: "user_explicit"},
		{Kind: types.RuntimeTargetKindThread, Thread: "GPU completion", Source: "user_explicit"},
	}
	if _, _, ok = traceSupplementDeriveTarget(ctx); ok {
		t.Fatal("mixed thread-only labels must fail open")
	}
	// A typed process target stays process-scoped for frame discovery. The
	// supplement must not drop RuntimeTarget.Kind and silently turn a process
	// question into an exact-TID frame query.
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{
		Kind: types.RuntimeTargetKindProcess, PID: 32788, Source: "user_explicit",
	}}
	target, source, ok = traceSupplementDeriveTarget(ctx)
	if !ok || target.PID != 32788 || target.TargetScope != tracequery.TargetScopeProcess || source != "user" {
		t.Fatalf("typed process target scope must survive derivation: %+v source=%q ok=%t", target, source, ok)
	}
}

// --- SUPP-TARGET (§29.90.1, 2026-07-15) entities-lane fallback pins ------------

// TestTraceSupplementEntitiesFallbackTargetDerivation pins the precise parse
// gate (name-pid split + uniqueness, 歧义即弃) and the trigger gate (typed
// lane minted NOTHING at all — an ambiguous typed lane is a deliberate skip
// the entities face must never override).
func TestTraceSupplementEntitiesFallbackTargetDerivation(t *testing.T) {
	build := func(entities []string, targets []types.RuntimeTarget) *types.BusContext {
		ctx := &types.BusContext{Mutable: types.NewMutableState("q")}
		ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
			AnalyzerHints:  types.AnalyzerHints{Entities: entities},
			RuntimeTargets: targets,
		}}
		return ctx
	}
	// One thread-shaped `name-pid` entity: fallback engages with the parsed
	// pid, the verbatim label, and the disclosed lane.
	target, source, ok := traceSupplementDeriveTarget(build([]string{"worker-200"}, nil))
	if !ok || target.PID != 200 || target.Thread != "worker-200" || source != "entities_fallback" {
		t.Fatalf("parseable entity must recover the target: %+v source=%q ok=%t", target, source, ok)
	}
	// Precise parse gate — every miss stays a skip:
	for name, entities := range map[string][]string{
		"no entities":               nil,
		"no dash-digit tail":        {"worker"},
		"bare numeric range":        {"100-200"},
		"decimal tail":              {"3.0-3.2"},
		"pid over the sanity cap":   {"worker-99999999"},
		"unsafe label characters":   {"bad/worker-200"},
		"two distinct thread pids":  {"worker-200", "peer-300"},
		"dash tail only":            {"-200"},
		"trailing dash without pid": {"worker-"},
	} {
		if _, _, ok := traceSupplementDeriveTarget(build(entities, nil)); ok {
			t.Errorf("%s must fail open: entities=%v", name, entities)
		}
	}
	// One pid under two spellings: pid-only (lane-unification precedent).
	target, source, ok = traceSupplementDeriveTarget(build([]string{"Worker-200", "worker-200 "}, nil))
	if !ok || target.PID != 200 || source != "entities_fallback" {
		t.Fatalf("case-equal spellings must keep the pid: %+v source=%q ok=%t", target, source, ok)
	}
	target, _, ok = traceSupplementDeriveTarget(build([]string{"workerA-200", "workerB-200"}, nil))
	if !ok || target.PID != 200 || target.Thread != "" {
		t.Fatalf("one-pid mixed-label entities must go pid-only: %+v ok=%t", target, ok)
	}
	// Trigger gate: ANY minted typed target (even an ambiguous or unsafe
	// pair) keeps the fallback out — rung 3 recovers only the
	// nothing-was-minted variant.
	ambiguous := []types.RuntimeTarget{
		{Kind: types.RuntimeTargetKindThread, PID: 200, Thread: "worker", Source: "user_explicit"},
		{Kind: types.RuntimeTargetKindThread, PID: 300, Thread: "peer", Source: "user_explicit"},
	}
	if _, _, ok := traceSupplementDeriveTarget(build([]string{"worker-200"}, ambiguous)); ok {
		t.Fatal("an ambiguous typed lane is a deliberate skip — entities must not override it")
	}
	// A consistent cursor lane still outranks the entities face.
	cursor := []types.RuntimeTarget{
		{Kind: types.RuntimeTargetKindThread, PID: 999, Thread: "elsewhere", Source: types.RuntimeTargetSourceExplicitToolCall},
	}
	target, source, ok = traceSupplementDeriveTarget(build([]string{"worker-200"}, cursor))
	if !ok || target.PID != 999 || source != "cursor" {
		t.Fatalf("cursor lane must outrank the entities fallback: %+v source=%q ok=%t", target, source, ok)
	}
}

// TestTraceSupplementEntitiesFallbackRedToGreen — the h2 20260714-221545
// variant, full chain: the classifier minted NO runtime_targets (the thread
// identity sits only on the entities list) and every model call was
// pattern-only (zero cursor targets). Pre-fix this skipped no_typed_target
// and three hard answer faces failed to mint; the fallback recovers the
// typed target and the supplement executes with disclosed provenance.
func TestTraceSupplementEntitiesFallbackRedToGreen(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, types.AttachedTraceBlobBasename), []byte(suppCoreTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("为什么 worker 线程卡顿")}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{Entities: []string{"worker-200"}},
	}}
	// Pattern-only model call: records the call window but NO cursor target.
	suppCoreModelCall(t, ctx, `{"view":"event_search","pattern":"worker","time_start":3.0,"time_end":3.2}`)

	out := RunTraceQuerySystemSupplement(ctx)
	if !out.Attempted || out.SkipReason != "" || len(out.Executed) == 0 {
		t.Fatalf("entities fallback must let the supplement execute: %+v", out)
	}
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil {
		t.Fatal("executed supplement must store typed meta")
	}
	if meta.TargetPID != 200 || meta.TargetThread != "worker-200" || meta.TargetSource != "entities_fallback" {
		t.Fatalf("meta target = pid=%d thread=%q source=%q, want 200/worker-200/entities_fallback",
			meta.TargetPID, meta.TargetThread, meta.TargetSource)
	}
	// The recovered target mints the anchored faces (the h2 disease killed
	// exactly these).
	after := suppCoreTreeFence(t, ctx)
	for _, anchored := range []string{"⊚ worker-200 ‹用户关注线程›", "➊", "自身·D-state", "等待对象 dma_fence_default_wait"} {
		if !strings.Contains(after, anchored) {
			t.Fatalf("fallback-supplemented fence must carry %q:\n%s", anchored, after)
		}
	}
}

// TestTraceSupplementEntitiesFallbackUnparseableStillSkips — the negative
// twin: entities WITHOUT a thread-shaped `name-pid` form keep the original
// fail-open skip byte-identical (宁缺勿假).
func TestTraceSupplementEntitiesFallbackUnparseableStillSkips(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, types.AttachedTraceBlobBasename), []byte(suppCoreTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("为什么 worker 线程卡顿")}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{Entities: []string{"worker", "D state", "不可中断等待"}},
	}}
	suppCoreModelCall(t, ctx, `{"view":"event_search","pattern":"worker","time_start":3.0,"time_end":3.2}`)
	suppCoreAssertFailOpen(t, ctx, RunTraceQuerySystemSupplement(ctx), "no_typed_target")
}

// --- 修复轮 (SHIP-WITH-FIXES, 2026-07-14) pins -----------------------------------

// 件1: an explore REOPEN clears the supplement lane + latch, so every
// explore-visible consumer face — including the transcript observation
// CHECKPOINT face (ObservationLedgerInputFromAgentContext) the M4-era pin
// could not discriminate — sees zero supplement records during the reopened
// exploration, and the supplement re-mints at the next boundary.
func TestTraceSupplementExploreReopenResetClearsLane(t *testing.T) {
	ctx := suppCoreContext(t)
	suppCoreH2FlatShape(t, ctx)
	if out := RunTraceQuerySystemSupplement(ctx); len(out.Executed) == 0 {
		t.Fatalf("supplement must execute: %+v", out)
	}
	if !ctx.Mutable.ResetSystemTraceSupplementForExploreReopen() {
		t.Fatal("reopen reset must report clearing after an executed supplement")
	}
	// Both ledger-input builders (bus + agent/checkpoint face) are clean.
	if got := types.ObservationLedgerInputFromBusContext(ctx, 64).SystemTraceSupplementResults; len(got) != 0 {
		t.Fatalf("bus-side ledger input must be clean after reopen, got %d results", len(got))
	}
	ac := promptctx.BuildAgentContext(ctx, types.AgentExplorer, types.StageExplore)
	if got := types.ObservationLedgerInputFromAgentContext(ac, 64).SystemTraceSupplementResults; len(got) != 0 {
		t.Fatalf("checkpoint-face ledger input must be clean after reopen, got %d results", len(got))
	}
	// Families revert to the flat-shape presence (no rank/chain).
	if families := traceSupplementFamilies(suppCoreLedger(ctx)); families.Rank || families.Chain {
		t.Fatalf("reopened explore must not see supplement families: %+v", families)
	}
	if ctx.Mutable.SystemTraceSupplementMeta() != nil || ctx.Mutable.SystemTraceSupplementAttempted() {
		t.Fatal("reopen reset must clear meta and the latch")
	}
	// The next boundary re-mints (latch cleared; call windows preserved).
	out := RunTraceQuerySystemSupplement(ctx)
	if !out.Attempted || len(out.Executed) == 0 {
		t.Fatalf("supplement must re-mint after the reopen: %+v", out)
	}
	// Idempotence: a reset with nothing attempted reports false.
	fresh := types.NewMutableState("q")
	if fresh.ResetSystemTraceSupplementForExploreReopen() {
		t.Fatal("reset must no-op when nothing was attempted")
	}
}

// 件2: a Success=false engine reject contributes zero ledger records, so it
// must not count as executed and must not mint a disclosure line.
func TestTraceSupplementRejectNotCountedNotDisclosed(t *testing.T) {
	ctx := suppCoreContext(t)
	suppCoreH2FlatShape(t, ctx)
	// Corrupt the attached blob AFTER the model calls: truncate (busts the
	// size-keyed engine index cache) and drop read permission, so the
	// supplement's Execute fails with a Success=false parse reject.
	blob := filepath.Join(ctx.WorkDir, types.AttachedTraceBlobBasename)
	if err := os.Truncate(blob, 0); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blob, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blob, 0o644) })
	out := RunTraceQuerySystemSupplement(ctx)
	if !out.Attempted || len(out.Executed) != 0 || out.SkipReason != "execution_failed" {
		t.Fatalf("all-reject run must fail open as execution_failed: %+v", out)
	}
	if len(ctx.Mutable.SystemTraceSupplementResults()) != 0 || ctx.Mutable.SystemTraceSupplementMeta() != nil {
		t.Fatal("rejected views must store nothing")
	}
	doc := &types.AnswerDocumentV2{}
	if materializeRuntimeTraceSupplementDisclosureCaveat(doc, ctx) || len(doc.Caveats) != 0 {
		t.Fatalf("rejected views must not mint a disclosure: %q", doc.Caveats)
	}
}

// 件3: the supplement's payload blobs enter the Q5-A escape-lane registry so
// the finalize model can open the refs its feed shows.
func TestTraceSupplementRegistersBlobRefs(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, ".codrax", "blob", "sess")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, types.AttachedTraceBlobBasename), []byte(suppCoreTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("q")}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeTargets: []types.RuntimeTarget{{
			Kind: types.RuntimeTargetKindThread, PID: 200, Thread: "worker",
			Source: "user_explicit", Confidence: 1,
		}},
	}}
	suppCoreH2FlatShape(t, ctx)
	if got := ctx.Mutable.TraceQueryBlobRefs(); len(got) != 0 {
		t.Fatalf("model-lane test calls bypass the dispatch chokepoint — registry must start empty, got %v", got)
	}
	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) == 0 {
		t.Fatalf("supplement must execute: %+v", out)
	}
	refs := ctx.Mutable.TraceQueryBlobRefs()
	if len(refs) == 0 {
		t.Fatal("supplement must register its payload blob refs on the escape-lane registry")
	}
	results := ctx.Mutable.SystemTraceSupplementResults()
	if len(results) == 0 || strings.TrimSpace(results[0].RawRef) == "" {
		t.Fatalf("fixture must produce a blob-backed supplement result: %+v", results)
	}
	found := false
	for _, ref := range refs {
		if ref == strings.TrimSpace(results[0].RawRef) {
			found = true
		}
	}
	if !found {
		t.Fatalf("registry %v must contain the supplement result's raw ref %q", refs, results[0].RawRef)
	}
}

// 件4: the supplement's own engine calls must not write back exploration
// cursors or call windows (feedback-loop guard).
func TestTraceSupplementNoCursorOrWindowWriteBack(t *testing.T) {
	ctx := suppCoreContext(t)
	// pid-only user target; the model call carries NO pid so the cursor
	// lane starts empty and any cursor entry after the supplement would be
	// the supplement's own write-back.
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{
		Kind: types.RuntimeTargetKindProcess, PID: 200, Source: "user_explicit", Confidence: 1,
	}}
	suppCoreModelCall(t, ctx, `{"view":"event_search","time_start":3.0,"time_end":3.2}`)
	windowsBefore := len(ctx.Mutable.TraceQueryCallWindows())
	cursorCount := func() int {
		n := 0
		for _, target := range ctx.AnalysisIR.RequestModel.RuntimeTargets {
			if types.RuntimeTargetIsExplorationCursorSource(target.Source) {
				n++
			}
		}
		if rm := ctx.Mutable.RequestModel(); rm != nil {
			for _, target := range rm.RuntimeTargets {
				if types.RuntimeTargetIsExplorationCursorSource(target.Source) {
					n++
				}
			}
		}
		return n
	}
	if cursorCount() != 0 {
		t.Fatalf("cursor lane must start empty, got %d", cursorCount())
	}
	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) == 0 {
		t.Fatalf("supplement must execute: %+v", out)
	}
	if got := cursorCount(); got != 0 {
		t.Fatalf("supplement calls must not mint exploration cursors, got %d", got)
	}
	if got := len(ctx.Mutable.TraceQueryCallWindows()); got != windowsBefore {
		t.Fatalf("supplement calls must not append call windows: %d -> %d", windowsBefore, got)
	}
	if ctx.Mutable.SystemTraceSupplementInProgress() {
		t.Fatal("in-progress flag must clear after the supplement returns")
	}
}

// 件5 (冷读 SC-F1): supplement-minted rows carry the origin=system_supplement
// audit token on the E# detail face; model-minted rows never do.
func TestTraceSupplementAuditOriginToken(t *testing.T) {
	renderDetail := func(ctx *types.BusContext) string {
		set := types.CompileTraceCausalProjectionSet(suppCoreLedger(ctx))
		if len(set.Projections) == 0 {
			return ""
		}
		evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
		buildRuntimeTraceProjTreeModel(set.Projections[0], evidence, true)
		// The audit tokens live on the E# evidence-index face.
		text, items := runtimeTraceProjEvidenceBlockParts(evidence, true)
		var b strings.Builder
		b.WriteString(text)
		for _, item := range items {
			b.WriteString("\n")
			b.WriteString(item.Text)
			for _, cell := range item.Cells {
				b.WriteString("\n")
				b.WriteString(cell)
			}
		}
		return b.String()
	}
	// Supplemented run: token present.
	ctx := suppCoreContext(t)
	suppCoreH2FlatShape(t, ctx)
	if out := RunTraceQuerySystemSupplement(ctx); len(out.Executed) == 0 {
		t.Fatalf("supplement must execute: %+v", out)
	}
	if detail := renderDetail(ctx); !strings.Contains(detail, "origin=system_supplement") {
		t.Fatalf("supplemented rows must carry the origin audit token:\n%s", detail)
	}
	// Model-dispatched run: token absent everywhere.
	ctrl := suppCoreContext(t)
	suppCoreModelCall(t, ctrl, `{"view":"root_cause_rank","pid":200,"time_start":3.0,"time_end":3.2}`)
	suppCoreModelCall(t, ctrl, `{"view":"critical_blocking_calls","pid":200,"time_start":3.0,"time_end":3.2}`)
	if detail := renderDetail(ctrl); strings.Contains(detail, "origin=system_supplement") {
		t.Fatalf("model-minted rows must not carry the origin audit token:\n%s", detail)
	}
}

func TestTraceSupplementDisclosureFlagsZeroValueObservationViews(t *testing.T) {
	// R3B-C2 (§13.8): Success=true 但零值观测的视图不得以「成功补跑」词面
	// 冒充 provenance——零计数配诚实子句(第五放悖论面的自诊断改造)。
	meta := &types.SystemTraceSupplementMeta{
		Views:                 []string{"root_cause_rank"},
		ViewValueObservations: []int{0},
		WindowStart:           3.0,
		WindowEnd:             3.2,
		TargetPID:             200,
		TargetThread:          "worker",
	}
	zh := runtimeTraceSupplementDisclosureText(meta, true)
	if !strings.Contains(zh, "值观测0条（本视图未产出可用观测，勿视为已补齐）") {
		t.Fatalf("zh zero-count clause missing: %q", zh)
	}
	en := runtimeTraceSupplementDisclosureText(meta, false)
	if !strings.Contains(en, "[value observations: 0 — this view produced no usable rows; do not treat it as recovered coverage]") {
		t.Fatalf("en zero-count clause missing: %q", en)
	}
}
