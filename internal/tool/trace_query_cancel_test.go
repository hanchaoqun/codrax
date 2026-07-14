package tool

// trace_query_cancel_test.go — SUPP-CANCEL (2026-07-14) tool/supplement-lane
// pins. FULL-CHAIN engine-minted fixtures where the lane is deterministically
// reachable (§28.7 fixture discipline); the one wall-clock-racy shape (mixed
// executed+canceled meta) pins its wording through the pure renderer with a
// typed meta value and says so.
//
// Pin family:
//   ① model lane: a warm-index Execute under a dead bus context returns the
//     honest typed partial — Success=true, ToolResult.TraceViewCancellation
//     mirrored from the engine record, the early summary banner, zero
//     published faces on the payload;
//   ② model lane, cold parse: a dead context during the parse yields the
//     honest canceled failure summary (never "failed to parse");
//   ③ supplement lane, pre-expired duration budget (批4 P3-5 + 20s knob
//     联动): every attempted view cancels, nothing is recorded, and the skip
//     is DISCLOSED through meta.CanceledViews (禁裸丢) — never the silent
//     execution_failed fail-open;
//   ④ disclosure wording (ATOMIC): canceled-only sentence zh/en + mixed-run
//     canceled tail zh/en;
//   ⑤ DET-1 tool face: two identical warm Executes (live context) return
//     byte-identical payloads — carrier wiring adds nothing untriggered.
//
// MUTATION self-checks (verified during the batch):
//   - dropping the supplement's canceled-views meta lane reds ③ (silent
//     execution_failed);
//   - dropping traceQueryToolViewCancellation reds ①;
//   - reverting the parse-cancel summary honesty reds ②.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// suppCancelContext: the suppCore fixture with a cancelable bus context.
func suppCancelContext(t *testing.T) (*types.BusContext, context.CancelFunc) {
	t.Helper()
	ctx := suppCoreContext(t)
	live, stop := context.WithCancel(context.Background())
	ctx.Ctx = live
	return ctx, stop
}

// ① warm index + dead context ⇒ typed partial with the mirrored record.
func TestTraceQueryCancelModelLaneWarmIndexTypedPartial(t *testing.T) {
	ctx, stop := suppCancelContext(t)
	// Warm the index cache with a live context (same params ⇒ same cache key).
	warm, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"view":"root_cause_rank","pid":200,"time_start":3.0,"time_end":3.2}`))
	if err != nil || !warm.Success {
		t.Fatalf("warm call failed: %v %s", err, warm.Summary)
	}
	if warm.TraceViewCancellation != nil {
		t.Fatalf("live-context run must not carry a cancellation record: %+v", warm.TraceViewCancellation)
	}
	stop()
	res, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"view":"root_cause_rank","pid":200,"time_start":3.0,"time_end":3.2}`))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("warm-cache canceled run must return the typed partial, got failure: %s", res.Summary)
	}
	vc := res.TraceViewCancellation
	if vc == nil || vc.View != "root_cause_rank" || vc.Reason != "canceled" {
		t.Fatalf("mirrored cancellation record missing/wrong: %+v", vc)
	}
	if len(vc.DiscardedFaces) == 0 {
		t.Fatalf("cancellation record must name the discarded faces: %+v", vc)
	}
	if !strings.Contains(res.Summary, "view_cancellation=true reason=canceled") {
		t.Fatalf("summary must carry the early cancellation banner: %s", res.Summary)
	}
	if len(res.Observations) != 0 {
		t.Fatalf("a pre-canceled run publishes no faces, so no observations: %d", len(res.Observations))
	}
}

// ② cold parse + dead context ⇒ honest canceled failure, never parse blame.
func TestTraceQueryCancelColdParseHonestSummary(t *testing.T) {
	ctx, stop := suppCancelContext(t)
	stop()
	res, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"view":"root_cause_rank","pid":200,"time_start":3.0,"time_end":3.2}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.Success {
		t.Fatalf("cold parse under a dead context cannot succeed: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "canceled before completion") {
		t.Fatalf("canceled parse must say canceled: %s", res.Summary)
	}
	if strings.Contains(res.Summary, "failed to parse") {
		t.Fatalf("canceled parse must not blame the file format: %s", res.Summary)
	}
}

