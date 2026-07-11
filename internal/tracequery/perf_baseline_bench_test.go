package tracequery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// BenchmarkParseFileThroughput is a baseline (no regression gate): it
// measures full-file parse throughput over a synthetic trace mixing valid
// sched/mark lines with unparseable noise so the UnparsedLines counter path
// is exercised in the hot loop.
func BenchmarkParseFileThroughput(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "throughput.trace")
	const lines = 50_000
	var sb strings.Builder
	for i := 0; i < lines; i++ {
		ts := 100.0 + float64(i)*0.00001
		switch i % 10 {
		case 7:
			fmt.Fprintf(&sb, "free-form noise line %d that matches no trace format\n", i)
		case 8:
			fmt.Fprintf(&sb, "          app-20    (  20) [001] d..3 %.6f: print: B|20|Choreographer#doFrame %d\n", ts, i)
		default:
			fmt.Fprintf(&sb, "          app-20    (  20) [001] d..3 %.6f: sched_switch: prev_comm=app prev_pid=20 prev_prio=100 prev_state=S ==> next_comm=worker next_pid=30 next_prio=120\n", ts)
		}
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		b.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		b.Fatal(err)
	}
	b.Logf("Event size: %d bytes; trace: %d lines / %d bytes", unsafe.Sizeof(Event{}), lines, info.Size())
	b.ReportAllocs()
	b.SetBytes(info.Size())
	b.ResetTimer()
	var idx *Index
	for i := 0; i < b.N; i++ {
		idx, err = parseFile(context.Background(), path, info.Size(), info.ModTime().UnixNano(), BuildOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if idx != nil {
		elapsed := b.Elapsed().Seconds()
		if elapsed > 0 {
			b.Logf("parsed_events=%d unparsed_lines=%d events_per_sec=%.0f lines_per_sec=%.0f",
				len(idx.Events), idx.UnparsedLines,
				float64(len(idx.Events))*float64(b.N)/elapsed,
				float64(lines)*float64(b.N)/elapsed)
		}
		if idx.UnparsedLines == 0 {
			b.Fatal("bench trace must exercise the UnparsedLines counter path")
		}
	}
}

// BenchmarkParseFileThroughputSparseKinds is the P4 side-table allocation
// lane: same shape as BenchmarkParseFileThroughput but with 10% sparse-family
// events (5% perf_sample + 5% binder_transaction, realistic app-trace
// density), so the per-event group allocations (*PerfFields / *BinderFields)
// that the dense fixture never exercises show up in allocs/op and B/op.
// Baseline (no regression gate); TestSparseSideTableAllocCost carries the
// deterministic absolute per-lane allocation ratchets.
func BenchmarkParseFileThroughputSparseKinds(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "throughput_sparse.trace")
	const lines = 50_000
	var sb strings.Builder
	for i := 0; i < lines; i++ {
		ts := 100.0 + float64(i)*0.00001
		switch i % 20 {
		case 5:
			fmt.Fprintf(&sb, "          app-20    (  20) [002] d..3 %.6f: perf_sample: pid=20 tid=21 period=100000 event=cpu-cycles symbol=doWork dso=/system/lib64/libfoo.so ip=0x7f001234 callchain=doWork;main source=hiperf_proto sample_kind=on_cpu clock=boottime\n", ts)
		case 15:
			fmt.Fprintf(&sb, "          app-20    (  20) [001] d..3 %.6f: binder_transaction: transaction=%d dest_node=311264 dest_proc=99 dest_thread=0 reply=0 flags=0x12 code=0x1\n", ts, i)
		case 7, 17:
			fmt.Fprintf(&sb, "free-form noise line %d that matches no trace format\n", i)
		case 8, 18:
			// print kept at the dense fixture's 10% so the ONLY mix delta vs
			// BenchmarkParseFileThroughput is the sched->sparse swap and the
			// bench-pair differential stays clean.
			fmt.Fprintf(&sb, "          app-20    (  20) [001] d..3 %.6f: print: B|20|Choreographer#doFrame %d\n", ts, i)
		default:
			fmt.Fprintf(&sb, "          app-20    (  20) [001] d..3 %.6f: sched_switch: prev_comm=app prev_pid=20 prev_prio=100 prev_state=S ==> next_comm=worker next_pid=30 next_prio=120\n", ts)
		}
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		b.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(info.Size())
	b.ResetTimer()
	var idx *Index
	for i := 0; i < b.N; i++ {
		idx, err = parseFile(context.Background(), path, info.Size(), info.ModTime().UnixNano(), BuildOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if idx != nil {
		perf, binder := 0, 0
		for i := range idx.Events {
			switch idx.Events[i].Type {
			case EventPerfSample:
				perf++
			case EventBinderTransaction:
				binder++
			}
		}
		if perf != lines/20 || binder != lines/20 {
			b.Fatalf("sparse fixture rot: parsed perf=%d binder=%d, want %d each — the sparse lanes are not being exercised", perf, binder, lines/20)
		}
		b.Logf("sparse events: perf_sample=%d binder_transaction=%d (10%% of lines); side_table_bytes=%d", perf, binder, idx.RetainedSideTableBytes)
	}
}

// TestSparseSideTableAllocCost applies absolute per-event allocation tripwires
// to the sched, perf and binder parse lanes independently. The original P4
// gate subtracted an all-sched fixture from a sparse mix. That differential is
// not stable under a legitimate sched parser optimization: making sched rows
// cheaper increases "sparse - sched" even when neither sparse lane allocates
// one additional byte. Absolute lane ceilings retain the intended allocation-
// storm protection without penalizing improvements in a different lane.
//
// The ceilings are deliberately ratchets, not budgets. They sit above the
// 2026-07-11 Go 1.26.3/darwin-arm64 measured costs (sched core≈17.5, Donghu
// suffix≈25.5, perf≈45.5, binder≈25 allocs/event) while remaining tight enough
// to catch allocation storms such as several new structural allocations per
// event.
func TestSparseSideTableAllocCost(t *testing.T) {
	if raceBuildEnabled {
		t.Skip("allocation ratchets are calibrated for non-race builds; race instrumentation changes allocation counts")
	}
	dir := t.TempDir()
	const lines = 2000
	type lane struct {
		name       string
		ceiling    float64
		renderLine func(*strings.Builder, int, float64)
		validEvent func(Event) bool
	}
	lanes := []lane{
		{
			name:    "sched_switch",
			ceiling: 22,
			renderLine: func(sb *strings.Builder, _ int, ts float64) {
				fmt.Fprintf(sb, "          app-20    (  20) [001] d..3 %.6f: sched_switch: prev_comm=app prev_pid=20 prev_prio=100 prev_state=S ==> next_comm=worker next_pid=30 next_prio=120\n", ts)
			},
			validEvent: func(ev Event) bool {
				return ev.Type == EventSchedSwitch && ev.PrevPID == 20 && ev.NextPID == 30 && ev.NextInfo == ""
			},
		},
		{
			name:    "sched_switch_donghu_suffix",
			ceiling: 30,
			renderLine: func(sb *strings.Builder, _ int, ts float64) {
				fmt.Fprintf(sb, "          app-20    (  20) [001] d..3 %.6f: sched_switch: prev_comm=app prev_pid=20 prev_prio=142 prev_state=R+ ==> next_comm=worker next_pid=30 next_prio=-2 next_info=3fff,89,3,0,2,0 cg=top-app\n", ts)
			},
			validEvent: func(ev Event) bool {
				return ev.Type == EventSchedSwitch && ev.PrevPrio == 142 && ev.NextPrio == -2 &&
					ev.NextInfo == "3fff,89,3,0,2,0" && ev.CGroup == "top-app" &&
					ev.NextInfoAffinity == "3fff" && ev.NextInfoLoad == 89 && ev.NextInfoGroup == 3
			},
		},
		{
			name:    "perf_sample",
			ceiling: 55,
			renderLine: func(sb *strings.Builder, _ int, ts float64) {
				fmt.Fprintf(sb, "          app-20    (  20) [002] d..3 %.6f: perf_sample: pid=20 tid=21 period=100000 event=cpu-cycles symbol=doWork dso=/system/lib64/libfoo.so ip=0x7f001234 callchain=doWork;main source=hiperf_proto sample_kind=on_cpu clock=boottime\n", ts)
			},
			validEvent: func(ev Event) bool {
				return ev.Type == EventPerfSample && ev.PerfFields != nil && ev.PerfFields.Symbol == "doWork"
			},
		},
		{
			name:    "binder_transaction",
			ceiling: 32,
			renderLine: func(sb *strings.Builder, i int, ts float64) {
				fmt.Fprintf(sb, "          app-20    (  20) [001] d..3 %.6f: binder_transaction: transaction=%d dest_node=311264 dest_proc=99 dest_thread=0 reply=0 flags=0x12 code=0x1\n", ts, i)
			},
			validEvent: func(ev Event) bool {
				return ev.Type == EventBinderTransaction && ev.BinderFields != nil && ev.BinderFields.DestProc == 99
			},
		},
	}
	build := func(name string, renderLine func(*strings.Builder, int, float64)) string {
		var sb strings.Builder
		for i := 0; i < lines; i++ {
			ts := 100.0 + float64(i)*0.00001
			renderLine(&sb, i, ts)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	parse := func(path string) func() {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		return func() {
			if _, err := parseFile(context.Background(), path, info.Size(), info.ModTime().UnixNano(), BuildOptions{}); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, tc := range lanes {
		t.Run(tc.name, func(t *testing.T) {
			path := build(tc.name+".trace", tc.renderLine)
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			idx, err := parseFile(context.Background(), path, info.Size(), info.ModTime().UnixNano(), BuildOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if len(idx.Events) != lines || idx.UnparsedLines != 0 || idx.ParsedKnown != lines {
				t.Fatalf("allocation fixture admission changed: lane=%s events=%d parsed_known=%d unparsed=%d want=%d/%d/0", tc.name, len(idx.Events), idx.ParsedKnown, idx.UnparsedLines, lines, lines)
			}
			for i := range idx.Events {
				if !tc.validEvent(idx.Events[i]) {
					t.Fatalf("allocation fixture typed lane changed: lane=%s event_index=%d event=%+v", tc.name, i, idx.Events[i])
				}
			}
			allocs := testing.AllocsPerRun(5, parse(path))
			perEvent := allocs / lines
			t.Logf("alloc lane=%s total=%.0f events=%d per_event=%.2f ceiling=%.2f", tc.name, allocs, lines, perEvent, tc.ceiling)
			if perEvent > tc.ceiling {
				t.Fatalf("%s allocation cost %.2f/event exceeds absolute storm tripwire %.2f: investigate structural per-event allocation before shipping", tc.name, perEvent, tc.ceiling)
			}
		})
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
