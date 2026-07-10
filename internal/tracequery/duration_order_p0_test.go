package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDurationOrderRollbackFailsClosedOnlyAffectedFamilies(t *testing.T) {
	idx := buildTraceIndex(t, "duration-rollback.systrace", strings.Join([]string{
		`app-10 (10) [000] .... 2.000000: print: B|10|CompileTask`,
		`app-10 (10) [000] .... 1.000000: print: E|10`,
		`app-10 (10) [000] .... 2.100000: print: C|10|queue_depth|7`,
		`app-10 (10) [000] .... 1.100000: print: C|10|queue_depth|3`,
		`worker-20 (20) [001] .... 2.200000: workqueue_execute_start: work=0xff function=flush_cookie`,
		`worker-20 (20) [001] .... 1.200000: workqueue_execute_end: work=0xff function=flush_cookie`,
		`display-30 (30) [002] .... 2.300000: dma_fence_wait_start: driver=display timeline=present seqno=9`,
		`display-30 (30) [002] .... 1.300000: dma_fence_wait_end: driver=display timeline=present seqno=9`,
		`io-40 (40) [003] .... 2.400000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]`,
		`irq-2 (2) [003] .... 1.400000: block_rq_complete: 8,0 R () 123 + 8 [0]`,
		`io-40 (40) [003] .... 2.450000: mmc_request_start: dev=mmcblk0 op=read`,
		`io-40 (40) [003] .... 1.450000: mmc_request_done: dev=mmcblk0 op=read`,
	}, "\n")+"\n")

	stats := ComputeWindowStats(idx, Query{TimeStart: 0.5, TimeEnd: 2.5})
	if len(stats.TraceSpans) != 0 || len(stats.CounterDeltas) != 0 || len(stats.WorkqueueActivity) != 0 || len(stats.DMAFenceActivity) != 0 || len(stats.IOLatencies) != 0 || len(stats.StorageLatencyByLayer) != 0 {
		t.Fatalf("physical rollback minted affected duration families: spans=%+v counters=%+v work=%+v dma=%+v block=%+v storage=%+v", stats.TraceSpans, stats.CounterDeltas, stats.WorkqueueActivity, stats.DMAFenceActivity, stats.IOLatencies, stats.StorageLatencyByLayer)
	}
	// TraceCounters is a count/value inventory, not an order-derived delta.
	if len(stats.TraceCounters) == 0 {
		t.Fatalf("counter rollback must not suppress the order-independent counter inventory: %+v", stats)
	}
	for _, want := range []string{"family=trace_span", "family=trace_counter_delta", "family=workqueue", "family=dma_fence", "family=block_io", "family=storage_latency"} {
		if !containsSubstring(stats.Caveats, want) {
			t.Fatalf("missing family-scoped fail-close caveat %q: %v", want, stats.Caveats)
		}
	}
	if spans, caveats := FindSpanWindows(idx, Query{SpanName: "CompileTask", TimeStart: 0.5, TimeEnd: 2.5}, 8); len(spans) != 0 || !containsSubstring(caveats, "duration_pairing_fail_closed=true") {
		t.Fatalf("span_window bypassed the physical-order gate: spans=%+v caveats=%v", spans, caveats)
	}
}

