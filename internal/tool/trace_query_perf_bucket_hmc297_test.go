package tool

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Exercise the real tool, strict parameters, immutable payload, and engine;
// a renderer-only fixture cannot establish that the requested width ran.
func TestHMC297PerfTimelinePublicBucketContract(t *testing.T) {
	ctx, _, _ := hmc17NamedPathContext(t)
	path := filepath.Join(t.TempDir(), "samples.perftrace")
	var body strings.Builder
	for _, row := range []struct {
		ts          float64
		tid, weight int
	}{
		{10.001, 101, 2}, {10.005, 202, 900}, {10.011, 101, 3}, {10.021, 101, 5}, {10.199, 101, 7},
		{10.2, 101, 800},
	} {
		fmt.Fprintf(&body, "worker-%d (100) [000] .... %.6f: perf_sample: pid=100 tid=%d cpu=0 cpu_known=true sample_weight=%d event=cycles weight_unit=cycles symbol=work source=raw_perfdata_fallback\n", row.tid, row.ts, row.tid, row.weight)
	}
	if err := os.WriteFile(path, []byte(body.String()), 0600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	var baseline *tracequery.PerfSamplingTreeResult
	for _, tc := range []struct {
		name    string
		extra   map[string]any
		width   float64
		buckets int
		counts  []int
	}{
		{"default", nil, 1, 4, []int{1, 1, 1, 1}},
		{"twenty", map[string]any{"bucket_ms": 20}, 20, 3, []int{2, 1, 1}},
		{"ten", map[string]any{"bucket_ms": 10}, 10, 4, []int{1, 1, 1, 1}},
		{"legacy", map[string]any{"min_duration_ms": 20}, 20, 3, []int{2, 1, 1}},
		{"explicit_wins", map[string]any{"bucket_ms": 20, "min_duration_ms": 10}, 20, 3, []int{2, 1, 1}},
		{"duration_string", map[string]any{"bucket_ms": "20ms"}, 20, 3, []int{2, 1, 1}},
		{"cap", map[string]any{"bucket_ms": 0.01}, 1, 4, []int{1, 1, 1, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{"source": "path", "path": path, "view": "perf_timeline", "pid": 101, "time_start": 10, "time_end": 10.2}
			for k, v := range tc.extra {
				args[k] = v
			}
			params, _ := json.Marshal(args)
			out, err := (&TraceQuery{}).Execute(ctx, params)
			if err != nil || !out.Success {
				t.Fatalf("execute: %v %s", err, out.Summary)
			}
			var payload tracequery.Result
			found := false
			for _, o := range out.Observations {
				if o.SourceRef.PayloadRef == "" {
					continue
				}
				data, err := os.ReadFile(o.SourceRef.PayloadRef)
				if err != nil {
					t.Fatal(err)
				}
				if json.Unmarshal(data, &payload) == nil && payload.PerfTimeline != nil {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("missing original native timeline: %s", out.Summary)
			}
			timeline := payload.PerfTimeline
			if math.Abs(timeline.BucketMs-tc.width) > 1e-9 {
				t.Fatalf("requested width ignored: got %g want %g; timeline=%+v; tree=%+v", timeline.BucketMs, tc.width, timeline, payload.PerfSamplingTree)
			}
			wantSource := "bucket_ms"
			wantRequested := tc.width
			if tc.name == "default" {
				wantSource = "default"
			}
			if tc.name == "legacy" {
				wantSource = "min_duration_ms"
			}
			if tc.name == "cap" {
				wantRequested = 0.01
			}
			if timeline.RequestedBucketMs != wantRequested || timeline.BucketWidthSource != wantSource {
				t.Fatalf("missing exact requested/effective provenance: %+v", timeline)
			}
			if tc.name == "cap" && !strings.Contains(strings.Join(timeline.Caveats, " "), "max_buckets=200") {
				t.Fatal("silent capacity adjustment")
			}
			if timeline.Window.StartTs != 10 || timeline.Window.EndTs != 10.2 || len(timeline.Buckets) != tc.buckets {
				t.Fatalf("scope/buckets changed: %+v", timeline)
			}
			counts := []int{}
			var weight int64
			for _, b := range timeline.Buckets {
				counts = append(counts, b.SampleCount)
				weight += b.Period
				for _, id := range b.ThreadIdentities {
					if id.TID != 101 {
						t.Fatalf("foreign target: %+v", id)
					}
				}
			}
			if !reflect.DeepEqual(counts, tc.counts) || weight != 17 {
				t.Fatalf("population/weights changed: counts=%v weight=%d", counts, weight)
			}
			if !strings.Contains(out.Summary, fmt.Sprintf("perf_timeline bucket_ms=%.3f", tc.width)) {
				t.Fatalf("summary disagrees with payload: %s", out.Summary)
			}
			seenNativeSummary := false
			for _, observation := range out.Observations {
				if strings.Contains(observation.Summary, "bucket_width_source="+wantSource) && strings.Contains(observation.Summary, fmt.Sprintf("effective_bucket_ms=%g", timeline.BucketMs)) {
					seenNativeSummary = true
				}
			}
			if !seenNativeSummary {
				t.Fatal("bucket provenance did not reach native observation summary")
			}
			if payload.PerfSamplingTree == nil || payload.PerfSamplingTree.SampleCount != 4 {
				t.Fatalf("lost full independent denominator: %+v", payload.PerfSamplingTree)
			}
			if baseline == nil {
				baseline = payload.PerfSamplingTree
			} else if !reflect.DeepEqual(baseline, payload.PerfSamplingTree) {
				t.Fatal("bucket presentation changed independent full sampling tree")
			}
		})
	}
	after, _ := os.ReadFile(path)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("query changed source")
	}
}

func TestHMC297PerfTimelinePublicBoundaries(t *testing.T) {
	ctx, _, _ := hmc17NamedPathContext(t)
	path := filepath.Join(t.TempDir(), "tail.perftrace")
	var body strings.Builder
	for i := 0; i <= 200; i++ {
		fmt.Fprintf(&body, "worker-101 (100) [000] .... %.6f: perf_sample: pid=100 tid=101 cpu=0 cpu_known=true sample_weight=2 event=cycles weight_unit=cycles source=raw_perfdata_fallback\n", float64(i)/200)
	}
	if err := os.WriteFile(path, []byte(body.String()), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		params    map[string]any
		count     int
		lastEnd   float64
		effective float64
		inclusive bool
	}{
		{"explicit_zero_half_open", map[string]any{"time_start": 0, "time_end": 1, "bucket_ms": 1.0}, 200, 1, 5, false},
		{"default_includes_last", map[string]any{"bucket_ms": 1.0}, 201, 1, 5, true},
		{"default_coarse_includes_last", map[string]any{"bucket_ms": 20.0}, 201, 1, 20, true},
		{"short_tail", map[string]any{"time_start": 0, "time_end": 0.025, "bucket_ms": 20.0}, 5, 0.025, 20, false},
		{"true_zero", map[string]any{"time_start": 2, "time_end": 3, "bucket_ms": 20.0}, 0, 0, 20, false},
		{"tiny_positive", map[string]any{"time_start": 0, "time_end": 1, "bucket_ms": math.SmallestNonzeroFloat64}, 200, 1, 5, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{"source": "path", "path": path, "view": "perf_timeline", "pid": 101}
			for k, v := range tc.params {
				args[k] = v
			}
			out := hmc17NamedQuery(t, ctx, args)
			payload := hmc17NamedPayload(t, out)
			r := payload.PerfTimeline
			if r == nil || r.BucketMs != tc.effective || r.RequestedBucketMs != tc.params["bucket_ms"].(float64) {
				t.Fatalf("wrong actual width: %+v", r)
			}
			count := 0
			var weight int64
			for i, b := range r.Buckets {
				count += b.SampleCount
				weight += b.Period
				if len(b.CPUs) != 0 {
					t.Fatalf("unknown kind gained execution CPU: %+v", b)
				}
				if b.EndInclusive != (tc.inclusive && i == len(r.Buckets)-1) {
					t.Fatalf("wrong endpoint admission disclosure: %+v", b)
				}
			}
			if count != tc.count || weight != int64(tc.count*2) || len(r.Buckets) > 200 {
				t.Fatalf("lost tail/population: count=%d weight=%d buckets=%d", count, weight, len(r.Buckets))
			}
			if count > 0 && r.Buckets[len(r.Buckets)-1].EndTs != tc.lastEnd {
				t.Fatalf("wrong tail: %+v", r.Buckets[len(r.Buckets)-1])
			}
			if payload.PerfSamplingTree == nil || payload.PerfSamplingTree.SampleCount != tc.count {
				t.Fatalf("tree denominator changed: %+v", payload.PerfSamplingTree)
			}
		})
	}
}

