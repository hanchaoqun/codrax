package tracequery

// cluster_ceilings_test.go — CFC batch (§7.10 VS-2c 设计) coverage: the
// shared per-CPU sample predicate, the shared cluster-ceiling core (ladder +
// clustering + big-cluster pick), and the WindowStats snapshot including its
// DELIBERATE limits-caliber fork (head-governing + in-window vs
// stats.CPUFrequencyLimits' strict in-window) and its non-observation status
// (json:"-", no registry token).

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestIsPerCPUFrequencySample(t *testing.T) {
	intern := newStringInterner()
	parse := func(line string) Event {
		t.Helper()
		ev, ok := ParseLine(1, line, intern)
		if !ok {
			t.Fatalf("line must parse: %s", line)
		}
		return ev
	}
	genuine := parse(`      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1000000 cpu_id=2`)
	if !isPerCPUFrequencySample(genuine) {
		t.Fatalf("genuine cpu_frequency sample must pass: %+v", genuine)
	}
	lane := parse(`          clk-90 (  90) [000] .... 5.005000: clock_set_rate: cpu_freq 3000000`)
	if lane.Type != EventCPUFrequency || lane.Name != "clock_set_rate" {
		t.Fatalf("fixture drift: the lane line must be a reclassified clock_set_rate: %+v", lane)
	}
	if isPerCPUFrequencySample(lane) {
		t.Fatalf("reclassified clock lane must be excluded: %+v", lane)
	}
	// Keyed lane WITH cpu_id: attribution would be correct, but the VS-2c
	// ruling excludes clock lanes from the per-CPU basis wholesale (verbatim
	// Name), matching the fold face.
	keyedLane := parse(`          clk-90 (  90) [000] .... 5.006000: clock_set_rate: pid_freq state=1800000 cpu_id=0`)
	if keyedLane.Type != EventCPUFrequency || keyedLane.Name != "clock_set_rate" {
		t.Fatalf("fixture drift: keyed lane must be a reclassified clock_set_rate: %+v", keyedLane)
	}
	if isPerCPUFrequencySample(keyedLane) {
		t.Fatalf("keyed clock lane must be excluded even with cpu_id attribution: %+v", keyedLane)
	}
	if isPerCPUFrequencySample(Event{Type: EventCPUFrequency, Name: "cpu_frequency", Frequency: 0}) {
		t.Fatalf("zero-frequency sample must be excluded")
	}
	if isPerCPUFrequencySample(Event{Type: EventClockSetRate, Name: "clock_set_rate", Frequency: 100}) {
		t.Fatalf("non-cpu_frequency type must be excluded")
	}
}

