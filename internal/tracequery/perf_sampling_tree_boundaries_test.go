package tracequery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func TestPerfSamplingTreeCensusBudgetOverflowAndGenerations(t *testing.T) {
	frames := tracewire.QuotePerfKVValue(tracewire.EncodePerfFrames([]tracewire.PerfFrame{{Symbol: "work", DSO: "app.so", IP: "0x1", SourceRef: "frame:0", SourceDepth: 0}}, "available"))
	line := func(ts, weight string) string {
		return "worker-11 (10) [000] .... " + ts + ": perf_sample: pid=10 tid=11 cpu=0 cpu_known=true sample_weight=" + weight + " event=cycles source=raw_perfdata_fallback frames_v1=" + frames + "\n"
	}
	write := func(text string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "capture.perftrace")
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	q := Query{View: ViewPerfSamplingTree, TimeStart: 1, TimeEnd: 2, TimeStartSet: true, TimeEndSet: true}
	path := write(line("1.1", "9223372036854775807") + line("1.2", "1") + line("1.3", "invalid"))
	r, err := StreamPerfSamplingTree(context.Background(), path, q)
	if err != nil || !ValidPerfSamplingTree(*r.PerfSamplingTree) {
		t.Fatalf("invalid overflow report %v %+v", err, r.PerfSamplingTree)
	}
	if r.PerfSamplingTree.CohortCount != 2 {
		t.Fatal("invalid sample weight contaminated native weight cohort")
	}
	for _, c := range r.PerfSamplingTree.Cohorts {
		if c.WeightUnit == "cycles" && (c.WeightStatus != "aggregate_overflow" || len(c.Nodes) != 0 || c.SampleCount != 2) {
			t.Fatalf("overflow claimed exact zero: %+v", c)
		}
		if c.WeightUnit == "sample_count_unweighted" && (c.TotalWeight != 1 || c.SampleCount != 1) {
			t.Fatal("invalid weight not retained as separate count inventory")
		}
	}
	r, err = streamPerfSamplingTree(context.Background(), path, q, 1, 1<<20)
	if err != nil || r.PerfSamplingTree.Status != "unavailable" || len(r.PerfSamplingTree.Cohorts) != 0 || r.ScannedLineCount != 3 {
		t.Fatalf("budget published partial census %v %+v", err, r)
	}
	var dense strings.Builder
	dense.WriteString(line("1.1", "2"))
	for i := 0; i < 70000; i++ {
		fmt.Fprintln(&dense, "other-99 (99) [000] .... 1.200000: tracing_mark_write: C|99|value|1")
	}
	dense.WriteString("worker-11 (10) [000] .... 1.300000: sched_switch: prev_comm=worker prev_pid=11 prev_prio=120 prev_state=X ==> next_comm=other next_pid=99 next_prio=120\n")
	dense.WriteString("worker-11 (10) [000] .... 1.400000: tracing_mark_write: C|10|value|1\n")
	dense.WriteString(line("1.5", "3"))
	path = write(dense.String())
	r, err = StreamPerfSamplingTree(context.Background(), path, q)
	if err != nil || r.PerfSamplingTree.Status != "available" || r.PerfSamplingTree.SampleCount != 2 || r.PerfSamplingTree.CohortCount != 2 {
		t.Fatalf("unrelated rows exhausted budget or reused thread merged: %v %+v", err, r.PerfSamplingTree)
	}
	for _, c := range r.PerfSamplingTree.Cohorts {
		if c.ThreadIdentity == nil {
			t.Fatal("complete source lost generation")
		}
	}
}

func TestPerfSamplingTreeRetainedFrameByteBudget(t *testing.T) {
	var frames []tracewire.PerfFrame
	for i := 0; i < 20; i++ {
		frames = append(frames, tracewire.PerfFrame{Symbol: strings.Repeat("s", 500), DSO: strings.Repeat("d", 500), IP: fmt.Sprint(i), SourceRef: fmt.Sprint(i), SourceDepth: int64(i)})
	}
	carrier := tracewire.EncodePerfFrames(frames, "available")
	if p, ok := tracewire.DecodePerfFrames(carrier); !ok || p.Status != "available" || len(carrier) < 20000 {
		t.Fatal("test requires a legal large frame carrier")
	}
	path := filepath.Join(t.TempDir(), "large.perftrace")
	line := "worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 sample_weight=1 event=cycles frames_v1=" + tracewire.QuotePerfKVValue(carrier) + "\n"
	if err := os.WriteFile(path, []byte(line+line), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := streamPerfSamplingTree(context.Background(), path, Query{View: ViewPerfSamplingTree, TimeStart: 1, TimeEnd: 2}, 100, 4000)
	if err != nil || r.PerfSamplingTree.Status != "unavailable" || r.PerfSamplingTree.Reason != "complete_scan_retention_budget_exceeded" || r.ScannedLineCount != 2 {
		t.Fatalf("retained carrier bytes escaped budget: err=%v result=%+v", err, r)
	}
}

func TestPerfSamplingTreeScopedIdentityWithheld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conflict.perftrace")
	body := "worker-7 (70) [000] .... 1.1: perf_sample: pid=70 tid=7 thread_comm=worker thread_identity_known=true sample_weight=3 event=cycles\nworker-7 (71) [000] .... 1.2: perf_sample: pid=71 tid=7 thread_comm=worker thread_identity_known=true sample_weight=4 event=cycles\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	for _, q := range []Query{{View: ViewPerfSamplingTree, TimeStart: 1, TimeEnd: 2, PID: 7}, {View: ViewPerfSamplingTree, TimeStart: 1, TimeEnd: 2, Thread: "worker"}} {
		r, err := StreamPerfSamplingTree(context.Background(), path, q)
		if err != nil {
			t.Fatal(err)
		}
		p := r.PerfSamplingTree
		caveats := strings.Join(p.Caveats, "\n")
		if p.Status != "unavailable" || p.SampleCount != 0 || len(p.Cohorts) != 0 || !strings.Contains(caveats, "perf_thread_selector_withheld=true") || !strings.Contains(caveats, "perf_thread_tgid_conflict=true") {
			t.Fatalf("unproved selector reported true absence: %+v", p)
		}
	}
}
