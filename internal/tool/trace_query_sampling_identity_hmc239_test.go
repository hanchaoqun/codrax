package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func samplingIdentityQueryHMC239(t *testing.T, path, view string, selector map[string]any) (types.ToolResult, tracequery.Result) {
	t.Helper()
	dir := t.TempDir()
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("selected samples")}
	params := map[string]any{"source": "path", "path": path, "view": view, "time_start": 1, "time_end": 2}
	for k, v := range selector {
		params[k] = v
	}
	raw, _ := json.Marshal(params)
	result, err := (&TraceQuery{}).Execute(ctx, raw)
	if err != nil || !result.Success || result.Repair != nil {
		t.Fatalf("honest population availability must not force tool retry: %v %+v", err, result)
	}
	payload, _, _ := samplingRepairPayloadHMC235(t, result)
	return result, payload
}

func assertSamplingIdentityPayloadHMC239(t *testing.T, p tracequery.Result, status string, count int) {
	t.Helper()
	if p.PerfSamplingTree == nil || p.PerfSamplingTree.Status != status || p.PerfSamplingTree.SampleCount != count {
		t.Fatalf("wrong selected population: %+v", p.PerfSamplingTree)
	}
	if status != "unavailable" {
		return
	}
	if p.PerfSamplingTree.Reason != "typed_identity_unproven" {
		t.Fatalf("lost unknown identity boundary: %+v", p.PerfSamplingTree)
	}
	for _, flat := range []*tracequery.PerfContext{p.PerfStats, func() *tracequery.PerfContext {
		if p.WindowStats != nil {
			return p.WindowStats.PerfSamples
		}
		return nil
	}()} {
		if flat != nil && (flat.SampleCount != 0 || len(flat.Cohorts) != 0 || !strings.Contains(strings.Join(flat.Caveats, " "), "typed_identity_unproven")) {
			t.Fatalf("legacy statistics present unknown as a complete denominator: %+v", flat)
		}
	}
	if timeline := p.PerfTimeline; timeline != nil && (len(timeline.Buckets) != 0 || !strings.Contains(strings.Join(timeline.Caveats, " "), "typed_identity_unproven") || strings.Contains(strings.Join(timeline.Caveats, " "), "no perf_sample events matched")) {
		t.Fatalf("timeline contradicts unknown selected population: %+v", timeline)
	}
}

func TestSamplingUnknownIdentityPublicHMC239(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unknown.perftrace")
	const data = "worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 thread_comm=worker thread_identity_known=false sample_weight=7 event=cycles symbol=work source=raw\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	for _, view := range samplingSelectionViewsHMC235 {
		t.Run(view, func(t *testing.T) {
			dir := t.TempDir()
			ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("selected samples")}
			raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": 11, "time_start": 1, "time_end": 2})
			result, err := (&TraceQuery{}).Execute(ctx, raw)
			if err != nil || !result.Success || result.Repair != nil {
				t.Fatalf("unknown population is an honest result, not a failed tool invocation: %v %+v", err, result)
			}
			payload, _, _ := samplingRepairPayloadHMC235(t, result)
			p := payload.PerfSamplingTree
			if p == nil || p.Status != "unavailable" || p.Reason != "typed_identity_unproven" {
				t.Errorf("source-only unknown published as measured zero: %+v", p)
			}
			for _, record := range result.Observations {
				if record.Predicate != TracePerfSamplingTreePredicate {
					continue
				}
				bound, ok := DecodeTracePerfSamplingTree(record)
				if !ok || !strings.Contains(TracePerfSamplingTreeText(bound), "status=unavailable reason=typed_identity_unproven") {
					t.Errorf("finalizer native fact presents unknown as available zero: bound=%t text=%s", ok, TracePerfSamplingTreeText(bound))
				}
				if _, selected := TraceQueryAggregateSelectionSubject(record); selected {
					t.Error("unknown identity acquired selected target authority")
				}
			}
		})
	}
}