func TestInterruptLaneMonotonicityPreservesCrossCPUPhysicalInterleave(t *testing.T) {
	idx := buildTraceIndex(t, "irq-cross-cpu.systrace", strings.Join([]string{
		`irq-7 (7) [000] .... 1.000000: irq_handler_entry: irq=17 name=timer`,
		`irq-8 (8) [001] .... 100.000000: irq_handler_entry: irq=23 name=network`,
		`irq-7 (7) [000] .... 1.000500: irq_handler_exit: irq=17 name=timer ret=handled`,
	}, "\n")+"\n")

	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 100.1})
	if containsSubstring(stats.Caveats, "family=irq") {
		t.Fatalf("unrelated CPU future row poisoned a monotonic IRQ lane: %v", stats.Caveats)
	}
	var burst *IRQBurstSummary
	for i := range stats.IRQBursts {
		if stats.IRQBursts[i].CPU == 0 && stats.IRQBursts[i].IRQ == 17 {
			burst = &stats.IRQBursts[i]
			break
		}
	}
	if burst == nil || burst.Count != 2 || burst.SpanMs < 0.49 || burst.SpanMs > 0.51 || burst.DurationMs != 0 {
		t.Fatalf("cross-CPU future row split/flushed CPU0 IRQ burst: %+v", stats.IRQBursts)
	}
	var activity *InterruptActivity
	for i := range stats.IRQActivity {
		if stats.IRQActivity[i].CPU == 0 && stats.IRQActivity[i].Vector == 17 {
			activity = &stats.IRQActivity[i]
			break
		}
	}
	if activity == nil || activity.PairedCount != 1 || activity.ActiveMs < 0.49 || activity.ActiveMs > 0.51 {
		t.Fatalf("cross-CPU future row broke exact IRQ entry/exit pairing: %+v", stats.IRQActivity)
	}
}

func TestInterruptSameLaneRollbackFailsClosed(t *testing.T) {
	idx := buildTraceIndex(t, "irq-rollback.systrace", strings.Join([]string{
		`irq-7 (7) [000] .... 2.000000: irq_handler_entry: irq=17 name=timer`,
		`irq-7 (7) [000] .... 1.000000: irq_handler_exit: irq=17 name=timer ret=handled`,
		// An unrelated family stays publishable.
		`worker-20 (20) [001] .... 1.100000: workqueue_execute_start: work=0xff function=flush_cookie`,
		`worker-20 (20) [001] .... 1.200000: workqueue_execute_end: work=0xff function=flush_cookie`,
	}, "\n")+"\n")

	stats := ComputeWindowStats(idx, Query{TimeStart: 0.5, TimeEnd: 2.5})
	if len(stats.IRQBursts) != 0 || len(stats.IRQActivity) != 0 || !containsSubstring(stats.Caveats, "family=irq") {
		t.Fatalf("same-lane IRQ rollback did not fail closed: %+v", stats)
	}
	if len(stats.WorkqueueActivity) != 1 || stats.WorkqueueActivity[0].PairedCount != 1 {
		t.Fatalf("IRQ family failure suppressed unrelated workqueue duration: %+v", stats.WorkqueueActivity)
	}
}

func TestSoftIRQSameLaneRollbackFailsClosedWithoutPoisoningHardIRQ(t *testing.T) {
	idx := buildTraceIndex(t, "softirq-rollback.systrace", strings.Join([]string{
		`irq-7 (7) [000] .... 2.000000: softirq_entry: vec=3 action=NET_RX`,
		`irq-7 (7) [000] .... 1.000000: softirq_exit: vec=3 action=NET_RX`,
		`irq-8 (8) [001] .... 1.100000: irq_handler_entry: irq=17 name=timer`,
		`irq-8 (8) [001] .... 1.200000: irq_handler_exit: irq=17 ret=handled`,
	}, "\n")+"\n")

	stats := ComputeWindowStats(idx, Query{TimeStart: 0.5, TimeEnd: 2.5})
	if len(stats.SoftIRQActivity) != 0 || !containsSubstring(stats.Caveats, "family=softirq") {
		t.Fatalf("same-lane softirq rollback did not fail closed: %+v", stats)
	}
	if len(stats.IRQActivity) != 1 || stats.IRQActivity[0].PairedCount != 1 {
		t.Fatalf("softirq failure poisoned independent hard IRQ family: %+v", stats.IRQActivity)
	}
}

