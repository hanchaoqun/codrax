package tracediag

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func nonEventSchemaBeforeBusinessTree(t *testing.T, typ reflect.Type, schema string) string {
	t.Helper()
	if typ != reflect.TypeOf(tracequery.WindowStats{}) {
		return schema
	}
	const added = "BusinessTree|*tracequery.TraceMarkerTreeStats|business_tree,omitempty"
	var prior []string
	count := 0
	for _, field := range strings.Split(schema, ";") {
		if field == added {
			count++
		} else {
			prior = append(prior, field)
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one reviewed business tree addition, got %d: %s", count, schema)
	}
	return strings.Join(prior, ";")
}

func TestBusinessTreeSchemaEvolutionIsAdditive(t *testing.T) {
	typ := reflect.TypeOf(tracequery.WindowStats{})
	_, schema := detailSchemaFingerprint(typ)
	prior := nonEventSchemaBeforeBusinessTree(t, typ, schema)
	sum := sha256.Sum256([]byte(prior))
	const want = "8618c0fbf7e95f6c9e92daf85fb162781d32e020e50156f1d6e71f49edfc98a6"
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("tree addition changed prior WindowStats: got=%s want=%s", got, want)
	}
}

func TestBusinessTreeNestedFieldDisposition(t *testing.T) {
	cases := []struct {
		typ  reflect.Type
		want string
	}{
		{reflect.TypeOf(tracequery.TraceMarkerTreeStats{}), "Window|tracequery.TraceMarkerTreeWindow|window;WindowUnavailableReason|string|window_unavailable_reason,omitempty;NodeCount|int|node_count;Nodes|[]tracequery.TraceMarkerTreeNode|nodes,omitempty;OmittedNodes|int|omitted_nodes;Coverage|string|coverage;Caveats|[]string|caveats,omitempty"},
		{reflect.TypeOf(tracequery.TraceMarkerTreeWindow{}), "StartTs|float64|start_ts;EndTs|float64|end_ts"},
		{reflect.TypeOf(tracequery.TraceMarkerTreeNode{}), "ID|string|id;ParentID|string|parent_id,omitempty;ParentStatus|string|parent_status;SourcePath|string|source_path;Thread|tracequery.ThreadRef|thread;Name|string|name;StartLine|int|start_line;EndLine|int|end_line,omitempty;ActualStartTs|float64|actual_start_ts;ActualEndTs|*float64|actual_end_ts,omitempty;Closure|string|closure;DirectChildCount|int|direct_child_count;Inclusive|*tracequery.TraceMarkerTreeAccount|inclusive,omitempty;Self|*tracequery.TraceMarkerTreeAccount|self,omitempty"},
		{reflect.TypeOf(tracequery.TraceMarkerTreeAccount{}), "DurationMs|float64|duration_ms;Segments|[]tracequery.TraceMarkerTreeWindow|segments,omitempty;OmittedSegments|int|omitted_segments;States|*tracequery.TraceMarkerTreeStates|states,omitempty"},
		{reflect.TypeOf(tracequery.TraceMarkerTreeStates{}), "Coverage|string|coverage;Values|*tracequery.TraceMarkerTreeStateValues|values,omitempty;UnknownMs|float64|unknown_ms;Reasons|[]string|reasons,omitempty"},
		{reflect.TypeOf(tracequery.TraceMarkerTreeStateValues{}), "RunningMs|float64|running_ms;RunnableMs|float64|runnable_ms;SleepMs|float64|sleep_ms;DStateMs|float64|d_state_ms;IOWaitMs|float64|io_wait_ms;StoppedMs|float64|stopped_ms;DeadMs|float64|dead_ms;SleepIOWaitMs|float64|sleep_io_wait_ms;AccountedMs|float64|accounted_ms"},
	}
	for _, tc := range cases {
		_, schema := detailSchemaFingerprint(tc.typ)
		if schema != tc.want {
			t.Errorf("%s requires explicit field disposition review: got %s want %s", tc.typ, schema, tc.want)
		}
		for i := 0; i < tc.typ.NumField(); i++ {
			if policySkipsDetailField(&nonEventDetailPolicy, tc.typ, tc.typ.Field(i).Name) {
				t.Errorf("tree field must retain its detail owner: %s.%s", tc.typ, tc.typ.Field(i).Name)
			}
		}
	}
}

func TestBusinessTreeDetailPreservesNativePrecisionAndMeasurements(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nano.systrace")
	body := "idle-0 (0) [001] .... 0.000000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=41 next_prio=120\n" +
		"worker-41 (41) [001] .... 0.000000001: tracing_mark_write: B|41|load\n" +
		"worker-41 (41) [001] .... 0.000000002: tracing_mark_write: B|41|parse\n" +
		"worker-41 (41) [001] .... 0.000000003: tracing_mark_write: E|41\n" +
		"worker-41 (41) [001] .... 0.000000004: tracing_mark_write: E|41\n" +
		"worker-41 (41) [001] .... 0.000000005: sched_switch: prev_comm=worker prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	res := tracequery.Run(idx, tracequery.Query{View: "window_stats", TimeStartSet: true, TimeEndSet: true, TimeEnd: .000000010})
	if res.WindowStats == nil || res.WindowStats.BusinessTree == nil || len(res.WindowStats.BusinessTree.Nodes) != 2 {
		t.Fatalf("real public query must first produce tree: %+v", res.WindowStats)
	}
	before, _ := json.Marshal(res)
	report := strings.Join(renderStepBody(&Step{View: "window_stats", effMaxLines: 1000}, stepOutcome{result: &res}).lines, "\n")
	for _, want := range []string{"business_tree: 业务层级 node_count=2 omitted_nodes=0", "business_tree.window: start_ts=0 end_ts=0.00000001", "actual_start_ts=0.000000001 actual_end_ts=0.000000004", "duration_ms=0.000003", "duration_ms=0.000002", "start_ts=0.000000003 end_ts=0.000000004", "runnable_ms=0", "sleep_io_wait_ms=0", "parent_id="} {
		if !strings.Contains(report, want) {
			t.Errorf("lost native measurement/identity %q:\n%s", want, report)
		}
	}
	after, _ := json.Marshal(res)
	if !bytes.Equal(before, after) {
		t.Fatal("render mutated tree measurements")
	}
}

