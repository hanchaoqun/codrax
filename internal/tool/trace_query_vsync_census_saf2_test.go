package tool

// trace_query_vsync_census_saf2_test.go — SA-F2 + C-lite (DISPATCH-IND 批4,
// 2026-07-14) tool-layer pins, production-minted against the REAL tieba
// capture (eval/fixtures/real_traces/donghu_tieba_frame.systrace — the exact
// witness artifact: the model answered "124.14ms between two frames" off two
// Choreographer callbacks while VSyncGenerator-1682's own period print said
// period:16552213 ≈ 16.55ms).
//
// Pinned surfaces:
//   ① window_stats banner + typed observation (window_population caliber);
//   ② event_search banner + typed observation (matched_rows caliber — the
//     witness's actual dispatch shape, pattern=vsync);
//   ③ C-lite: the windowless lightweight supplement arm (typed vsync-family
//     keyword gate → one whole-trace streaming event_search → census enters
//     the supplement lane with SystemSupplement stamping + the windowless
//     disclosure line); fail-open pins (family miss / generator-less trace /
//     families-present precedence).
//
// MUTATION self-checks: dropping the window_stats census wiring reds ①;
// dropping the stream census accumulation reds ②; dropping the C-lite arm
// (or its census-minted storage gate) reds ③.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

const vsyncSAF2RealTraceRel = "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace"

// vsyncSAF2ExecuteOnRealTrace copies the REAL capture bytes into a temp
// workdir (blob payloads must never land in the repo tree) and executes one
// trace_query through the production runner. The params string references the
// trace as "tieba_real.systrace" inside that workdir.
func vsyncSAF2ExecuteOnRealTrace(t *testing.T, params string) types.ToolResult {
	t.Helper()
	raw, err := os.ReadFile(vsyncSAF2RealTraceRel)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tieba_real.systrace"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	res, execErr := (&TraceQuery{}).Execute(ctx, json.RawMessage(params))
	if execErr != nil {
		t.Fatalf("trace_query %s failed: %v", params, execErr)
	}
	if !res.Success {
		t.Fatalf("trace_query %s unsuccessful: %s", params, res.Summary)
	}
	return res
}

func vsyncSAF2ObservationsByPredicate(res types.ToolResult, predicate string) []types.ObservationRecord {
	var out []types.ObservationRecord
	for _, record := range res.Observations {
		if strings.TrimSpace(record.Predicate) == predicate {
			out = append(out, record)
		}
	}
	return out
}

func TestTraceQueryWindowStatsRendersVsyncGeneratorCensus(t *testing.T) {
	res := vsyncSAF2ExecuteOnRealTrace(t,
		`{"source":"path","path":"tieba_real.systrace","view":"window_stats","pid":59566,"time_start":34579.450627,"time_end":34579.475857}`)
	for _, want := range []string{
		// Census header: caliber sentence + the consumer-spacing guidance
		// (the anti-124.14 teaching rides the banner, soft guidance only).
		"- vsync_generator_census(VSync/帧节拍发生器普查) generators=1 — census over ALL events in the queried window",
		"consumer callback spacing (e.g. Choreographer#onVsync intervals) measures frame pacing",
		// Thread row: identity + counts + the authoritative period with its
		// derived ms/Hz forms.
		"- vsync_generator_census_thread VSyncGenerator-1682",
		"period:16552213ns(≈16.552ms≈60.4Hz)×2",
		"identified_by=thread_name+period_print",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("window_stats summary missing %q:\n%s", want, res.Summary)
		}
	}
	records := vsyncSAF2ObservationsByPredicate(res, "vsync_generator_census")
	if len(records) != 1 {
		t.Fatalf("want exactly 1 vsync_generator_census observation, got %d", len(records))
	}
	record := records[0]
	if record.Subject != "VSyncGenerator-1682" || record.ClaimKey != "vsync_generator_census:VSyncGenerator-1682" {
		t.Fatalf("census observation identity = subject %q claim %q", record.Subject, record.ClaimKey)
	}
	if !strings.Contains(record.Value, "period:16552213ns") {
		t.Fatalf("census observation Value = %q, want the authoritative period", record.Value)
	}
	notes := strings.Join(record.RichNotes, "\n")
	for _, want := range []string{
		"vsync_generator_census_caliber=window_population",
		"vsync_generator_census_period_ns=16552213",
		"vsync_generator_census_period_prints=2",
		"vsync_generator_census_woken=2",
		"vsync_generator_census_identified_by=thread_name+period_print",
	} {
		if !strings.Contains(notes, want) {
			t.Fatalf("census observation notes missing %q:\n%s", want, notes)
		}
	}
}