func TestComputeClusterFrequencyCeilings(t *testing.T) {
	t.Run("ladder and grouping across three clusters", func(t *testing.T) {
		observed := map[int]int{0: 1000000, 1: 1100000, 4: 2000000, 7: 3000000}
		coreByCPU := map[int]string{0: "small", 1: "small", 4: "middle", 7: "big"}
		limits := func(cpu int) int {
			if cpu == 7 {
				return 3200000
			}
			return 0
		}
		out := computeClusterFrequencyCeilings(observed, coreByCPU, limits)
		if len(out) != 3 {
			t.Fatalf("want 3 clusters, got %+v", out)
		}
		small, middle, big := out[0], out[1], out[2]
		if small.CoreClass != "small" || len(small.CPUs) != 2 || small.CPUs[0] != 0 || small.CPUs[1] != 1 ||
			small.FmaxKHz != 1100000 || small.Source != SupplyFoldFmaxSourceObserved {
			t.Fatalf("small cluster wrong: %+v", small)
		}
		if middle.CoreClass != "middle" || middle.FmaxKHz != 2000000 || middle.Source != SupplyFoldFmaxSourceObserved {
			t.Fatalf("middle cluster wrong: %+v", middle)
		}
		// VS-2b ladder: the governing limits Max (3.2GHz) beats the observed
		// 3.0GHz sample on the big cluster.
		if big.CoreClass != "big" || big.FmaxKHz != 3200000 || big.Source != SupplyFoldFmaxSourceLimit {
			t.Fatalf("big cluster must take the limits rung: %+v", big)
		}
		if picked := pickBigClusterCeiling(out); picked == nil || picked.CoreClass != "big" {
			t.Fatalf("pick must select the highest known class: %+v", picked)
		}
	})

	t.Run("unknown topology folds one unclassified pool", func(t *testing.T) {
		observed := map[int]int{0: 1000000, 3: 2000000}
		out := computeClusterFrequencyCeilings(observed, map[int]string{}, nil)
		if len(out) != 1 || out[0].CoreClass != "" || len(out[0].CPUs) != 2 || out[0].FmaxKHz != 2000000 || out[0].Source != SupplyFoldFmaxSourceObserved {
			t.Fatalf("want single unclassified pool at observed max: %+v", out)
		}
		if picked := pickBigClusterCeiling(out); picked == nil || picked.CoreClass != "" {
			t.Fatalf("pick must fall back to the unclassified pool: %+v", picked)
		}
	})

	t.Run("mixed known and unknown classes stay separate and pick prefers known", func(t *testing.T) {
		observed := map[int]int{0: 900000, 5: 2500000}
		coreByCPU := map[int]string{5: "middle"}
		out := computeClusterFrequencyCeilings(observed, coreByCPU, nil)
		if len(out) != 2 {
			t.Fatalf("want middle + unclassified, got %+v", out)
		}
		if out[0].CoreClass != "middle" || out[1].CoreClass != "" {
			t.Fatalf("sort order must be known classes then unclassified: %+v", out)
		}
		if picked := pickBigClusterCeiling(out); picked == nil || picked.CoreClass != "middle" {
			t.Fatalf("pick must prefer the known class over the unclassified pool: %+v", picked)
		}
	})

	t.Run("a limits row without any observed sample never mints a ceiling", func(t *testing.T) {
		// CPU 9 has a limit but no cpu_frequency sample: same membership rule
		// the fold face always used — no observed governance, no ceiling.
		observed := map[int]int{1: 1500000}
		out := computeClusterFrequencyCeilings(observed, map[int]string{}, func(cpu int) int {
			if cpu == 9 {
				return 3000000
			}
			return 0
		})
		if len(out) != 1 || len(out[0].CPUs) != 1 || out[0].CPUs[0] != 1 || out[0].FmaxKHz != 1500000 {
			t.Fatalf("limit-only CPU must not participate: %+v", out)
		}
	})

	t.Run("empty input yields no ceilings and nil pick", func(t *testing.T) {
		if out := computeClusterFrequencyCeilings(map[int]int{}, map[int]string{}, nil); len(out) != 0 {
			t.Fatalf("want empty, got %+v", out)
		}
		if picked := pickBigClusterCeiling(nil); picked != nil {
			t.Fatalf("nil input must pick nil, got %+v", picked)
		}
	})
}

