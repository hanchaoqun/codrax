package tracequery

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1638B3DerivedSchedulerRankSourcesFromActualInputs(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{View: "root_cause_rank", PID: 9163, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MaxBranches: 8, MaxChainNodes: 1, MinDurationMs: .5,
		TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
	result := Run(idx, q)
	if result.RootCauseRank == nil {
		t.Fatal("actual rank result missing")
	}
	want := map[int]struct {
		typ, source string
		ms          float64
	}{
		1: {"cpu_affinity_or_cpuset", "window_stats.cpu_constraints", 48.519},
		2: {"scheduler_latency", "scheduler_latency_stats", 40.071},
		5: {"low_frequency", "window_stats.compute_supply", 13.884},
	}
	seen := 0
	for _, item := range result.RootCauseRank.Items {
		expect, ok := want[item.Rank]
		if !ok || item.Thread.PID != 9163 || item.Source != expect.source {
			continue
		}
		seen++
		if item.Thread.PID != 9163 || item.Type != expect.typ || item.Source != expect.source || math.Abs(item.ImpactMs-expect.ms) > .0005 {
			t.Fatalf("original rank/value premise changed: rank=%d type=%s source=%s impact=%.3f", item.Rank, item.Type, item.Source, item.ImpactMs)
		}
		if item.MeasurementSources == nil || item.MeasurementSources.HasUnknown || len(item.MeasurementSources.Domains) == 0 {
			t.Errorf("actual %s loses its own native account source at rank %d (%.3fms)", item.Source, item.Rank, item.ImpactMs)
			continue
		}
		for _, d := range item.MeasurementSources.Domains {
			if d.Status != "constructed_partition" || d.Method != "off_cpu_sweep" || d.TargetTID != 9163 || d.QueryLineStart != q.LineStart || d.QueryLineEnd != q.LineEnd || d.PartitionID == "" {
				t.Errorf("rank borrowed a foreign or fabricated source: %+v", d)
			}
		}
	}
	if seen != len(want) {
		t.Fatalf("expected all three real producer branches, got %d", seen)
	}
}

func b1638b3DerivedDomain(id string, tid int) *types.TraceSchedulerMeasurementDomain {
	return &types.TraceSchedulerMeasurementDomain{Version: 1, Status: "constructed_partition", Method: "off_cpu_sweep", TargetTID: tid,
		WindowStartTs: 1, WindowEndTs: 2, QueryLineStart: 3, QueryLineEnd: 90, PartitionID: id}
}

func TestB1638B3SupplySourcesFollowConsumedRowsNotSiblingIdentity(t *testing.T) {
	a, b := b1638b3DerivedDomain("a", 42), b1638b3DerivedDomain("b", 42)
	rows := []ThreadDuration{
		{Thread: ThreadRef{PID: 42}, CPU: 0, DurationMs: 9, MeasurementDomain: a},
		{Thread: ThreadRef{PID: 42}, CPU: 1, DurationMs: 8},
		{Thread: ThreadRef{PID: 42}, CPU: 2, DurationMs: 7, MeasurementDomain: a,
			MeasurementSources: &types.TraceSchedulerMeasurementSources{Domains: []types.TraceSchedulerMeasurementDomain{*b}, HasUnknown: true}},
		{Thread: ThreadRef{PID: 42}, CPU: -1, DurationMs: 60, MeasurementDomain: b},
	}
	stats := WindowStats{RunnableTop: rows, TopRunning: []ThreadDuration{{Thread: ThreadRef{PID: 43}, CPU: 0, DurationMs: 6, MeasurementDomain: b}}}
	got := computeSupplySummaries(stats, 0)
	legacy := stats
	legacy.RunnableTop = append([]ThreadDuration(nil), rows...)
	legacy.TopRunning = append([]ThreadDuration(nil), stats.TopRunning...)
	for _, roster := range [][]ThreadDuration{legacy.RunnableTop, legacy.TopRunning} {
		for i := range roster {
			roster[i].MeasurementDomain, roster[i].MeasurementSources = nil, nil
		}
	}
	want := computeSupplySummaries(legacy, 0)
	if len(got) != 4 {
		t.Fatalf("old CPU eligibility/count changed: %d", len(got))
	}
	clean := append([]ComputeSupplySummary(nil), got...)
	for i := range clean {
		clean[i].MeasurementSources = nil
	}
	if !reflect.DeepEqual(clean, want) {
		t.Fatal("source metadata changed existing supply fields")
	}
	for _, item := range got {
		switch item.DurationMs {
		case 9:
			if !reflect.DeepEqual(item.MeasurementSources, types.TraceSchedulerMeasurementSourcesFromDomain(a)) {
				t.Fatal("own input missing")
			}
			item.MeasurementSources.Domains[0].PartitionID = "mutated"
		case 8:
			if item.MeasurementSources != nil {
				t.Fatal("same-PID/nearby CPU source borrowed for unknown input")
			}
		case 7:
			if !reflect.DeepEqual(item.MeasurementSources, rows[2].MeasurementSources) {
				t.Fatal("explicit mixed source repaired from legacy domain")
			}
			item.MeasurementSources.Domains[0].PartitionID = "mutated"
		case 6:
			if item.State != "running" || !reflect.DeepEqual(item.MeasurementSources, types.TraceSchedulerMeasurementSourcesFromDomain(b)) {
				t.Fatal("running input source/state changed")
			}
		}
	}
	if a.PartitionID != "a" || b.PartitionID != "b" || rows[2].MeasurementSources.Domains[0].PartitionID != "b" {
		t.Fatal("supply aliases native input")
	}
}

func TestB1638B3ConstraintEpochSourcesUseOnlyBookedRestrictionInputs(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(fmt.Sprint("unknown=", unknown), func(t *testing.T) {
			a, b := b1638b3DerivedDomain("a", 42), b1638b3DerivedDomain("b", 42)
			if unknown {
				b = nil
			}
			events := []Event{
				{Line: 3, Ts: 1, Type: EventCPUConstraint, ConstraintFields: &ConstraintFields{PID: 42, Allowed: []int{0}}},
				{Line: 20, Ts: 1.5, Type: EventCPUConstraint, ConstraintFields: &ConstraintFields{PID: 42, Allowed: []int{0, 1}}},
			}
			seg := func(tid int, start, end float64, known bool, domain *types.TraceSchedulerMeasurementDomain) runnableWaitSegment {
				return runnableWaitSegment{thread: ThreadRef{PID: tid}, startTs: start, endTs: end, durationMs: (end - start) * 1000, cpu: 0, cpuKnown: known, measurementDomain: domain}
			}
			segments := []runnableWaitSegment{
				seg(42, 1.1, 1.2, true, a), seg(42, 1.3, 1.4, true, b),
				seg(42, 1.6, 1.7, true, b1638b3DerivedDomain("unrestricted", 42)),
				seg(43, 1.1, 1.2, true, b1638b3DerivedDomain("foreign", 43)),
				seg(42, 1.2, 1.3, false, b1638b3DerivedDomain("unknown-cpu", 42)),
			}
			run := func(in []runnableWaitSegment) cpuConstraintEpochAccounting {
				return computeCPUConstraintEpochAccounting(events, []int{0, 1}, Query{TimeStart: 1, TimeEnd: 2}, 2, in,
					map[int]bool{0: true, 1: true}, nil, coreCapabilityMap{}, nil)[42]
			}
			got := run(segments)
			if math.Abs(got.restrictedRunnableWaitMs-200) > 1e-8 {
				t.Fatalf("original restricted union changed: %v", got.restrictedRunnableWaitMs)
			}
			wantSource := types.MergeTraceSchedulerMeasurementSources(types.TraceSchedulerMeasurementSourcesFromDomain(a), types.TraceSchedulerMeasurementSourcesFromDomain(b))
			if !reflect.DeepEqual(got.measurementSources, wantSource) {
				t.Fatalf("wrong epoch source inputs: %+v", got.measurementSources)
			}
			legacySegments := append([]runnableWaitSegment(nil), segments...)
			for i := range legacySegments {
				legacySegments[i].measurementDomain = nil
			}
			legacy := run(legacySegments)
			clean := got
			clean.measurementSources = nil
			if !reflect.DeepEqual(clean, legacy) {
				t.Fatal("epoch sources changed existing proof/value/roster fields")
			}
			summary := CPUConstraintSummary{RunnableWaitMs: 999, MeasurementSources: types.TraceSchedulerMeasurementSourcesFromDomain(b1638b3DerivedDomain("fallback-total", 42))}
			applyCPUConstraintEpochOverlay(&summary, got)
			if !reflect.DeepEqual(summary.MeasurementSources, got.measurementSources) {
				t.Fatal("epoch overlay borrowed full runnable-total source")
			}
			summary.MeasurementSources.Domains[0].PartitionID = "mutated"
			if !reflect.DeepEqual(got.measurementSources, wantSource) {
				t.Fatal("overlay aliases native accounting")
			}
			applyCPUConstraintEpochOverlay(&summary, legacy)
			if summary.MeasurementSources != nil {
				t.Fatal("unknown restricted source repaired from previous/total source")
			}
		})
	}
}

