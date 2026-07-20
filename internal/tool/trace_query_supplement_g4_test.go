package tool

// trace_query_supplement_g4_test.go — G4-ENGINE pins (2026-07-20; §29.145
// filing "blocked_reason↔D 段 typed 配对" engine lane; c2 honest-red witness
// = eval/cases/real_traces/real_trace_c2_dstate_iowait.case).
//
// Disease chain: an event_search-only run (locator probes, no consistent
// analysis window) leaves the SUPP-CORE window derivation with nothing, the
// supplement silently skips, and BOTH blocked_reason↔D-segment pairing faces
// (pid-keyed census inventory + self D/IO seat Σ) structurally never reach
// the ledger — the census-consumption teaching (§29.145 件1) has no census
// to bind and the answer cannot honestly carry the Σ. Second, independent
// gap: on IO-busy traces the census pid cap evicted the ANALYSIS TARGET's
// own low-count row (engine-side pins live in
// internal/tracequery/blocked_reason_census_test.go).
//
// Pinned surfaces:
//   ① red→green — the c2 dispatch shape (ONE windowless event_search, D
//     family named on the typed analyzer face) arms the windowless
//     whole-trace root_cause_rank fallback and the census + rank families
//     reach the supplement lane;
//   ② 禁猜 stands — the SAME dispatch shape WITHOUT a D-family keyword
//     keeps the silent no_typed_window skip byte-identical (the existing
//     FailOpen pins double as this arm's baseline);
//   ③ derived windows win — a consistent model window keeps the ordinary
//     windowed re-run (fallback flag never set);
//   ④ REAL c2 chain — the donghu fixture end-to-end: windowless
//     event_search only → fallback → ledger carries the target's census row
//     (total=3, sync_buffer_read_wi×3) and the self io_wait seat 0.635 with
//     the verbatim caller note (the three EVALGUARD anchors' faces);
//   ⑤ disclosure honesty — the windowless lane speaks the whole-trace
//     clause, never a fabricated 0.000000..0.000000 window.
//
// MUTATION self-checks: dropping the fallback arm reds ①/④; dropping the
// family gate reds ②; dropping the engine target admission reds ④'s census
// assertions; stamping time bounds onto the fallback params reds ④ (the
// whole-trace values change caliber); dropping the disclosure fork reds ⑤.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// suppG4Keywords arms the typed D-state family face on the analyzer hints
// (the c2 question's own words).
func suppG4Keywords(ctx *types.BusContext) {
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Keywords = []string{"D 状态", "IO 等待", "不可中断等待"}
}

func suppG4LedgerRecords(ctx *types.BusContext) []types.ObservationRecord {
	return suppCoreLedger(ctx).Records
}

// --- ① red→green (synthetic, full production chain) --------------------------

func TestTraceSupplementDStateWindowlessFallbackRedToGreen(t *testing.T) {
	ctx := suppCoreContext(t)
	suppG4Keywords(ctx)
	// The c2 dispatch shape: ONE pattern-only event_search — no explicit
	// time bounds, so the call-window registry stays empty.
	suppCoreModelCall(t, ctx, `{"view":"event_search","pid":200,"pattern":"sched_blocked_reason"}`)
	before := suppG4LedgerRecords(ctx)
	for _, record := range before {
		if strings.TrimSpace(record.Predicate) == "blocked_reason_census" {
			t.Fatalf("precondition: event_search-only ledger must carry no census, got %q", record.Summary)
		}
	}
	out := RunTraceQuerySystemSupplement(ctx)
	if !out.Attempted || len(out.Executed) != 1 || out.Executed[0] != "root_cause_rank" {
		t.Fatalf("windowless fallback must execute exactly root_cause_rank: %+v", out)
	}
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || !meta.WindowlessFallback {
		t.Fatalf("meta must carry the windowless-fallback marker: %+v", meta)
	}
	if meta.WindowlessFallbackReason != types.TraceSupplementReasonNoTypedWindow {
		t.Fatalf("fallback reason must echo the derivation failure, got %q", meta.WindowlessFallbackReason)
	}
	if meta.WindowStart != 0 || meta.WindowEnd != 0 {
		t.Fatalf("windowless lane must never claim a derived window: %+v", meta)
	}
	census, rank := false, false
	for _, record := range suppG4LedgerRecords(ctx) {
		if !record.SystemSupplement {
			continue
		}
		if strings.TrimSpace(record.Predicate) == "blocked_reason_census" {
			census = true
		}
		if strings.HasPrefix(strings.TrimSpace(record.ClaimKey), "root_cause_") {
			rank = true
		}
	}
	if !census || !rank {
		t.Fatalf("fallback must mint the census + rank families on the supplement lane (census=%t rank=%t)", census, rank)
	}
}