// ⑤ DET-1 tool face: identical warm runs with a live context byte-match.
func TestTraceQueryCancelDET1WarmRunsByteIdentical(t *testing.T) {
	ctx, stop := suppCancelContext(t)
	defer stop()
	params := json.RawMessage(`{"view":"window_stats","pid":200,"time_start":3.0,"time_end":3.2}`)
	first, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !first.Success {
		t.Fatalf("first: %v %s", err, first.Summary)
	}
	second, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !second.Success {
		t.Fatalf("second: %v %s", err, second.Summary)
	}
	if first.TraceViewCancellation != nil || second.TraceViewCancellation != nil {
		t.Fatal("live-context runs must not mint cancellation records")
	}
	// Summaries embed blob refs; the deterministic comparison face is the
	// typed observation set.
	a, err := json.Marshal(first.Observations)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(second.Observations)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatalf("warm runs diverged:\n%s\n%s", a, b)
	}
}

// ③ supplement lane: pre-expired duration budget ⇒ disclosed cancellation.
func TestTraceSupplementInViewCancellationCanceledOnlyDisclosed(t *testing.T) {
	suppCoreSetConfig(t, true, 2<<30, time.Nanosecond, 120)
	ctx := suppCoreContext(t)
	// event_search-only shape: every core family missing ⇒ both views planned.
	suppCoreModelCall(t, ctx, `{"view":"event_search","pid":200,"time_start":3.0,"time_end":3.2}`)
	out := RunTraceQuerySystemSupplement(ctx)
	if !out.Attempted {
		t.Fatal("supplement must attempt")
	}
	if len(out.Executed) != 0 {
		t.Fatalf("a pre-expired budget records nothing: %+v", out)
	}
	if out.SkipReason != "duration_budget_exceeded" {
		t.Fatalf("skip reason = %q, want duration_budget_exceeded", out.SkipReason)
	}
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil {
		t.Fatal("the cancellation must be DISCLOSED through the meta lane, not silently skipped (禁裸丢)")
	}
	if len(meta.Views) != 0 {
		t.Fatalf("no view completed, Views must stay empty (false provenance otherwise): %+v", meta.Views)
	}
	if strings.Join(meta.CanceledViews, ",") != "root_cause_rank,critical_blocking_calls" {
		t.Fatalf("canceled views = %v", meta.CanceledViews)
	}
	if meta.SkipReason != "duration_budget_exceeded" || meta.DurationBudgetS <= 0 {
		t.Fatalf("meta budget fields: %+v", meta)
	}
	if len(ctx.Mutable.SystemTraceSupplementResults()) != 0 {
		t.Fatal("nothing was recorded, the results lane must stay empty")
	}
	// No supplement family may have been minted.
	families := traceSupplementFamilies(suppCoreLedger(ctx))
	if families.Rank || families.Chain || families.Critical {
		t.Fatalf("canceled supplement must mint nothing: %+v", families)
	}
	// ④ canceled-only disclosure wording (ATOMIC pin, zh + en).
	doc := &types.AnswerDocumentV2{}
	if !materializeRuntimeTraceSupplementDisclosureCaveat(doc, ctx) || len(doc.Caveats) != 1 {
		t.Fatalf("canceled run must disclose exactly one caveat: %q", doc.Caveats)
	}
	wantZH := "系统补采: 未完成成文前确定性补跑——根因排序（root_cause_rank）、关键阻塞调用（critical_blocking_calls）超 1e-09 秒时长预算在执行中被取消,未采信任何部分结果(窗 3.000000..3.200000);缩小时间窗后可补齐该窗结果"
	if doc.Caveats[0] != wantZH {
		t.Fatalf("zh canceled disclosure = %q, want %q", doc.Caveats[0], wantZH)
	}
	en := runtimeTraceSupplementDisclosureText(meta, false)
	wantEN := "System supplement: pre-report re-run incomplete — root_cause_rank, critical_blocking_calls canceled mid-run over the 1e-09s duration budget; no partial aggregates were kept (window 3.000000..3.200000); narrow the time window to fill it in"
	if en != wantEN {
		t.Fatalf("en canceled disclosure = %q, want %q", en, wantEN)
	}
}

