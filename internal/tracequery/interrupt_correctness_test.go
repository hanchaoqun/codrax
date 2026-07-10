package tracequery

import (
	"fmt"
	"strings"
	"testing"
)

func TestSoftIRQRaiseIsInstantInventoryAndNeverClosesAsDuration(t *testing.T) {
	idx := buildTraceIndex(t, "softirq-raise.systrace", strings.Join([]string{
		`irq-7 (7) [000] .... 1.000000: softirq_raise: vec=3 action=NET_RX`,
		`irq-7 (7) [000] .... 1.001000: softirq_exit: vec=3 action=NET_RX`,
		`irq-7 (7) [000] .... 1.002000: softirq_entry: vec=3 action=NET_RX`,
		`irq-7 (7) [000] .... 1.003500: softirq_exit: vec=3 action=NET_RX`,
	}, "\n")+"\n")

	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.1})
	if len(stats.SoftIRQActivity) != 1 {
		t.Fatalf("expected one exact softirq lane, got %+v", stats.SoftIRQActivity)
	}
	got := stats.SoftIRQActivity[0]
	if got.Count != 4 || got.PairedCount != 1 || !near(got.ActiveMs, 1.5, 0.001) {
		t.Fatalf("softirq_raise or orphan exit minted duration: %+v", got)
	}
}

func TestInterruptActivityPairsOnlyExactCPUAndVector(t *testing.T) {
	idx := buildTraceIndex(t, "irq-exact-lanes.systrace", strings.Join([]string{
		`irq-7 (7) [000] .... 1.000000: irq_handler_entry: irq=17 name=timer`,
		`irq-7 (7) [001] .... 1.001000: irq_handler_exit: irq=17 ret=handled`,
		`irq-7 (7) [000] .... 1.002000: irq_handler_exit: irq=18 ret=handled`,
		`irq-7 (7) [000] .... 1.003000: irq_handler_exit: irq=17 ret=handled`,
	}, "\n")+"\n")

	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.1})
	var exact InterruptActivity
	for _, item := range stats.IRQActivity {
		if item.CPU == 0 && item.Vector == 17 {
			exact = item
		}
		if item.ActiveMs != 0 && (item.CPU != 0 || item.Vector != 17) {
			t.Fatalf("different CPU/vector closed another lane: %+v", item)
		}
	}
	if exact.PairedCount != 1 || !near(exact.ActiveMs, 3, 0.001) {
		t.Fatalf("exact CPU/vector pair was not preserved: %+v", stats.IRQActivity)
	}
}

func TestMalformedInterruptEndpointFailsClosedWithBoundedWitness(t *testing.T) {
	lines := []string{
		`irq-7 (7) [000] .... 1.000000: softirq_entry: action=NET_RX`,
		`irq-7 (7) [000] .... 1.001000: softirq_entry: vec=3 action=NET_RX`,
		`irq-7 (7) [000] .... 1.002000: softirq_exit: vec=3 action=NET_RX`,
	}
	for i := 0; i < durationOrderFailureCap+8; i++ {
		lines = append(lines, fmt.Sprintf(`irq-7 (7) [000] .... 1.%06d: softirq_exit: action=NET_RX`, 10000+i))
	}
	idx := buildTraceIndex(t, "softirq-malformed.systrace", strings.Join(lines, "\n")+"\n")
	if len(idx.durationOrderFailures) > durationOrderFailureCap {
		t.Fatalf("malformed endpoint witness set is unbounded: %d", len(idx.durationOrderFailures))
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.2})
	if len(stats.SoftIRQActivity) != 0 {
		t.Fatalf("malformed endpoint did not fail-close duration family: %+v", stats.SoftIRQActivity)
	}
	if !containsSubstring(stats.Caveats, "duration_endpoint_parse_incomplete") || !containsSubstring(stats.Caveats, "missing_or_invalid=vec") {
		t.Fatalf("missing bounded malformed-endpoint caveat: %v", stats.Caveats)
	}
}

