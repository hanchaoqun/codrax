package tracequery

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestTraceCounterWireSchemaStandardHarmonyCarvedAndGlobal(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		owner       int
		scope       string
		counter     string
		value       float64
		trailingTag string
	}{
		{
			name:  "standard atrace",
			line:  `writer-11 (11) [000] .... 1.000000: tracing_mark_write: C|42|Heap size (KB)|157028`,
			owner: 42, scope: traceCounterOwnerPayloadProcess, counter: "Heap size (KB)", value: 157028,
		},
		{
			name:  "harmony tag",
			line:  `rs-11 (11) [000] .... 1.000000: print: C|1252|H:VSync-rs|0|I38`,
			owner: 1252, scope: traceCounterOwnerPayloadProcess, counter: "H:VSync-rs", value: 0, trailingTag: "I38",
		},
		{
			name:  "production name with pipe separated unit",
			line:  `SharedPreferenc-64221 (63993) [008] .... 1.000000: print: C|23106|Heap size |(KB)|205455`,
			owner: 23106, scope: traceCounterOwnerPayloadProcess, counter: "Heap size |(KB)", value: 205455,
		},
		{
			name:  "production unit name with harmony tag",
			line:  `SharedPreferenc-64221 (63993) [008] .... 1.000000: print: C|23106|Heap size |(KB)|205455|I38`,
			owner: 23106, scope: traceCounterOwnerPayloadProcess, counter: "Heap size |(KB)", value: 205455, trailingTag: "I38",
		},
		{
			name:  "address carved harmony",
			line:  `rs-11 (11) [000] .... 1.000000: print: 0x0: 1252|H:DVSyncRateManagerPeriod|16.67|M0538`,
			owner: 1252, scope: traceCounterOwnerPayloadProcess, counter: "H:DVSyncRateManagerPeriod", value: 16.67, trailingTag: "M0538",
		},
		{
			name:  "explicit global",
			line:  `kernel-0 (0) [000] .... 1.000000: print: C|0|cpu_total_load|70.0`,
			owner: 0, scope: traceCounterOwnerGlobal, counter: "cpu_total_load", value: 70,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev, ok := ParseLine(1, tc.line, newStringInterner())
			if !ok || ev.Type != EventTraceMark || ev.SpanAction != "C" {
				t.Fatalf("counter row not recognised: ok=%v event=%+v", ok, ev)
			}
			sample := parseTraceCounterSample(ev)
			if !sample.identityOK || !sample.numericValid || sample.issueReason != "" {
				t.Fatalf("valid counter rejected: %+v", sample)
			}
			if sample.ownerPID != tc.owner || sample.ownerScope != tc.scope || sample.name != tc.counter || sample.numericValue != tc.value || sample.trailingTag != tc.trailingTag {
				t.Fatalf("parsed sample=%+v want owner=%d scope=%s name=%q value=%g tag=%q", sample, tc.owner, tc.scope, tc.counter, tc.value, tc.trailingTag)
			}
		})
	}
}