// --- ② 禁猜 stands without the family keyword --------------------------------

func TestTraceSupplementDStateFallbackRequiresFamilyKeyword(t *testing.T) {
	ctx := suppCoreContext(t)
	// No D-family keyword on the typed analyzer face: the derivation-failure
	// skip must stay byte-identical (silent fail-open, nothing stored).
	suppCoreModelCall(t, ctx, `{"view":"event_search","pid":200,"pattern":"sched_blocked_reason"}`)
	suppCoreAssertFailOpen(t, ctx, RunTraceQuerySystemSupplement(ctx), "no_typed_window")
}

// --- ②b inconsistent windows also route to the fallback ----------------------

func TestTraceSupplementDStateFallbackOnInconsistentWindows(t *testing.T) {
	ctx := suppCoreContext(t)
	suppG4Keywords(ctx)
	suppCoreModelCall(t, ctx, `{"view":"event_search","pid":200,"time_start":3.0,"time_end":3.05}`)
	suppCoreModelCall(t, ctx, `{"view":"event_search","pid":200,"time_start":3.1,"time_end":3.2}`)
	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) != 1 || out.Executed[0] != "root_cause_rank" {
		t.Fatalf("inconsistent-window D-family shape must fall back windowless: %+v", out)
	}
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || !meta.WindowlessFallback || meta.WindowlessFallbackReason != types.TraceSupplementReasonWindowInconsistent {
		t.Fatalf("fallback reason must echo window_inconsistent: %+v", meta)
	}
}

// --- ③ a derived window always wins over the fallback ------------------------

func TestTraceSupplementDStateDerivedWindowStaysWindowed(t *testing.T) {
	ctx := suppCoreContext(t)
	suppG4Keywords(ctx)
	suppCoreModelCall(t, ctx, `{"view":"event_search","pid":200,"time_start":3.0,"time_end":3.2}`)
	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) == 0 {
		t.Fatalf("derived-window run must execute the ordinary windowed supplement: %+v", out)
	}
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || meta.WindowlessFallback || meta.WindowlessFallbackReason != "" {
		t.Fatalf("windowed lane must never wear the fallback marker: %+v", meta)
	}
	if meta.WindowStart != 3.0 || meta.WindowEnd != 3.2 {
		t.Fatalf("windowed lane must keep the derived window: %+v", meta)
	}
}

// --- family-hit unit pins -----------------------------------------------------

