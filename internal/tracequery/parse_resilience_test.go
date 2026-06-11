package tracequery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeParseLine_PanicDegradesToTypedCounter(t *testing.T) {
	idx := &Index{}
	orig := parseLineFn
	parseLineFn = func(int, string, *stringInterner) (Event, bool) { panic("malformed input") }
	defer func() { parseLineFn = orig }()
	ev, ok := safeParseLine(1, "any line", nil, idx)
	if ok {
		t.Fatalf("panicked parse must report not-ok, got %+v", ev)
	}
	if idx.ParseLinePanics != 1 {
		t.Fatalf("panic must increment the typed counter, got %d", idx.ParseLinePanics)
	}
}

func TestParseFile_ClockRegressionCounted(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "regress.trace")
	content := "" +
		"          <idle>-0     (-----) [000] d..3  100.000000: sched_switch: prev_comm=swapper prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=10 next_prio=100\n" +
		"             app-10    (   10) [000] d..3   99.500000: sched_switch: prev_comm=app prev_pid=10 prev_prio=100 prev_state=S ==> next_comm=swapper next_pid=0 next_prio=120\n" +
		"          <idle>-0     (-----) [000] d..3  101.000000: sched_switch: prev_comm=swapper prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=10 next_prio=100\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(t.Context(), p)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if idx.ClockRegressions != 1 {
		t.Fatalf("expected exactly one clock regression, got %d (events=%d)", idx.ClockRegressions, len(idx.Events))
	}
}
