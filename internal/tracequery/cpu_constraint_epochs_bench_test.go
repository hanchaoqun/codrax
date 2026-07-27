package tracequery

import (
	"fmt"
	"testing"
)

// 批己 (§15.12 PIN-HYG / E2 alloc): the window-stats epoch sweep is on the
// per-event path of every HarmonyOS trace — the REPEAT shape (identical
// consecutive next_info, the overwhelmingly common case) must not
// materialize a discarded epoch + signature per event.

func epochBenchEvents(b testing.TB, repeats int) ([]Event, []int) {
	intern := newStringInterner()
	events := make([]Event, 0, repeats)
	for i := 0; i < repeats; i++ {
		line := fmt.Sprintf(`       idle/0-0   (    0) [000] .... 1.%06d: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52 next_info=3,4,1,0,0 cg=top-app`, i)
		ev, ok := ParseLine(i+1, line, intern)
		if !ok {
			b.Fatalf("bench line %d must parse", i)
		}
		events = append(events, ev)
	}
	indexes := make([]int, len(events))
	for i := range indexes {
		indexes[i] = i
	}
	return events, indexes
}

func BenchmarkCPUConstraintEpochAccountingRepeatHeavy(b *testing.B) {
	events, indexes := epochBenchEvents(b, 4096)
	q := Query{TimeStart: 1, TimeEnd: 2}
	universe := map[int]bool{0: true, 1: true}
	core := map[int]string{0: "small", 1: "small"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		computeCPUConstraintEpochAccounting(events, indexes, q, 2, nil, universe, core, coreCapabilityMap{}, nil)
	}
}

// TestCPUConstraintEpochAccountingRepeatAllocRatchet — the repeat shape's
// whole-sweep allocation budget (4096 identical-payload events): the mint
// path allocates once per EPOCH (maps, slices, merge carry) and the repeat
// path only grows snapshotTs. Re-introducing a per-event epoch/signature
// materialization (the pre-批己 shape: ~6 allocs/event, 24k+ per sweep)
// goes red here immediately.
func TestCPUConstraintEpochAccountingRepeatAllocRatchet(t *testing.T) {
	events, indexes := epochBenchEvents(t, 4096)
	q := Query{TimeStart: 1, TimeEnd: 2}
	universe := map[int]bool{0: true, 1: true}
	core := map[int]string{0: "small", 1: "small"}
	allocs := testing.AllocsPerRun(5, func() {
		computeCPUConstraintEpochAccounting(events, indexes, q, 2, nil, universe, core, coreCapabilityMap{}, nil)
	})
	const budget = 64
	if allocs > budget {
		t.Fatalf("repeat-shape sweep allocations %.0f exceed the ratchet budget %d — a per-event materialization crept back onto the hot path", allocs, budget)
	}
}
