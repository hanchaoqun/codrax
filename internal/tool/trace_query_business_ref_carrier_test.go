package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This matrix first asserts the real native carrier, then checks publication.
// A view name, a nested rank, a frame-shaped summary, or an event label alone
// is not sufficient to mint an exact physical business-instance reference.
func TestTraceQueryBusinessRefPublicCarrierMatrix(t *testing.T) {
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name        string
		params      map[string]any
		stats       bool
		spanWindows int
		wantRefs    int
	}{
		{"window_stats_control", map[string]any{"view": "window_stats"}, true, 0, 2},
		{"root_cause_rank", map[string]any{"view": "root_cause_rank"}, true, 0, 2},
		{"trace_perf_bundle", map[string]any{"view": "trace_perf_bundle"}, true, 0, 2},
		{"frame_root_cause_bundle", map[string]any{"view": "frame_root_cause_bundle"}, true, 0, 2},
		{"perf_stats", map[string]any{"view": "perf_stats"}, true, 0, 2},
		{"evidence_pack", map[string]any{"view": "evidence_pack"}, true, 0, 2},
		{"recipe_io_wait", map[string]any{"view": "recipe", "recipe_name": "io_wait"}, true, 0, 2},
		{"recipe_runnable", map[string]any{"view": "recipe", "recipe_name": "runnable_delay"}, true, 0, 2},
		{"wakeup_stats_default", map[string]any{"view": "wakeup_chain"}, true, 0, 2},
		{"wakeup_stats_true", map[string]any{"view": "wakeup_chain", "include_window_stats": true}, true, 0, 2},
		{"wakeup_stats_false", map[string]any{"view": "wakeup_chain", "include_window_stats": false}, false, 0, 0},
		{"recipe_binder_internal_rank_without_carrier", map[string]any{"view": "recipe", "recipe_name": "binder_wait"}, false, 0, 0},
		{"critical_blocking_without_carrier", map[string]any{"view": "critical_blocking_calls"}, false, 0, 0},
		{"thread_timeline_without_carrier", map[string]any{"view": "thread_timeline"}, false, 0, 0},
		{"thread_timeline_real_span_name", map[string]any{"view": "thread_timeline", "span_name": "OpenDocument"}, false, 1, 1},
		{"perf_timeline_without_carrier", map[string]any{"view": "perf_timeline"}, false, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, path := businessRefTestContext(t, string(body))
			p := map[string]any{"path": path, "pid": 100, "time_start": 1, "time_end": 1.051, "limit": 64}
			for k, v := range tc.params {
				p[k] = v
			}
			result := businessRefTestQuery(t, ctx, p)
			if !result.Success {
				t.Fatalf("public query failed: %s", result.Summary)
			}
			native := carrierNativePayload(t, result, ctx.WorkDir)
			if (native.WindowStats != nil) != tc.stats || len(native.SpanWindows) != tc.spanWindows {
				t.Fatalf("fixture carrier differs from declared matrix: stats=%t spans=%d want stats=%t spans=%d", native.WindowStats != nil, len(native.SpanWindows), tc.stats, tc.spanWindows)
			}
			if tc.stats && len(native.WindowStats.TraceSpans) != 2 {
				t.Fatalf("fixture lost complete native span pair(s): %+v", native.WindowStats.TraceSpans)
			}
			if len(result.TraceBusinessSpanRefs) != tc.wantRefs {
				t.Fatalf("complete native carrier must determine navigation availability: stats=%t SpanWindows=%d refs=%d want=%d", tc.stats, len(native.SpanWindows), len(result.TraceBusinessSpanRefs), tc.wantRefs)
			}
			if tc.wantRefs == 0 {
				if strings.Contains(result.Summary, "business_span_ref=") {
					t.Fatal("empty carrier invented a visible reference")
				}
				repeat := businessRefTestQuery(t, ctx, p)
				if !repeat.Success || !repeat.ReusedFromRunMemo || repeat.RawRef != result.RawRef || !reflect.DeepEqual(repeat.Observations, result.Observations) {
					t.Fatal("non-navigation query lost its existing verbatim pure-memo behavior")
				}
				return
			}
			physical, err := filepath.EvalSymlinks(path)
			if err != nil {
				t.Fatal(err)
			}
			var selected types.TraceBusinessSpanRef
			for _, ref := range result.TraceBusinessSpanRefs {
				d := ref.Data()
				if d.Path != physical || d.Kind != "sync" || !strings.Contains(result.Summary, fmt.Sprintf("business_span_ref=%q", ref.Token())) {
					t.Fatalf("wrong source or invisible native reference: %+v", d)
				}
				if resolved, ok := ctx.Mutable.ResolveTraceBusinessSpanRef(ref.Token()); !ok || !reflect.DeepEqual(resolved.Data(), d) {
					t.Fatal("published reference is not registered with its whole tuple")
				}
				if d.Name == "OpenDocument" {
					selected = ref
				}
			}
			d := selected.Data()
			if d.TID != 100 || d.StartTs != 1 || d.EndTs != 1.05 || d.StartLine != 4 || d.EndLine != 19 {
				t.Fatalf("OpenDocument exact pair changed: %+v", d)
			}
			if status, _ := ctx.Mutable.AcceptedTraceBusinessFocus(); status != types.TraceBusinessFocusNone {
				t.Fatalf("navigation accepted focus: %s", status)
			}
			follow := businessRefTestQuery(t, ctx, map[string]any{"view": "root_cause_rank", "business_span_ref": selected.Token()})
			if !follow.Success {
				t.Fatalf("whole-tuple followup failed: %s", follow.Summary)
			}
			nativeFollow := carrierNativePayload(t, follow, ctx.WorkDir)
			if nativeFollow.TargetWindowStates == nil || nativeFollow.TargetWindowStates.Window.StartTs != 1 || nativeFollow.TargetWindowStates.Window.EndTs != 1.05 {
				t.Fatalf("navigation copied discovery's 51ms window: %+v", nativeFollow.TargetWindowStates)
			}
			if status, _ := ctx.Mutable.AcceptedTraceBusinessFocus(); status != types.TraceBusinessFocusNone {
				t.Fatal("querying a reference promoted it without accepted completion")
			}
			// References require a fresh native read, not replay of a pure memo.
			repeat := businessRefTestQuery(t, ctx, p)
			if !repeat.Success || repeat.ReusedFromRunMemo || len(repeat.TraceBusinessSpanRefs) != tc.wantRefs {
				t.Fatalf("repeat lost read-bound navigation: memo=%t refs=%d", repeat.ReusedFromRunMemo, len(repeat.TraceBusinessSpanRefs))
			}
			if repeat.TraceBusinessSpanRefs[0].Token() == result.TraceBusinessSpanRefs[0].Token() {
				t.Fatal("new publication replayed an old token")
			}
		})
	}
}