// TestTraceQueryWindowStatsVsyncCensusRendersBeforeTail (修复轮 件5, P3-6,
// 2026-07-14) pins the census banner's EARLY stanza position: busy real-trace
// windows hit the banner width cliff and a tail placement never reached the
// model face (the whole advisory tail was cut on the tieba witness window).
// On a small window where the full stanza fits, the census line must render
// BEFORE the stanza tail (the "- counts " line and the canonical-view
// advisory sections) — moving the call back to the tail block goes red here.
func TestTraceQueryWindowStatsVsyncCensusRendersBeforeTail(t *testing.T) {
	res := vsyncSAF2ExecuteOnRealTrace(t,
		`{"source":"path","path":"tieba_real.systrace","view":"window_stats","pid":59566,"time_start":34579.450627,"time_end":34579.453000}`)
	censusAt := strings.Index(res.Summary, "- vsync_generator_census(")
	countsAt := strings.Index(res.Summary, "- counts ")
	if censusAt < 0 {
		t.Fatalf("small-window summary carries no census line:\n%s", res.Summary)
	}
	if countsAt < 0 {
		t.Fatalf("small-window summary lost the stanza tail ('- counts ') — the position pin needs both:\n%s", res.Summary)
	}
	if censusAt > countsAt {
		t.Fatalf("census line renders AFTER the stanza tail (censusAt=%d countsAt=%d) — the width-cliff regression shape", censusAt, countsAt)
	}
}

func TestTraceQueryEventSearchRendersVsyncGeneratorCensus(t *testing.T) {
	// The witness dispatch shape verbatim: event_search pattern=vsync, no
	// time bounds (whole trace, streaming lane).
	res := vsyncSAF2ExecuteOnRealTrace(t,
		`{"source":"path","path":"tieba_real.systrace","view":"event_search","pattern":"vsync"}`)
	for _, want := range []string{
		"- vsync_generator_census(VSync/帧节拍发生器普查) generators=1 — census over ALL pattern-matched rows in the queried window BEFORE the chronological display truncation (thread/pid filters not applied",
		"- vsync_generator_census_thread VSyncGenerator-1682",
		"period:16552213ns(≈16.552ms≈60.4Hz)×2",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("event_search summary missing %q:\n%s", want, res.Summary)
		}
	}
	records := vsyncSAF2ObservationsByPredicate(res, "vsync_generator_census")
	if len(records) != 1 {
		t.Fatalf("want exactly 1 vsync_generator_census observation, got %d", len(records))
	}
	if notes := strings.Join(records[0].RichNotes, "\n"); !strings.Contains(notes, "vsync_generator_census_caliber=matched_rows") {
		t.Fatalf("event_search census must carry the matched_rows caliber note:\n%s", notes)
	}
}

// --- C-lite ------------------------------------------------------------------

// vsyncSAF2CensusLiteContext builds an attached-trace bus whose blob is the
// REAL tieba capture, with the typed analyzer keyword face carrying the
// vsync family (the witness question's analyzer output shape: "VSync, vsync,
// onVsync, Choreographer, 垂直同步, 周期性…").
func vsyncSAF2CensusLiteContext(t *testing.T, keywords []string) *types.BusContext {
	t.Helper()
	raw, err := os.ReadFile(vsyncSAF2RealTraceRel)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, types.AttachedTraceBlobBasename), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("这份 trace 里有没有 VSync 类周期信号")}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{Keywords: keywords},
	}}
	return ctx
}