func TestSamplingIdentityPopulationPublicMatrixHMC239(t *testing.T) {
	const known = "worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 sample_weight=3 event=cycles symbol=work source=raw\n"
	const unknown = "placeholder-999 (99) [000] .... 1.3: perf_sample: pid=99 tid=999 thread_identity_known=false sample_weight=7 event=cycles symbol=unassigned source=raw\n"
	for _, tc := range []struct {
		name, data, status string
		selector           map[string]any
		count              int
	}{
		{"mixed_same_source", known + unknown, "unavailable", map[string]any{"pid": 11}, 0},
		{"mixed_same_name", known + unknown, "unavailable", map[string]any{"thread": "worker"}, 0},
		{"unknown_header_cannot_exclude", unknown, "unavailable", map[string]any{"pid": 11}, 0},
		{"known_other_is_zero", known, "available", map[string]any{"pid": 12}, 0},
		{"known_target_outside_is_zero", strings.Replace(known, "1.1:", "2.1:", 1), "available", map[string]any{"pid": 11}, 0},
		{"true_empty_window", unknown, "available", map[string]any{"pid": 11, "time_start": 3, "time_end": 4}, 0},
		{"unknown_outside", known + strings.Replace(unknown, "1.3:", "2.3:", 1), "available", map[string]any{"pid": 11}, 1},
		{"unknown_right_boundary", known + strings.Replace(unknown, "1.3:", "2.0:", 1), "available", map[string]any{"pid": 11}, 1},
		{"source_wide_preserves_unknown", known + unknown, "available", nil, 2},
		{"same_name_distinct_tid", known + strings.ReplaceAll(known, "11", "12"), "available", map[string]any{"thread": "worker"}, 2},
		{"conflicting_target", known + strings.Replace(known, "pid=10", "pid=20", 1), "unavailable", map[string]any{"pid": 11}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "samples.perftrace")
			if err := os.WriteFile(path, []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			for _, view := range samplingSelectionViewsHMC235 {
				t.Run(view, func(t *testing.T) {
					_, payload := samplingIdentityQueryHMC239(t, path, view, tc.selector)
					assertSamplingIdentityPayloadHMC239(t, payload, tc.status, tc.count)
					if tc.name == "source_wide_preserves_unknown" {
						weights := map[int64]bool{}
						for _, c := range payload.PerfSamplingTree.Cohorts {
							weights[c.TotalWeight] = true
						}
						if len(weights) != 2 || !weights[3] || !weights[7] {
							t.Fatalf("anonymous source inventory lost independent weights: %+v", payload.PerfSamplingTree)
						}
					}
				})
			}
		})
	}
}

func TestSamplingIdentityPhysicalSourceIsolationPublicHMC239(t *testing.T) {
	dir := t.TempDir()
	write := func(name, data string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	known := write("known.perftrace", "worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 sample_weight=3 event=cycles symbol=work source=raw\n"+
		"worker-11 (10) [000] .... 3.1: perf_sample: pid=10 tid=11 sample_weight=5 event=cycles symbol=work source=raw\n")
	unknown := write("unknown.perftrace", "worker-11 (10) [000] .... 1.2: perf_sample: pid=10 tid=11 thread_identity_known=false sample_weight=7 event=cycles symbol=unassigned source=raw\n"+
		"worker-11 (10) [000] .... 2.2: perf_sample: pid=10 tid=11 thread_identity_known=false sample_weight=11 event=cycles symbol=unassigned source=raw\n")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	writeToolTraceBundleV2Fixture(t, bundle, []byte(`{"version":"test","artifacts":[
 {"type":"perftrace","path":"known.perftrace","perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true,"thread_identity":"sample_pid_tid_thread_comm"}},
 {"type":"perftrace","path":"unknown.perftrace","perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true,"thread_identity":"sample_pid_tid_thread_comm"}}
 ],"perf_clock_alignments":[
 {"artifact_path":"known.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","confidence":"same_domain","calibrated":false},
 {"artifact_path":"unknown.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","confidence":"same_domain","calibrated":false}]}`))
	for _, view := range samplingSelectionViewsHMC235 {
		t.Run(view, func(t *testing.T) {
			_, p := samplingIdentityQueryHMC239(t, bundle, view, map[string]any{"pid": 11})
			assertSamplingIdentityPayloadHMC239(t, p, "available", 1)
			if !strings.Contains(strings.Join(p.PerfSamplingTree.Caveats, " "), "sampling_identity_scope=proven_physical_sources") ||
				p.PerfSamplingTree.Cohorts[0].TotalWeight != 3 || filepath.Base(p.PerfSamplingTree.Cohorts[0].SourcePath) != filepath.Base(known) {
				t.Fatalf("different source poisoned or silently broadened proved target: %+v", p.PerfSamplingTree)
			}
			_, p = samplingIdentityQueryHMC239(t, bundle, view, map[string]any{"pid": 11, "time_start": 2, "time_end": 3})
			assertSamplingIdentityPayloadHMC239(t, p, "available", 0)
			if !strings.Contains(strings.Join(p.PerfSamplingTree.Caveats, " "), "sampling_identity_scope=proven_physical_sources") {
				t.Fatal("known target's empty window silently claimed capture-wide absence")
			}
			var flat *tracequery.PerfContext
			if view == "perf_stats" {
				flat = p.PerfStats
			} else if view == "window_stats" || view == "trace_perf_bundle" {
				flat = p.WindowStats.PerfSamples
			}
			if view != "perf_sampling_tree" && view != "perf_timeline" && (flat == nil || !strings.Contains(strings.Join(flat.Caveats, " "), "sampling_identity_scope=proven_physical_sources")) {
				t.Fatalf("zero-valued legacy context dropped its physical-source coverage: %+v", flat)
			}
			if p.PerfTimeline != nil && !strings.Contains(strings.Join(p.PerfTimeline.Caveats, " "), "sampling_identity_scope=proven_physical_sources") {
				t.Fatal("empty timeline dropped physical-source coverage")
			}
			_, p = samplingIdentityQueryHMC239(t, unknown, view, map[string]any{"pid": 11})
			assertSamplingIdentityPayloadHMC239(t, p, "unavailable", 0)
			_, p = samplingIdentityQueryHMC239(t, bundle, view, nil)
			assertSamplingIdentityPayloadHMC239(t, p, "available", 2)
			if p.PerfSamplingTree.CohortCount != 2 {
				t.Fatal("source-wide unknown and known populations merged")
			}
		})
	}
}
