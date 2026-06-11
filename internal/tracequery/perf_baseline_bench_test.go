package tracequery

import (
	"testing"
	"unsafe"
)

func buildBenchIndex(n int) *Index {
	idx := &Index{Path: "bench.trace"}
	for i := 0; i < n; i++ {
		idx.Events = append(idx.Events, Event{
			Line: i + 1,
			Ts:   100.0 + float64(i)*0.001,
			Type: EventSchedSwitch,
			Name: "sched_switch",
			Comm: "app",
			PID:  10,
		})
	}
	idx.FirstTs = 100.0
	idx.LastTs = 100.0 + float64(n)*0.001
	idx.LineCount = n
	return idx
}

func BenchmarkDeriveWindowedIndex_200k_MidWindow(b *testing.B) {
	full := buildBenchIndex(200_000)
	b.Logf("Event size: %d bytes", unsafe.Sizeof(Event{}))
	opts := BuildOptions{TimeStart: 150.0, TimeStartSet: true, TimeEnd: 180.0, TimeEndSet: true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		deriveWindowedIndex(full, opts)
	}
}

func TestDeriveWindowedIndex_ContiguousSharesBacking(t *testing.T) {
	full := buildBenchIndex(1000)
	out := deriveWindowedIndex(full, BuildOptions{TimeStart: 100.2, TimeStartSet: true, TimeEnd: 100.5, TimeEndSet: true})
	if len(out.Events) == 0 {
		t.Fatal("window must select events")
	}
	if &out.Events[0] != &full.Events[200] {
		t.Fatalf("contiguous window must share the parent backing array (got line %d)", out.Events[0].Line)
	}
	if out.FirstTs == 0 || out.ParsedKnown != len(out.Events) {
		t.Fatalf("stats must be populated: %+v", out)
	}
}

func TestDeriveWindowedIndex_NonContiguousFallsBackToCopy(t *testing.T) {
	full := buildBenchIndex(100)
	// Inject a clock regression INSIDE the window span: an event whose
	// ts falls outside the window sits between in-window rows.
	full.Events[50].Ts = 50.0
	out := deriveWindowedIndex(full, BuildOptions{TimeStart: 100.02, TimeStartSet: true, TimeEnd: 100.08, TimeEndSet: true})
	if len(out.Events) == 0 {
		t.Fatal("window must select events")
	}
	for _, ev := range out.Events {
		if ev.Ts < 100.02 || ev.Ts > 100.08 {
			t.Fatalf("fallback copy must exclude out-of-window rows, got ts=%f", ev.Ts)
		}
	}
	if &out.Events[0] == &full.Events[20] {
		t.Fatal("non-contiguous window must not share the parent backing array")
	}
}
