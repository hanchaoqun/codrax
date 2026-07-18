package tracequery

import (
	"math"
	"strings"
	"testing"
)

func TestPerfAggregateSeparatesEventUnitsAndDSOIdentity(t *testing.T) {
	idx := &Index{Events: []Event{
		perfAggregateTestEvent(1, 1.001, "cpu-cycles", "cycles", "liba.so", 100),
		perfAggregateTestEvent(2, 1.002, "instructions", "cycles", "liba.so", 7),
	}}
	ctx := computePerfContext(idx, Query{TimeStart: 1, TimeEnd: 2}, 8)
	if ctx == nil || ctx.SampleCount != 2 || ctx.CohortCount != 2 || len(ctx.Cohorts) != 2 {
		t.Fatalf("mixed aggregate = %+v", ctx)
	}
	if ctx.TotalPeriod != 0 || len(ctx.TopSymbols) != 0 || len(ctx.TopThreads) != 0 {
		t.Fatalf("legacy weighted projection crossed cohorts: %+v", ctx)
	}
	cycles := perfAggregateCohortByEvent(t, ctx, "cpu-cycles")
	instructions := perfAggregateCohortByEvent(t, ctx, "instructions")
	if cycles.WeightUnit != "cycles" || cycles.TotalPeriod != 100 || len(cycles.TopSymbols) != 1 || cycles.TopSymbols[0].Percent != 100 {
		t.Fatalf("cycles cohort = %+v", cycles)
	}
	if instructions.WeightUnit != "instructions" || instructions.TotalPeriod != 7 || len(instructions.TopSymbols) != 1 || instructions.TopSymbols[0].Percent != 100 {
		t.Fatalf("instructions cohort = %+v", instructions)
	}
	if !containsSubstring(ctx.Caveats, "legacy_weighted_projection=withheld") {
		t.Fatalf("mixed-cohort caveat missing: %+v", ctx.Caveats)
	}
	rootSummary := rootCausePerfSummary(ctx)
	if !strings.Contains(rootSummary, "perf_cohorts=2") || !strings.Contains(rootSummary, "cohort=cpu-cycles/cycles:exact:100") || !strings.Contains(rootSummary, "cohort=instructions/instructions:exact:7") || strings.Contains(rootSummary, "sample_weight=0") {
		t.Fatalf("root summary crossed cohort units: %s", rootSummary)
	}
	roleSummary := rootCausePerfRoleSummary([]RootCausePerfRoleContext{{Role: "candidate_thread", PerfContext: ctx}}, 4)
	if !strings.Contains(roleSummary, "cohorts=2") || strings.Contains(roleSummary, "sample_weight=0") {
		t.Fatalf("role summary crossed cohort units: %s", roleSummary)
	}

	dsoIdx := &Index{Events: []Event{
		perfAggregateTestEvent(1, 2.001, "cpu-cycles", "same", "liba.so", 5),
		perfAggregateTestEvent(2, 2.002, "cpu-cycles", "same", "libb.so", 4),
	}}
	dsoCtx := computePerfContext(dsoIdx, Query{TimeStart: 2, TimeEnd: 3}, 8)
	if dsoCtx == nil || dsoCtx.CohortCount != 1 || len(dsoCtx.TopSymbols) != 2 {
		t.Fatalf("same symbol in distinct DSOs was folded: %+v", dsoCtx)
	}
	if dsoCtx.TopSymbols[0].DSO != "liba.so" || dsoCtx.TopSymbols[1].DSO != "libb.so" {
		t.Fatalf("DSO identity/order = %+v", dsoCtx.TopSymbols)
	}
}

func TestPerfAggregateOverflowIsCohortLocalAndSticky(t *testing.T) {
	idx := &Index{Events: []Event{
		perfAggregateTestEvent(1, 1.001, "cpu-cycles", "overflow", "lib.so", math.MaxInt64),
		perfAggregateTestEvent(2, 1.002, "cpu-cycles", "overflow", "lib.so", 1),
		perfAggregateTestEvent(3, 1.003, "cpu-cycles", "overflow", "lib.so", 9),
		perfAggregateTestEvent(4, 1.004, "instructions", "healthy", "lib.so", 11),
	}}
	ctx := computePerfContext(idx, Query{TimeStart: 1, TimeEnd: 2}, 8)
	if ctx == nil || ctx.SampleCount != 4 || ctx.CohortCount != 2 {
		t.Fatalf("overflow aggregate = %+v", ctx)
	}
	overflow := perfAggregateCohortByEvent(t, ctx, "cpu-cycles")
	if overflow.WeightStatus != perfAggregateStatusOverflow || overflow.SampleCount != 3 || overflow.TotalPeriod != 0 || len(overflow.TopSymbols) != 0 {
		t.Fatalf("overflow cohort regained weighted authority: %+v", overflow)
	}
	if overflow.Quality == nil || overflow.Quality.WeightStatus != "sample_count_only" || len(overflow.Quality.Sources) == 0 || overflow.Quality.Sources[0].Period != 0 {
		t.Fatalf("overflow quality did not withdraw weighted values: %+v", overflow.Quality)
	}
	healthy := perfAggregateCohortByEvent(t, ctx, "instructions")
	if healthy.WeightStatus != perfAggregateStatusExact || healthy.TotalPeriod != 11 || len(healthy.TopSymbols) != 1 || healthy.TopSymbols[0].Symbol != "healthy" {
		t.Fatalf("healthy sibling was poisoned: %+v", healthy)
	}
	if healthy.Quality == nil || healthy.Quality.WeightStatus != perfAggregateStatusExact || len(healthy.Quality.Sources) == 0 || healthy.Quality.Sources[0].Period != 11 {
		t.Fatalf("healthy quality lost weighted values: %+v", healthy.Quality)
	}
	facts := evidenceFromPerfContext(ctx)
	if len(facts) == 0 {
		t.Fatalf("healthy cohort evidence missing: %+v", facts)
	}
	for _, fact := range facts {
		if !strings.Contains(fact.Summary, "weight_unit=instructions") || strings.Contains(fact.Summary, "overflow") {
			t.Fatalf("evidence published the overflow cohort: %+v", facts)
		}
	}
}

