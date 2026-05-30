package tool

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryExplicitPathProducesRuntimeArtifactSummary(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "sample.systrace")
	trace := strings.Join([]string{
		`waker-10 (10) [000] .... 1.000000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 1.010000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`waker-10 (10) [000] .... 1.050000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 1.080000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":     "path",
		"path":       "sample.systrace",
		"view":       "wakeup_chain",
		"pid":        20,
		"time_start": 1.0,
		"time_end":   1.1,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{"origin=runtime_artifact", "artifact_kind=trace", "timestamp_unit=seconds", "priority_semantics=", "Wakeup chain", "waker-10"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, res.Summary)
		}
	}
	if res.RawRef == "" {
		t.Fatalf("expected payload raw ref")
	}
}

func TestTraceQuerySchemaDocumentsViews(t *testing.T) {
	body := (&TraceQuery{}).Description() + "\n" + string((&TraceQuery{}).Parameters())
	for _, want := range []string{"wakeup_chain", "thread_timeline", "window_stats", "ipc_graph", "event_search", "attached_trace", "seconds", "microsecond precision", "81774 us", "larger numeric priority", "1-40=CFS", "cpu_frequency", "clock_set_rate", "block_bio_remap", "sched_blocked_reason", "binder_transaction_received"} {
		if !strings.Contains(body, want) {
			t.Fatalf("trace_query schema/description missing %q:\n%s", want, body)
		}
	}
}

func TestTraceQueryIPCGraphSummary(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "ipc.systrace")
	trace := strings.Join([]string{
		`client-20 (20) [001] .... 3.010000: binder_transaction: transaction=42 dest_proc=100 dest_thread=101 reply=1 flags=0x0 code=0x3`,
		`binder:100_1-101 (100) [002] .... 3.012000: binder_transaction_received: transaction=42`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":     "path",
		"path":       "ipc.systrace",
		"view":       "ipc_graph",
		"pid":        20,
		"time_start": 3.0,
		"time_end":   3.02,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{"IPC graph", "transaction=42", "client-20", "binder:100_1-101", "send_line=1", "receive_line=2"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryAcceptsTimestampStringsAndAppliesTinyTolerance(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "time.systrace")
	trace := strings.Join([]string{
		`app-20 (20) [001] .... 1.010004: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 1.020000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":      "path",
		"path":        "time.systrace",
		"view":        "event_search",
		"time_start":  "1.01000s",
		"time_end":    "1.01000 秒",
		"event_types": []string{"sched_wakeup"},
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{"line=1", "time_start=1.010000", "time_end=1.010000", "normalized=1.010000", "query_tolerance_seconds"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryAcceptsStringifiedScalarAndEventTypeParams(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "stringy.systrace")
	trace := strings.Join([]string{
		`waker-10 (10) [000] .... 1.000000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 1.010000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`waker-10 (10) [000] .... 1.050000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 1.080000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params := json.RawMessage(`{
		"source": "path",
		"path": "stringy.systrace",
		"view": "event_search",
		"pid": "20",
		"time_start": "1.00000s",
		"time_end": "1.08000 秒",
		"line_start": "1",
		"line_end": "4",
		"event_types": "sched_wakeup, sched_switch",
		"max_depth": "4",
		"max_branches": "2",
		"min_duration_ms": "1ms",
		"include_window_stats": "true",
		"limit": "2"
	}`)
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{
		"line_start=1",
		"line_end=4",
		"line=1 ts=1.000000 type=sched_wakeup",
		"line=2 ts=1.010000 type=sched_switch",
		"query_tolerance_seconds",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceSecondParsesMilliseconds(t *testing.T) {
	var holder struct {
		T TraceSecond `json:"t"`
	}
	if err := json.Unmarshal([]byte(`{"t":"1010ms"}`), &holder); err != nil {
		t.Fatal(err)
	}
	if math.Abs(holder.T.Seconds()-1.01) > 0.000000001 {
		t.Fatalf("expected milliseconds to normalize to seconds, got %.9f", holder.T.Seconds())
	}
	if holder.T.QueryToleranceSeconds() <= 0 || holder.T.QueryToleranceSeconds() > 0.0005 {
		t.Fatalf("unexpected tolerance: %.9f", holder.T.QueryToleranceSeconds())
	}
}

func TestFlexTraceQueryDurationParsesUnitsToMilliseconds(t *testing.T) {
	for raw, want := range map[string]float64{
		`"1ms"`:   1,
		`"0.5s"`:  500,
		`"250us"`: 0.25,
		`2`:       2,
	} {
		var got FlexFloat
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			t.Fatalf("unmarshal %s: %v", raw, err)
		}
		if math.Abs(got.Float64()-want) > 0.000000001 {
			t.Fatalf("%s: got %.9f want %.9f", raw, got.Float64(), want)
		}
	}
}