func TestTraceCounterCompatibilityInventorySharesTypedParserWithDeltas(t *testing.T) {
	idx := buildTraceIndex(t, "counter-inventory-parity.systrace", strings.Join([]string{
		// Exact production pipe-unit witness plus a second sample for delta.
		`SharedPreferenc-64221 (63993) [008] .... 1.000000: print: C|23106|Heap size |(KB)|205455`,
		`SharedPreferenc-64221 (63993) [008] .... 1.010000: print: C|23106|Heap size |(KB)|205460`,
		// Same grammar with an exact trailing Harmony tag.
		`SharedPreferenc-64221 (63993) [008] .... 1.020000: print: C|23106|Tagged heap |(KB)|7|M0538`,
		`SharedPreferenc-64221 (63993) [008] .... 1.030000: print: C|23106|Tagged heap |(KB)|9|M0538`,
		// Non-numeric and invalid rows remain legacy inventory, while the typed
		// quality face explains why they cannot mint counter_deltas.
		`SharedPreferenc-64221 (63993) [008] .... 1.040000: print: C|23106|Pending |(count)|NaN|I38`,
		`SharedPreferenc-64221 (63993) [008] .... 1.050000: print: C|bad|queue_depth|17`,
		`SharedPreferenc-64221 (63993) [008] .... 1.060000: print: C|23106|bad_extra|1|not-a-track`,
	}, "\n")+"\n")

	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.1})
	if stats.CounterQuality == nil || stats.CounterQuality.Rows != 7 || stats.CounterQuality.InvalidRows != 2 || stats.CounterQuality.NonNumericRows != 1 {
		t.Fatalf("counter quality did not retain invalid/non-numeric rows: %+v", stats.CounterQuality)
	}
	if len(stats.TraceCounters) != 7 {
		t.Fatalf("compatibility inventory lost a physical counter shape: %+v", stats.TraceCounters)
	}
	findInventory := func(name, value, tag string) *TraceCounterSummary {
		for i := range stats.TraceCounters {
			row := &stats.TraceCounters[i]
			if row.Name == name && row.Value == value && row.TrailingTag == tag {
				return row
			}
		}
		return nil
	}
	findDelta := func(name, tag string) *TraceCounterDeltaSummary {
		for i := range stats.CounterDeltas {
			row := &stats.CounterDeltas[i]
			if row.Name == name && row.TrailingTag == tag {
				return row
			}
		}
		return nil
	}

	production := findInventory("Heap size |(KB)", "205455", "")
	productionDelta := findDelta("Heap size |(KB)", "")
	if production == nil || productionDelta == nil {
		t.Fatalf("production pipe-unit row diverged across inventory/delta: inventory=%+v deltas=%+v", stats.TraceCounters, stats.CounterDeltas)
	}
	if production.OwnerPID != productionDelta.OwnerPID || production.OwnerScope != productionDelta.OwnerScope || production.OwnerRaw != "23106" || productionDelta.First != 205455 {
		t.Fatalf("production owner/value parity mismatch: inventory=%+v delta=%+v", production, productionDelta)
	}
	tagged := findInventory("Tagged heap |(KB)", "7", "M0538")
	taggedDelta := findDelta("Tagged heap |(KB)", "M0538")
	if tagged == nil || taggedDelta == nil || tagged.OwnerPID != taggedDelta.OwnerPID || tagged.OwnerScope != taggedDelta.OwnerScope || taggedDelta.First != 7 {
		t.Fatalf("tagged pipe-unit parity mismatch: inventory=%+v delta=%+v", tagged, taggedDelta)
	}
	if row := findInventory("Pending |(count)", "NaN", "I38"); row == nil || row.OwnerPID != 23106 || row.OwnerScope != traceCounterOwnerPayloadProcess {
		t.Fatalf("non-numeric typed identity disappeared from compatibility inventory: %+v", stats.TraceCounters)
	}
	if row := findInventory("queue_depth", "17", ""); row == nil || row.OwnerRaw != "bad" {
		t.Fatalf("invalid-owner row disappeared from compatibility inventory: %+v", stats.TraceCounters)
	}
	if row := findInventory("bad_extra", "1", ""); row == nil || row.OwnerRaw != "23106" {
		t.Fatalf("unsupported-extra row lost its bounded inventory fields: %+v", stats.TraceCounters)
	}
	if !containsSubstring(stats.Caveats, "trace_counter_quality_degraded=true") {
		t.Fatalf("invalid/non-numeric inventory was not paired with quality disclosure: %v", stats.Caveats)
	}
}