func TestTraceSupplementCensusLiteZeroDispatchShape(t *testing.T) {
	// S13 shape: the model never dispatched trace_query at all — no typed
	// target, no call windows. The full supplement cannot run; C-lite mints
	// the generator census from one windowless streaming pass.
	ctx := vsyncSAF2CensusLiteContext(t, []string{"VSync", "垂直同步", "周期性"})
	out := RunTraceQuerySystemSupplement(ctx)
	if !out.Attempted {
		t.Fatal("supplement not attempted")
	}
	if len(out.Executed) != 1 || out.Executed[0] != "event_search" {
		t.Fatalf("C-lite executed = %v (skip=%s), want [event_search]", out.Executed, out.SkipReason)
	}
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || !meta.CensusLite || meta.CensusLitePattern != "vsync" {
		t.Fatalf("meta = %+v, want CensusLite=true pattern=vsync", meta)
	}
	if meta.WindowStart != 0 || meta.WindowEnd != 0 || meta.TargetPID != 0 {
		t.Fatalf("C-lite meta must carry no window/target claim, got %+v", meta)
	}
	// 修复轮 件2: lite-only lanes keep meta.Views EMPTY — Views is reserved
	// for windowed executions (the disclosure's windowed sentence).
	if len(meta.Views) != 0 {
		t.Fatalf("lite-only meta.Views = %v, want empty", meta.Views)
	}
	// The census reaches the compiled ledger through the dedicated system
	// lane with the structural provenance stamp.
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	found := false
	for _, record := range ledger.Records {
		if record.Predicate != "vsync_generator_census" {
			continue
		}
		found = true
		if !record.SystemSupplement {
			t.Fatalf("C-lite census record must be stamped SystemSupplement=true: %+v", record)
		}
		if record.Subject != "VSyncGenerator-1682" {
			t.Fatalf("C-lite census subject = %q, want VSyncGenerator-1682", record.Subject)
		}
		if !strings.Contains(record.Value, "period:16552213ns") {
			t.Fatalf("C-lite census Value = %q, want the authoritative period", record.Value)
		}
	}
	if !found {
		t.Fatal("C-lite census observation never reached the compiled ledger")
	}
	// Windowless disclosure wording, both language faces (修复轮 件2: the
	// lite-only sentence is lane-neutral — it claims only what ran).
	zh := runtimeTraceSupplementDisclosureText(meta, true)
	if !strings.Contains(zh, "系统补采:") || !strings.Contains(zh, "VSync/帧节拍发生器轻量普查") ||
		!strings.Contains(zh, "event_search·vsync") || !strings.Contains(zh, "无时间窗") {
		t.Fatalf("zh C-lite disclosure = %q", zh)
	}
	en := runtimeTraceSupplementDisclosureText(meta, false)
	if !strings.Contains(en, "System supplement:") || !strings.Contains(en, "windowless") ||
		!strings.Contains(en, "VSync/frame-pacing generator census") {
		t.Fatalf("en C-lite disclosure = %q", en)
	}
	if strings.Contains(zh, "窗 0.000000") || strings.Contains(en, "window 0.000000") {
		t.Fatalf("C-lite disclosure must not claim a zero window: zh=%q en=%q", zh, en)
	}
}

