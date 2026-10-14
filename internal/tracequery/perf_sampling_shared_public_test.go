package tracequery

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPerfSamplingLegacyViewsShareSourceDenominators(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
	if err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	baseline := Run(idx, Query{View: ViewPerfSamplingTree, PID: 11, TimeStart: 1, TimeEnd: 2, MinDurationMs: 1000}).PerfSamplingTree
	for _, view := range []string{"perf_stats", "window_stats", "trace_perf_bundle", "perf_timeline"} {
		t.Run(view, func(t *testing.T) {
			r := Run(idx, Query{View: view, PID: 11, TimeStart: 1, TimeEnd: 2, MinDurationMs: 1000})
			if r.PerfSamplingTree == nil || r.PerfSamplingTree.Status != "available" || r.PerfSamplingTree.SampleCount != 5 || r.PerfSamplingTree.CohortCount != 3 {
				t.Fatalf("legacy view lost exact native population: %+v", r.PerfSamplingTree)
			}
			if !reflect.DeepEqual(r.PerfSamplingTree, baseline) {
				t.Fatal("view choice changed raw native sample tree")
			}
			native := map[string]PerfSamplingCohort{}
			for _, c := range r.PerfSamplingTree.Cohorts {
				native[c.ID] = c
			}
			var context *PerfContext
			if r.PerfStats != nil {
				context = r.PerfStats
			} else if r.WindowStats != nil {
				context = r.WindowStats.PerfSamples
			}
			if context != nil {
				if context.CohortCount != 3 || context.SampleCount != 5 || context.TotalPeriod != 0 {
					t.Fatalf("legacy denominator mixed producers: %+v", context)
				}
				seen := map[string]int64{}
				for _, c := range context.Cohorts {
					if c.Scope == nil || native[c.Scope.ID].TotalWeight != c.TotalPeriod || native[c.Scope.ID].SampleCount != c.SampleCount {
						t.Fatalf("legacy/native cohort identities or denominators diverged: %+v", c)
					}
					if c.Quality == nil || len(c.Quality.Sources) != 1 {
						t.Fatalf("mixed producer cohort: %+v", c)
					}
					seen[c.Quality.Sources[0].Value+"/"+c.Event] = c.TotalPeriod
				}
				if seen["raw_perfdata_fallback/cycles"] != 35 || seen["simpleperf_report_sample/cycles"] != 7 || seen["raw_perfdata_fallback/instructions"] != 100 {
					t.Fatalf("wrong source denominators: %+v", seen)
				}
			}
			if r.PerfTimeline != nil {
				if len(r.PerfTimeline.Buckets) != 1 || r.PerfTimeline.Buckets[0].CohortCount != 3 || r.PerfTimeline.Buckets[0].Period != 0 {
					t.Fatalf("timeline merged domains: %+v", r.PerfTimeline)
				}
				for _, c := range r.PerfTimeline.Buckets[0].Cohorts {
					if c.Scope == nil || native[c.Scope.ID].TotalWeight != c.Period || native[c.Scope.ID].SampleCount != c.SampleCount {
						t.Fatalf("timeline/native cohort drift: %+v", c)
					}
				}
			}
		})
	}
}