func TestMalformedInterruptEndpointDoesNotPoisonUnrelatedLaterWindow(t *testing.T) {
	idx := buildTraceIndex(t, "softirq-malformed-scoped.systrace", strings.Join([]string{
		`irq-7 (7) [000] .... 1.000000: softirq_entry: action=NET_RX`,
		`irq-7 (7) [000] .... 2.000000: softirq_entry: vec=3 action=NET_RX`,
		`irq-7 (7) [000] .... 2.001000: softirq_exit: vec=3 action=NET_RX`,
	}, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.9, TimeEnd: 2.1})
	if containsSubstring(stats.Caveats, "duration_endpoint_parse_incomplete") || len(stats.SoftIRQActivity) != 1 || stats.SoftIRQActivity[0].PairedCount != 1 {
		t.Fatalf("pre-window malformed endpoint poisoned an unrelated exact pair: %+v", stats)
	}
}

func TestGenericInterruptSuffixCannotMintDuration(t *testing.T) {
	idx := buildTraceIndex(t, "irq-generic-suffix.systrace", strings.Join([]string{
		`irq-7 (7) [000] .... 1.000000: irq_work_entry: irq=17 name=work`,
		`irq-7 (7) [000] .... 1.010000: irq_work_exit: irq=17 name=work`,
	}, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.1})
	for _, item := range stats.IRQActivity {
		if item.PairedCount != 0 || item.ActiveMs != 0 {
			t.Fatalf("non-handler IRQ suffix fabricated a duration: %+v", item)
		}
	}
}

func TestIRQBurstIsInventoryAndDoesNotCompeteWithPairedActivity(t *testing.T) {
	idx := buildTraceIndex(t, "irq-rank-single-seat.systrace", strings.Join([]string{
		`irq-7 (7) [000] .... 1.000000: irq_handler_entry: irq=17 name=timer`,
		`irq-7 (7) [000] .... 1.000500: irq_handler_exit: irq=17 ret=handled`,
	}, "\n")+"\n")
	q := Query{TimeStart: 0.9, TimeEnd: 1.1, Limit: 16}
	stats := ComputeWindowStats(idx, q)
	if len(stats.IRQBursts) != 1 || !near(stats.IRQBursts[0].SpanMs, 0.5, 0.001) || stats.IRQBursts[0].DurationMs != 0 {
		t.Fatalf("IRQ burst must be count/span inventory only: %+v", stats.IRQBursts)
	}
	if len(stats.IRQActivity) != 1 || !near(stats.IRQActivity[0].ActiveMs, 0.5, 0.001) {
		t.Fatalf("paired IRQ active duration missing: %+v", stats.IRQActivity)
	}

	rank := BuildRootCauseRank(idx, q)
	activitySeats := 0
	for _, item := range rank.Items {
		switch item.Type {
		case "irq_burst":
			t.Fatalf("inventory envelope consumed a root-rank seat: %+v", item)
		case "irq_activity":
			activitySeats++
			if !near(item.CumulativeImpactMs, 0.5, 0.001) {
				t.Fatalf("ranked IRQ impact did not come from paired active sum: %+v", item)
			}
		}
	}
	if activitySeats != 1 {
		t.Fatalf("expected exactly one paired IRQ rank seat, got %d in %+v", activitySeats, rank.Items)
	}
}

func TestUnpairedIRQBurstNeverBecomesRootCause(t *testing.T) {
	idx := buildTraceIndex(t, "irq-unpaired-inventory.systrace", strings.Join([]string{
		`irq-7 (7) [000] .... 1.000000: irq_handler_entry: irq=17 name=timer`,
		`irq-7 (7) [000] .... 1.008000: irq_handler_entry: irq=17 name=timer`,
	}, "\n")+"\n")
	rank := BuildRootCauseRank(idx, Query{TimeStart: 0.9, TimeEnd: 1.1, Limit: 16})
	for _, item := range rank.Items {
		if item.Type == "irq_burst" || item.Type == "irq_activity" {
			t.Fatalf("unclosed IRQ rows minted a root cause: %+v", item)
		}
	}
}

