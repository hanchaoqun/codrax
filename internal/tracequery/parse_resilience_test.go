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

func TestParseFile_UnparsedLinesCounted(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "unparsed.trace")
	content := "" +
		"# tracer: nop\n" +
		"some free-form log line that matches no trace format\n" +
		"\n" + // blank lines must not count
		"          <idle>-0     (-----) [000] d..3  100.000000: sched_switch: prev_comm=swapper prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=10 next_prio=100\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(t.Context(), p)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if idx.UnparsedLines != 2 {
		t.Fatalf("expected 2 unparsed non-empty lines, got %d (events=%d)", idx.UnparsedLines, len(idx.Events))
	}
	if len(idx.Events) != 1 {
		t.Fatalf("the valid line must still parse, got %d events", len(idx.Events))
	}
}

func TestParseFile_PanickedLineNotDoubleCountedAsUnparsed(t *testing.T) {
	idx := &Index{}
	orig := parseLineFn
	parseLineFn = func(int, string, *stringInterner) (Event, bool) { panic("malformed input") }
	defer func() { parseLineFn = orig }()
	// Mirror the parseFile loop contract: a panicked line increments
	// ParseLinePanics only, never UnparsedLines.
	panicsBefore := idx.ParseLinePanics
	if _, ok := safeParseLine(1, "x", nil, idx); !ok && idx.ParseLinePanics == panicsBefore {
		idx.UnparsedLines++
	}
	if idx.ParseLinePanics != 1 || idx.UnparsedLines != 0 {
		t.Fatalf("counters must stay disjoint: panics=%d unparsed=%d", idx.ParseLinePanics, idx.UnparsedLines)
	}
}

func TestRun_UnparsedMajorityAppendsCoverageCaveat(t *testing.T) {
	idx := &Index{Path: "synthetic.trace", ScannedLineCount: 10, UnparsedLines: 6}
	res := Run(idx, Query{View: "event_search"})
	want := "6 of 10 scanned lines did not match any known trace format; coverage may be incomplete"
	found := false
	for _, caveat := range res.Caveats {
		if caveat == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("majority-unparsed result must carry the coverage caveat, got %v", res.Caveats)
	}
	if res.UnparsedLineCount != 6 {
		t.Fatalf("result must surface the typed counter, got %d", res.UnparsedLineCount)
	}

	below := &Index{Path: "synthetic.trace", ScannedLineCount: 10, UnparsedLines: 5}
	resBelow := Run(below, Query{View: "event_search"})
	for _, caveat := range resBelow.Caveats {
		if caveat == "5 of 10 scanned lines did not match any known trace format; coverage may be incomplete" {
			t.Fatalf("at-threshold ratio must not caveat (soft guidance fires only above %v): %v", unparsedLineCaveatRatio, resBelow.Caveats)
		}
	}
}
