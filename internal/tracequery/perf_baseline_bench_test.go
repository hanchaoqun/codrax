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