// TestComputeWindowStats_ClusterFrequencyCeilingsSnapshot pins the P1 window
// snapshot: per-CPU observed fmax reads the F1-governed residency timeline;
// the limits rung reads the head-governing + in-window caliber — the
// PRE-WINDOW limits row governs the snapshot while stats.CPUFrequencyLimits
// (strict in-window, untouched caliber) stays empty. That caliber fork is
// deliberate (limits rows only fire on change).
func TestComputeWindowStats_ClusterFrequencyCeilingsSnapshot(t *testing.T) {
	idx := buildTraceIndex(t, "cfc_snapshot.systrace", `
      <idle>-0 (-----) [000] .... 4.900000: cpu_frequency: state=1000000 cpu_id=0
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1200000 cpu_id=2
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=2000000 cpu_id=7
      <idle>-0 (-----) [000] .... 4.950000: cpu_frequency_limits: min=500000 max=2400000 cpu_id=7
        app-100 (100) [000] .... 5.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120
        app-100 (100) [000] .... 5.002000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
	`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 5.0, TimeEnd: 5.010})
	if len(stats.CPUFrequencyLimits) != 0 {
		t.Fatalf("strict in-window limits caliber must stay untouched (pre-window row excluded): %+v", stats.CPUFrequencyLimits)
	}
	ceilings := stats.ClusterFrequencyCeilings
	if len(ceilings) != 3 {
		t.Fatalf("want small/middle/big ceilings, got %+v", ceilings)
	}
	if ceilings[0].CoreClass != "small" || ceilings[0].FmaxKHz != 1000000 || ceilings[0].Source != SupplyFoldFmaxSourceObserved {
		t.Fatalf("small ceiling wrong: %+v", ceilings[0])
	}
	if ceilings[1].CoreClass != "middle" || ceilings[1].FmaxKHz != 1200000 || ceilings[1].Source != SupplyFoldFmaxSourceObserved {
		t.Fatalf("middle ceiling wrong: %+v", ceilings[1])
	}
	// Big cluster: head-governing limits row (2.4GHz, pre-window) beats the
	// observed 2.0GHz sample.
	if ceilings[2].CoreClass != "big" || len(ceilings[2].CPUs) != 1 || ceilings[2].CPUs[0] != 7 ||
		ceilings[2].FmaxKHz != 2400000 || ceilings[2].Source != SupplyFoldFmaxSourceLimit {
		t.Fatalf("big ceiling must take the head-governing limits rung: %+v", ceilings[2])
	}

	// Non-observation pin (CFC ruling: no new token, internal structure):
	// the snapshot must not appear on any JSON surface of WindowStats.
	blob, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(strings.ToLower(string(blob)), "ceiling") {
		t.Fatalf("ClusterFrequencyCeilings leaked into the JSON observation face")
	}

	// Consumer dedup witness: the compute-supply ledger's per-CPU fmax reads
	// the SAME window observed source (residency caliber) — cpu0 observed via
	// sched_switch must carry MaxFrequencyKHz 1.0GHz.
	if stats.ComputeSupplyBalance == nil || len(stats.ComputeSupplyBalance.PerCPU) != 1 {
		t.Fatalf("want a single observed-CPU ledger row: %+v", stats.ComputeSupplyBalance)
	}
	if per := stats.ComputeSupplyBalance.PerCPU[0]; per.CPU != 0 || per.MaxFrequencyKHz != 1000000 || !per.FrequencyKnown {
		t.Fatalf("compute-supply per-CPU fmax must read the shared observed source: %+v", per)
	}
}

