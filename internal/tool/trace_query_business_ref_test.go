package tool

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryBusinessRefCarriesOneExactInstance(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("business instance navigation")}
	run := func(p map[string]any) types.ToolResult {
		t.Helper()
		raw, _ := json.Marshal(p)
		r, err := (&TraceQuery{}).Execute(ctx, raw)
		if err != nil || !r.Success {
			t.Fatalf("query failed: %v %s", err, r.Summary)
		}
		ctx.Mutable.AppendDispatchToolResult(r)
		return r
	}
	discovery := run(map[string]any{"path": path, "view": "span_window", "span_name": "OpenDocument"})
	match := regexp.MustCompile(`business_span_ref="([^"]+)"`).FindStringSubmatch(discovery.Summary)
	if len(match) != 2 {
		t.Fatalf("paired span must publish an atomic query reference: %s", discovery.Summary)
	}
	got := run(map[string]any{"business_span_ref": match[1], "view": "root_cause_rank"})
	var result tracequery.Result
	for _, obs := range got.Observations {
		if obs.SourceRef.PayloadRef == "" {
			continue
		}
		data, err := os.ReadFile(obs.SourceRef.PayloadRef)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		break
	}
	if result.TargetWindowStates == nil {
		t.Fatalf("missing measured target state account: %s", got.Summary)
	}
	data, _ := json.Marshal(result.TargetWindowStates)
	if result.TargetWindowStates.Window.StartTs != 1 || result.TargetWindowStates.Window.EndTs != 1.05 {
		t.Fatalf("mixed business and discovery bounds: %+v", result.TargetWindowStates.Window)
	}
	if math.Abs(result.TargetWindowStates.RunningMs-5) > 1e-6 {
		t.Fatalf("expected all 5ms inside exact 50ms response: %s", data)
	}
}

func businessRefTestContext(t *testing.T, trace string) (*types.BusContext, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "instance.systrace")
	if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	return &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("instance reference regression")}, path
}

func businessRefTestQuery(t *testing.T, ctx *types.BusContext, p map[string]any) types.ToolResult {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	r, err := (&TraceQuery{}).Execute(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if r.Success {
		ctx.Mutable.AppendDispatchToolResult(r)
	}
	return r
}

func TestTraceQueryBusinessRefDistinguishesNestedAndCrossThreadInstances(t *testing.T) {
	ctx, path := businessRefTestContext(t, `# tracer: nop
worker-200 (100) [001] .... 1.000000: tracing_mark_write: B|100|Repeat
worker-200 (100) [001] .... 1.010000: tracing_mark_write: B|100|Repeat
other-300 (100) [002] .... 1.020000: tracing_mark_write: B|100|Repeat
worker-200 (100) [001] .... 1.030000: tracing_mark_write: E|100
other-300 (100) [002] .... 1.040000: tracing_mark_write: E|100
worker-200 (100) [001] .... 1.050000: tracing_mark_write: E|100
`)
	r := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "span_window", "span_name": "Repeat", "pid": 100, "target_scope": "process"})
	if !r.Success || len(r.TraceBusinessSpanRefs) != 3 {
		t.Fatalf("expected three independent physical instances: %+v", r)
	}
	seen := map[string]bool{}
	for _, ref := range r.TraceBusinessSpanRefs {
		if seen[ref.Token()] {
			t.Fatal("different instances share a navigation token")
		}
		seen[ref.Token()] = true
		d := ref.Data()
		if d.TID == 100 || (d.TID != 200 && d.TID != 300) {
			t.Fatalf("marker process became scheduler TID: %+v", d)
		}
		p, resolved, reject := traceQueryApplyBusinessRef(ctx, traceQueryParams{BusinessSpanRef: ref.Token(), View: "window_stats"})
		if reject != nil || resolved.Token() != ref.Token() || p.PID.Int() != d.TID || p.TimeStart.Seconds() != d.StartTs || p.TimeEnd.Seconds() != d.EndTs {
			t.Fatalf("atomic instance changed: %+v %+v", p, reject)
		}
	}
	// Merely returning several references never elects a target or a window.
	if len(ctx.Mutable.TraceQueryCallWindows()) != 0 {
		t.Fatal("discovery autonomously selected a business window")
	}
}