func TestB1638B3ConstraintSnapshotSourcesSurviveDisplayCap(t *testing.T) {
	var events []Event
	var indexes []int
	var segments []runnableWaitSegment
	for i := 0; i < 18; i++ {
		ts := 1 + float64(i+1)*.001
		events = append(events, Event{Type: EventSchedSwitch, Line: i + 3, Ts: ts, CPU: 0, NextPID: 42,
			NextInfo: fmt.Sprintf("1,%d,2,0,0", i), NextInfoAffinity: "1", NextInfoAllowedCPUs: []int{0}})
		indexes = append(indexes, i)
		segments = append(segments, runnableWaitSegment{thread: ThreadRef{PID: 42}, cpu: 0, cpuKnown: true,
			startTs: ts - .0005, endTs: ts, durationMs: .5, measurementDomain: b1638b3DerivedDomain(fmt.Sprint(i), 42)})
	}
	got := computeCPUConstraintEpochAccounting(events, indexes, Query{TimeStart: 1, TimeEnd: 1.02}, 1.02, segments, map[int]bool{0: true, 1: true}, nil, coreCapabilityMap{}, nil)[42]
	if got.total != 18 || len(got.epochs) != 16 || math.Abs(got.restrictedRunnableWaitMs-9) > 1e-8 {
		t.Fatal("snapshot/accounting cap premise changed")
	}
	if got.measurementSources == nil || got.measurementSources.HasUnknown || len(got.measurementSources.Domains) != 18 {
		t.Fatal("display cap dropped real snapshot inputs")
	}
	got.measurementSources.Domains[0].PartitionID = "mutated"
	for _, segment := range segments {
		if segment.measurementDomain.PartitionID == "mutated" {
			t.Fatal("epoch union aliases input")
		}
	}
}

