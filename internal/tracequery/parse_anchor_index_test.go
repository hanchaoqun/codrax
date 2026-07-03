package tracequery

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// anchorTestTrace writes n lines spanning ts 100.0..100.0+n*0.0001 with a
// deliberate CLOCK REGRESSION around line regressAt: that line carries a ts
// HIGHER than its neighbors' running trend would suggest is safe to skip —
// the running-max anchor rule must keep seeks strictly before it.
func anchorTestTrace(tb testing.TB, n, regressAt int) string {
	tb.Helper()
	dir := tb.TempDir()
	path := filepath.Join(dir, "anchor.systrace")
	var b strings.Builder
	b.WriteString("# tracer: nop\n")
	for i := 1; i <= n; i++ {
		ts := 100.0 + float64(i)*0.0001
		if i == regressAt {
			// A far-future spike followed by normal lines: running max
			// jumps here, so anchors after this line must not be eligible
			// for windows starting below the spike.
			ts = 100.0 + float64(n)*0.0001 + 0.5
		}
		fmt.Fprintf(&b, "  app-42 (42) [000] .... %.6f: sched_wakeup: comm=worker pid=%d prio=120 target_cpu=000\n", ts, 1000+i%5)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		tb.Fatal(err)
	}
	return path
}

func resetAnchorCaches() {
	anchorCache = &traceAnchorCache{items: map[traceAnchorKey]*traceAnchorSet{}}
	indexCache = newTraceIndexCache(traceIndexCacheBudgetBytes)
}

// TestAnchorSeekBuildsIdenticalIndex is the P1 correctness bar: a windowed
// build served via anchor seek must produce a byte-identical event set,
// caveats, bounds, and flavor versus the same window built cold.
func TestAnchorSeekBuildsIdenticalIndex(t *testing.T) {
	path := anchorTestTrace(t, 3*traceAnchorLineInterval, 0)
	opts := BuildOptions{
		AllowWindowedParse: true,
		TimeStart:          100.0 + float64(2*traceAnchorLineInterval)*0.0001, TimeStartSet: true,
		TimeEnd: 100.0 + float64(2*traceAnchorLineInterval+2000)*0.0001, TimeEndSet: true,
	}

	// Cold: no anchors — scans from byte 0 and records them.
	resetAnchorCaches()
	cold, err := BuildIndexWithOptions(t.Context(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	key := traceAnchorKey{path: cold.Path, size: cold.Size, modUnix: cold.ModTime.UnixNano(), version: ParserVersion}
	set := anchorCache.load(key)
	if set == nil || len(set.Anchors) == 0 || !set.FlavorSet {
		t.Fatalf("cold build must record anchors + flavor, got %+v", set)
	}

	// Warm: same window again bypassing the index cache — must seek.
	indexCache = newTraceIndexCache(traceIndexCacheBudgetBytes)
	warm, err := BuildIndexWithOptions(t.Context(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	if warm.ScannedLineCount != cold.ScannedLineCount {
		t.Fatalf("LineCount bookkeeping drifted: cold=%d warm=%d", cold.ScannedLineCount, warm.ScannedLineCount)
	}
	if !reflect.DeepEqual(cold.Events, warm.Events) {
		t.Fatalf("anchor-seek event set diverged: cold=%d warm=%d events", len(cold.Events), len(warm.Events))
	}
	if cold.FirstTs != warm.FirstTs || cold.LastTs != warm.LastTs {
		t.Fatalf("bounds diverged: cold=[%f,%f] warm=[%f,%f]", cold.FirstTs, cold.LastTs, warm.FirstTs, warm.LastTs)
	}
	if cold.TraceFlavor != warm.TraceFlavor {
		t.Fatalf("flavor must be stable across seek builds: %q vs %q", cold.TraceFlavor, warm.TraceFlavor)
	}
}

// TestAnchorSeekRespectsClockRegression pins the running-max rule: a
// far-future ts spike early in the file poisons every later anchor's
// running max, so a window covering the spike's ts can never seek past it.
func TestAnchorSeekRespectsClockRegression(t *testing.T) {
	n := 3 * traceAnchorLineInterval
	spikeLine := traceAnchorLineInterval / 2
	path := anchorTestTrace(t, n, spikeLine)
	spikeTs := 100.0 + float64(n)*0.0001 + 0.5

	resetAnchorCaches()
	// Prime anchors with a full un-windowed build.
	if _, err := BuildIndex(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	// Window starting just below the spike ts: every anchor's running max
	// includes the spike, so no anchor is eligible — the spike line MUST
	// appear in the window even though it sits early in the file.
	opts := BuildOptions{AllowWindowedParse: true, TimeStart: spikeTs - 0.001, TimeStartSet: true, TimeEnd: spikeTs + 1.0, TimeEndSet: true}
	indexCache = newTraceIndexCache(traceIndexCacheBudgetBytes)
	idx, err := BuildIndexWithOptions(t.Context(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ev := range idx.Events {
		if ev.Line == spikeLine+1 { // +1 for the header line
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("clock-regression spike line must not be skipped by anchor seek (events=%d)", len(idx.Events))
	}
}

// TestCounterDeltasAndTransactInterfaceJoin pins the T3 coverage additions
// against the customer trace shapes: C| counters aggregate numerically per
// (thread, name) with first/last/min/max/delta ranked by |delta|, and an
// ipc edge whose send sits inside a `transact[Interface:code]` span on the
// same thread carries the interface verbatim.
func TestCounterDeltasAndTransactInterfaceJoin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cov.systrace")
	trace := strings.Join([]string{
		"# tracer: nop",
		"  app-42 (42) [000] .... 100.000100: print: C|42|Heap size (KB)|157028",
		"  app-42 (42) [000] .... 100.000200: print: B|42|transact[android.net.IConnectivityManager:9]",
		"  app-42 (42) [000] .... 100.000300: binder_transaction: transaction=4407138 dest_node=311264 dest_proc=10756 dest_thread=0 reply=0 flags=0x12 code=0x1",
		"  app-42 (42) [000] .... 100.000350: print: E|42",
		"  binder:486_1-10803 (10756) [001] .... 100.000400: binder_transaction_received: transaction=4407138",
		"  app-42 (42) [000] .... 100.000500: print: C|42|Heap size (KB)|158100",
		"  app-42 (42) [000] .... 100.000600: print: C|42|JNI Weak Global Refs|198",
		"  app-42 (42) [000] .... 100.000700: print: C|42|Heap size (KB)|156000",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	resetAnchorCaches()
	idx, err := BuildIndex(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	stats := ComputeWindowStats(idx, Query{})
	if len(stats.CounterDeltas) == 0 {
		t.Fatal("counter deltas missing")
	}
	top := stats.CounterDeltas[0]
	if top.Name != "Heap size (KB)" || top.Samples != 3 {
		t.Fatalf("top delta must be the heap counter with 3 samples: %+v", top)
	}
	if top.First != 157028 || top.Last != 156000 || top.Min != 156000 || top.Max != 158100 || top.Delta != -1028 {
		t.Fatalf("heap delta aggregation wrong: %+v", top)
	}

	graph := BuildIPCGraph(idx, Query{})
	if len(graph.Edges) == 0 {
		t.Fatal("ipc edge missing")
	}
	if graph.Edges[0].Interface != "android.net.IConnectivityManager:9" {
		t.Fatalf("transact interface join missing: %+v", graph.Edges[0])
	}
}
