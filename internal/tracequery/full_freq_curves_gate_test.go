package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// full_freq_curves_gate_test.go — ELIM-SELF-FIX 件5① (RNB-4 双复核 P1-1,
// §29.94 problem list, 2026-07-15): the R6-rule-4 completeness precision gate
// (`!seeked && buildReachedEOF` in parseSingleTraceFile) gets its direct pin
// — the MUT-R3 shape: a build whose scan provably did NOT cover the whole
// file (windowed early-stop / seek) must publish NO full-file curves
// (collected=false) and every consumer must fall back to the (window-cropped)
// event basis. Mutating the gate to publish anyway (finalize(true)) turns the
// cropped curves into a false "full-file" claim — every assertion below goes
// red on that mutation.

const fullFreqGateTrace = `
   probe-10  (   10) [000] .... 1.000000: cpu_frequency: state=800000 cpu_id=0
   probe-10  (   10) [000] .... 1.000100: sched_switch: prev_comm=probe prev_pid=10 prev_prio=100 prev_state=S ==> next_comm=app next_pid=100 next_prio=100
     app-100 (  100) [000] .... 5.000000: cpu_frequency: state=1000000 cpu_id=0
     app-100 (  100) [000] .... 5.000200: sched_switch: prev_comm=app prev_pid=100 prev_prio=100 prev_state=S ==> next_comm=probe next_pid=10 next_prio=100
   probe-10  (   10) [000] .... 5.120000: sched_switch: prev_comm=probe prev_pid=10 prev_prio=100 prev_state=S ==> next_comm=app next_pid=100 next_prio=100
     app-100 (  100) [000] .... 5.130000: sched_switch: prev_comm=app prev_pid=100 prev_prio=100 prev_state=S ==> next_comm=probe next_pid=10 next_prio=100
   probe-10  (   10) [000] .... 9.000000: cpu_frequency: state=3000000 cpu_id=0
   probe-10  (   10) [000] .... 9.000100: sched_switch: prev_comm=probe prev_pid=10 prev_prio=100 prev_state=S ==> next_comm=app next_pid=100 next_prio=100
     app-100 (  100) [000] .... 9.500000: sched_switch: prev_comm=app prev_pid=100 prev_prio=100 prev_state=S ==> next_comm=probe next_pid=10 next_prio=100
`

func writeFullFreqGateTrace(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "full_freq_gate.systrace")
	if err := os.WriteFile(path, []byte(fullFreqGateTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Positive control: a cold full scan covers byte 0 → EOF, publishes the
// curves and the trace-global basis sees the post-window 3000000 peak.
func TestFullFreqGateFullScanPublishes(t *testing.T) {
	idx, err := BuildIndex(context.Background(), writeFullFreqGateTrace(t))
	if err != nil {
		t.Fatal(err)
	}
	if !idx.fullFreq.collected {
		t.Fatalf("a byte-0→EOF scan must publish the full-file curves")
	}
	if tls, ok := idx.fullFrequencyTimelines(); !ok || len(tls[0]) != 3 {
		t.Fatalf("full-file curves must hold all 3 samples: ok=%v %v", ok, tls)
	}
	if idx.PaddingTruncated {
		t.Fatalf("fixture setup: the full scan must not truncate")
	}
	cache := newChainQueryCache(idx, nil)
	fm, _, _ := cache.supplyFoldGlobalMaxBasis(cache.coreCapability(""))
	if fm.khz != 3000000 {
		t.Fatalf("full-file basis must see the post-window peak: %d", fm.khz)
	}
}

// The MUT-R3 pin: a windowed build whose event budget trips in padding
// early-stops before EOF (PaddingTruncated break — the 9.0 peak is never
// scanned) — the completeness gate must fail CLOSED: no full-file curves,
// consumers on the window-cropped event basis (the pre-window 800000 sample
// at ts=1.0 was OBSERVED by the side collector but is outside the retained
// event set, so the event-basis timeline must NOT contain it, and the basis
// must NOT claim the unseen 3000000 peak). Mutating the gate to publish the
// partial curves anyway (finalize(true)) reddens every assertion below.
func TestFullFreqGateWindowedEarlyStopFailsClosed(t *testing.T) {
	idx, err := BuildIndexWithOptions(context.Background(), writeFullFreqGateTrace(t), BuildOptions{
		TimeStart: 4.9, TimeEnd: 5.1, TimeStartSet: true, TimeEndSet: true,
		TimePaddingBefore: 0.05, TimePaddingAfter: 0.05, AllowWindowedParse: true,
		MaxEvents: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !idx.Windowed {
		t.Fatalf("fixture setup: build must be windowed")
	}
	if !idx.PaddingTruncated {
		t.Fatalf("fixture setup: the padding budget trip must engage (early stop before EOF)")
	}
	if idx.fullFreq.collected {
		t.Fatalf("MUT-R3 gate: an early-stopped scan must NOT publish full-file curves (from-0→EOF unproven)")
	}
	if _, ok := idx.fullFrequencyTimelines(); ok {
		t.Fatalf("MUT-R3 gate: the accessor must refuse incomplete curves")
	}
	// Consumer fallback shape: the freq index rebuilds from the retained
	// (window-cropped) events — the pre-window ts=1.0 sample (collector-
	// observed but unretained) and the unscanned ts=9.0 peak must both be
	// absent, and the basis claims only what the events saw.
	cache := newChainQueryCache(idx, nil)
	cache.buildFreqIndex()
	if len(cache.freqByCPU[0]) == 0 {
		t.Fatalf("event-basis fallback must keep the retained in-window sample")
	}
	for _, sample := range cache.freqByCPU[0] {
		if sample.khz == 800000 || sample.khz == 3000000 {
			t.Fatalf("event-basis fallback must not resurrect unscanned/unretained samples: %+v", cache.freqByCPU[0])
		}
	}
	fm, _, _ := cache.supplyFoldGlobalMaxBasis(cache.coreCapability(""))
	if fm.khz != 1000000 {
		t.Fatalf("the cropped event basis must claim only what it saw (1000000), got %d", fm.khz)
	}
}
