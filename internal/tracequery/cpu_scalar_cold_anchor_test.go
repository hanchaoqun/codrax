package tracequery

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestCPUScalarRejectedOuterHeaderPoisonsColdAndWarmFullCurveAuthority(t *testing.T) {
	for _, tc := range []struct {
		name           string
		valid          string
		rejected       string
		unaffected     string
		frequency      bool
		knownCPU       bool
		wantUnaffected int
	}{
		{
			name:       "frequency known cpu",
			valid:      `idle-0 (0) [000] .... 1.000000: cpu_frequency: state=1200000 cpu_id=0`,
			rejected:   `idle-0 (0) [5000] .... 1.100000: cpu_frequency: state=1800000 cpu_id=0`,
			unaffected: `idle-0 (0) [001] .... 1.200000: cpu_frequency_limits: min=400000 max=2200000 cpu_id=1`,
			frequency:  true, knownCPU: true, wantUnaffected: 1,
		},
		{
			name:       "frequency unknown cpu",
			valid:      `idle-0 (0) [000] .... 1.000000: cpu_frequency: state=1200000 cpu_id=0`,
			rejected:   `idle-bad (0) [000] .... 1.100000: cpu_frequency: state=1800000`,
			unaffected: `idle-0 (0) [001] .... 1.200000: cpu_frequency_limits: min=400000 max=2200000 cpu_id=1`,
			frequency:  true, wantUnaffected: 1,
		},
		{
			name:       "limits known cpu",
			valid:      `idle-0 (0) [000] .... 1.000000: cpu_frequency_limits: min=300000 max=1800000 cpu_id=0`,
			rejected:   `idle-0 (0) [bad] .... 1.100000: cpu_frequency_limits: min=400000 max=2000000 cpu_id=0`,
			unaffected: `idle-0 (0) [001] .... 1.200000: cpu_frequency: state=2200000 cpu_id=1`,
			knownCPU:   true, wantUnaffected: 1,
		},
		{
			name:           "limits unknown cpu",
			valid:          `idle-0 (0) [000] .... 1.000000: cpu_frequency_limits: min=300000 max=1800000 cpu_id=0`,
			rejected:       `idle-0 (0) [000] .... bad-time: cpu_frequency_limits: min=400000 max=2000000`,
			unaffected:     `idle-0 (0) [001] .... 1.200000: cpu_frequency: state=2200000 cpu_id=1`,
			wantUnaffected: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cold-anchor.systrace")
			writeBundleProvenanceFixture(t, path, strings.Join([]string{
				tc.valid,
				tc.rejected,
				tc.unaffected,
				`idle-0 (0) [000] .... 2.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=42 next_prio=120`,
				`app-42 (42) [000] .... 2.500000: sched_switch: prev_comm=app prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
			}, "\n")+"\n")

			cold, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
				AllowWindowedParse: true,
				TimeStart:          2,
				TimeStartSet:       true,
				TimeEnd:            2.5,
				TimeEndSet:         true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !cold.Windowed || !cold.fullFreq.collected {
				t.Fatalf("fixture must complete a cold window scan and publish its full-file receipt: %+v", cold.fullFreq)
			}
			assertCPUScalarRejectedOuterHeaderPoison(t, cold, tc.frequency, tc.knownCPU, tc.wantUnaffected)

			// Windowed indices are not held in indexCache. The complete cold scan
			// above stamps FullFreq into the per-file anchor record; this different
			// window therefore exercises receipt reuse rather than an exact result
			// cache hit.
			warm, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
				AllowWindowedParse: true,
				TimeStart:          2.1,
				TimeStartSet:       true,
				TimeEnd:            2.4,
				TimeEndSet:         true,
			})
			if err != nil {
				t.Fatal(err)
			}
			assertCPUScalarRejectedOuterHeaderPoison(t, warm, tc.frequency, tc.knownCPU, tc.wantUnaffected)
		})
	}
}

func assertCPUScalarRejectedOuterHeaderPoison(t *testing.T, idx *Index, frequency, knownCPU bool, unaffectedCPU int) {
	t.Helper()
	if !idx.fullFreq.collected {
		t.Fatalf("full-file receipt was not retained: %+v", idx.fullFreq)
	}
	if frequency {
		if idx.fullFreq.freqAll != !knownCPU || knownCPU && !idx.fullFreq.freqUnsafe[0] || idx.fullFreq.limitAll || idx.fullFreq.limitUnsafe[0] {
			t.Fatalf("frequency rejected-row poison scope drifted: %+v", idx.fullFreq)
		}
		if got := indexFreqSampleTimelines(idx); len(got[0]) != 0 {
			t.Fatalf("rejected frequency row allowed old cpu0 state to bridge: %v", got)
		}
		if got, ok := idx.fullFrequencyLimitTimelines(); !ok || len(got[unaffectedCPU]) == 0 {
			t.Fatalf("frequency poison crossed into unaffected limits family: ok=%t limits=%v", ok, got)
		}
		globalCaveats := frequencyOrderIntegrityForGlobalDerivation(idx).globalCaveats()
		if !containsSubstring(globalCaveats, "authority=trace_global_derivation") ||
			!containsSubstring(globalCaveats, "source="+idx.Path) ||
			!containsSubstring(globalCaveats, "line=2") {
			t.Fatalf("frequency receipt withdrew the lane without a public fail-close explanation")
		}
		return
	}
	if idx.fullFreq.limitAll != !knownCPU || knownCPU && !idx.fullFreq.limitUnsafe[0] || idx.fullFreq.freqAll || idx.fullFreq.freqUnsafe[0] {
		t.Fatalf("limits rejected-row poison scope drifted: %+v", idx.fullFreq)
	}
	cache := newChainQueryCache(idx, nil)
	cache.buildFreqLimitIndex()
	if len(cache.freqLimitByCPU[0]) != 0 || cache.governedLimitMaxKHz(0, 0, 3) != 0 {
		t.Fatalf("rejected limits row allowed old cpu0 ceiling to bridge: %v", cache.freqLimitByCPU)
	}
	if got, ok := idx.fullFrequencyTimelines(); !ok || len(got[unaffectedCPU]) == 0 {
		t.Fatalf("limits poison crossed into unaffected frequency family: ok=%t curves=%v", ok, got)
	}
	globalCaveats := frequencyOrderIntegrityForGlobalDerivation(idx).globalCaveats()
	if !containsSubstring(globalCaveats, "authority=trace_global_derivation") ||
		!containsSubstring(globalCaveats, "source="+idx.Path) ||
		!containsSubstring(globalCaveats, "line=2") {
		t.Fatalf("limits receipt withdrew the lane without a public fail-close explanation")
	}
}

func TestCPUScalarLooseRejectedHeaderDoesNotReadNestedPrintPayload(t *testing.T) {
	rejectedOuterRows := []string{
		`outer-1 (1) [bad] .... 1.100000: print: nested-7 (7) [000] .... 1.200000: cpu_frequency: state=broken cpu_id=0`,
		`outer-1 (1) [000] .... bad-time: print: nested-7 (7) [000] .... 1.300000: cpu_frequency: state=1800000 cpu_id=0`,
	}
	for _, line := range rejectedOuterRows {
		if got := matchFtraceLine(line); len(got) != 0 {
			t.Fatalf("fixture row must miss the strict outer envelope before loose fallback: %q", line)
		}
		loose := loosePhysicalFtraceLine(line)
		if len(loose) == 0 || strings.TrimSuffix(strings.TrimSpace(loose[6]), ":") != "print" {
			t.Fatalf("fixture row must loose-match only the outer physical print event: match=%v line=%q", loose, line)
		}
		var scan lineScan
		scan.reset(2, line)
		if got := cpuScalarRejectedRowFailureScan(&scan); got != nil {
			t.Fatalf("loose fallback reinterpreted nested print payload as CPU scalar failure: %+v", got)
		}
	}

	path := filepath.Join(t.TempDir(), "nested-print.systrace")
	lines := append([]string{
		`idle-0 (0) [000] .... 1.000000: cpu_frequency: state=1200000 cpu_id=0`,
	}, rejectedOuterRows...)
	writeBundleProvenanceFixture(t, path, strings.Join(lines, "\n")+"\n")
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if idx.fullFreq.freqAll || idx.fullFreq.freqUnsafe[0] {
		t.Fatalf("nested print payload was reinterpreted as a physical CPU transition: %+v", idx.fullFreq)
	}
	if integrity := frequencyOrderIntegrityForQuery(idx, Query{}); integrity.freqAll || integrity.frequencyUnsafe(0) || len(integrity.localCaveats()) != 0 {
		t.Fatalf("nested print payload entered the query-local CPU scalar ledger: %+v caveats=%v", integrity, integrity.localCaveats())
	}
	if got := indexFreqSampleTimelines(idx); len(got[0]) != 1 || got[0][0].khz != 1200000 {
		t.Fatalf("nested print payload erased the genuine frequency lane: %v", got)
	}
}

func TestCPUScalarFullFilePoisonDoesNotSuppressBoundedWindowDisplay(t *testing.T) {
	idx := buildTraceIndex(t, "cpu-scalar-global-vs-window.systrace", strings.Join([]string{
		`idle-0 (0) [000] .... 1.000000: cpu_frequency: state=1200000 cpu_id=0`,
		`idle-0 (0) [000] .... 1.010000: cpu_frequency_limits: min=400000 max=1800000 cpu_id=0`,
		`idle-0 (0) [000] .... 1.100000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=42 next_prio=120`,
		`app-42 (42) [000] .... 1.600000: sched_switch: prev_comm=app prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
		`idle-0 (0) [000] .... 9.000000: cpu_frequency: state=broken cpu_id=0`,
		`idle-0 (0) [000] .... 9.100000: cpu_frequency_limits: min=broken max=1800000 cpu_id=0`,
	}, "\n")+"\n")

	stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 2})
	var sawFrequency, sawLimit bool
	for _, cpu := range stats.CPU {
		if cpu.CPU == 0 && cpu.Frequency == 1200000 && len(cpu.FrequencyResidency) > 0 {
			sawFrequency = true
		}
	}
	for _, limit := range stats.CPUFrequencyLimits {
		if limit.CPU == 0 && limit.MaxFrequency == 1800000 {
			sawLimit = true
		}
	}
	if !sawFrequency || !sawLimit {
		t.Fatalf("window-after poison leaked into earlier bounded display: frequency=%t limit=%t stats=%+v", sawFrequency, sawLimit, stats)
	}
	if containsSubstring(stats.Caveats, "authority=query_window") {
		t.Fatalf("bounded window claimed its local display was withdrawn by a later transition: %v", stats.Caveats)
	}
	if !containsSubstring(stats.Caveats, "authority=trace_global_derivation") ||
		!containsSubstring(stats.Caveats, "trace-global cluster membership") ||
		containsSubstring(stats.Caveats, "omitted affected CPU lanes from query-window frequency residency") {
		t.Fatalf("bounded window lost or mis-described its independent global-derivation downgrade: %v", stats.Caveats)
	}
	if got := indexFreqSampleTimelines(idx); len(got[0]) != 0 {
		t.Fatalf("global derivation ignored its full-file frequency poison: %v", got)
	}
	cache := newChainQueryCache(idx, nil)
	cache.buildFreqLimitIndex()
	if len(cache.freqLimitByCPU[0]) != 0 ||
		!containsSubstring(cache.frequencyOrderCaveats(), "authority=trace_global_derivation") ||
		!containsSubstring(cache.frequencyOrderCaveats(), "family=cpu_frequency_limits") {
		t.Fatalf("global limits derivation ignored poison or lost disclosure: limits=%v caveats=%v", cache.freqLimitByCPU, cache.frequencyOrderCaveats())
	}
}
