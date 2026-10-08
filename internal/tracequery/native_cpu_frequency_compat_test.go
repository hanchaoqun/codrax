package tracequery

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func nativeFrequencyTestRow(t *testing.T, id, start, duration, frequency int64, cpu int) string {
	t.Helper()
	line, err := tracewire.FormatCPUMeasureInterval(tracewire.CPUMeasureInterval{RowID: id, FilterID: int64(cpu + 1), CPU: cpu,
		Kind: "frequency", Encoding: "khz", StartNS: &start, DurationNS: &duration, Value: &frequency})
	if err != nil {
		t.Fatal(err)
	}
	return line + "\n"
}

const nativeFrequencyRunningTrace = `idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=work next_pid=41 next_prio=120
work-41 (41) [000] .... 1.100000: sched_switch: prev_comm=work prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
`

func TestNativeCPUFrequencyWindowPreservesResidencyAndRunningContext(t *testing.T) {
	text := nativeFrequencyTestRow(t, 1, 990000000, 60000000, 1000000, 0) +
		nativeFrequencyTestRow(t, 2, 1050000000, 100000000, 2000000, 0) + nativeFrequencyRunningTrace
	idx := buildTraceIndex(t, "native.systrace", text)
	q := Query{TimeStart: 1, TimeEnd: 1.1, PID: 41}
	stats := ComputeWindowStats(idx, q)
	if len(stats.CPU) != 1 || len(stats.CPU[0].FrequencyResidency) != 2 {
		t.Fatalf("explicit native residency lost: %+v", stats.CPU)
	}
	assertJointNear(t, stats.CPU[0].FrequencyResidency[0].DurationMs, 50)
	assertJointNear(t, stats.CPU[0].FrequencyResidency[1].DurationMs, 50)
	if len(stats.TopRunning) != 1 || stats.TopRunning[0].Frequency != 1000000 || stats.TopRunning[0].freqKnownMs < 99.999999 {
		t.Fatalf("thread start-frequency/coverage lost: %+v", stats.TopRunning)
	}
	bal := stats.ComputeSupplyBalance
	if bal == nil || !bal.PerCPU[0].FrequencyKnown {
		t.Fatalf("fully measured native running supply withheld: %+v", bal)
	}
	assertJointNear(t, bal.DeliveredComputeMs, 75)
	assertJointNear(t, bal.LowFrequencyLossMs, 25)
	if stats.CPUFrequencySampleRowCount != 0 || stats.EventCounts[EventCPUFrequency] != 0 {
		t.Fatal("interval endpoints fabricated hardware frequency events")
	}
	if stats.TopRunning[0].freqInSegmentSamples != 0 || !stats.TopRunning[0].freqExplicitIntervals {
		t.Fatal("explicit intervals fabricated sampling points")
	}
	for _, s := range computeSupplySummaries(stats, 8) {
		if s.FrequencySample == FrequencySampleNearestFallback {
			t.Fatal("explicit interval integration mislabeled as nearest sample fallback")
		}
	}
	if got := stats.targetCPUFrequencyCensus.rows[threadCPUKey(ThreadRef{Comm: "work", PID: 41}, 0)]; got.FrequencyKHz != 1000000 || got.ClusterDonorCPU != nil {
		t.Fatalf("target representative changed caliber or borrowed frequency: %+v", got)
	}
}

