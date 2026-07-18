package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceQueryPerfContextRendersCohortsWithoutCrossUnitTotal(t *testing.T) {
	ctx := tracequery.PerfContext{
		SampleCount: 3,
		CohortCount: 2,
		Cohorts: []tracequery.PerfCohort{
			{
				Event: "cpu-cycles", WeightUnit: "cycles", WeightStatus: "aggregate_overflow", SampleCount: 2,
			},
			{
				Event: "instructions", WeightUnit: "instructions", WeightStatus: "exact", SampleCount: 1, TotalPeriod: 9,
				TopSymbols: []tracequery.PerfHotspot{{Symbol: "healthy", DSO: "lib.so", Event: "instructions", WeightUnit: "instructions", Period: 9, SampleCount: 1, Percent: 100}},
			},
		},
		Caveats: []string{"perf_aggregate_weight_overflow=true event=cpu-cycles weight_unit=cycles"},
	}
	result := tracequery.Result{View: "window_stats", WindowStats: &tracequery.WindowStats{PerfSamples: &ctx}}
	summary := traceQuerySummary(result, traceQueryParams{View: "window_stats"}, "path", "/tmp/payload.json")
	for _, want := range []string{
		"perf_samples sample_count=3 cohort_count=2 weighted_projection=cohort_only",
		"perf_cohort event=cpu-cycles weight_unit=cycles weight_status=aggregate_overflow samples=2",
		"perf_cohort event=instructions weight_unit=instructions weight_status=exact samples=1 total_sample_weight=9",
		"perf_cohort_top_symbol event=instructions weight_unit=instructions symbol=healthy dso=lib.so sample_weight=9 samples=1 percent=100.00",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("cohort summary missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "perf_samples sample_count=3 total_sample_weight=0") || strings.Contains(summary, "event=cpu-cycles weight_unit=cycles weight_status=aggregate_overflow samples=2 total_sample_weight=") {
		t.Fatalf("renderer fabricated a cross-unit/overflow total:\n%s", summary)
	}
}

func TestTraceQueryPerfTimelineRendersPerBucketCohorts(t *testing.T) {
	result := tracequery.Result{View: "perf_timeline", PerfTimeline: &tracequery.PerfTimelineResult{
		Window:   tracequery.TimeWindow{StartTs: 1, EndTs: 2},
		BucketMs: 1000,
		Buckets: []tracequery.PerfTimelineBucket{{
			StartTs: 1, EndTs: 2, SampleCount: 2, CohortCount: 2,
			Cohorts: []tracequery.PerfTimelineCohort{
				{Event: "cpu-cycles", WeightUnit: "cycles", WeightStatus: "exact", SampleCount: 1, Period: 7, TopSymbol: "cycleHot"},
				{Event: "instructions", WeightUnit: "instructions", WeightStatus: "exact", SampleCount: 1, Period: 3, TopSymbol: "instructionHot"},
			},
		}},
	}}
	summary := traceQuerySummary(result, traceQueryParams{View: "perf_timeline"}, "path", "/tmp/payload.json")
	for _, want := range []string{
		"perf_bucket 1.000000..2.000000 samples=2 cohort_count=2 weighted_projection=cohort_only",
		"perf_bucket_cohort event=cpu-cycles weight_unit=cycles weight_status=exact samples=1 sample_weight=7 top_symbol=cycleHot",
		"perf_bucket_cohort event=instructions weight_unit=instructions weight_status=exact samples=1 sample_weight=3 top_symbol=instructionHot",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("timeline cohort summary missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "perf_bucket 1.000000..2.000000 sample_weight=") {
		t.Fatalf("timeline renderer fabricated a cross-unit bucket total:\n%s", summary)
	}
}

func TestTraceQueryPerfTypedObservationsSkipOverflowCohort(t *testing.T) {
	ctx := tracequery.PerfContext{
		SampleCount: 3,
		CohortCount: 2,
		Cohorts: []tracequery.PerfCohort{
			{
				Event: "cpu-cycles", WeightUnit: "cycles", WeightStatus: "aggregate_overflow", SampleCount: 2,
				TopSymbols: []tracequery.PerfHotspot{{Symbol: "must-not-publish", Event: "cpu-cycles", WeightUnit: "cycles", Period: 1, SampleCount: 2}},
			},
			{
				Event: "instructions", WeightUnit: "instructions", WeightStatus: "exact", SampleCount: 1, TotalPeriod: 9,
				TopSymbols: []tracequery.PerfHotspot{{Symbol: "healthy", DSO: "lib.so", Event: "instructions", WeightUnit: "instructions", Period: 9, SampleCount: 1, Percent: 100}},
			},
		},
	}
	result := tracequery.Result{View: "window_stats", WindowStats: &tracequery.WindowStats{PerfSamples: &ctx}}
	observations := traceQueryTypedObservations(result, "perf.systrace", "payload", "raw", "", time.Unix(0, 0))

	var perfObservations int
	for _, observation := range observations {
		if observation.Predicate != "perf_sample_top_symbol" {
			continue
		}
		perfObservations++
		if observation.Subject != "healthy" || observation.Value != "9" {
			t.Fatalf("typed observation escaped the exact healthy cohort: %+v", observation)
		}
		if strings.Contains(observation.Summary, "cpu-cycles") || !strings.Contains(observation.Summary, "event=instructions weight_unit=instructions") {
			t.Fatalf("typed observation lost cohort provenance: %+v", observation)
		}
	}
	if perfObservations != 1 {
		t.Fatalf("perf typed observation count=%d, want only the healthy exact cohort; all=%+v", perfObservations, observations)
	}
}