func TestHMC297PerfTimelinePublicCohortsAndInputContract(t *testing.T) {
	ctx, _, _ := hmc17NamedPathContext(t)
	path := filepath.Join(t.TempDir(), "cohorts.perftrace")
	text := "worker-101 (100) [000] .... 1.001000: perf_sample: pid=100 tid=101 cpu=0 sample_weight=3 event=cycles weight_unit=cycles source=raw_perfdata_fallback\nworker-101 (100) [000] .... 1.002000: perf_sample: pid=100 tid=101 cpu=0 sample_weight=5 event=instructions weight_unit=instructions source=raw_perfdata_fallback\n"
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"source": "path", "path": path, "view": "perf_timeline", "pid": 101, "time_start": 1, "time_end": 1.1, "bucket_ms": 20}
	r := hmc17NamedPayload(t, hmc17NamedQuery(t, ctx, args)).PerfTimeline
	if r == nil || len(r.Buckets) != 1 || r.Buckets[0].CohortCount != 2 || r.Buckets[0].Period != 0 {
		t.Fatalf("cross-unit total invented: %+v", r)
	}
	weights := map[string]int64{}
	for _, c := range r.Buckets[0].Cohorts {
		weights[c.WeightUnit] = c.Period
	}
	if !reflect.DeepEqual(weights, map[string]int64{"cycles": 3, "instructions": 5}) {
		t.Fatalf("cohorts changed: %v", weights)
	}
	for _, bad := range []any{"NaN", "Inf", "1e999"} {
		args["bucket_ms"] = bad
		params, _ := json.Marshal(args)
		if out, err := (&TraceQuery{}).Execute(ctx, params); err == nil && out.Success {
			t.Fatalf("invalid width accepted: %v", bad)
		}
	}
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&TraceQuery{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"perf_timeline", "precedence", "default 1ms", "200", "requested_bucket_ms", "Other views do not consume"} {
		if !strings.Contains(schema.Properties["bucket_ms"].Description, want) {
			t.Fatalf("bucket teaching missing %q", want)
		}
	}
	for _, want := range []string{"perf_timeline", "not a sample-duration filter", "bucket_ms"} {
		if !strings.Contains(schema.Properties["min_duration_ms"].Description, want) {
			t.Fatalf("legacy teaching missing %q", want)
		}
	}
}