func TestPerfSamplingLegacyTargetAndWindowPopulation(t *testing.T) {
	idx := buildTraceIndex(t, "targets.perftrace", `
 target-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 sample_weight=3 event=cycles symbol=target source=one
 other-12 (10) [000] .... 1.2: perf_sample: pid=10 tid=12 sample_weight=5 event=cycles symbol=other source=one
 unknown-11 (10) [000] .... 1.3: perf_sample: pid=10 tid=11 thread_identity_known=false sample_weight=7 event=cycles symbol=unknown source=one
 target-11 (10) [000] .... 2.1: perf_sample: pid=10 tid=11 sample_weight=11 event=cycles symbol=outside source=one
`)
	for _, selector := range []Query{{PID: 11}, {Thread: "target"}} {
		for _, view := range []string{"perf_stats", "window_stats", "trace_perf_bundle", "perf_timeline"} {
			q := selector
			q.View, q.TimeStart, q.TimeEnd = view, 1, 2
			r := Run(idx, q)
			if r.PerfSamplingTree == nil || r.PerfSamplingTree.Status != "unavailable" || r.PerfSamplingTree.Reason != "typed_identity_unproven" || len(r.PerfSamplingTree.Cohorts) != 0 {
				t.Fatalf("%s incomplete same-source population claimed an exact target denominator: %+v", view, r.PerfSamplingTree)
			}
			if r.WindowStats != nil && (r.WindowStats.PerfSamples == nil || r.WindowStats.PerfSamples.SampleCount != 0 || !strings.Contains(strings.Join(r.WindowStats.PerfSamples.Caveats, " "), "typed_identity_unproven")) {
				t.Fatalf("flat target differs from tree: %+v", r.WindowStats.PerfSamples)
			}
		}
	}
	r := Run(idx, Query{View: "perf_stats", TimeStart: 1, TimeEnd: 2})
	if r.PerfStats.SampleCount != 3 || r.PerfStats.CohortCount != 3 || r.PerfStats.ThreadIdentityUnknownSampleCount != 1 || r.PerfStats.TotalPeriod != 0 {
		t.Fatalf("global inventory lost original unknown/target rows: %+v", r.PerfStats)
	}
	// Window arithmetic still has an exact oracle on a wholly attributed
	// source; removing unknown rows at query time would not prove completeness.
	known := buildTraceIndex(t, "known-targets.perftrace", `
 target-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 sample_weight=3 event=cycles symbol=target source=one
 other-12 (10) [000] .... 1.2: perf_sample: pid=10 tid=12 sample_weight=5 event=cycles symbol=other source=one
 target-11 (10) [000] .... 2.1: perf_sample: pid=10 tid=11 sample_weight=11 event=cycles symbol=outside source=one
`)
	r = Run(known, Query{View: "perf_stats", PID: 11, TimeStart: 1, TimeEnd: 2.1})
	if r.PerfStats.SampleCount != 1 || r.PerfStats.TotalPeriod != 3 || r.PerfSamplingTree.Window.EndInclusive || r.PerfStats.Cohorts[0].Scope.Window.EndInclusive {
		t.Fatal("explicit half-open sampling window included its right endpoint")
	}
	r = Run(known, Query{View: "perf_stats", PID: 11})
	if r.PerfStats.SampleCount != 2 || r.PerfStats.TotalPeriod != 14 || !r.PerfSamplingTree.Window.EndInclusive || !r.PerfStats.Cohorts[0].Scope.Window.EndInclusive {
		t.Fatal("complete capture lost its final timestamp sample")
	}
}

func TestPerfSamplingLegacyPhysicalSourcesDoNotMergeOrDuplicate(t *testing.T) {
	dir := t.TempDir()
	for i, name := range []string{"first.perftrace", "second.perftrace"} {
		writeBundleProvenanceFixture(t, filepath.Join(dir, name), fmt.Sprintf("worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 sample_weight=%d event=cycles symbol=work dso=app.so source=raw\n", i+3))
	}
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	writeBundleProvenanceFixture(t, bundle, `{"version":"test","artifacts":[
 {"type":"perftrace","path":"first.perftrace","perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true,"thread_identity":"sample_pid_tid_thread_comm"}},
 {"type":"perftrace","path":"second.perftrace","perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true,"thread_identity":"sample_pid_tid_thread_comm"}}
 ],"perf_clock_alignments":[
 {"artifact_path":"first.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","confidence":"same_domain","calibrated":false},
 {"artifact_path":"second.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","confidence":"same_domain","calibrated":false}]}`)
	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	r := Run(idx, Query{View: "perf_stats", TimeStart: 1, TimeEnd: 2})
	if r.PerfStats == nil || r.PerfStats.SampleCount != 2 || r.PerfStats.CohortCount != 2 || r.PerfStats.TotalPeriod != 0 || r.PerfSamplingTree == nil || r.PerfSamplingTree.CohortCount != 2 {
		t.Fatalf("source merged or duplicate declaration counted twice: %+v %+v", r.PerfStats, r.PerfSamplingTree)
	}
	seen := map[string]bool{}
	for _, c := range r.PerfStats.Cohorts {
		if c.Scope == nil || c.Scope.SourceGeneration == "" || seen[c.Scope.SourcePath] || c.TopSymbols[0].Source != "raw" || c.TopSymbols[0].Percent != 100 {
			t.Fatalf("first-source impersonation: %+v", c)
		}
		seen[c.Scope.SourcePath] = true
		want := int64(3)
		if filepath.Base(c.Scope.SourcePath) == "second.perftrace" {
			want = 4
		}
		if c.TotalPeriod != want {
			t.Fatalf("physical source denominator: %+v", c)
		}
	}
}