func carrierNativePayload(t *testing.T, result types.ToolResult, workDir string) tracequery.Result {
	t.Helper()
	var path string
	for _, obs := range result.Observations {
		if obs.SourceRef.PayloadRef != "" {
			path = obs.SourceRef.PayloadRef
			break
		}
	}
	if path == "" {
		// Empty, honestly non-measured views may carry no observations. The
		// public payload is still written in their isolated query workspace.
		paths, err := filepath.Glob(filepath.Join(workDir, "trace-query-result-*.json"))
		if err != nil {
			t.Fatal(err)
		}
		if len(paths) != 1 {
			t.Fatalf("cannot identify public payload for empty view: %v", paths)
		}
		path = paths[0]
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var native tracequery.Result
	if err := json.Unmarshal(data, &native); err != nil {
		t.Fatal(err)
	}
	return native
}

func TestTraceQueryBusinessRefPublicCarrierKeepsDistinctInstances(t *testing.T) {
	trace := `# tracer: nop
worker-200 (100) [001] .... 1.000000: tracing_mark_write: B|100|Repeat
worker-200 (100) [001] .... 1.010000: tracing_mark_write: B|100|Repeat
other-300 (100) [002] .... 1.020000: tracing_mark_write: B|100|Repeat
worker-200 (100) [001] .... 1.030000: tracing_mark_write: E|100
other-300 (100) [002] .... 1.040000: tracing_mark_write: E|100
worker-200 (100) [001] .... 1.050000: tracing_mark_write: E|100
`
	for _, view := range []string{"root_cause_rank", "trace_perf_bundle", "frame_root_cause_bundle", "evidence_pack"} {
		t.Run(view, func(t *testing.T) {
			ctx, path := businessRefTestContext(t, trace)
			result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": view, "time_start": 1, "time_end": 1.06, "limit": 64})
			if !result.Success {
				t.Fatal(result.Summary)
			}
			native := carrierNativePayload(t, result, ctx.WorkDir)
			if native.WindowStats == nil || len(native.WindowStats.TraceSpans) != 3 {
				t.Fatalf("fixture must retain all native pairs: %+v", native.WindowStats)
			}
			if len(result.TraceBusinessSpanRefs) != 3 {
				t.Fatalf("multi-instance native carrier lost navigation: refs=%d", len(result.TraceBusinessSpanRefs))
			}
			want := map[string]bool{"200/1.000000/1.050000": true, "200/1.010000/1.030000": true, "300/1.020000/1.040000": true}
			tokens := map[string]bool{}
			for _, ref := range result.TraceBusinessSpanRefs {
				d := ref.Data()
				key := fmt.Sprintf("%d/%.6f/%.6f", d.TID, d.StartTs, d.EndTs)
				if !want[key] || tokens[ref.Token()] {
					t.Fatalf("mixed, duplicate or first/longest-selected instance: %+v", d)
				}
				delete(want, key)
				tokens[ref.Token()] = true
			}
			if len(want) != 0 {
				t.Fatalf("lost exact instances: %v", want)
			}
			if status, _ := ctx.Mutable.AcceptedTraceBusinessFocus(); status != types.TraceBusinessFocusNone {
				t.Fatal("multi-instance discovery elected a focus")
			}
		})
	}
}

func TestTraceQueryBusinessRefPublicCarrierDoesNotInventPairs(t *testing.T) {
	trace := `# tracer: nop
worker-200 (100) [001] .... 1.000000: tracing_mark_write: S|100|Async|1
worker-200 (100) [001] .... 1.010000: tracing_mark_write: B|100|Incomplete
other-300 (100) [002] .... 1.050000: tracing_mark_write: F|100|Async|1
`
	for _, view := range []string{"window_stats", "root_cause_rank", "trace_perf_bundle", "frame_root_cause_bundle", "evidence_pack"} {
		t.Run(view, func(t *testing.T) {
			ctx, path := businessRefTestContext(t, trace)
			result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": view, "time_start": 1, "time_end": 1.06, "limit": 64})
			if !result.Success || len(result.TraceBusinessSpanRefs) != 0 {
				t.Fatalf("unpaired or async events granted synchronous instance: success=%t refs=%d %s", result.Success, len(result.TraceBusinessSpanRefs), result.Summary)
			}
		})
	}
}

func TestTraceQueryBusinessRefPublicSyntheticFrameCarrierCannotMint(t *testing.T) {
	trace := `# tracer: nop
app-20 (20) [001] .... 7.000000: print: B|20|Expected Timeline frame=77
app-20 (20) [001] .... 7.004000: print: E|20
app-20 (20) [001] .... 7.005000: print: B|20|Choreographer#doFrame frame=77
app-20 (20) [001] .... 7.016000: print: E|20
RSUniRenderThre-2096 (1716) [000] .... 7.017000: print: B|1716|H:RenderFrame frame=77
RSUniRenderThre-2096 (1716) [000] .... 7.030000: print: E|1716
`
	for _, view := range []string{"frame_window", "frame_timeline", "frame_flow", "render_pipeline"} {
		t.Run(view, func(t *testing.T) {
			ctx, path := businessRefTestContext(t, trace)
			result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": view, "time_start": 7, "time_end": 7.04})
			if !result.Success {
				t.Fatal(result.Summary)
			}
			native := carrierNativePayload(t, result, ctx.WorkDir)
			if native.WindowStats != nil || len(native.SpanWindows) == 0 {
				t.Fatalf("must exercise populated synthetic frame-only carrier: %+v", native)
			}
			for _, span := range native.SpanWindows {
				if span.Kind != "" || span.SourcePath != "" {
					t.Fatalf("fixture unexpectedly carries native source/kind: %+v", span)
				}
			}
			if len(result.TraceBusinessSpanRefs) != 0 {
				t.Fatal("synthetic frame summary granted physical pair authority")
			}
		})
	}
}

func TestTraceQueryBusinessRefPublicCompositeCarrierCannotMint(t *testing.T) {
	ctx, _ := businessRefTestContext(t, "app-20 (20) [001] .... 1.000000: print: B|20|Work\napp-20 (20) [001] .... 1.050000: print: E|20\n")
	perf := filepath.Join(ctx.RepoRoot, "samples.perftrace")
	if err := os.WriteFile(perf, []byte("app-20 (20) [001] .... 1.015000: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=Work dso=libapp.so source=test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(ctx.RepoRoot, "joined.tracebundle.json")
	writeToolTraceBundleV2Fixture(t, manifest, []byte(`{"version":"test","systrace":"instance.systrace","artifacts":[{"type":"systrace","path":"instance.systrace"},{"type":"perftrace","path":"samples.perftrace","perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true}}],"perf_clock_alignments":[{"artifact_path":"samples.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","confidence":"same_domain","calibrated":false}]}`))
	for _, view := range []string{"window_stats", "root_cause_rank", "trace_perf_bundle"} {
		t.Run(view, func(t *testing.T) {
			result := businessRefTestQuery(t, ctx, map[string]any{"path": manifest, "view": view, "pid": 20, "time_start": 1, "time_end": 1.06})
			if !result.Success {
				t.Fatal(result.Summary)
			}
			native := carrierNativePayload(t, result, ctx.WorkDir)
			if len(native.TraceArtifacts) != 2 || native.WindowStats == nil || len(native.WindowStats.TraceSpans) == 0 {
				t.Fatalf("must test real paired span inside a composite result: sources=%d stats=%+v", len(native.TraceArtifacts), native.WindowStats)
			}
			if len(result.TraceBusinessSpanRefs) != 0 {
				t.Fatal("composite coordinates granted a single physical instance")
			}
		})
	}
}

func TestTraceQueryBusinessRefPublicCarrierClippingRetainsExplicitScope(t *testing.T) {
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	ctx, path := businessRefTestContext(t, string(body))
	start, end := 1.01, 1.04
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, SourceQuote: "only this explicit interval", TimeStart: &start, TimeEnd: &end}}}
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "root_cause_rank", "pid": 100, "time_start": start, "time_end": end})
	if !result.Success {
		t.Fatal(result.Summary)
	}
	if len(result.TraceBusinessSpanRefs) == 0 {
		t.Fatal("actual full pair should remain available as navigation, despite clipped stats")
	}
	var ref types.TraceBusinessSpanRef
	for _, candidate := range result.TraceBusinessSpanRefs {
		if candidate.Data().Name == "OpenDocument" {
			ref = candidate
		}
	}
	if ref.Data().StartTs != 1 || ref.Data().EndTs != 1.05 {
		t.Fatalf("clipped interval was relabeled complete: %+v", ref.Data())
	}
	denied := businessRefTestQuery(t, ctx, map[string]any{"view": "window_stats", "business_span_ref": ref.Token()})
	if denied.Success || !strings.Contains(denied.Summary, "explicitly requested") {
		t.Fatalf("navigation overrode user window: %+v", denied)
	}
}