func TestUnpairedDurationRowsNeverMintEnvelopeTime(t *testing.T) {
	idx := buildTraceIndex(t, "unpaired-duration.systrace", strings.Join([]string{
		`worker-20 (20) [001] .... 1.000000: workqueue_execute_start: work=0xaa function=first`,
		`worker-20 (20) [001] .... 1.500000: workqueue_execute_start: work=0xbb function=second`,
		`display-30 (30) [002] .... 1.100000: dma_fence_wait_start: driver=display timeline=present seqno=1`,
		`display-30 (30) [002] .... 1.600000: dma_fence_wait_start: driver=display timeline=present seqno=2`,
		`irq-7 (7) [000] .... 1.200000: irq_handler_entry: irq=17 name=timer`,
		`irq-7 (7) [000] .... 1.700000: irq_handler_entry: irq=18 name=gpu`,
	}, "\n")+"\n")

	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.8})
	for _, item := range stats.WorkqueueActivity {
		if item.DurationMs != 0 || item.PairedCount != 0 {
			t.Fatalf("unpaired workqueue rows minted an envelope duration: %+v", item)
		}
	}
	for _, item := range stats.DMAFenceActivity {
		if item.WaitMs != 0 || item.PairedCount != 0 {
			t.Fatalf("unpaired DMA rows minted an envelope duration: %+v", item)
		}
	}
	for _, item := range stats.IRQActivity {
		if item.ActiveMs != 0 || item.PairedCount != 0 {
			t.Fatalf("unpaired IRQ rows minted an envelope duration: %+v", item)
		}
	}
}

func TestWarmCompositeWindowPreservesDurationRollbackPoison(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "duration-rollback.systrace")
	bundle := filepath.Join(dir, "duration-rollback.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, strings.Join([]string{
		`app-10 (10) [000] .... 2.000000: print: B|10|CompileTask`,
		`app-10 (10) [000] .... 1.000000: print: E|10`,
	}, "\n")+"\n")
	writeBundleProvenanceFixture(t, bundle, `{"version":"test","systrace":"duration-rollback.systrace","artifacts":[{"type":"systrace","path":"duration-rollback.systrace"}]}`)

	full, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.durationOrderFailures) == 0 {
		t.Fatal("canonical composite lost child-local duration rollback proof")
	}
	windowed, err := BuildIndexWithOptions(context.Background(), bundle, BuildOptions{
		AllowWindowedParse: true,
		TimeStartSet:       true,
		TimeStart:          0.5,
		TimeEndSet:         true,
		TimeEnd:            2.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !windowed.Windowed || len(windowed.durationOrderFailures) == 0 {
		t.Fatalf("warm derived composite lost duration rollback poison: %+v", windowed.durationOrderFailures)
	}
	stats := ComputeWindowStats(windowed, Query{TimeStart: 0.5, TimeEnd: 2.5})
	if len(stats.TraceSpans) != 0 || !containsSubstring(stats.Caveats, "family=trace_span") {
		t.Fatalf("warm composite fabricated trace span across physical rollback: %+v", stats)
	}
}

func TestColdWindowPruningPreservesPhysicalDurationRollbackPoison(t *testing.T) {
	resetAnchorCaches()
	path := filepath.Join(t.TempDir(), "window-duration-rollback.systrace")
	if err := os.WriteFile(path, []byte(strings.Join([]string{
		`app-10 (10) [000] .... 2.000000: print: B|10|CompileTask`,
		// Outside the retained core window, but physically after the start and
		// backwards on the same open span lane.
		`app-10 (10) [000] .... 1.000000: print: E|10`,
		`other-20 (20) [001] .... 2.100000: print: C|20|healthy_counter|1`,
	}, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		AllowWindowedParse: true,
		TimeStartSet:       true,
		TimeStart:          1.8,
		TimeEndSet:         true,
		TimeEnd:            2.2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !idx.Windowed || len(idx.durationOrderFailures) == 0 {
		t.Fatalf("cold window gate/pruning erased physical duration rollback: windowed=%v failures=%+v", idx.Windowed, idx.durationOrderFailures)
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.8, TimeEnd: 2.2})
	if len(stats.TraceSpans) != 0 || !containsSubstring(stats.Caveats, "family=trace_span") {
		t.Fatalf("window-pruned index fabricated a span after losing its regressed close: %+v", stats)
	}
}