func TestTraceCounterDeltaUsesPayloadOwnerTagAndPhysicalSourceIdentity(t *testing.T) {
	idx := buildTraceIndex(t, "counter-identity.systrace", strings.Join([]string{
		`emitter-a-10 (10) [000] .... 1.000000: print: C|100|queue_depth|1|I1`,
		`emitter-b-11 (11) [001] .... 1.100000: print: C|100|queue_depth|4|I1`,
		`emitter-a-10 (10) [000] .... 1.200000: print: C|200|queue_depth|10|I1`,
		`emitter-a-10 (10) [000] .... 1.300000: print: C|200|queue_depth|7|I1`,
		`emitter-a-10 (10) [000] .... 1.400000: print: C|100|queue_depth|20|I2`,
		`emitter-b-11 (11) [001] .... 1.500000: print: C|100|queue_depth|25|I2`,
		`emitter-a-10 (10) [000] .... 1.600000: print: C|0|queue_depth|30`,
		`emitter-b-11 (11) [001] .... 1.700000: print: C|0|queue_depth|32`,
	}, "\n")+"\n")

	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.8})
	if len(stats.CounterDeltas) != 4 {
		t.Fatalf("payload owner/tag identities were merged or split by emitter: %+v", stats.CounterDeltas)
	}
	if stats.CounterQuality == nil || stats.CounterQuality.Rows != 8 || stats.CounterQuality.TotalSeries != 4 || stats.CounterQuality.PublishedSeries != 4 {
		t.Fatalf("counter quality inventory mismatch: %+v", stats.CounterQuality)
	}
	find := func(scope string, owner int, tag string) *TraceCounterDeltaSummary {
		for i := range stats.CounterDeltas {
			row := &stats.CounterDeltas[i]
			if row.OwnerScope == scope && row.OwnerPID == owner && row.TrailingTag == tag {
				return row
			}
		}
		return nil
	}
	owner100I1 := find(traceCounterOwnerPayloadProcess, 100, "I1")
	if owner100I1 == nil || owner100I1.Samples != 2 || owner100I1.First != 1 || owner100I1.Last != 4 || owner100I1.Delta != 3 {
		t.Fatalf("cross-emitter payload series not joined exactly: %+v", owner100I1)
	}
	if owner100I1.Thread.PID != 10 || owner100I1.OwnerPID != 100 {
		t.Fatalf("owner and emitter identities were conflated: %+v", owner100I1)
	}
	if row := find(traceCounterOwnerPayloadProcess, 200, "I1"); row == nil || row.Delta != -3 {
		t.Fatalf("second payload owner was merged with owner=100: %+v", row)
	}
	if row := find(traceCounterOwnerPayloadProcess, 100, "I2"); row == nil || row.Delta != 5 {
		t.Fatalf("trailing Harmony tag was not part of identity: %+v", row)
	}
	if row := find(traceCounterOwnerGlobal, 0, ""); row == nil || row.Delta != 2 {
		t.Fatalf("explicit pid=0 global series missing: %+v", row)
	}
	for _, row := range stats.CounterDeltas {
		if row.Baseline != traceCounterBaselineInWindowFirstSample || row.UnitStatus != traceCounterUnitUnknown {
			t.Fatalf("baseline/unit policy not explicit: %+v", row)
		}
		if row.SourcePath == "" || row.FirstLocalLine <= 0 || row.LastLocalLine <= 0 {
			t.Fatalf("physical provenance missing: %+v", row)
		}
	}
}