// TestPerCPULimitSampleAdmissionCrossFaceIdentity is the F1 (2026-07-05
// review) cross-face pin, same treatment as the freq-predicate pins: BOTH
// cpu_frequency_limits collection faces (window face limitTimelineByCPU and
// fold face chainQueryCache.freqLimitByCPU) must admit exactly the event set
// the ONE shared predicate isPerCPULimitSample admits — over the same events,
// their admission result sets are equal. Window filtering stays a per-caller
// convention and is neutralized here (fold face: full trace; window face:
// witnessed through the head-governing ceilings rung).
func TestPerCPULimitSampleAdmissionCrossFaceIdentity(t *testing.T) {
	// Discriminating rows: an admissible keyed limits row (emitted on cpu0,
	// attributed to cpu3 by cpu_id), a zero-Max limits row (excluded), a
	// genuine cpu_frequency sample and a reclassified clock lane (both
	// non-limits), plus in-window sched activity so the window face mints
	// stats for cpu3.
	idx := buildTraceIndex(t, "f1_limits_admission.systrace", `
      <idle>-0 (-----) [003] .... 4.900000: cpu_frequency: state=1000000 cpu_id=3
      <idle>-0 (-----) [000] .... 4.910000: cpu_frequency_limits: min=500000 max=2000000 cpu_id=3
      <idle>-0 (-----) [003] .... 4.920000: cpu_frequency_limits: min=0 max=0 cpu_id=3
          clk-90 (  90) [003] .... 4.930000: clock_set_rate: cpu_freq 5000000
        app-100 (100) [003] .... 5.000000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120
        app-100 (100) [003] .... 5.002000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
	`)

	// Fixture drift guards: the zero-Max row must exist as a typed limits
	// event (so its exclusion is a live probe), and the keyed row must carry
	// the cpu_id attribution.
	var sawZeroMax bool
	for _, ev := range idx.Events {
		if ev.Type == EventCPUFrequencyLimit && ev.FrequencyMax == 0 {
			sawZeroMax = true
		}
	}
	if !sawZeroMax {
		t.Fatalf("fixture drift: the zero-Max cpu_frequency_limits row must parse as a typed limits event")
	}

	// The single-definition admission set.
	expected := map[int][]freqSample{}
	for _, ev := range idx.Events {
		if cpu, ok := isPerCPULimitSample(ev); ok {
			expected[cpu] = append(expected[cpu], freqSample{ts: ev.Ts, khz: ev.FrequencyMax})
		}
	}
	if len(expected) != 1 || len(expected[3]) != 1 || expected[3][0].khz != 2000000 {
		t.Fatalf("admission set wrong: zero-Max/lane/cpu_frequency rows must be excluded and the keyed row must attribute to cpu3: %+v", expected)
	}

	// Fold face: member-identical to the single definition.
	c := newChainQueryCache(idx)
	c.buildFreqLimitIndex()
	if !reflect.DeepEqual(c.freqLimitByCPU, expected) {
		t.Fatalf("fold-face limits admission diverged from isPerCPULimitSample:\n got %+v\nwant %+v", c.freqLimitByCPU, expected)
	}

	// Window face witness on the same events: the head-governing ceilings
	// rung must read EXACTLY the admitted 2.0GHz row — an admitted zero-Max
	// row would flip the head-governing sample to khz=0 (Source falls back to
	// observed 1.0GHz), an admitted 5.0e6 lane row would raise it.
	stats := ComputeWindowStats(idx, Query{TimeStart: 5.0, TimeEnd: 5.010})
	if len(stats.ClusterFrequencyCeilings) != 1 {
		t.Fatalf("want a single ceiling for cpu3: %+v", stats.ClusterFrequencyCeilings)
	}
	if got := stats.ClusterFrequencyCeilings[0]; len(got.CPUs) != 1 || got.CPUs[0] != 3 ||
		got.FmaxKHz != 2000000 || got.Source != SupplyFoldFmaxSourceLimit {
		t.Fatalf("window-face limits admission diverged from isPerCPULimitSample: %+v", got)
	}

	// Direct predicate probes for the unkeyed CPU-attribution fallback (no
	// cpu_id key → emitting CPU) — the shared rule both faces used to
	// hand-roll separately.
	if cpu, ok := isPerCPULimitSample(Event{Type: EventCPUFrequencyLimit, FrequencyMax: 800000, CPU: 5}); !ok || cpu != 5 {
		t.Fatalf("unkeyed limits row must admit on the emitting CPU: cpu=%d ok=%t", cpu, ok)
	}
	if cpu, ok := isPerCPULimitSample(Event{Type: EventCPUFrequencyLimit, FrequencyMax: 800000, CPU: 5, CPUForFieldValid: true, CPUForField: 7}); !ok || cpu != 7 {
		t.Fatalf("keyed limits row must admit on the cpu_id CPU: cpu=%d ok=%t", cpu, ok)
	}
	if _, ok := isPerCPULimitSample(Event{Type: EventCPUFrequencyLimit, FrequencyMax: 0, CPU: 5}); ok {
		t.Fatalf("zero-Max limits row must be excluded")
	}
	if _, ok := isPerCPULimitSample(Event{Type: EventCPUFrequency, Frequency: 800000, FrequencyMax: 800000, CPU: 5}); ok {
		t.Fatalf("non-limits event type must be excluded")
	}
}