func TestTraceQueryBusinessRefCarrierNativeQualificationBoundary(t *testing.T) {
	ctx, path := businessRefTestContext(t, "worker-200 (100) [001] .... 1.000000: tracing_mark_write: B|100|Work\nworker-200 (100) [001] .... 1.050000: tracing_mark_write: E|100\n")
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	native := tracequery.Run(idx, tracequery.Query{View: "root_cause_rank", TimeStart: 1, TimeEnd: 1.06, Limit: 64})
	if native.WindowStats == nil || len(native.WindowStats.TraceSpans) != 1 {
		t.Fatal("native exact-pair fixture lost")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*tracequery.Result)
	}{
		{"virtual_source", func(r *tracequery.Result) { r.TraceArtifacts[0].VirtualLineBase = 100 }},
		{"different_physical_source", func(r *tracequery.Result) { r.TraceArtifacts[0].SourcePath += ".other" }},
		{"missing_sources", func(r *tracequery.Result) { r.TraceArtifacts = nil }},
		{"native_kind_not_sync", func(r *tracequery.Result) { r.WindowStats.TraceSpans[0].Kind = "async" }},
		{"native_source_unknown", func(r *tracequery.Result) { r.WindowStats.TraceSpans[0].SourcePath = "" }},
		{"unpaired_line", func(r *tracequery.Result) {
			r.WindowStats.TraceSpans[0].EndLine = r.WindowStats.TraceSpans[0].StartLine
		}},
		{"canceled", func(r *tracequery.Result) {
			r.ViewCancellation = &tracequery.ViewCancellation{View: "root_cause_rank", Reason: "canceled", DiscardedFaces: []string{"window_stats"}}
		}},
		{"lifecycle_suppression", func(r *tracequery.Result) {
			r.LifecycleSuppressions = []tracequery.TraceLifecycleSuppression{{ConflictTID: 200, Signal: "sched_process_exit", BoundaryTs: 1.02}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, _ := json.Marshal(native)
			var copy tracequery.Result
			if err := json.Unmarshal(data, &copy); err != nil {
				t.Fatal(err)
			}
			tc.mutate(&copy)
			if got := traceQueryBusinessSpanCandidates(traceQueryParams{View: "root_cause_rank"}, copy); len(got) != 0 {
				t.Fatalf("qualified absent witness: %+v", got)
			}
		})
	}
	// Publishing a native candidate without a current source-read receipt
	// cannot mint a reference, even when its path happens to exist.
	forged := types.ToolResult{ToolName: "trace_query", Success: true, TraceBusinessSpanCandidates: traceQueryBusinessSpanCandidates(traceQueryParams{View: "root_cause_rank"}, native)}
	ctx.Mutable.StampTraceBusinessSpanRefs(&forged)
	if len(forged.TraceBusinessSpanRefs) != 0 {
		t.Fatal("unwitnessed native-looking candidate minted a reference")
	}
}