func TestNativeCPUFrequencyGapsOverlapAndMissingKeepSupplyUnknown(t *testing.T) {
	for _, tc := range []struct{ name, rows string }{
		{"hole", nativeFrequencyTestRow(t, 1, 1000000000, 40000000, 1000000, 0) + nativeFrequencyTestRow(t, 2, 1060000000, 40000000, 2000000, 0)},
		{"overlap", nativeFrequencyTestRow(t, 1, 1000000000, 100000000, 1000000, 0) + nativeFrequencyTestRow(t, 2, 1040000000, 20000000, 2000000, 0)},
		{"other_cpu", nativeFrequencyTestRow(t, 1, 1000000000, 100000000, 2000000, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx := buildTraceIndex(t, "native_gap.systrace", tc.rows+nativeFrequencyRunningTrace)
			q := Query{TimeStart: 1, TimeEnd: 1.1, CoreTopology: "0:little,1:little"}
			stats := ComputeWindowStats(idx, q)
			if stats.ComputeSupplyBalance != nil || !strings.Contains(strings.Join(stats.Caveats, "\n"), "compute_supply_balance_unavailable=native_frequency_does_not_cover_running_intervals") {
				t.Fatalf("unmeasured gap became full-speed or zero loss: %+v / %v", stats.ComputeSupplyBalance, stats.Caveats)
			}
			w := nativeCPUFrequencyForWindow(idx, q)
			if w.at(0, 1.05) != 0 || w.segment(0, 1, 1.1).known {
				t.Fatal("hole, overlap or other CPU supplied a frequency")
			}
			if tc.name != "other_cpu" {
				if len(stats.CPU[0].FrequencyResidency) != 2 {
					t.Fatalf("known sides of gap unnecessarily lost: %+v", stats.CPU)
				}
				assertJointNear(t, stats.CPU[0].FrequencyResidency[0].EndTs, 1.04)
				assertJointNear(t, stats.CPU[0].FrequencyResidency[1].StartTs, 1.06)
			}
		})
	}
}

func TestNativeCPUFrequencyCompatibilityUsesUncappedAuthority(t *testing.T) {
	var text strings.Builder
	for i := 0; i < 80; i++ {
		text.WriteString(nativeFrequencyTestRow(t, int64(i+1), int64(i)*1000000, 1000000, int64(1000000+i), 0))
	}
	idx := buildTraceIndex(t, "native_many.systrace", text.String())
	q := Query{TimeStartSet: true, TimeEnd: .08}
	w := nativeCPUFrequencyForWindow(idx, q)
	if len(w.byCPU[0]) != 80 || w.at(0, .0795) != 1000079 {
		t.Fatalf("display cap truncated measurement: %+v", w)
	}
	fs := w.segment(0, 0, .08)
	if !fs.known || math.Abs(fs.weightedKHz-1000039.5) > 1e-5 {
		t.Fatalf("uncapped integration incorrect: %+v", fs)
	}
}

func TestNativeCPUFrequencyCompatibilitySourceAndMalformedGates(t *testing.T) {
	for _, change := range []func(*Index){
		func(i *Index) { i.Windowed = true },
		func(i *Index) { i.RelationScoped = true },
		func(i *Index) { i.CPUIntervalMalformed++ },
		func(i *Index) { i.TraceArtifacts = []TraceArtifactSource{{SourcePath: "different"}} },
	} {
		idx := buildTraceIndex(t, "native_source.systrace", nativeFrequencyTestRow(t, 1, 1000000000, 100000000, 1000000, 0)+nativeFrequencyRunningTrace)
		change(idx)
		w := nativeCPUFrequencyForWindow(idx, Query{TimeStart: 1, TimeEnd: 1.1})
		if w == nil || w.reason == "" || w.at(0, 1.02) != 0 {
			t.Fatalf("source/integrity gate bypass: %+v", w)
		}
	}
	idx := buildTraceIndex(t, "ordinary.systrace", cpuJointLine("1.000000", "cpu_frequency", "1000000", "0"))
	if got := nativeCPUFrequencyForWindow(idx, Query{TimeStart: 1, TimeEnd: 1.1}); got != nil {
		t.Fatal("ordinary ftrace behavior changed")
	}
}

func TestNativeCPUFrequencyInvalidCombinedFieldsCannotHideCoverage(t *testing.T) {
	start := int64(1040000000)
	bad, err := tracewire.FormatCPUMeasureInterval(tracewire.CPUMeasureInterval{RowID: 2, FilterID: 1, CPU: 0,
		Kind: "frequency", Encoding: "khz", StartNS: &start, Issue: "invalid_value"})
	if err != nil {
		t.Fatal(err)
	}
	idx := buildTraceIndex(t, "native_bad.systrace", nativeFrequencyTestRow(t, 1, 1000000000, 100000000, 1000000, 0)+bad+"\n"+nativeFrequencyRunningTrace)
	w := nativeCPUFrequencyForWindow(idx, Query{TimeStart: 1, TimeEnd: 1.1})
	if w.at(0, 1.03) != 1000000 || w.at(0, 1.05) != 0 {
		t.Fatalf("combined missing duration/value lost unknown mask: %s", fmt.Sprint(w.byCPU))
	}
}