func TestTraceCounterUnknownUnitAndInvalidRowsFailLoudBounded(t *testing.T) {
	lines := []string{
		`writer-10 (10) [000] .... 1.000000: print: C|42|Heap size (KB)|100`,
		`writer-10 (10) [000] .... 1.010000: print: C|42|Heap size (KB)|NaN`,
		`writer-10 (10) [000] .... 1.020000: print: C|42|Heap size (KB)|120`,
		`writer-10 (10) [000] .... 1.030000: print: C|bad|queue_depth|1`,
		`writer-10 (10) [000] .... 1.040000: print: C|43||1`,
		`writer-10 (10) [000] .... 1.050000: print: C|44|queue_depth|Inf`,
		`writer-10 (10) [000] .... 1.060000: print: C|45|queue_depth|1|not-a-track`,
	}
	// More than the per-reason witness cap: counts remain complete while the
	// retained samples stay bounded.
	for i := 0; i < 5; i++ {
		lines = append(lines, `writer-10 (10) [000] .... 1.070000: print: C|46|bad_numeric|NaN`)
	}
	idx := buildTraceIndex(t, "counter-invalid.systrace", strings.Join(lines, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.2})

	if len(stats.CounterDeltas) != 0 {
		t.Fatalf("series with invalid endpoints/members minted a delta: %+v", stats.CounterDeltas)
	}
	if len(stats.TraceCounters) == 0 {
		t.Fatal("compatibility TraceCounters inventory must retain invalid/non-numeric rows")
	}
	quality := stats.CounterQuality
	if quality == nil || quality.Rows != len(lines) || quality.InvalidRows != 3 || quality.NonNumericRows != 7 || quality.SuppressedSeries != 3 {
		t.Fatalf("typed invalid inventory mismatch: %+v", quality)
	}
	if !containsSubstring(stats.Caveats, "trace_counter_quality_degraded=true") {
		t.Fatalf("invalid counters were fail-silent: %v", stats.Caveats)
	}
	var nonDecimal *TraceCounterIssueSummary
	for i := range quality.Issues {
		if quality.Issues[i].Reason == "non_decimal_or_non_finite_value" {
			nonDecimal = &quality.Issues[i]
		}
	}
	if nonDecimal == nil || nonDecimal.Count != 7 || len(nonDecimal.Samples) != traceCounterIssueSampleCap {
		t.Fatalf("issue count/sample cap mismatch: %+v", nonDecimal)
	}
}

func TestTraceCounterArbitraryExtraFieldsDoNotMasqueradeAsNameUnits(t *testing.T) {
	for _, payload := range []string{
		"C|42|queue_depth|1|not-a-track",
		"C|42|queue_depth|fabricated|1",
		"C|42|queue_depth|(KB|1",
	} {
		ev, ok := ParseLine(1, traceMarkTestLine("writer", 10, 1, payload), newStringInterner())
		if !ok || ev.SpanAction != "C" {
			t.Fatalf("counter inventory row missing for %q: %+v", payload, ev)
		}
		sample := parseTraceCounterSample(ev)
		if sample.identityOK && sample.numericValid {
			t.Fatalf("arbitrary extra fields minted a numeric series for %q: %+v", payload, sample)
		}
	}
}

func TestTraceCounterBaselineDoesNotGuessWindowHeadCarry(t *testing.T) {
	idx := buildTraceIndex(t, "counter-window-baseline.systrace", strings.Join([]string{
		`writer-10 (10) [000] .... 0.900000: print: C|42|Heap size (KB)|90`,
		`writer-10 (10) [000] .... 1.100000: print: C|42|Heap size (KB)|100`,
		`writer-10 (10) [000] .... 1.200000: print: C|42|Heap size (KB)|130`,
	}, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.3})
	if len(stats.CounterDeltas) != 1 {
		t.Fatalf("window counter missing: %+v", stats.CounterDeltas)
	}
	row := stats.CounterDeltas[0]
	if row.First != 100 || row.Last != 130 || row.Delta != 30 || row.Baseline != traceCounterBaselineInWindowFirstSample {
		t.Fatalf("pre-window sample was silently treated as carry-in: %+v", row)
	}
	if row.UnitStatus != traceCounterUnitUnknown {
		t.Fatalf("unit was guessed from '(KB)' name text: %+v", row)
	}
}