func TestTraceSupplementDStateFamilyHitTokens(t *testing.T) {
	hit := func(keywords ...string) bool {
		ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{Keywords: keywords},
		}}}
		return traceSupplementDStateFamilyHit(ctx)
	}
	for _, keywords := range [][]string{
		{"D 状态"}, {"D状态"}, {"D-state"}, {"D state"}, {"不可中断等待"},
		{"uninterruptible sleep"}, {"iowait"}, {"io_wait"}, {"IO wait"},
		{"IO 等待"}, {"IO等待"}, {"sched_blocked_reason"}, {"blocked_reason"},
		// P2-C boundary positives: CJK neighbors and punctuation are valid
		// word boundaries for the connected ASCII forms.
		{"等待iowait严重"}, {"io_wait?"},
	} {
		if !hit(keywords...) {
			t.Fatalf("family tokens must hit: %v", keywords)
		}
	}
	for _, keywords := range [][]string{
		// "d state" ⊂ "thread state" and "io wait" ⊂ "audio wait" are kept
		// EXACT-only (the vsync arm's "frame"⊂"framework" precedent).
		{"thread state"}, {"audio waits"}, {"卡顿"}, {"framework"}, {},
		// 返工 P2-C (2026-07-20, 双官同抓): the connected forms are
		// word-bounded — audio_wait/audiowait/radio_wait hosts contain
		// io_wait/iowait as plain substrings and must NOT arm a whole-trace
		// rank (宁漏勿假指: the gate's own carve applies to itself).
		{"audio_wait"}, {"audiowait"}, {"radio_wait"}, {"AudioWaitTimeout"},
		{"card-state"},
	} {
		if hit(keywords...) {
			t.Fatalf("non-family tokens must miss (宁漏勿假指): %v", keywords)
		}
	}
}

// --- ④ REAL c2 chain (donghu fixture, engine-minted end to end) --------------

// TestTraceSupplementG4C2EventSearchOnlyChain replays the c2 honest-red
// mechanism against the REAL capture and pins the repaired chain: an
// event_search-only run for com.baidu.tieba 59566 now ends with the ledger
// carrying (a) the target's own census row — total=3, sync_buffer_read_wi×3
// (needs BOTH the windowless fallback and the engine's target-aware census
// admission: the whole-trace census is led by 16 busier background pids) —
// and (b) the self io_wait seat with the PROFREBASE-pinned Σ 0.635ms and the
// verbatim caller note. These are the faces behind the c2 EVALGUARD anchors
// (count=3 / Σ=0.635 / proven-caller honest face).
func TestTraceSupplementG4C2EventSearchOnlyChain(t *testing.T) {
	raw, err := os.ReadFile(vsyncSAF2RealTraceRel)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, types.AttachedTraceBlobBasename), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("c2 D-state replay")}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeTargets: []types.RuntimeTarget{{
			Kind: types.RuntimeTargetKindThread, PID: 59566, Thread: "com.baidu.tieba",
			Source: "user_explicit", Confidence: 1,
		}},
	}}
	suppG4Keywords(ctx)
	suppCoreModelCall(t, ctx, `{"view":"event_search","pid":59566,"pattern":"sched_blocked_reason"}`)
	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) != 1 || out.Executed[0] != "root_cause_rank" {
		t.Fatalf("c2 shape must arm the windowless fallback: %+v", out)
	}
	var censusRow, ioSeat *types.ObservationRecord
	for _, record := range suppG4LedgerRecords(ctx) {
		if !record.SystemSupplement || !strings.Contains(record.Subject, "59566") {
			continue
		}
		record := record
		if strings.TrimSpace(record.Predicate) == "blocked_reason_census" {
			censusRow = &record
		}
		if strings.HasPrefix(strings.TrimSpace(record.ClaimKey), "root_cause_") &&
			strings.TrimSpace(record.Object) == "io_wait" && strings.TrimSpace(record.Value) == "0.635" {
			ioSeat = &record
		}
	}
	if censusRow == nil {
		t.Fatal("the target's own census row must survive the busy-background pid cap (A1 target admission + A2 fallback)")
	}
	if censusRow.Value != "3" || !strings.Contains(strings.Join(censusRow.RichNotes, "\n"), "sync_buffer_read_wi×3") {
		t.Fatalf("census row must carry the c2 truth verbatim (total=3, sync_buffer_read_wi×3): value=%q notes=%q", censusRow.Value, censusRow.RichNotes)
	}
	if ioSeat == nil {
		t.Fatal("the self io_wait seat (Σ 0.635ms) must reach the ledger on the windowless fallback")
	}
	if !strings.Contains(strings.Join(ioSeat.RichNotes, "\n"), "blocked_reason_caller=sync_buffer_read_wi") {
		t.Fatalf("the io_wait seat must carry the verbatim proven-caller note: %q", ioSeat.RichNotes)
	}
	if !strings.Contains(ioSeat.Summary, "d_state=0.000ms io_wait=0.635ms") {
		t.Fatalf("the seat summary must keep the mutually-exclusive D/IO account verbatim: %q", ioSeat.Summary)
	}
}

