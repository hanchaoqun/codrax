package tracequery

import (
	"math"
	"strings"
	"testing"
)

// ENG audit #9 (P2, §29.25 处置委托 2026-07-10): computeInterruptActivity
// allowed an unbounded open stack per exact lane (type+CPU+vector/reason) and
// LIFO-paired it. Same-lane depth>=2 is physically impossible (one CPU's
// exact IRQ/softirq/IPI lane cannot self-nest) and can only come from event
// loss; LIFO then minted two OVERLAPPING pairs whose ActiveMs sum
// double-counted the overlap and booked unproven wall clock as hard active
// time — feeding root_cause_rank seats and supply_pressure with zero caveats.
// The lane now follows the block-pairing cohort discipline: depth>=2 marks
// the whole cohort ambiguous, its durations are withheld and disclosed.

func interruptLaneRow(items []InterruptActivity, cpu, vector int) *InterruptActivity {
	for i := range items {
		if items[i].CPU == cpu && items[i].Vector == vector {
			return &items[i]
		}
	}
	return nil
}

func TestInterruptSameLaneNestedEndpointsFailCloseAsCohort(t *testing.T) {
	idx := buildTraceIndex(t, "irq_depth.systrace", `
 irq-3 (3) [003] .... 1.000000: irq_handler_entry: irq=11 name=eth0
 irq-3 (3) [003] .... 1.010000: irq_handler_entry: irq=11 name=eth0
 irq-3 (3) [003] .... 1.020000: irq_handler_exit: irq=11 ret=handled
 irq-3 (3) [003] .... 1.030000: irq_handler_exit: irq=11 ret=handled
 irq-9 (9) [002] .... 1.040000: irq_handler_entry: irq=7 name=uart
 irq-9 (9) [002] .... 1.045000: irq_handler_exit: irq=7 ret=handled
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.1})
	poisoned := interruptLaneRow(stats.IRQActivity, 3, 11)
	if poisoned == nil {
		t.Fatalf("lane inventory must survive: %+v", stats.IRQActivity)
	}
	if poisoned.PairedCount != 0 || poisoned.ActiveMs != 0 || poisoned.MaxActiveMs != 0 {
		t.Fatalf("physically impossible same-lane nesting must withhold the whole cohort's durations (LIFO double-counted the overlap): %+v", poisoned)
	}
	if !containsSubstring(stats.Caveats, "interrupt_pairing_ambiguous") {
		t.Fatalf("cohort suppression must be disclosed: %+v", stats.Caveats)
	}
	control := interruptLaneRow(stats.IRQActivity, 2, 7)
	if control == nil || control.PairedCount != 1 || math.Abs(control.ActiveMs-5) > 0.001 {
		t.Fatalf("unrelated exact lane must keep pairing: %+v", control)
	}
}

// Sequential (non-overlapping) same-lane pairs are the physical norm and must
// keep minting exactly as before.
func TestInterruptSequentialSameLanePairsStayPaired(t *testing.T) {
	idx := buildTraceIndex(t, "irq_sequential.systrace", `
 irq-3 (3) [003] .... 1.000000: irq_handler_entry: irq=11 name=eth0
 irq-3 (3) [003] .... 1.010000: irq_handler_exit: irq=11 ret=handled
 irq-3 (3) [003] .... 1.020000: irq_handler_entry: irq=11 name=eth0
 irq-3 (3) [003] .... 1.035000: irq_handler_exit: irq=11 ret=handled
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.1})
	row := interruptLaneRow(stats.IRQActivity, 3, 11)
	if row == nil || row.PairedCount != 2 || math.Abs(row.ActiveMs-25) > 0.001 {
		t.Fatalf("sequential exact-lane pairs stay paired: %+v", row)
	}
	if containsSubstring(stats.Caveats, "interrupt_pairing_ambiguous") {
		t.Fatalf("sequential pairs are not ambiguous: %+v", stats.Caveats)
	}
}