func TestPerfTimelineSeparatesCohortsAndWithholdsLegacyWeight(t *testing.T) {
	idx := &Index{Events: []Event{
		perfAggregateTestEvent(1, 1.001, "cpu-cycles", "hot", "lib.so", 13),
		perfAggregateTestEvent(2, 1.002, "instructions", "hot", "lib.so", 5),
	}}
	res := BuildPerfTimeline(idx, Query{TimeStart: 1, TimeEnd: 1.01, MinDurationMs: 10})
	if len(res.Buckets) != 1 {
		t.Fatalf("timeline buckets = %+v", res)
	}
	bucket := res.Buckets[0]
	if bucket.CohortCount != 2 || len(bucket.Cohorts) != 2 || bucket.Period != 0 || bucket.TopSymbol != "" || bucket.TopEvent != "" {
		t.Fatalf("timeline legacy projection crossed units: %+v", bucket)
	}
	if !containsSubstring(res.Caveats, "perf_timeline_event_unit_cohorts=true") {
		t.Fatalf("timeline cohort caveat missing: %+v", res.Caveats)
	}
	facts := evidenceFromPerfTimeline(res)
	if len(facts) != 2 || !strings.Contains(facts[0].Summary, "weight_unit=cycles") || !strings.Contains(facts[1].Summary, "weight_unit=instructions") {
		t.Fatalf("timeline evidence lost cohort identity: %+v", facts)
	}
}

func TestPerfAggregateUnweightedSampleDoesNotShareCyclesDenominator(t *testing.T) {
	weighted := perfAggregateTestEvent(1, 1.001, "cpu-cycles", "hot", "lib.so", 9)
	unweighted := perfAggregateTestEvent(2, 1.002, "cpu-cycles", "hot", "lib.so", 0)
	unweighted.PerfFields.PerfWeightInvalid = true
	ctx := computePerfContext(&Index{Events: []Event{weighted, unweighted}}, Query{TimeStart: 1, TimeEnd: 2}, 8)
	if ctx == nil || ctx.CohortCount != 2 {
		t.Fatalf("weighted/unweighted cohorts = %+v", ctx)
	}
	units := map[string]int64{}
	for _, cohort := range ctx.Cohorts {
		units[cohort.WeightUnit] = cohort.TotalPeriod
	}
	if units["cycles"] != 9 || units["sample_count_unweighted"] != 1 {
		t.Fatalf("weighted/unweighted totals = %+v", units)
	}
}

func TestPerfAggregateSeparatesOnOffCPUClockAndCanonicalizesEmptyEvent(t *testing.T) {
	onCPU := perfAggregateTestEvent(1, 1.001, "cpu-clock", "on", "lib.so", 4)
	onCPU.PerfFields.SampleKind = "on_cpu"
	offCPU := perfAggregateTestEvent(2, 1.002, "cpu-clock", "off", "lib.so", 6)
	offCPU.PerfFields.SampleKind = "off_cpu"
	empty := perfAggregateTestEvent(3, 1.003, "", "unknownEvent", "lib.so", 2)
	ctx := computePerfContext(&Index{Events: []Event{onCPU, offCPU, empty}}, Query{TimeStart: 1, TimeEnd: 2}, 8)
	if ctx == nil || ctx.CohortCount != 3 {
		t.Fatalf("clock/empty cohorts = %+v", ctx)
	}
	want := map[string]bool{"cpu-clock/ns_on_cpu_event": false, "cpu-clock/ns_off_cpu_event": false, "unknown/event_count": false}
	for _, cohort := range ctx.Cohorts {
		key := cohort.Event + "/" + cohort.WeightUnit
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Fatalf("cohort %s missing from %+v", key, ctx.Cohorts)
		}
	}
}

func perfAggregateTestEvent(line int, ts float64, event, symbol, dso string, period int64) Event {
	return Event{
		Line: line,
		Ts:   ts,
		Type: EventPerfSample,
		PerfFields: &PerfFields{
			EventName: event,
			Symbol:    symbol,
			DSO:       dso,
			Period:    period,
			Source:    "fixture",
		},
	}
}

func perfAggregateCohortByEvent(t *testing.T, ctx *PerfContext, event string) PerfCohort {
	t.Helper()
	for _, cohort := range ctx.Cohorts {
		if cohort.Event == event {
			return cohort
		}
	}
	t.Fatalf("cohort %q missing from %+v", event, ctx)
	return PerfCohort{}
}