func TestTraceCounterPayloadLaneRollbackSurvivesEmitterAndHostLifecycleChanges(t *testing.T) {
	idx := buildTraceIndex(t, "counter-payload-rollback.systrace", strings.Join([]string{
		`writer-a-500 (500) [000] .... 2.000000: print: C|42|queue_depth|7|I1`,
		`creator-7 (7) [001] .... 2.100000: sched_wakeup_new: comm=host42 pid=42 prio=20 target_cpu=001`,
		`writer-b-501 (501) [002] .... 1.500000: print: C|42|queue_depth|3|I1`,
	}, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 2.2})
	if len(stats.CounterDeltas) != 0 || !containsSubstring(stats.Caveats, "family=trace_counter_delta") {
		t.Fatalf("canonical/emitter/lifecycle changes hid a physical payload-lane rollback: deltas=%+v caveats=%v", stats.CounterDeltas, stats.Caveats)
	}
}

func TestTraceCounterFiniteEndpointsWithInfiniteDeltaFailClosed(t *testing.T) {
	max := strconv.FormatFloat(math.MaxFloat64, 'f', -1, 64)
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 1, Type: EventTraceMark, SpanAction: "C", SpanPID: 42, SpanName: "extreme", SpanValue: "-" + max, PID: 10, Comm: "writer"},
		{Line: 2, Ts: 2, Type: EventTraceMark, SpanAction: "C", SpanPID: 42, SpanName: "extreme", SpanValue: max, PID: 10, Comm: "writer"},
	}}
	deltas, quality := computeCounterDeltas(idx, Query{}, 8)
	if len(deltas) != 0 || quality == nil || quality.DerivedInvalidSeries != 1 || quality.SuppressedSeries != 1 {
		t.Fatalf("finite endpoints minted a non-finite derived delta: deltas=%+v quality=%+v", deltas, quality)
	}
	var issue *TraceCounterIssueSummary
	for i := range quality.Issues {
		if quality.Issues[i].Reason == "derived_delta_non_finite" {
			issue = &quality.Issues[i]
		}
	}
	if issue == nil || issue.Count != 1 || len(issue.Samples) != 1 {
		t.Fatalf("derived overflow was not disclosed with a bounded typed witness: %+v", quality)
	}
}

func TestTraceCounterSeriesBudgetFailsClosedWithoutIncompleteTopN(t *testing.T) {
	const overflow = 7
	events := make([]Event, 0, traceCounterSeriesBudget+overflow)
	for i := 0; i < traceCounterSeriesBudget+overflow; i++ {
		name := "counter_" + strconv.Itoa(i)
		value := strconv.Itoa(i)
		events = append(events, Event{
			Line: i + 1, Ts: 1 + float64(i)/100000, Type: EventTraceMark,
			SpanAction: "C", SpanPID: 42, SpanName: name, SpanValue: value,
			FieldText: "C|42|" + name + "|" + value, PID: 10, Comm: "writer",
		})
	}
	idx := &Index{Events: events}
	stats := ComputeWindowStats(idx, Query{})
	if len(stats.CounterDeltas) != 0 {
		t.Fatalf("series-budget overflow published top-N from an incomplete universe: %+v", stats.CounterDeltas)
	}
	quality := stats.CounterQuality
	if quality == nil || !quality.SeriesBudgetExceeded || quality.SeriesBudget != traceCounterSeriesBudget || quality.OverflowRows != overflow {
		t.Fatalf("series budget overflow not typed exactly: %+v", quality)
	}
	if quality.TotalSeries != traceCounterSeriesBudget || quality.TotalSeriesStatus != "lower_bound" || quality.PublishedSeries != 0 || quality.SuppressedSeries != traceCounterSeriesBudget {
		t.Fatalf("over-budget series accounting is not fail-closed: %+v", quality)
	}
	var budgetIssue *TraceCounterIssueSummary
	for i := range quality.Issues {
		if quality.Issues[i].Reason == "series_budget_exceeded" {
			budgetIssue = &quality.Issues[i]
		}
	}
	if budgetIssue == nil || budgetIssue.Count != overflow || len(budgetIssue.Samples) != traceCounterIssueSampleCap {
		t.Fatalf("budget issue count/witness cap mismatch: %+v", budgetIssue)
	}
	if len(stats.TraceCounters) == 0 {
		t.Fatal("series budget must not suppress the compatibility TraceCounters inventory")
	}
	if !containsSubstring(stats.Caveats, "budget_exceeded=true") {
		t.Fatalf("series budget fail-close was not disclosed: %v", stats.Caveats)
	}
}