func TestNativeCPUFrequencyBoundaryAndOwnership(t *testing.T) {
	text := nativeFrequencyTestRow(t, 1, 990000000, 40000000, 1000000, 0) +
		nativeFrequencyTestRow(t, 2, 1050000000, 40000000, 2000000, 0) +
		nativeFrequencyTestRow(t, 3, 1100000000, 10000000, 3000000, 0)
	idx := buildTraceIndex(t, "native_boundary.systrace", text)
	w := nativeCPUFrequencyForWindow(idx, Query{TimeStart: 1, TimeEnd: 1.1})
	for _, tc := range []struct {
		at   float64
		want int64
	}{{1, 1000000}, {1.03, 0}, {1.049, 0}, {1.05, 2000000}, {1.09, 0}, {1.1, 0}} {
		if got := w.at(0, tc.at); got != tc.want {
			t.Fatalf("half-open lookup at %f = %d, want %d", tc.at, got, tc.want)
		}
	}
	if w.segment(0, 1.04, 1.049).known || w.segment(0, 1.08, 1.1).known {
		t.Fatal("nearest or preceding frequency filled absent explicit coverage")
	}
	if got := eventCPUForStats(Event{Type: EventCPUMeasureInterval, CPU: 0}); got != -1 {
		t.Fatal("native interval without payload ownership borrowed emitter CPU")
	}
}

func TestNativeCPUFrequencyMixedRunningSegmentsDoNotClaimFullCoverage(t *testing.T) {
	const segments = `idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=work next_pid=41 next_prio=120
work-41 (41) [000] .... 1.040000: sched_switch: prev_comm=work prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
idle-0 (0) [000] .... 1.060000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=work next_pid=41 next_prio=120
work-41 (41) [000] .... 1.100000: sched_switch: prev_comm=work prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
`
	for _, start := range []int64{1000000000, 1060000000} {
		t.Run(fmt.Sprint(start), func(t *testing.T) {
			idx := buildTraceIndex(t, "native_partial_thread.systrace", nativeFrequencyTestRow(t, 1, start, 40000000, 1000000, 0)+segments)
			stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.1, PID: 41})
			if len(stats.TopRunning) != 1 {
				t.Fatalf("expected one physical thread/CPU bucket: %+v", stats.TopRunning)
			}
			td := stats.TopRunning[0]
			if td.freqExplicitIntervals || td.freqKnownMs != 0 || td.weightedFrequencyKHz() != 0 || td.freqInSegmentSamples != 0 {
				t.Fatalf("one known segment promoted the whole thread to measured coverage: %+v", td)
			}
			if stats.ComputeSupplyBalance != nil {
				t.Fatal("partial native thread coverage became measured supply")
			}
			assertJointNear(t, td.DurationMs, 80)
			if len(stats.CPU[0].FrequencyResidency) != 1 {
				t.Fatal("known native interval discarded with unsupported whole-thread scalar")
			}
		})
	}
}

func TestNativeCPUFrequencyExplicitCoverageAggregatesWithAND(t *testing.T) {
	for _, tc := range []struct {
		name       string
		firstKnown bool
		lastKnown  bool
		lastCPU    int
		wantKnown  bool
	}{
		{"both", true, true, 0, true},
		{"unknown_tail", true, false, 0, false},
		{"unknown_head", false, true, 0, false},
		{"migration", true, true, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			member := ThreadDuration{Thread: ThreadRef{Comm: "work", PID: 41}, CPU: 0, DurationMs: 10, StartTs: 1, EndTs: 1.01,
				freqKnownMs: 10, freqWeightKHzMs: 10000000, freqExplicitIntervals: tc.firstKnown}
			last := member
			last.StartTs, last.EndTs, last.CPU, last.freqExplicitIntervals = 1.02, 1.03, tc.lastCPU, tc.lastKnown
			got := aggregateChainRunnableCensusByThread(map[string]ThreadDuration{"a": member, "b": last}, map[int]bool{41: true}, 0)
			if len(got) != 1 || got[0].freqExplicitIntervals != tc.wantKnown {
				t.Fatalf("aggregation promoted partial or cross-CPU evidence: %+v", got)
			}
		})
	}
}