func TestBusinessTreeBulkPreservesOlderDetailAndUnknown(t *testing.T) {
	res := &tracequery.Result{View: "window_stats", WindowStats: &tracequery.WindowStats{
		TopRunning:    []tracequery.ThreadDuration{{Thread: tracequery.ThreadRef{PID: 41, Comm: "worker"}, DurationMs: 57.828, CPU: 12}},
		ComputeSupply: []tracequery.ComputeSupplySummary{{Thread: tracequery.ThreadRef{PID: 41, Comm: "worker"}, Summary: "existing frequency donor evidence"}},
	}}
	baseline := renderStepBody(&Step{View: "window_stats", effMaxLines: 1000}, stepOutcome{result: res})
	res.WindowStats.BusinessTree = &tracequery.TraceMarkerTreeStats{NodeCount: 1, Nodes: []tracequery.TraceMarkerTreeNode{{
		ID: "node", Name: "load", SourcePath: "/private/capture.systrace", Thread: tracequery.ThreadRef{PID: 41}, ParentStatus: "unknown_prefix", Closure: "closed",
		Inclusive: &tracequery.TraceMarkerTreeAccount{DurationMs: 1, States: &tracequery.TraceMarkerTreeStates{Coverage: "unavailable", UnknownMs: 1}},
	}}}
	before, _ := json.Marshal(res)
	bounded := renderStepBody(&Step{View: "window_stats", effMaxLines: len(baseline.lines)}, stepOutcome{result: res})
	if !reflect.DeepEqual(bounded.lines, baseline.lines) {
		t.Fatalf("tree evicted previous measurements: %v want %v", bounded.lines, baseline.lines)
	}
	full := renderStepBody(&Step{View: "window_stats", effMaxLines: 1000}, stepOutcome{result: res})
	report := strings.Join(full.lines, "\n")
	if bounded.total != full.total || full.total <= len(bounded.lines) || !strings.Contains(report, "unknown_ms=1") || !strings.Contains(report, "前序层级未知") || !strings.Contains(report, "不能按零处理") || strings.Contains(report, "runnable_ms=") || strings.Contains(report, "/private/") {
		t.Fatalf("tree omission/unknown/privacy boundary lost: bounded=%+v full=%+v", bounded, full)
	}
	after, _ := json.Marshal(res)
	if !bytes.Equal(before, after) {
		t.Fatal("render mutated unknown state")
	}
}

func TestBusinessTreeStateZeroRequiresAvailableValues(t *testing.T) {
	for _, coverage := range []string{"complete", "partial", "unavailable", "future"} {
		for _, values := range []*tracequery.TraceMarkerTreeStateValues{nil, {}} {
			state := tracequery.TraceMarkerTreeStates{Coverage: coverage, Values: values}
			var lines []string
			renderBusinessTreeDetail(state, "business_tree.nodes[0].self.states", func(s string) { lines = append(lines, s) }, 0, &nonEventDetailPolicy)
			report := strings.Join(lines, "\n")
			measured := values != nil && (coverage == "complete" || coverage == "partial")
			if strings.Contains(report, "running_ms=0") != measured || !strings.Contains(report, "unknown_ms=0") {
				t.Fatalf("unknown or measured zero conflated for %s/%v: %s", coverage, values, report)
			}
		}
	}
}

func TestBusinessTreeLineSelectionDoesNotInventContinuousWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "line-only.systrace")
	body := "worker-41 (41) [001] .... 1.000000: tracing_mark_write: B|41|load\n" +
		"worker-41 (41) [001] .... 1.010000: tracing_mark_write: E|41\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	res := tracequery.Run(idx, tracequery.Query{View: "window_stats", LineStart: 1, LineEnd: 2})
	if res.WindowStats == nil || res.WindowStats.BusinessTree == nil || res.WindowStats.BusinessTree.WindowUnavailableReason == "" {
		t.Fatalf("real line-selected query must first disclose unavailable time window: %+v", res.WindowStats)
	}
	before, _ := json.Marshal(res)
	report := strings.Join(renderStepBody(&Step{View: "window_stats", effMaxLines: 1000}, stepOutcome{result: &res}).lines, "\n")
	for _, want := range []string{"business_tree.window: 仅按行选择，连续时间窗未确定", "actual_start_ts=1 actual_end_ts=1.01", "business_tree.nodes[0].inclusive.segments[0]: start_ts=1 end_ts=1.01", "不能按零处理"} {
		if !strings.Contains(report, want) {
			t.Errorf("missing line-selection measurement boundary %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "business_tree.window: start_ts=") {
		t.Fatalf("placeholder query endpoints became a measured time window: %s", report)
	}
	after, _ := json.Marshal(res)
	if !bytes.Equal(before, after) {
		t.Fatal("renderer mutated line-selected query evidence")
	}
}