func TestTraceQueryBusinessRefPublicMultiWindowCarrier(t *testing.T) {
	old := traceQueryWindowedIndexMinBytes
	traceQueryWindowedIndexMinBytes = 1
	t.Cleanup(func() { traceQueryWindowedIndexMinBytes = old })
	for _, tc := range []struct {
		name          string
		second        float64
		wantDuplicate bool
	}{
		{"separate_instances", 9, false},
		{"overlapping_discovery_windows", 5.2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf("app-20 (20) [000] .... 5.000000: print: B|20|Repeat\napp-20 (20) [000] .... 5.020000: print: E|20\napp-20 (20) [000] .... %.6f: print: B|20|Repeat\napp-20 (20) [000] .... %.6f: print: E|20\n", tc.second, tc.second+.03)
			ctx, path := businessRefTestContext(t, body)
			result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "trace_perf_bundle", "pattern": "Repeat"})
			if !result.Success {
				t.Fatal(result.Summary)
			}
			var payload struct {
				Mode    string                      `json:"mode"`
				Results []traceQueryAutoWindowChild `json:"results"`
			}
			var raw []byte
			for _, obs := range result.Observations {
				if obs.SourceRef.PayloadRef != "" {
					var err error
					raw, err = os.ReadFile(obs.SourceRef.PayloadRef)
					if err != nil {
						t.Fatal(err)
					}
					break
				}
			}
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Mode != "large_trace_pattern_auto_windows" || len(payload.Results) != 2 {
				t.Fatalf("must exercise real multiwindow wrapper: mode=%s children=%d", payload.Mode, len(payload.Results))
			}
			spanCount := 0
			for _, child := range payload.Results {
				if child.Error != "" || child.Result.WindowStats == nil {
					t.Fatalf("fixture child missing native stats: %+v", child)
				}
				spanCount += len(child.Result.WindowStats.TraceSpans)
			}
			if (spanCount > 2) != tc.wantDuplicate {
				t.Fatalf("fixture duplicate shape mismatch: child_spans=%d", spanCount)
			}
			if len(result.TraceBusinessSpanRefs) != 2 {
				t.Fatalf("multiwindow native carrier lost exact instances: children=%d native_spans=%d refs=%d", len(payload.Results), spanCount, len(result.TraceBusinessSpanRefs))
			}
			want := map[string]bool{fmt.Sprintf("%.6f..%.6f", 5.0, 5.02): true, fmt.Sprintf("%.6f..%.6f", tc.second, tc.second+.03): true}
			for _, ref := range result.TraceBusinessSpanRefs {
				d := ref.Data()
				key := fmt.Sprintf("%.6f..%.6f", d.StartTs, d.EndTs)
				if d.TID != 20 || !want[key] {
					t.Fatalf("wrapper selected/unioned/clipped an instance: %+v", d)
				}
				delete(want, key)
				if _, ok := ctx.Mutable.ResolveTraceBusinessSpanRef(ref.Token()); !ok {
					t.Fatal("multiwindow ref not current")
				}
			}
			if len(want) > 0 {
				t.Fatalf("wrapper lost instance: %+v", want)
			}
			if status, _ := ctx.Mutable.AcceptedTraceBusinessFocus(); status != types.TraceBusinessFocusNone {
				t.Fatal("multiwindow discovery chose a focus")
			}
		})
	}
}