func TestPairedIPIActiveDurationOwnsOneRootCauseSeat(t *testing.T) {
	idx := buildTraceIndex(t, "ipi-rank-single-seat.systrace", strings.Join([]string{
		`irq-2 (2) [004] .... 1.000000: ipi_entry: (Rescheduling interrupts)`,
		`irq-2 (2) [004] .... 1.001500: ipi_exit: (Rescheduling interrupts)`,
	}, "\n")+"\n")

	q := Query{TimeStart: 0.9, TimeEnd: 1.1, Limit: 16}
	stats := ComputeWindowStats(idx, q)
	if stats.SupplyPressureSummary == nil || !near(stats.SupplyPressureSummary.IPIActiveMs, 1.5, 0.001) {
		t.Fatalf("supply evidence must retain paired IPI context: %+v", stats.SupplyPressureSummary)
	}

	rank := BuildRootCauseRank(idx, q)
	ipiSeats := 0
	for _, item := range rank.Items {
		switch item.Type {
		case "ipi_activity":
			ipiSeats++
			if !near(item.CumulativeImpactMs, 1.5, 0.001) {
				t.Fatalf("paired IPI impact changed: %+v", item)
			}
		case "supply_pressure":
			t.Fatalf("pure IPI activity occupied a second supply-pressure seat: %+v", item)
		}
	}
	if ipiSeats != 1 {
		t.Fatalf("expected one paired IPI rank seat, got %d in %+v", ipiSeats, rank.Items)
	}
}

func TestInterruptActivityClipsCarryInAndCarryOutToSelectedWindow(t *testing.T) {
	idx := buildTraceIndex(t, "irq-window-boundaries.systrace", strings.Join([]string{
		`irq-7 (7) [000] .... 0.998000: irq_handler_entry: irq=17 name=head`,
		`irq-7 (7) [000] .... 1.002000: irq_handler_exit: irq=17 ret=handled`,
		`irq-7 (7) [001] .... 1.008000: irq_handler_entry: irq=18 name=tail`,
		`irq-7 (7) [001] .... 1.012000: irq_handler_exit: irq=18 ret=handled`,
		`irq-7 (7) [002] .... 0.997000: irq_handler_entry: irq=19 name=cover`,
		`irq-7 (7) [002] .... 1.013000: irq_handler_exit: irq=19 ret=handled`,
	}, "\n")+"\n")

	stats := ComputeWindowStats(idx, Query{TimeStart: 1.000, TimeEnd: 1.010})
	got := map[int]InterruptActivity{}
	for _, item := range stats.IRQActivity {
		got[item.Vector] = item
	}
	if len(got) != 3 {
		t.Fatalf("all three boundary-overlapping pairs must survive: %+v", stats.IRQActivity)
	}
	for vector, want := range map[int]float64{17: 2, 18: 2, 19: 10} {
		item := got[vector]
		if item.PairedCount != 1 || !near(item.ActiveMs, want, 0.001) || !near(item.WindowOverlapMs, want, 0.001) {
			t.Fatalf("vector=%d clipped activity=%+v want %.3fms", vector, item, want)
		}
	}
}

func TestMalformedEndpointInsideCarryInPairSuppressesOnlyThatPair(t *testing.T) {
	idx := buildTraceIndex(t, "irq-carry-malformed.systrace", strings.Join([]string{
		`irq-7 (7) [000] .... 0.998000: softirq_entry: vec=3 action=NET_RX`,
		`irq-7 (7) [000] .... 0.999000: softirq_entry: action=UNKNOWN`,
		`irq-7 (7) [000] .... 1.002000: softirq_exit: vec=3 action=NET_RX`,
		`irq-7 (7) [000] .... 1.004000: softirq_entry: vec=4 action=TIMER`,
		`irq-7 (7) [000] .... 1.005000: softirq_exit: vec=4 action=TIMER`,
	}, "\n")+"\n")

	// The malformed pre-window endpoint sits inside vec=3's physical pair, so
	// that carry-in duration is withheld. The later exact in-window vec=4 pair
	// remains independently provable and must not be family-wide poisoned.
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.000, TimeEnd: 1.010})
	got := map[int]InterruptActivity{}
	for _, item := range stats.SoftIRQActivity {
		got[item.Vector] = item
	}
	if item := got[3]; item.ActiveMs != 0 || item.PairedCount != 0 {
		t.Fatalf("malformed nesting fabricated carry-in activity: %+v", item)
	}
	if item := got[4]; item.PairedCount != 1 || !near(item.ActiveMs, 1, 0.001) {
		t.Fatalf("unrelated exact pair must survive: activity=%+v caveats=%+v", stats.SoftIRQActivity, stats.Caveats)
	}
}