func TestHMC297PerfTimelinePublicGridCoordinates(t *testing.T) {
	ctx, _, _ := hmc17NamedPathContext(t)
	for _, tc := range []struct {
		name      string
		times     []float64
		params    map[string]any
		counts    []int
		inclusive bool
	}{
		{"decimal_tail", []float64{0.1, 0.4}, map[string]any{"bucket_ms": 100}, []int{1, 1}, true},
		{"interior_boundary_and_predecessor", []float64{10, math.Nextafter(10.02, math.Inf(-1)), 10.02}, map[string]any{"time_start": 10, "time_end": 10.05, "bucket_ms": 20}, []int{2, 1}, false},
		{"published_grid_matches_assignment", []float64{0, 0.6}, map[string]any{"time_start": 0, "time_end": 1, "bucket_ms": 100}, []int{1, 1}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "coordinates.perftrace")
			var body strings.Builder
			for _, ts := range tc.times {
				fmt.Fprintf(&body, "worker-101 (100) [000] .... %.17f: perf_sample: pid=100 tid=101 cpu=0 sample_weight=1 event=cycles weight_unit=cycles\n", ts)
			}
			if err := os.WriteFile(path, []byte(body.String()), 0600); err != nil {
				t.Fatal(err)
			}
			args := map[string]any{"source": "path", "path": path, "view": "perf_timeline", "pid": 101}
			for k, v := range tc.params {
				args[k] = v
			}
			r := hmc17NamedPayload(t, hmc17NamedQuery(t, ctx, args)).PerfTimeline
			var counts []int
			for _, b := range r.Buckets {
				if b.EndTs <= b.StartTs {
					t.Fatalf("zero-width tail: %+v", b)
				}
				inside := 0
				for _, ts := range tc.times {
					if ts >= b.StartTs && (ts < b.EndTs || b.EndInclusive && ts == b.EndTs) {
						inside++
					}
				}
				if inside != b.SampleCount {
					t.Fatalf("published bucket does not contain its actual members: inside=%d bucket=%+v", inside, b)
				}
				counts = append(counts, b.SampleCount)
			}
			if !reflect.DeepEqual(counts, tc.counts) || r.Buckets[len(r.Buckets)-1].EndInclusive != tc.inclusive {
				t.Fatalf("grid assignment: %+v", r)
			}
		})
	}
}