// --- wire-shape claim hygiene ------------------------------------------------

// TestTraceSupplementWindowlessFallbackParamsCarryNoTimeBounds pins the
// fallback call's WIRE shape: the params must omit time_start/time_end
// entirely (the whole-trace engine default; a literal 0 would be a CLAIMED
// bound — the C-lite precedent). The current engine happens to normalize an
// explicit 0..0 to the whole trace too, so this claim-hygiene promise is
// only observable on the wire — hence the direct byte pin (mutation M4:
// swapping in the windowed params struct with zero bounds goes red here).
func TestTraceSupplementWindowlessFallbackParamsCarryNoTimeBounds(t *testing.T) {
	raw, err := traceSupplementMarshalWindowlessFallbackParams(
		"root_cause_rank",
		traceQueryRequestTarget{PID: 59566, Thread: "com.baidu.tieba"},
		[]types.TraceQueryCallWindow{{View: "event_search", TimeStart: 1, TimeEnd: 2, TraceFlavor: "hitrace"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	wire := string(raw)
	if strings.Contains(wire, "time_start") || strings.Contains(wire, "time_end") {
		t.Fatalf("windowless fallback params must carry NO time bounds: %s", wire)
	}
	for _, want := range []string{`"view":"root_cause_rank"`, `"pid":59566`, `"thread":"com.baidu.tieba"`, `"trace_flavor":"hitrace"`} {
		if !strings.Contains(wire, want) {
			t.Fatalf("windowless fallback params missing %s: %s", want, wire)
		}
	}
}

// TestTraceSupplementWindowlessFallbackDispatchCarriesNoTimeBounds (返工
// P2-B, 2026-07-20, 冷读官): pins the params the fallback lane ACTUALLY
// DISPATCHES, captured at the dispatch site through the test-only hook — the
// helper-level pin above cannot see a dispatch-site bypass that marshals the
// windowed struct's zero-valued claimed bounds inline (TimeStart/TimeEnd
// carry no omitempty, so such a wire would claim "time_start":0).
func TestTraceSupplementWindowlessFallbackDispatchCarriesNoTimeBounds(t *testing.T) {
	ctx := suppCoreContext(t)
	suppG4Keywords(ctx)
	suppCoreModelCall(t, ctx, `{"view":"event_search","pid":200,"pattern":"sched_blocked_reason"}`)
	var dispatched [][]byte
	traceSupplementFallbackParamsHook = func(raw []byte) {
		dispatched = append(dispatched, append([]byte(nil), raw...))
	}
	defer func() { traceSupplementFallbackParamsHook = nil }()
	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) != 1 || len(dispatched) != 1 {
		t.Fatalf("fallback must dispatch exactly one captured call: executed=%v captured=%d", out.Executed, len(dispatched))
	}
	wire := string(dispatched[0])
	if strings.Contains(wire, "time_start") || strings.Contains(wire, "time_end") {
		t.Fatalf("the DISPATCHED fallback params must carry NO time bounds: %s", wire)
	}
	for _, want := range []string{`"view":"root_cause_rank"`, `"pid":200`} {
		if !strings.Contains(wire, want) {
			t.Fatalf("dispatched fallback params missing %s: %s", want, wire)
		}
	}
}

// TestTraceSupplementWindowlessFallbackCanceledByCaller (返工 P3-⑤,
// 2026-07-20): the windowless lane's zero-execution cancellation path end to
// end — a pre-canceled bus context cancels the fallback rank at the engine
// entry, the skip is disclosed as canceled_by_caller (never blamed on the
// duration budget), and the canceled-only disclosure speaks the windowless
// clause with the ask-for-a-window advice, never a fabricated 0..0 window.
func TestTraceSupplementWindowlessFallbackCanceledByCaller(t *testing.T) {
	ctx := suppCoreContext(t)
	suppG4Keywords(ctx)
	suppCoreModelCall(t, ctx, `{"view":"event_search","pid":200,"pattern":"sched_blocked_reason"}`)
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	ctx.Ctx = cctx
	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) != 0 || out.SkipReason != types.TraceSupplementReasonCanceledByCaller {
		t.Fatalf("pre-canceled caller context must disclose canceled_by_caller with zero execution: %+v", out)
	}
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || !meta.WindowlessFallback || len(meta.CanceledViews) != 1 || meta.CanceledViews[0] != "root_cause_rank" {
		t.Fatalf("canceled windowless meta must keep the fallback marker and the canceled view: %+v", meta)
	}
	zh := runtimeTraceSupplementDisclosureText(meta, true)
	if !strings.Contains(zh, "全 trace 无时间窗") || !strings.Contains(zh, "取消信号中止") || strings.Contains(zh, "0.000000") {
		t.Fatalf("canceled windowless disclosure must speak the whole-trace clause: %q", zh)
	}
}

// --- ⑤ disclosure honesty ----------------------------------------------------

func TestTraceSupplementWindowlessDisclosureText(t *testing.T) {
	windowless := &types.SystemTraceSupplementMeta{
		Views:                    []string{"root_cause_rank"},
		TargetPID:                59566,
		TargetThread:             "com.baidu.tieba",
		WindowlessFallback:       true,
		WindowlessFallbackReason: types.TraceSupplementReasonNoTypedWindow,
	}
	zh := runtimeTraceSupplementDisclosureText(windowless, true)
	if !strings.Contains(zh, "全 trace 无时间窗——本次调查未确定统一分析时间窗") || strings.Contains(zh, "0.000000") {
		t.Fatalf("zh windowless disclosure must speak the whole-trace clause, never a 0..0 window: %q", zh)
	}
	en := runtimeTraceSupplementDisclosureText(windowless, false)
	if !strings.Contains(en, "whole trace, windowless — no consistent analysis window was established") || strings.Contains(en, "0.000000") {
		t.Fatalf("en windowless disclosure must speak the whole-trace clause: %q", en)
	}
	// Canceled-only windowless form: the advice must ask for a window, never
	// advise narrowing one that does not exist.
	canceled := &types.SystemTraceSupplementMeta{
		CanceledViews:            []string{"root_cause_rank"},
		SkipReason:               types.TraceSupplementReasonDurationBudgetExceeded,
		DurationBudgetS:          20,
		WindowlessFallback:       true,
		WindowlessFallbackReason: types.TraceSupplementReasonNoTypedWindow,
	}
	zhCanceled := runtimeTraceSupplementDisclosureText(canceled, true)
	if !strings.Contains(zhCanceled, "提供明确时间窗后可补齐结果") || strings.Contains(zhCanceled, "0.000000") {
		t.Fatalf("zh canceled windowless form must ask for an explicit window: %q", zhCanceled)
	}
	enCanceled := runtimeTraceSupplementDisclosureText(canceled, false)
	if !strings.Contains(enCanceled, "provide an explicit time window to fill it in") || strings.Contains(enCanceled, "0.000000") {
		t.Fatalf("en canceled windowless form must ask for an explicit window: %q", enCanceled)
	}
	// The ordinary windowed sentence keeps its byte shape.
	windowed := &types.SystemTraceSupplementMeta{
		Views:       []string{"root_cause_rank"},
		WindowStart: 3.0,
		WindowEnd:   3.2,
		TargetPID:   200,
	}
	zhWindowed := runtimeTraceSupplementDisclosureText(windowed, true)
	if !strings.Contains(zhWindowed, "窗 3.000000..3.200000") {
		t.Fatalf("windowed disclosure byte shape must be unchanged: %q", zhWindowed)
	}
}