func TestTraceSupplementCensusLiteNoWindowDispatchShape(t *testing.T) {
	// The witness run's actual shape: event_search dispatches with a pid but
	// NO explicit time bounds — a cursor target exists, window derivation
	// fails, C-lite covers it.
	ctx := vsyncSAF2CensusLiteContext(t, []string{"Choreographer#onVsync"})
	// Consumer-only pattern: the model-lane result must NOT mint the census
	// itself (修复轮 件2 gate: C-lite arms only while the census family is
	// absent from the ledger — a generator-matching model call would make
	// C-lite correctly stay dark).
	res, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"view":"event_search","pattern":"Choreographer","pid":59566}`))
	if err != nil || !res.Success {
		t.Fatalf("model-lane event_search failed: %v %s", err, res.Summary)
	}
	ctx.ToolResults = append(ctx.ToolResults, res)
	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) != 1 || out.Executed[0] != "event_search" {
		t.Fatalf("C-lite executed = %v (skip=%s), want [event_search]", out.Executed, out.SkipReason)
	}
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || !meta.CensusLite {
		t.Fatalf("meta = %+v, want CensusLite=true", meta)
	}
}

func TestTraceSupplementCensusLiteSurvivesInjectedRuntimeTarget(t *testing.T) {
	// run-2 witness shape (2026-07-14): the request model carries a typed
	// USER runtime target naming the CONSUMER thread (com.baidu.tieba-59566),
	// so target derivation succeeds, window derivation fails, and the C-lite
	// Execute inherits the tool's auto-injected thread filter. Pre-fix the
	// census degenerated to events=0/woken=1 with no period (the generator's
	// rows sat outside the thread filter) and the model re-derived 124ms
	// from consumer callbacks. The census must stay full.
	ctx := vsyncSAF2CensusLiteContext(t, []string{"VSync", "垂直同步"})
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{
		Kind: types.RuntimeTargetKindThread, PID: 59566, Thread: "com.baidu.tieba",
		Source: "user_explicit", Confidence: 1,
	}}
	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) != 1 || out.Executed[0] != "event_search" {
		t.Fatalf("C-lite executed = %v (skip=%s), want [event_search]", out.Executed, out.SkipReason)
	}
	results := ctx.Mutable.SystemTraceSupplementResults()
	if len(results) != 1 {
		t.Fatalf("supplement results = %d, want 1", len(results))
	}
	records := vsyncSAF2ObservationsByPredicate(results[0], "vsync_generator_census")
	if len(records) != 1 {
		t.Fatalf("want exactly 1 census observation, got %d", len(records))
	}
	if !strings.Contains(records[0].Value, "period:16552213ns") {
		t.Fatalf("census degraded under the injected runtime target: Value=%q Summary=%q",
			records[0].Value, records[0].Summary)
	}
	if notes := strings.Join(records[0].RichNotes, "\n"); !strings.Contains(notes, "vsync_generator_census_events=32") {
		t.Fatalf("census event count degraded under the injected runtime target:\n%s", notes)
	}
}

func TestTraceSupplementCensusLiteFamilyMissFailOpen(t *testing.T) {
	// Same attached trace, but the typed keyword face carries no vsync/frame
	// family word: C-lite must stay dark and the original fail-open skip
	// stands (byte-identical output). 「卡顿」 is deliberately IN this list
	// (修复轮 件4): bare 卡顿 is the generic stutter word — an IO-stutter
	// question must not arm the vsync census scan.
	ctx := vsyncSAF2CensusLiteContext(t, []string{"卡顿", "binder", "IO 延迟"})
	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) != 0 || out.SkipReason != "no_typed_target" {
		t.Fatalf("outcome = %+v, want silent no_typed_target skip", out)
	}
	if meta := ctx.Mutable.SystemTraceSupplementMeta(); meta != nil {
		t.Fatalf("family-miss run must store no supplement meta, got %+v", meta)
	}
}

func TestTraceSupplementCensusLiteGeneratorlessTraceFailOpen(t *testing.T) {
	// Vsync-family keywords over a capture with no generator: the C-lite
	// pass runs, finds no census, and stays a silent no-op — never a claim
	// of absence, never a disclosed "补跑" with an empty account.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, types.AttachedTraceBlobBasename), []byte(suppCoreTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("帧率为什么低")}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{Keywords: []string{"vsync", "帧率"}},
	}}
	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) != 0 || out.SkipReason != "no_typed_target" {
		t.Fatalf("outcome = %+v, want silent no_typed_target skip on a generator-less capture", out)
	}
	if meta := ctx.Mutable.SystemTraceSupplementMeta(); meta != nil {
		t.Fatalf("generator-less run must store no supplement meta, got %+v", meta)
	}
}

func TestTraceSupplementCensusLiteWindowedAdjunct(t *testing.T) {
	// 修复轮 件2 core pin (cold-read 残差修根): the WINDOWED success path arms
	// the census scan too. Shape = the run-1 residual: windows derivable
	// (model event_search with explicit bounds on the generator-FREE jank
	// window), core families missing → the windowed supplement executes its
	// views, none of which mints the census (no generator inside the derived
	// window) → the lite adjunct appends the whole-trace census to the SAME
	// supplement.
	ctx := vsyncSAF2CensusLiteContext(t, []string{"VSync", "掉帧"})
	res, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(
		`{"view":"event_search","pattern":"Choreographer","pid":59566,"time_start":34579.472865,"time_end":34579.475857}`))
	if err != nil || !res.Success {
		t.Fatalf("model-lane event_search failed: %v %s", err, res.Summary)
	}
	ctx.ToolResults = append(ctx.ToolResults, res)
	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) != 3 || out.Executed[2] != "event_search" {
		t.Fatalf("executed = %v (skip=%s), want [root_cause_rank critical_blocking_calls event_search]", out.Executed, out.SkipReason)
	}
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || !meta.CensusLite {
		t.Fatalf("meta = %+v, want CensusLite=true on the windowed adjunct", meta)
	}
	// meta.Views keeps ONLY the windowed executions — the disclosure's
	// windowed sentence must not claim the whole-trace scan ran on the
	// derived window.
	if len(meta.Views) != 2 || meta.Views[0] != "root_cause_rank" || meta.Views[1] != "critical_blocking_calls" {
		t.Fatalf("meta.Views = %v, want the two windowed views only", meta.Views)
	}
	results := ctx.Mutable.SystemTraceSupplementResults()
	if len(results) != 3 {
		t.Fatalf("supplement results = %d, want 3 (two windowed + the census adjunct)", len(results))
	}
	records := vsyncSAF2ObservationsByPredicate(results[2], "vsync_generator_census")
	if len(records) != 1 || !strings.Contains(records[0].Value, "period:16552213ns") {
		t.Fatalf("adjunct census = %+v, want the authoritative period", records)
	}
	// Combined disclosure: windowed sentence + the lite tail clause.
	zh := runtimeTraceSupplementDisclosureText(meta, true)
	if !strings.Contains(zh, "成文前确定性补跑") || !strings.Contains(zh, ";另对全 trace 补跑 VSync/帧节拍发生器轻量普查(event_search·vsync)") {
		t.Fatalf("zh combined disclosure = %q", zh)
	}
	en := runtimeTraceSupplementDisclosureText(meta, false)
	if !strings.Contains(en, "deterministic pre-report re-run") || !strings.Contains(en, "also ran a lightweight whole-trace VSync/frame-pacing generator census") {
		t.Fatalf("en combined disclosure = %q", en)
	}
}

func TestTraceSupplementCensusLiteCensusPresentStaysDark(t *testing.T) {
	// 修复轮 件2 gate pin: a ledger that already carries the census keeps
	// every C-lite lane byte-identical — here the model's own pattern=vsync
	// call minted the full census (SA-F2 件①), window derivation fails, and
	// the lite arm must NOT run (no meta, plain skip).
	ctx := vsyncSAF2CensusLiteContext(t, []string{"VSync"})
	res, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"view":"event_search","pattern":"vsync","pid":59566}`))
	if err != nil || !res.Success {
		t.Fatalf("model-lane event_search failed: %v %s", err, res.Summary)
	}
	if recs := vsyncSAF2ObservationsByPredicate(res, "vsync_generator_census"); len(recs) != 1 {
		t.Fatalf("model-lane call minted %d census records, want 1 (test premise)", len(recs))
	}
	ctx.ToolResults = append(ctx.ToolResults, res)
	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) != 0 || out.SkipReason != "no_typed_window" {
		t.Fatalf("outcome = %+v, want plain no_typed_window skip (census already on the ledger)", out)
	}
	if meta := ctx.Mutable.SystemTraceSupplementMeta(); meta != nil {
		t.Fatalf("census-present run must store no supplement meta, got %+v", meta)
	}
}

