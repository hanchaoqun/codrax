package tool

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func samplingRepairQueryHMC235(t *testing.T, ctx *types.BusContext, selectors map[string]any) types.ToolResult {
	t.Helper()
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
	if err != nil {
		t.Fatal(err)
	}
	p := map[string]any{"source": "path", "path": path, "view": "perf_sampling_tree", "pid": 11, "time_start": 1, "time_end": 2}
	for k, v := range selectors {
		p[k] = v
	}
	raw, _ := json.Marshal(p)
	before := append([]byte(nil), raw...)
	r, err := (&TraceQuery{}).Execute(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, raw) {
		t.Fatal("query mutated caller selectors")
	}
	return r
}

func samplingRepairPayloadHMC235(t *testing.T, r types.ToolResult) (tracequery.Result, string, []byte) {
	t.Helper()
	for _, o := range r.Observations {
		if o.Predicate != TracePerfSamplingTreePredicate {
			continue
		}
		raw, err := os.ReadFile(o.SourceRef.PayloadRef)
		if err != nil {
			t.Fatal(err)
		}
		var payload tracequery.Result
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
		return payload, o.SourceRef.PayloadRef, raw
	}
	t.Fatalf("missing native unavailable/available receipt: %+v", r)
	return tracequery.Result{}, "", nil
}

func TestSamplingTreeUnsupportedSelectorPublicRepairHMC235(t *testing.T) {
	for _, tc := range []struct {
		name      string
		selectors map[string]any
		fields    []string
	}{
		{"pattern", map[string]any{"pattern": "document-worker"}, []string{"pattern"}},
		{"span", map[string]any{"span_name": "work"}, []string{"span_name"}},
		{"events", map[string]any{"event_types": []string{"perf_sample"}}, []string{"event_types"}},
		{"lines", map[string]any{"line_start": 2}, []string{"line_start"}},
		{"combined", map[string]any{"pattern": "work", "line_start": 2, "span_name": "work"}, []string{"line_start", "pattern", "span_name"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("sampling")}
			bad := samplingRepairQueryHMC235(t, ctx, tc.selectors)
			if bad.Success || bad.Repair == nil || bad.Repair.Code != "trace_query_sampling_selector_unsupported" || !reflect.DeepEqual(bad.Repair.Fields, tc.fields) {
				t.Fatalf("unsupported direct tree needs precise actionable failure: %+v", bad)
			}
			if bad.Repair.Metadata["retry_scope"] != "same_source_and_window" || bad.Repair.Metadata["status"] != types.ToolRepairStatusActionRequired {
				t.Fatal("repair lost explicit scope/status")
			}
			bp, badPath, badRaw := samplingRepairPayloadHMC235(t, bad)
			if bp.PerfSamplingTree == nil || bp.PerfSamplingTree.Status != "unavailable" || bp.PerfSamplingTree.Reason != "requires_time_and_thread_selectors_only" {
				t.Fatal("original unavailable reason was replaced")
			}
			good := samplingRepairQueryHMC235(t, ctx, nil)
			if !good.Success || good.Repair != nil {
				t.Fatalf("explicit supported retry failed: %+v", good)
			}
			gp, _, _ := samplingRepairPayloadHMC235(t, good)
			b, g := bp.PerfSamplingTree, gp.PerfSamplingTree
			if g == nil || g.Status != "available" || g.SampleCount != 5 || g.CohortCount != 3 || b.SourcePath != g.SourcePath || b.Window != g.Window || b.TargetPID != g.TargetPID {
				t.Fatalf("retry changed source/window/target/population: %+v", g)
			}
			after, err := os.ReadFile(badPath)
			if err != nil || !bytes.Equal(after, badRaw) {
				t.Fatal("successful retry erased original failure evidence")
			}
		})
	}
}

func TestSamplingTreeRepairPreservesLegacyFilteredViewsHMC235(t *testing.T) {
	for _, view := range []string{"perf_stats", "window_stats", "trace_perf_bundle", "perf_timeline"} {
		t.Run(view, func(t *testing.T) {
			dir := t.TempDir()
			ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("sampling")}
			r := samplingRepairQueryHMC235(t, ctx, map[string]any{"view": view, "pattern": "/app/B.so"})
			if !r.Success || r.Repair != nil {
				t.Fatalf("companion tree failed legitimate filtered statistics: %+v", r)
			}
			p, _, _ := samplingRepairPayloadHMC235(t, r)
			if p.PerfSamplingTree == nil || p.PerfSamplingTree.Reason != "requires_time_and_thread_selectors_only" {
				t.Fatal("companion unavailable evidence was lost")
			}
			count := 0
			switch {
			case p.PerfStats != nil:
				count = p.PerfStats.SampleCount
			case p.WindowStats != nil && p.WindowStats.PerfSamples != nil:
				count = p.WindowStats.PerfSamples.SampleCount
			case p.PerfTimeline != nil:
				for _, b := range p.PerfTimeline.Buckets {
					count += b.SampleCount
				}
			default:
				t.Fatal("missing primary statistics")
			}
			if count != 1 {
				t.Fatalf("valid filter was silently removed: count=%d", count)
			}
		})
	}
}

func TestSamplingTreeRepairPreservesOtherOutcomesHMC235(t *testing.T) {
	base := types.ToolResult{ToolName: "trace_query", Success: true, Summary: "original receipt", RawRef: "original.json"}
	for _, tc := range []struct {
		name, view, status, reason string
		pattern                    bool
	}{
		{"identity", "perf_sampling_tree", "unavailable", "invalid_thread_selector", true},
		{"budget", "perf_sampling_tree", "unavailable", "complete_scan_retention_budget_exceeded", true},
		{"window", "perf_sampling_tree", "unavailable", "requires_finite_window", true},
		{"zero", "perf_sampling_tree", "available", "", false},
		{"no_selector", "perf_sampling_tree", "unavailable", "requires_time_and_thread_selectors_only", false},
		{"companion", "perf_stats", "unavailable", "requires_time_and_thread_selectors_only", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := tracequery.Query{View: tc.view}
			if tc.pattern {
				q.Pattern = "x"
			}
			p := tracequery.Result{PerfSamplingTree: &tracequery.PerfSamplingTreeResult{Status: tc.status, Reason: tc.reason}}
			if got := traceQuerySamplingTreeRepairResult(base, p, q); !reflect.DeepEqual(got, base) {
				t.Fatal("unrelated outcome was rewritten")
			}
		})
	}
	for _, out := range []types.ToolResult{{Success: false, Summary: "cancelled"}, {Success: true, Repair: &types.ToolRepair{Code: "existing"}}} {
		p := tracequery.Result{PerfSamplingTree: &tracequery.PerfSamplingTreeResult{Status: "unavailable", Reason: "requires_time_and_thread_selectors_only"}}
		if got := traceQuerySamplingTreeRepairResult(out, p, tracequery.Query{View: "perf_sampling_tree", Pattern: "x"}); !reflect.DeepEqual(got, out) {
			t.Fatal("prior failure or repair was overwritten")
		}
	}
}

func TestSamplingTreeRepairTeachingSingleSourceHMC235(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&TraceQuery{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	description := schema.Properties["view"].Description
	if strings.Count(description, tracequery.PerfSamplingTreeTeaching) != 1 || !strings.Contains(tracequery.PerfSamplingTreeTeaching, "time windows are supported") || !strings.Contains(tracequery.PerfSamplingTreeTeaching, "Never silently drop") {
		t.Fatal("schema lost single-source selector/retry teaching")
	}
}