func TestPerfSamplingLegacyUnavailableAndWeightBoundaries(t *testing.T) {
	idx := buildTraceIndex(t, "weights.perftrace", `
 worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 sample_weight=9223372036854775807 event=cycles source=raw
 worker-11 (10) [000] .... 1.2: perf_sample: pid=10 tid=11 sample_weight=1 event=cycles source=raw
 worker-11 (10) [000] .... 1.3: perf_sample: pid=10 tid=11 sample_weight=invalid event=cycles source=raw
`)
	r := Run(idx, Query{View: "perf_stats", TimeStart: 1, TimeEnd: 2})
	if r.PerfStats.SampleCount != 3 || r.PerfStats.CohortCount != 2 || r.PerfSamplingTree.SampleCount != 3 {
		t.Fatal("overflow/invalid sample disappeared")
	}
	for _, c := range r.PerfStats.Cohorts {
		if c.WeightUnit == "cycles" && (c.WeightStatus != "aggregate_overflow" || c.SampleCount != 2 || c.TotalPeriod != 0 || len(c.TopSymbols) != 0) {
			t.Fatalf("overflow fabricated exact aggregate: %+v", c)
		}
		if c.WeightUnit == "sample_count_unweighted" && (c.SampleCount != 1 || c.TotalPeriod != 1) {
			t.Fatalf("invalid weight fabricated event weight: %+v", c)
		}
	}
	idx.Windowed = true
	r = Run(idx, Query{View: "perf_stats", TimeStart: 1, TimeEnd: 2})
	if r.PerfSamplingTree == nil || r.PerfSamplingTree.Status != "unavailable" || r.PerfSamplingTree.Reason != "requires_complete_source_scan" {
		t.Fatalf("windowed tree claimed full denominator: %+v", r.PerfSamplingTree)
	}
	conflict := buildTraceIndex(t, "conflict.perftrace", `
 worker-7 (70) [000] .... 1.1: perf_sample: pid=70 tid=7 thread_comm=worker thread_identity_known=true sample_weight=3 event=cycles
 worker-7 (71) [000] .... 1.2: perf_sample: pid=71 tid=7 thread_comm=worker thread_identity_known=true sample_weight=4 event=cycles
`)
	r = Run(conflict, Query{View: "perf_stats", PID: 7, TimeStart: 1, TimeEnd: 2})
	if r.PerfSamplingTree == nil || r.PerfSamplingTree.Reason != "typed_identity_unproven" || r.PerfStats == nil || r.PerfStats.SampleCount != 0 || !strings.Contains(strings.Join(r.PerfStats.Caveats, " "), "perf_thread_selector_withheld=true") {
		t.Fatalf("unproved target reported true empty: %+v %+v", r.PerfStats, r.PerfSamplingTree)
	}
	empty := buildTraceIndex(t, "no-samples.systrace", "worker-11 (10) [000] .... 1.1: tracing_mark_write: C|10|value|1\n")
	if got := Run(empty, Query{View: "window_stats", TimeStart: 1, TimeEnd: 2}); got.PerfSamplingTree != nil {
		t.Fatal("non-sampling statistics gained a tree warning")
	}
}

func TestPerfSamplingLegacyClockAndKindDomains(t *testing.T) {
	idx := buildTraceIndex(t, "domains.perftrace", `
 worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 sample_weight=3 event=cycles source=raw sample_kind=on_cpu clock=mono
 worker-11 (10) [000] .... 1.2: perf_sample: pid=10 tid=11 sample_weight=5 event=cycles source=raw sample_kind=unknown clock=mono
 worker-11 (10) [000] .... 1.3: perf_sample: pid=10 tid=11 sample_weight=7 event=cycles source=raw sample_kind=on_cpu clock=boot
`)
	r := Run(idx, Query{View: "perf_stats", PID: 11, TimeStart: 1, TimeEnd: 2})
	if r.PerfStats.CohortCount != 3 || r.PerfStats.TotalPeriod != 0 || r.PerfSamplingTree.CohortCount != 3 {
		t.Fatal("same event/unit merged independent sample-kind/clock domains")
	}
	want := map[string]int64{"on_cpu/mono": 3, "unknown/mono": 5, "on_cpu/boot": 7}
	for _, c := range r.PerfStats.Cohorts {
		if c.Scope == nil || c.TotalPeriod != want[c.Scope.SampleKind+"/"+c.Scope.Clock] {
			t.Fatalf("domain weight drift: %+v", c)
		}
	}
	for _, c := range r.PerfSamplingTree.Cohorts {
		if c.TotalWeight != want[c.SampleKind+"/"+c.Clock] || c.UnavailableWeight != c.TotalWeight || c.UnavailableSampleCount != 1 || len(c.Nodes) != 0 {
			t.Fatalf("legacy stack fabricated nodes or lost denominator: %+v", c)
		}
	}
	empty := Run(idx, Query{View: "perf_stats", PID: 11, TimeStart: 1.4, TimeEnd: 1.5})
	if empty.PerfStats != nil || empty.PerfSamplingTree == nil || empty.PerfSamplingTree.Status != "available" || empty.PerfSamplingTree.SampleCount != 0 {
		t.Fatal("empty selected window fell back to full capture")
	}
}

func TestPerfSamplingLegacyViewDoesNotRemoveFiltersForTree(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	r := Run(idx, Query{View: "perf_stats", PID: 11, TimeStart: 1, TimeEnd: 2, Pattern: "work"})
	if r.PerfSamplingTree == nil || r.PerfSamplingTree.Status != "unavailable" || r.PerfSamplingTree.Reason != "requires_time_and_thread_selectors_only" {
		t.Fatalf("filter silently widened: %+v", r.PerfSamplingTree)
	}
}