func TestTraceSupplementCensusLiteFamiliesPresentPrecedence(t *testing.T) {
	// 修复轮 件2 更新: with every core family present the families_present
	// lane still ARMS the census scan when the census is absent — but on a
	// generator-less capture the scan mints nothing and the healthy no-op
	// verdict stands unchanged (silent, no meta, no disclosure).
	ctx := suppCoreContext(t)
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Keywords = []string{"vsync"}
	suppCoreModelCall(t, ctx, `{"view":"root_cause_rank","pid":200,"time_start":3.0,"time_end":3.2}`)
	suppCoreModelCall(t, ctx, `{"view":"critical_blocking_calls","pid":200,"time_start":3.0,"time_end":3.2}`)
	out := RunTraceQuerySystemSupplement(ctx)
	if out.SkipReason != "families_present" || len(out.Executed) != 0 {
		t.Fatalf("outcome = %+v, want families_present no-op", out)
	}
}

func TestTraceSupplementCensusLiteDisclosureUpsert(t *testing.T) {
	ctx := vsyncSAF2CensusLiteContext(t, []string{"vsync"})
	if out := RunTraceQuerySystemSupplement(ctx); len(out.Executed) != 1 {
		t.Fatalf("C-lite did not execute: %+v", out)
	}
	doc := &types.AnswerDocumentV2{Caveats: []string{"既有注意事项"}}
	if !materializeRuntimeTraceSupplementDisclosureCaveat(doc, ctx) {
		t.Fatal("disclosure caveat not materialized")
	}
	lite := 0
	for _, caveat := range doc.Caveats {
		if strings.HasPrefix(strings.TrimSpace(caveat), runtimeTraceSupplementDisclosurePrefixZH) ||
			strings.HasPrefix(strings.TrimSpace(caveat), runtimeTraceSupplementDisclosurePrefixEN) {
			lite++
		}
	}
	if lite != 1 {
		t.Fatalf("want exactly one supplement disclosure line, got %d in %v", lite, doc.Caveats)
	}
}
