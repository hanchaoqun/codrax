package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The public numeric parameters are intentionally only millisecond-precise.
// Their ±0.5ms lookup tolerance must never become the engine/report window.
// This is the full parameter -> tracequery engine -> typed observation ->
// deterministic report pin for the customer 5.000..5.007 witness.
func TestTraceQueryRequestedWindowIsTheOnlyCausalMetricDenominator(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "semantic-window.systrace")
	trace := `# tracer: nop
        app-100   (  100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200   (  200) [002] .... 5.000400: tracing_mark_write: B|200|VerifyClass com.example.Foo
     worker-200   (  200) [002] .... 5.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200   (  200) [002] .... 5.005000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200   (  200) [002] .... 5.005400: tracing_mark_write: E|200
     worker-200   (  200) [002] .... 5.005600: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100   (  100) [001] .... 5.005800: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100   (  100) [001] .... 5.007000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=R ==> next_comm=idle/1 next_pid=0 next_prio=120
`
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, err := json.Marshal(map[string]any{
		"source": "path", "path": "semantic-window.systrace",
		"view": "root_cause_rank", "pid": 100, "thread": "app-100",
		"time_start": 5.000, "time_end": 5.007,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("trace_query: err=%v success=%v summary=%s", err, res.Success, res.Summary)
	}
	if !strings.Contains(res.Summary, "selected_window=5.000000..5.007000 seconds") {
		t.Fatalf("engine summary must use the exact requested window:\n%s", res.Summary)
	}
	for _, forbidden := range []string{"selected_window=5.000000..5.007500", "matching_window=5.000000..5.007500"} {
		if strings.Contains(res.Summary, forbidden) {
			t.Fatalf("causal summary leaked lookup tolerance %q:\n%s", forbidden, res.Summary)
		}
	}

	sawSelectedWindow := false
	for _, record := range res.Observations {
		for _, note := range record.RichNotes {
			if strings.HasPrefix(note, "selected_window=") {
				sawSelectedWindow = true
				if note != "selected_window=5.000000..5.007000" {
					t.Fatalf("typed causal observation uses a non-requested denominator: id=%s note=%q", record.ID, note)
				}
			}
			if strings.HasPrefix(note, "matching_window=") {
				t.Fatalf("lookup-only matching window must not enter causal observations: id=%s note=%q", record.ID, note)
			}
		}
	}
	if !sawSelectedWindow {
		t.Fatalf("fixture did not publish a typed selected_window: %+v", res.Observations)
	}
	bus := audit730Bus("")
	bus.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"app-100", "5.000", "5.007"}
	md := audit730Render(t, bus, res.Observations, "")
	for _, want := range []string{
		"分析窗 5.000~5.007s，共 7.000ms。",
		"满格=窗口7.000ms",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("typed report missing exact requested-window contract %q:\n%s", want, md)
		}
	}
	for _, forbidden := range []string{"7.500ms", "分析窗 5.000~5.008s"} {
		if strings.Contains(md, forbidden) {
			t.Fatalf("typed report mixed in the lookup window %q:\n%s", forbidden, md)
		}
	}
	if got := strings.Count(md, "自身·sleep 5.000ms"); got != 1 {
		t.Fatalf("one physical target sleep segment must occupy one projection seat, got %d:\n%s", got, md)
	}
}

func TestTraceQueryEventSearchTolerancePublishesTypedLookupWindowContract(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "lookup-window.systrace")
	trace := `app-20 (20) [001] .... 1.010004: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params := json.RawMessage(`{
		"source":"path", "path":"lookup-window.systrace", "view":"event_search",
		"time_start":"1.01000s", "time_end":"1.01000s",
		"event_types":["sched_wakeup"]
	}`)
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("trace_query: err=%v success=%v summary=%s", err, res.Success, res.Summary)
	}
	for _, want := range []string{
		"line=1",
		"requested_window=1.010000..1.010000",
		"matching_window=1.009995..1.010005",
		"matching_window_policy=lookup_only_boundary_tolerance",
		"causal/statistical selected_window metrics, durations, percentages, and reports always use requested_window",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("lookup summary missing %q:\n%s", want, res.Summary)
		}
	}
	if len(res.Observations) == 0 {
		t.Fatalf("lookup fixture did not publish typed observations")
	}
	notes := strings.Join(res.Observations[0].RichNotes, "\n")
	for _, want := range []string{
		"requested_window=1.010000..1.010000",
		"matching_window=1.009995..1.010005",
		"matching_window_policy=lookup_only_boundary_tolerance",
		"metric_denominator_window=requested_window",
	} {
		if !strings.Contains(notes, want) {
			t.Fatalf("typed lookup observation missing %q: %+v", want, res.Observations[0])
		}
	}
}