// ④ mixed-run canceled tail wording (renderer pin). The end-to-end mixed
// shape (view 1 completes, view 2 cancels mid-run) is reachable only through
// a wall-clock race against the in-view deadline, so THIS pin feeds the pure
// renderer a typed meta value; the engine-side "published faces are complete"
// half is pinned deterministically in
// internal/tracequery/run_cancel_test.go (mid-flight sweep).
func TestTraceSupplementCanceledTailDisclosureWording(t *testing.T) {
	meta := &types.SystemTraceSupplementMeta{
		Views:           []string{"root_cause_rank"},
		CanceledViews:   []string{"critical_blocking_calls"},
		DurationBudgetS: 20,
		WindowStart:     3.0,
		WindowEnd:       3.2,
		TargetPID:       200,
		TargetThread:    "worker",
	}
	zh := runtimeTraceSupplementDisclosureText(meta, true)
	wantZH := "系统补采: 成文前确定性补跑 根因排序（root_cause_rank）(窗 3.000000..3.200000, 目标 worker-200);其中 关键阻塞调用（critical_blocking_calls） 在 20 秒时长预算处被取消,仅已完成的完整结果被记录,未完成部分整弃"
	if zh != wantZH {
		t.Fatalf("zh mixed disclosure = %q, want %q", zh, wantZH)
	}
	en := runtimeTraceSupplementDisclosureText(meta, false)
	wantEN := "System supplement: deterministic pre-report re-run of root_cause_rank (window 3.000000..3.200000, target worker-200); critical_blocking_calls canceled at the 20s duration budget — only fully-completed results were recorded, unfinished parts were discarded whole"
	if en != wantEN {
		t.Fatalf("en mixed disclosure = %q, want %q", en, wantEN)
	}
}

// suppCoreTracePathologicalBlob overwrites the fixture's attached trace with
// n timestamp-shifted copies of the suppCore body (monotonic clock, ~17·n
// lines) so a COLD parse has real wall-clock work for the deadline to
// interrupt. Returns the blob path.
func suppCoreTracePathologicalBlob(t *testing.T, ctx *types.BusContext, n int) string {
	t.Helper()
	var b strings.Builder
	lines := strings.Split(strings.TrimRight(suppCoreTrace, "\n"), "\n")
	for block := 0; block < n; block++ {
		offset := float64(block) * 0.3
		for _, line := range lines {
			i := strings.Index(line, " 3.")
			j := strings.Index(line, ": ")
			if i < 0 || j < 0 || j <= i {
				t.Fatalf("fixture line without timestamp: %q", line)
			}
			var ts float64
			if _, err := fmt.Sscanf(strings.TrimSpace(line[i:j]), "%f", &ts); err != nil {
				t.Fatalf("fixture timestamp parse: %v (%q)", err, line)
			}
			fmt.Fprintf(&b, "%s %.6f%s\n", line[:i], ts+offset, line[j:])
		}
	}
	path := filepath.Join(ctx.WorkDir, types.AttachedTraceBlobBasename)
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// 病理形 red/green (慢 IO / 大 trace 模拟): a trace large enough that its cold
// parse cannot finish inside a 1ms budget — the wall-clock deadline (not a
// pre-expired one) must interrupt the view mid-parse and disclose. GREEN =
// cancellation triggered + disclosed; the RED counterpart (generous budget)
// must complete with zero cancellation.
func TestTraceSupplementPathologicalDeadlineRedGreen(t *testing.T) {
	ctx := suppCoreContext(t)
	// Inflate the attached trace: repeat the fixture body with shifted
	// timestamps so the parser has real work (~100k lines).
	blob := suppCoreTracePathologicalBlob(t, ctx, 6000)
	_ = blob
	suppCoreModelCall(t, ctx, `{"view":"event_search","pid":200,"time_start":3.0,"time_end":3.2,"pattern":"worker"}`)

	// GREEN arm: 1ms wall-clock budget — the cold parse of ~100k lines
	// cannot finish; the deadline fires DURING the work.
	suppCoreSetConfig(t, true, 2<<30, time.Millisecond, 4000)
	out := RunTraceQuerySystemSupplement(ctx)
	if !out.Attempted || len(out.Executed) != 0 {
		t.Fatalf("pathological run must record nothing: %+v", out)
	}
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || len(meta.CanceledViews) == 0 {
		t.Fatalf("pathological run must disclose the canceled views: %+v", meta)
	}

	// RED arm: fresh fixture, generous budget — no cancellation anywhere.
	ctx2 := suppCoreContext(t)
	suppCoreTracePathologicalBlob(t, ctx2, 6000)
	suppCoreModelCall(t, ctx2, `{"view":"event_search","pid":200,"time_start":3.0,"time_end":3.2,"pattern":"worker"}`)
	suppCoreSetConfig(t, true, 2<<30, 120*time.Second, 4000)
	out2 := RunTraceQuerySystemSupplement(ctx2)
	if len(out2.Executed) == 0 {
		t.Fatalf("generous budget must execute: %+v", out2)
	}
	meta2 := ctx2.Mutable.SystemTraceSupplementMeta()
	if meta2 == nil || len(meta2.CanceledViews) != 0 {
		t.Fatalf("generous budget must not cancel: %+v", meta2)
	}
}