// Public native reads supply the exact result/observation/source witnesses;
// mutations below exercise the unpublished per-child rejection boundary,
// not a fabricated causal fact or a live race during file IO.
func TestTraceQueryBusinessRefCarrierBatchReceiptsRemainAtomic(t *testing.T) {
	const body = "worker-200 (100) [001] .... 1.000000: tracing_mark_write: B|100|Work\nworker-200 (100) [001] .... 1.050000: tracing_mark_write: E|100\n"
	ctx, path := businessRefTestContext(t, body)
	otherCtx, otherPath := businessRefTestContext(t, body)
	otherCtx.Mutable = ctx.Mutable
	read := func(ctx *types.BusContext, path string) (types.ToolResult, tracequery.Result) {
		r := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "root_cause_rank", "time_start": 1, "time_end": 1.06})
		if !r.Success || len(r.TraceBusinessSpanRefs) != 1 {
			t.Fatalf("public single-source setup failed: %+v", r)
		}
		return r, carrierNativePayload(t, r, ctx.WorkDir)
	}
	first, native := read(ctx, path)
	other, foreign := read(otherCtx, otherPath)
	clone := func() tracequery.Result {
		data, _ := json.Marshal(native)
		var r tracequery.Result
		if err := json.Unmarshal(data, &r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	for _, tc := range []struct {
		name           string
		mutate         func(*tracequery.Result)
		wantCandidates int
		wantRefs       int
	}{
		{"same_source_exact_duplicate", func(*tracequery.Result) {}, 1, 1},
		{"same_source_canceled_child", func(r *tracequery.Result) {
			r.WindowStats.TraceSpans[0].Name = "unpublished"
			r.ViewCancellation = &tracequery.ViewCancellation{View: r.View, Reason: "canceled", DiscardedFaces: []string{"window_stats"}}
		}, 1, 1},
		{"same_source_lifecycle_child", func(r *tracequery.Result) {
			r.WindowStats.TraceSpans[0].Name = "unpublished"
			r.LifecycleSuppressions = []tracequery.TraceLifecycleSuppression{{ConflictTID: 200, BoundaryTs: 1.02}}
		}, 1, 1},
		{"foreign_child_rejects_whole_source_receipt", func(r *tracequery.Result) { *r = foreign }, 2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			child := clone()
			tc.mutate(&child)
			receipt := ctx.Mutable.PrepareTraceQuerySourceRead(path)
			joined := types.ToolResult{ToolName: "trace_query", Success: true,
				Observations:                append(append([]types.ObservationRecord(nil), first.Observations...), other.Observations...),
				TraceQuerySourceRead:        traceQuerySourceReadCandidate(native, child),
				TraceBusinessSpanCandidates: traceQueryBusinessSpanCandidates(traceQueryParams{}, native, child)}
			if len(joined.TraceBusinessSpanCandidates) != tc.wantCandidates {
				t.Fatalf("per-child eligibility or exact dedup changed: candidates=%+v", joined.TraceBusinessSpanCandidates)
			}
			ctx.Mutable.StampTraceQuerySourceRead(receipt, &joined)
			ctx.Mutable.StampTraceBusinessSpanRefs(&joined)
			if len(joined.TraceBusinessSpanRefs) != tc.wantRefs {
				t.Fatalf("joined source receipt changed privilege: refs=%d want=%d", len(joined.TraceBusinessSpanRefs), tc.wantRefs)
			}
		})
	}
	for _, mode := range []string{"failed_result", "memo_replay", "stale_file", "new_run"} {
		t.Run(mode, func(t *testing.T) {
			local, localPath := businessRefTestContext(t, body)
			actual, native := read(local, localPath)
			receipt := local.Mutable.PrepareTraceQuerySourceRead(localPath)
			out := types.ToolResult{ToolName: "trace_query", Success: true, Observations: actual.Observations,
				TraceQuerySourceRead: traceQuerySourceReadCandidate(native), TraceBusinessSpanCandidates: traceQueryBusinessSpanCandidates(traceQueryParams{}, native)}
			switch mode {
			case "failed_result":
				out.Success = false
			case "memo_replay":
				out.ReusedFromRunMemo = true
			case "stale_file":
				if err := os.WriteFile(localPath, []byte(body+"# changed after native read\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "new_run":
				local.Mutable.ResetTurnAArtifacts()
			}
			local.Mutable.StampTraceQuerySourceRead(receipt, &out)
			local.Mutable.StampTraceBusinessSpanRefs(&out)
			if len(out.TraceBusinessSpanRefs) != 0 {
				t.Fatal("failed, memo, stale or another-run payload granted a new reference")
			}
		})
	}
}

func TestTraceQueryBusinessRefAutoWindowFailedChildrenCannotMint(t *testing.T) {
	ctx, path := businessRefTestContext(t, "worker-200 (100) [001] .... 1.000000: tracing_mark_write: B|100|Work\nworker-200 (100) [001] .... 1.050000: tracing_mark_write: E|100\n")
	dead, cancel := context.WithCancel(context.Background())
	cancel()
	ctx.Ctx = dead
	result := (&TraceQuery{}).runAutoWindowCandidates(ctx, traceQueryParams{View: "trace_perf_bundle"}, path, "path", "canceled-child-regression", []traceQueryAutoWindowCandidate{{Rank: 1, Start: 1, End: 1.06}, {Rank: 2, Start: 2, End: 2.06}}, "")
	if len(result.TraceBusinessSpanCandidates) != 0 || len(result.TraceBusinessSpanRefs) != 0 || len(result.Observations) != 0 {
		t.Fatalf("failed parse children published native instance authority: %+v", result)
	}
	paths, err := filepath.Glob(filepath.Join(ctx.WorkDir, "trace-query-auto-windows-*.json"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("missing actual failure wrapper: paths=%v err=%v", paths, err)
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Results []traceQueryAutoWindowChild `json:"results"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Results) != 2 {
		t.Fatalf("expected two failed children, got %d", len(payload.Results))
	}
	for _, child := range payload.Results {
		if child.Error == "" {
			t.Fatalf("fixture must exercise actual failed-child path: %+v", child)
		}
	}
}