// A window-relevant entry with no exit is disclosed instead of silently
// dropped (mirror of block's unpaired_start disclosure).
func TestInterruptUnpairedEntryIsDisclosed(t *testing.T) {
	idx := buildTraceIndex(t, "irq_unpaired.systrace", `
 irq-3 (3) [003] .... 1.000000: irq_handler_entry: irq=11 name=eth0
 app-9 (9) [002] .... 1.040000: sched_wakeup: comm=app pid=9 prio=100 target_cpu=002
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.1})
	row := interruptLaneRow(stats.IRQActivity, 3, 11)
	if row == nil || row.PairedCount != 0 || row.ActiveMs != 0 {
		t.Fatalf("no typed pair, no active time: %+v", row)
	}
	if !containsSubstring(stats.Caveats, "interrupt_pairing_unpaired") {
		t.Fatalf("window-relevant unpaired entry must be disclosed: %+v", stats.Caveats)
	}
}

// ENG audit #4b (P3): a doubly-malformed endpoint row (unparseable/overflowed
// timestamp AND a missing lane key) escaped both suppression nets — the
// endpoint_parse_incomplete relevance arm excluded CurrentTs=0 on windowed
// queries and the per-pair interval match never hits ts 0. The unparseable
// timestamp is now itself a recorded field, the violation is marked
// TsUnknown, and both nets treat it conservatively (line-scoped for pairs).
func TestInterruptEndpointUnparseableTimestampFailsConservative(t *testing.T) {
	unsafeTs := "1" + strings.Repeat("0", 305) + ".0"

	// Doubly malformed: overflowed ts + missing irq key.
	double := interruptEndpointValidationFailure(2, " irq-3 (3) [003] .... "+unsafeTs+": irq_handler_entry: name=eth0")
	if double == nil || !double.TsUnknown {
		t.Fatalf("doubly-malformed endpoint must record an unknown-timestamp violation: %+v", double)
	}
	fields := strings.Join(double.Fields, ",")
	if !strings.Contains(fields, "irq") || !strings.Contains(fields, "ts") {
		t.Fatalf("both damaged fields must be named: %+v", double)
	}

	// Sibling gap: overflowed ts with an intact lane key minted NOTHING before.
	tsOnly := interruptEndpointValidationFailure(2, " irq-3 (3) [003] .... "+unsafeTs+": irq_handler_entry: irq=11 name=eth0")
	if tsOnly == nil || !tsOnly.TsUnknown {
		t.Fatalf("timestamp-only malformed endpoint must still mint a witness: %+v", tsOnly)
	}

	// The windowed-relevance net must not exclude an unknown timestamp.
	violation := durationOrderViolation{Family: durationOrderIRQ, Issue: "endpoint_parse_incomplete", Line: 10, TsUnknown: true}
	if !durationOrderViolationRelevantToQuery(&violation, Query{TimeStart: 5, TimeEnd: 6}) {
		t.Fatalf("unknown-timestamp endpoint poison cannot be excluded by a time window it may well sit inside")
	}
	// Control: a known timestamp before the window stays excluded.
	known := durationOrderViolation{Family: durationOrderIRQ, Issue: "endpoint_parse_incomplete", Line: 10, CurrentTs: 1}
	if durationOrderViolationRelevantToQuery(&known, Query{TimeStart: 5, TimeEnd: 6}) {
		t.Fatalf("known pre-window endpoint poison stays scoped out")
	}

	// The per-pair net matches by physical line when the timestamp is unknown.
	idx := &Index{durationOrderFailures: []durationOrderViolation{{
		Family: durationOrderIRQ, Issue: "endpoint_parse_incomplete", Line: 5, TsUnknown: true,
	}}}
	if !interruptPairHasValidationFailure(idx, durationOrderIRQ, 1.0, 2.0, 3, 8) {
		t.Fatalf("unknown-ts poison physically between the pair endpoints must suppress that pair")
	}
	if interruptPairHasValidationFailure(idx, durationOrderIRQ, 1.0, 2.0, 10, 20) {
		t.Fatalf("unknown-ts poison outside the pair's physical lines must not suppress it")
	}
}