func TestTraceCounterColdWarmWindowAndCompositeProvenance(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "counter-window.systrace")
	body := strings.Join([]string{
		`writer-10 (10) [000] .... 0.900000: print: C|42|queue_depth|90|I1`,
		`writer-10 (10) [000] .... 1.100000: print: C|42|queue_depth|100|I1`,
		`writer-11 (11) [001] .... 1.200000: print: C|42|queue_depth|130|I1`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := BuildOptions{AllowWindowedParse: true, TimeStartSet: true, TimeStart: 1.0, TimeEndSet: true, TimeEnd: 1.3}
	q := Query{TimeStart: 1.0, TimeEnd: 1.3}

	resetAnchorCaches()
	if _, err := BuildIndex(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	warm, err := BuildIndexWithOptions(context.Background(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	warmStats := ComputeWindowStats(warm, q)

	resetAnchorCaches()
	cold, err := BuildIndexWithOptions(context.Background(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	coldStats := ComputeWindowStats(cold, q)
	if !reflect.DeepEqual(coldStats.CounterDeltas, warmStats.CounterDeltas) || !reflect.DeepEqual(coldStats.CounterQuality, warmStats.CounterQuality) {
		t.Fatalf("cold/warm counter semantics diverged:\ncold=%+v quality=%+v\nwarm=%+v quality=%+v", coldStats.CounterDeltas, coldStats.CounterQuality, warmStats.CounterDeltas, warmStats.CounterQuality)
	}

	// A composite's virtual-coordinate resolver is part of the series identity.
	// Two causally admitted source ledgers carrying the same payload token must
	// remain two series even after canonical timestamp sorting.
	composite := &Index{
		Path: "bundle.tracebundle.json",
		TraceArtifacts: []TraceArtifactSource{
			{SourcePath: filepath.Join(dir, "a.systrace"), TimeDomain: "trace_seconds", CanonicalTimeDomain: "trace_seconds", LocalLineCount: 2, CausalCompatible: true},
			{SourcePath: filepath.Join(dir, "b.systrace"), TimeDomain: "trace_seconds", CanonicalTimeDomain: "trace_seconds", VirtualLineBase: 100, LocalLineCount: 2, CausalCompatible: true},
		},
		Events: []Event{
			{Line: 1, Ts: 1.0, Type: EventTraceMark, SpanAction: "C", SpanPID: 42, SpanName: "queue_depth", SpanValue: "1", FieldText: "C|42|queue_depth|1", PID: 10},
			{Line: 101, Ts: 1.05, Type: EventTraceMark, SpanAction: "C", SpanPID: 42, SpanName: "queue_depth", SpanValue: "10", FieldText: "C|42|queue_depth|10", PID: 20},
			{Line: 2, Ts: 1.1, Type: EventTraceMark, SpanAction: "C", SpanPID: 42, SpanName: "queue_depth", SpanValue: "3", FieldText: "C|42|queue_depth|3", PID: 11},
			{Line: 102, Ts: 1.15, Type: EventTraceMark, SpanAction: "C", SpanPID: 42, SpanName: "queue_depth", SpanValue: "20", FieldText: "C|42|queue_depth|20", PID: 21},
		},
	}
	deltas, quality := computeCounterDeltas(composite, Query{TimeStart: 0.9, TimeEnd: 1.2}, 8)
	if len(deltas) != 2 || quality == nil || quality.TotalSeries != 2 {
		t.Fatalf("composite sources were merged by lookalike payload identity: deltas=%+v quality=%+v", deltas, quality)
	}
	if deltas[0].SourcePath == deltas[1].SourcePath {
		t.Fatalf("physical provenance absent from identity: %+v", deltas)
	}
}