func TestTraceQueryBusinessRefRejectsMixedCoordinatesAndPreservesExplicitWindow(t *testing.T) {
	ctx, path := businessRefTestContext(t, "# tracer: nop\nworker-200 (100) [001] .... 1.000000: tracing_mark_write: B|100|Work\nworker-200 (100) [001] .... 1.050000: tracing_mark_write: E|100\n")
	r := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "span_window", "span_name": "Work"})
	if len(r.TraceBusinessSpanRefs) != 1 {
		t.Fatalf("missing discovery: %+v", r)
	}
	token := r.TraceBusinessSpanRefs[0].Token()
	for key, value := range map[string]any{"path": path, "source": "attached_trace", "pid": 300, "thread": "other", "target_scope": "process", "time_start": 1.01, "time_end": 1.06, "line_start": 2, "line_end": 3, "span_name": "Other"} {
		t.Run(key, func(t *testing.T) {
			got := businessRefTestQuery(t, ctx, map[string]any{"view": "window_stats", "business_span_ref": token, key: value})
			if got.Success || !strings.Contains(got.Summary, "do not combine") {
				t.Fatalf("mixed coordinate accepted: %+v", got)
			}
		})
	}
	start, end := 1.01, 1.04
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow, SourceQuote: "explicit window", TimeStart: &start, TimeEnd: &end,
	}}}
	got := businessRefTestQuery(t, ctx, map[string]any{"view": "window_stats", "business_span_ref": token})
	if got.Success || !strings.Contains(got.Summary, "explicitly requested") {
		t.Fatalf("full span overrode narrower explicit window: %+v", got)
	}
	// Restored sessions can retain the request model only in MutableState.
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	ctx.AnalysisIR = nil
	got = businessRefTestQuery(t, ctx, map[string]any{"view": "window_stats", "business_span_ref": token})
	if got.Success || !strings.Contains(got.Summary, "explicitly requested") {
		t.Fatalf("restored explicit window was overridden: success=%t %s", got.Success, got.Summary)
	}
	ordinary := businessRefTestQuery(t, ctx, map[string]any{"view": "window_stats", "path": path, "pid": 200, "time_start": start, "time_end": end})
	if !ordinary.Success {
		t.Fatalf("ordinary explicit window was affected: %+v", ordinary)
	}
	start, end = .99, 1.06
	ctx.Mutable.SetRequestModel(types.RequestModel{RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow, SourceQuote: "explicit window", TimeStart: &start, TimeEnd: &end,
	}})
	got = businessRefTestQuery(t, ctx, map[string]any{"view": "window_stats", "business_span_ref": token})
	if !got.Success {
		t.Fatalf("contained instance drilldown rejected: %+v", got)
	}
	// A paired marker may have no scheduler or IPC measurements. Navigation
	// must preserve that honest empty result, not turn it into an emit failure.
	for _, view := range []string{"thread_timeline", "ipc_graph", "interaction_stats", "wakeup_chain"} {
		zero := businessRefTestQuery(t, ctx, map[string]any{"view": view, "business_span_ref": token})
		if !zero.Success {
			t.Fatalf("empty %s was rejected by navigation: %+v", view, zero)
		}
	}
	ctx.Mutable.ResetTurnAArtifacts()
	got = businessRefTestQuery(t, ctx, map[string]any{"view": "window_stats", "business_span_ref": token})
	if got.Success {
		t.Fatal("reference survived reset into another run")
	}
}

func TestTraceQueryBusinessRefDoesNotInferAsyncExecutionOrUnpairedEnds(t *testing.T) {
	ctx, path := businessRefTestContext(t, `# tracer: nop
worker-200 (100) [001] .... 1.000000: tracing_mark_write: S|100|Async|1
worker-200 (100) [001] .... 1.010000: tracing_mark_write: B|100|Incomplete
other-300 (100) [002] .... 1.050000: tracing_mark_write: F|100|Async|1
`)
	for _, name := range []string{"Async", "Incomplete"} {
		r := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "span_window", "span_name": name})
		if !r.Success || len(r.TraceBusinessSpanRefs) != 0 {
			t.Fatalf("invented executing thread or closing endpoint: %+v", r)
		}
	}
	got := businessRefTestQuery(t, ctx, map[string]any{"view": "window_stats", "business_span_ref": "business-span:forged"})
	if got.Success {
		t.Fatal("unknown reference granted coordinates")
	}
}