// TestClusterCeilingsHeaderContractPinned is the F2(b) (2026-07-05 review)
// header-honesty pin, following the repo's source-reading pin precedent
// (TestRootCauseTypeZHLabelCoversWeightUniverse): the cluster_ceilings.go
// header must keep saying that the two faces share the CORE FUNCTION ONLY,
// feed different input calibers, may diverge numerically, and must never
// substitute for each other. Each fragment sits on a single comment line so
// the substring match survives comment re-wrapping of OTHER sentences.
func TestClusterCeilingsHeaderContractPinned(t *testing.T) {
	src, err := os.ReadFile("cluster_ceilings.go")
	if err != nil {
		t.Fatalf("read cluster_ceilings.go: %v", err)
	}
	body := string(src)
	for _, fragment := range []string{
		"share the CORE FUNCTION ONLY",
		"numbers can legitimately diverge",
		"never substitute for the other",
	} {
		if !strings.Contains(body, fragment) {
			t.Errorf("cluster_ceilings.go header lost the F2 contract sentence fragment %q — the two faces' snapshots are caliber-forked and must not be presented as interchangeable", fragment)
		}
	}
}

// TestTraceCompletenessCaveat_ClockLaneBidirectional is the F3 (2026-07-05
// review) pin for the CFC behavior change inside hasFrequencyAtOrBefore /
// hasFrequencyAfter: both now read the shared isPerCPUFrequencySample
// predicate, so a reclassified clock lane can NEITHER silence NOR fabricate
// the "no initial frequency" completeness caveat. Both directions locked on
// lane-bearing traces:
//
//	(A) pre-window rows are lane-only, a genuine sample exists after the
//	    window start → the caveat APPEARS (pre-CFC the lane counted as the
//	    initial frequency and suppressed it), and surrounding lane samples
//	    never make it disappear;
//	(B) after the window start rows are lane-only and no genuine sample
//	    exists anywhere → the caveat is ABSENT (pre-CFC the lane counted as
//	    "rows after selected start" and fabricated it).
func TestTraceCompletenessCaveat_ClockLaneBidirectional(t *testing.T) {
	q := Query{TimeStart: 5.0, TimeEnd: 5.010}
	caveatsFor := func(t *testing.T, idx *Index) []string {
		t.Helper()
		// Fixture drift guard: the lane row must stay a reclassified
		// clock_set_rate (Type flipped, verbatim Name kept) so both probes
		// stay live.
		sawLane := false
		for _, ev := range idx.Events {
			if ev.Name == "clock_set_rate" && ev.Type == EventCPUFrequency && ev.Frequency == 1800000 {
				sawLane = true
			}
		}
		if !sawLane {
			t.Fatalf("fixture drift: expected a reclassified cpu-freq-named clock_set_rate lane row")
		}
		stats := ComputeWindowStats(idx, q)
		return traceCompletenessCaveats(idx, q, Result{WindowStats: &stats})
	}
	hasNoInitialFreq := func(caveats []string) bool {
		for _, caveat := range caveats {
			if strings.Contains(caveat, "no initial frequency") {
				return true
			}
		}
		return false
	}

	t.Run("A: pre-window lane must not silence the caveat", func(t *testing.T) {
		idx := buildTraceIndex(t, "f3_caveat_lane_before.systrace", `
          clk-90 (  90) [000] .... 4.950000: clock_set_rate: cpu_freq 1800000
        app-100 (100) [000] .... 5.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120
        app-100 (100) [000] .... 5.002000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
          clk-90 (  90) [000] .... 5.004000: clock_set_rate: cpu_freq 1800000
      <idle>-0 (-----) [000] .... 5.005000: cpu_frequency: state=1000000 cpu_id=0
	`)
		caveats := caveatsFor(t, idx)
		if !hasNoInitialFreq(caveats) {
			t.Fatalf("pre-window lane sample must not count as the initial frequency — caveat must appear (and not disappear because lanes surround the window head): %v", caveats)
		}
	})

	t.Run("B: post-start lane-only rows must not fabricate the caveat", func(t *testing.T) {
		idx := buildTraceIndex(t, "f3_caveat_lane_after.systrace", `
        app-100 (100) [000] .... 5.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120
        app-100 (100) [000] .... 5.002000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
          clk-90 (  90) [000] .... 5.005000: clock_set_rate: cpu_freq 1800000
	`)
		caveats := caveatsFor(t, idx)
		if hasNoInitialFreq(caveats) {
			t.Fatalf("a lane-only after-start row set must not count as cpu_frequency rows after the selected start: %v", caveats)
		}
	})
}
