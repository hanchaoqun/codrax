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
// Baseline (no regression gate); the per-sparse-event cost is the
// differential against BenchmarkParseFileThroughput's dense fixture — see
// TestSparseSideTableAllocCost for the deterministic differential.
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

// TestSparseSideTableAllocCost measures the P4 per-sparse-event allocation
// cost deterministically: two fixtures identical except that the sparse
// variant swaps 10% of sched_switch lines for perf_sample +
// binder_transaction lines, so the alloc differential per sparse event is the
// cost of taking the sparse-kind parse lane (kv-parsing differences INCLUDED,
// side-table allocation included — the honest lane cost, not the isolated
// struct alloc). Measured at introduction (2026-07-05): combined 7.8
// allocs/sparse event, decomposing per-lane (vs one sched_switch line) into
// perf_sample ≈ +18 — dominated by the perf lane's PRE-EXISTING kv/string
// handling, of which the P4-new side-table share is exactly the 1 *PerfFields
// alloc by construction — and binder_transaction ≈ −2.5 (the binder lane is
// cheaper than a sched_switch line even including its 1 *BinderFields alloc).
// Payload variance (distinct transaction ids / periods per line) shifted the
// numbers by <0.1 alloc/event. The ceiling is an alloc-STORM tripwire at ~2×
// the measured combined cost, not a budget: crossing it means the sparse lane
// started allocating structurally (per-field boxing, per-event map copies),
// not that the fixture mix drifted a little.
func TestSparseSideTableAllocCost(t *testing.T) {
	dir := t.TempDir()
	const lines = 2000
	build := func(name string, sparse bool) (string, int) {
		var sb strings.Builder
		sparseCount := 0
		for i := 0; i < lines; i++ {
			ts := 100.0 + float64(i)*0.00001
			switch {
			case sparse && i%20 == 5:
				fmt.Fprintf(&sb, "          app-20    (  20) [002] d..3 %.6f: perf_sample: pid=20 tid=21 period=100000 event=cpu-cycles symbol=doWork dso=/system/lib64/libfoo.so ip=0x7f001234 callchain=doWork;main source=hiperf_proto sample_kind=on_cpu clock=boottime\n", ts)
				sparseCount++
			case sparse && i%20 == 15:
				fmt.Fprintf(&sb, "          app-20    (  20) [001] d..3 %.6f: binder_transaction: transaction=%d dest_node=311264 dest_proc=99 dest_thread=0 reply=0 flags=0x12 code=0x1\n", ts, i)
				sparseCount++
			default:
				fmt.Fprintf(&sb, "          app-20    (  20) [001] d..3 %.6f: sched_switch: prev_comm=app prev_pid=20 prev_prio=100 prev_state=S ==> next_comm=worker next_pid=30 next_prio=120\n", ts)
			}
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return path, sparseCount
	}
	densePath, _ := build("dense.trace", false)
	sparsePath, sparseCount := build("sparse.trace", true)
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
	denseAllocs := testing.AllocsPerRun(5, parse(densePath))
	sparseAllocs := testing.AllocsPerRun(5, parse(sparsePath))
	perEvent := (sparseAllocs - denseAllocs) / float64(sparseCount)
	t.Logf("alloc lane: dense=%.0f sparse=%.0f delta=%.0f sparse_events=%d per_sparse_event=%.2f allocs", denseAllocs, sparseAllocs, sparseAllocs-denseAllocs, sparseCount, perEvent)
	if perEvent > 16 {
		t.Fatalf("per-sparse-event alloc cost %.2f exceeds the storm tripwire (16 ≈ 2× the 7.8 measured at introduction): the sparse parse lane is allocating structurally beyond its known cost — investigate before shipping", perEvent)
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