func TestTraceQueryBusinessRefKeepsFullPairAndCompositeBoundary(t *testing.T) {
	path := "/capture/one.systrace"
	r := tracequery.Result{SourcePath: path, TraceArtifacts: []tracequery.TraceArtifactSource{{SourcePath: path}}, WindowStats: &tracequery.WindowStats{
		TraceSpans: []tracequery.TraceSpanSummary{{SourcePath: path, Kind: "sync", Thread: tracequery.ThreadRef{Comm: "worker", PID: 200}, Name: "Work", StartLine: 2, EndLine: 7, StartTs: 1.01, EndTs: 1.04, ActualStartTs: 1, ActualEndTs: 1.05}},
	}}
	p := traceQueryParams{View: "window_stats"}
	got := traceQueryBusinessSpanCandidates(p, r)
	if len(got) != 1 || got[0].StartTs != 1 || got[0].EndTs != 1.05 {
		t.Fatalf("clipped window relabeled as complete pair: %+v", got)
	}
	r.TraceArtifacts[0].VirtualLineBase = 10
	if len(traceQueryBusinessSpanCandidates(p, r)) != 0 {
		t.Fatal("virtual composite coordinates granted physical-instance authority")
	}
}

func TestTraceQueryBusinessRefDiscoveryDoesNotMintFromMemo(t *testing.T) {
	ctx, path := businessRefTestContext(t, "# tracer: nop\nworker-200 (100) [001] .... 1.000000: tracing_mark_write: B|100|Work\nworker-200 (100) [001] .... 1.050000: tracing_mark_write: E|100\n")
	for _, params := range []map[string]any{
		{"path": path, "view": "span_window", "span_name": "Work"},
		{"path": path, "view": "window_stats", "pid": 200, "time_start": 1, "time_end": 1.05},
		{"path": path, "view": "recipe", "recipe_name": "span_locate", "span_name": "Work"},
	} {
		first := businessRefTestQuery(t, ctx, params)
		second := businessRefTestQuery(t, ctx, params)
		if !first.Success || !second.Success || second.ReusedFromRunMemo || len(first.TraceBusinessSpanRefs) != 1 || len(second.TraceBusinessSpanRefs) != 1 {
			t.Fatalf("discovery must perform a fresh source read: firstRefs=%d secondRefs=%d memo=%t %s", len(first.TraceBusinessSpanRefs), len(second.TraceBusinessSpanRefs), second.ReusedFromRunMemo, second.Summary)
		}
		if first.TraceBusinessSpanRefs[0].Token() == second.TraceBusinessSpanRefs[0].Token() {
			t.Fatal("navigation token was replayed from pure-result memo")
		}
	}
}

func TestTraceQueryBusinessRefRejectedPromotionDoesNotSeedSupplement(t *testing.T) {
	ctx, path := businessRefTestContext(t, "# tracer: nop\nworker-200 (100) [001] .... 1.000000: tracing_mark_write: B|100|Work\nworker-200 (100) [001] .... 1.050000: tracing_mark_write: E|100\n")
	ctx.AnalysisIR = &types.AnalysisIR{}
	r := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "span_window", "span_name": "Work"})
	if len(r.TraceBusinessSpanRefs) != 1 {
		t.Fatalf("missing reference: %+v", r)
	}
	manifest := strings.TrimSuffix(path, ".systrace") + ".tracebundle.json"
	writeToolTraceBundleV2Fixture(t, manifest, []byte(`{"version":"test","systrace":"instance.systrace","artifacts":[{"type":"systrace","path":"instance.systrace"}]}`))
	idx, err := tracequery.BuildIndex(context.Background(), path)
	physicalManifest, pathErr := filepath.EvalSymlinks(manifest)
	if err != nil || pathErr != nil || idx.Path != physicalManifest {
		t.Fatalf("fixture did not promote to validated sibling: index=%+v err=%v pathErr=%v", idx, err, pathErr)
	}
	got := businessRefTestQuery(t, ctx, map[string]any{"view": "window_stats", "business_span_ref": r.TraceBusinessSpanRefs[0].Token()})
	if got.Success || !strings.Contains(got.Summary, "single physical capture") {
		t.Fatalf("changed source universe accepted: success=%t %s", got.Success, got.Summary)
	}
	if len(ctx.AnalysisIR.RequestModel.RuntimeTargets) != 0 || len(ctx.Mutable.TraceQueryCallWindows()) != 0 {
		t.Fatalf("rejected reference seeded automatic supplement: targets=%+v windows=%+v", ctx.AnalysisIR.RequestModel.RuntimeTargets, ctx.Mutable.TraceQueryCallWindows())
	}
}