func TestB1638B3DerivedNativeClonesAndLegacyValues(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{View: "scheduler_latency", PID: 9163, TimeStart: 13762.791708, TimeEnd: 13763.024898, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 2}
	stats := ComputeWindowStats(idx, q)
	got := buildSchedulerLatencyStatsFromStats(idx, q, stats)
	if len(got.Items) == 0 || len(got.itemsCensus) <= len(got.Items) {
		t.Fatal("real public/full latency census premise missing")
	}
	legacyStats := stats
	legacyStats.runnableSegments = append([]runnableWaitSegment(nil), stats.runnableSegments...)
	for i := range legacyStats.runnableSegments {
		legacyStats.runnableSegments[i].measurementDomain = nil
	}
	want := buildSchedulerLatencyStatsFromStats(idx, q, legacyStats)
	clean := got
	clean.Items = cloneSchedulerLatencyItemsForAccounting(got.Items)
	clean.itemsCensus = cloneSchedulerLatencyItemsForAccounting(got.itemsCensus)
	for _, rows := range [][]SchedulerLatencyItem{clean.Items, clean.itemsCensus} {
		for i := range rows {
			rows[i].MeasurementSources = nil
		}
	}
	if !reflect.DeepEqual(clean, want) {
		t.Fatal("latency source propagation changes previous fields")
	}
	if got.Items[0].MeasurementSources == nil {
		t.Fatal("real segment source missing")
	}
	got.Items[0].MeasurementSources.Domains[0].PartitionID = "mutated"
	for _, item := range got.itemsCensus {
		if item.MeasurementSources != nil && item.MeasurementSources.Domains[0].PartitionID == "mutated" {
			t.Fatal("public latency aliases accounting census")
		}
	}
	for _, segment := range stats.runnableSegments {
		if segment.measurementDomain != nil && segment.measurementDomain.PartitionID == "mutated" {
			t.Fatal("latency aliases native segments")
		}
	}
	for i := range stats.runnableSegments {
		if stats.runnableSegments[i].measurementDomain == nil {
			continue
		}
		stats.runnableSegments[i].measurementDomain.PartitionID = "mutated-segment"
		for j, other := range stats.runnableSegments {
			if j != i && other.measurementDomain != nil && other.measurementDomain.PartitionID == "mutated-segment" {
				t.Fatal("sibling runnable segments alias")
			}
		}
		for _, td := range stats.RunnableTop {
			if td.MeasurementDomain != nil && td.MeasurementDomain.PartitionID == "mutated-segment" {
				t.Fatal("native segment aliases duration bucket")
			}
		}
		break
	}
	domain := b1638b3DerivedDomain("constraint", 42)
	constraint := CPUConstraintSummary{Thread: ThreadRef{PID: 42}, RunnableWaitMs: 10, MeasurementSources: types.TraceSchedulerMeasurementSourcesFromDomain(domain)}
	display := capCPUConstraintDisplay([]CPUConstraintSummary{constraint}, 1)
	display[0].MeasurementSources.Domains[0].PartitionID = "mutated"
	if constraint.MeasurementSources.Domains[0].PartitionID != "constraint" {
		t.Fatal("constraint display aliases accounting")
	}
	contextDisplay := capRunnableContextDisplay([]RunnableContextSummary{{CPUConstraint: &constraint}}, 1)
	contextDisplay[0].CPUConstraint.MeasurementSources.Domains[0].PartitionID = "mutated"
	if constraint.MeasurementSources.Domains[0].PartitionID != "constraint" {
		t.Fatal("context display aliases accounting constraint")
	}
}
