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
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC300WakeupMeasuredWaitSchemaEvolution(t *testing.T) {
	_, schema := detailSchemaFingerprint(reflect.TypeOf(tracequery.WakeupEdge{}))
	const added = "MeasuredWait|*types.TraceWakeupMeasuredWait|measured_wait,omitempty"
	var previous []string
	count := 0
	for _, field := range strings.Split(schema, ";") {
		if field == added {
			count++
			continue
		}
		previous = append(previous, field)
	}
	sum := sha256.Sum256([]byte(strings.Join(previous, ";")))
	if count != 1 || hex.EncodeToString(sum[:]) != "b94e0887e581b5d0bd3bd5bde3d7d29ac3cc92fbf07dab6601a71246c25daca8" {
		t.Fatalf("optional display addition changed unrelated wakeup fields: %s", schema)
	}
	typ := reflect.TypeOf(types.TraceWakeupMeasuredWait{})
	want := strings.Join([]string{
		"StartTs|float64|start_ts", "EndTs|float64|end_ts", "SegmentState|string|segment_state",
		"SegmentEndTs|float64|segment_end_ts", "StartLine|int|start_line", "ActualStartTs|*float64|actual_start_ts,omitempty",
	}, ";")
	gotHash, gotSchema := detailSchemaFingerprint(typ)
	if gotSchema != want || nonEventPrioritySchemaPins[typ] != gotHash {
		t.Fatalf("nested operands need independent exact schema and renderer disposition: schema=%s pin=%q actual=%s", gotSchema, nonEventPrioritySchemaPins[typ], gotHash)
	}
}

func TestHMC300WakeupMeasuredWaitPublicRunZeroOrigin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "zero.systrace")
	const raw = `# tracer: nop
app-100 (100) [001] .... 0.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
worker-200 (100) [002] .... 0.020000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
app-100 (100) [001] .... 0.021000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
app-100 (100) [001] .... 0.030000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=R ==> next_comm=idle/1 next_pid=0 next_prio=120
`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	native := tracequery.Run(idx, tracequery.Query{View: "wakeup_chain", PID: 100, TimeStart: 0, TimeEnd: .03})
	if native.WakeupChain == nil || len(native.WakeupChain.Edges) != 1 || native.WakeupChain.Edges[0].MeasuredWait == nil {
		t.Fatalf("fixture must provide a real native measured wait: %+v", native.WakeupChain)
	}
	wait := native.WakeupChain.Edges[0].MeasuredWait
	if wait.StartTs != 0 || wait.EndTs != .02 {
		t.Fatalf("fixture zero interval changed: %+v", wait)
	}
	script := filepath.Join(dir, "collect.yaml")
	if err := os.WriteFile(script, []byte("version: 1\nsteps:\n  - label: zero wait\n    view: wakeup_chain\n    pid: 100\n    window: 0..0.03\n    max_lines: 1000\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	failed, err := Run(context.Background(), Options{TracePath: path, ScriptPath: script}, &output)
	if err != nil || failed != 0 {
		t.Fatalf("public Run failed=%d err=%v\n%s", failed, err, output.String())
	}
	want := types.TraceWakeupMeasuredWaitDisplay(wait)
	if !strings.Contains(want, "measured_wait=[0.000000000,0.020000000)s") {
		t.Fatalf("shared display lost the independent native zero/half-open oracle: %q", want)
	}
	if !strings.Contains(output.String(), want) {
		t.Fatalf("public native renderer lost required zero or shared interval semantics; want %q\n%s", want, output.String())
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, []byte(raw)) {
		t.Fatal("read-only diagnostic changed original trace")
	}
}

func TestHMC300WakeupMeasuredWaitDetailBoundaries(t *testing.T) {
	zero := 0.0
	for _, tc := range []struct {
		name string
		wait *types.TraceWakeupMeasuredWait
	}{
		{"nil", nil},
		{"zero_known", &types.TraceWakeupMeasuredWait{StartTs: 0, EndTs: .02, SegmentEndTs: .02, SegmentState: "s_sleep", StartLine: 2, ActualStartTs: &zero}},
		{"zero_unknown", &types.TraceWakeupMeasuredWait{StartTs: 0, EndTs: .02, SegmentEndTs: .02, SegmentState: "s_sleep", StartLine: 2}},
		{"clipped", &types.TraceWakeupMeasuredWait{StartTs: .01, EndTs: .02, SegmentEndTs: .02, SegmentState: "d_sleep", StartLine: 2, ActualStartTs: &zero}},
		{"tolerance_tail", &types.TraceWakeupMeasuredWait{StartTs: .01, EndTs: .020001, SegmentEndTs: .02, SegmentState: "io_wait", StartLine: 2}},
		{"invalid_state", &types.TraceWakeupMeasuredWait{StartTs: .01, EndTs: .02, SegmentEndTs: .02, SegmentState: "running", StartLine: 2}},
		{"missing_boundary", &types.TraceWakeupMeasuredWait{StartTs: .01, EndTs: .02, SegmentEndTs: .02, SegmentState: "s_sleep"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := tracequery.Result{View: "wakeup_chain", WakeupChain: &tracequery.ChainResult{Edges: []tracequery.WakeupEdge{{WakeupTs: .02, LatencyMs: 20, LatencyCaliber: "sleep_start_to_sched_wakeup", MeasuredWait: tc.wait}}}}
			before, _ := json.Marshal(result)
			body := renderStepBody(&Step{View: result.View, effMaxLines: 1000}, stepOutcome{result: &result})
			report := strings.Join(body.lines, "\n")
			want := types.TraceWakeupMeasuredWaitDisplay(tc.wait)
			switch tc.name {
			case "zero_known":
				if !strings.Contains(want, "actual_segment_start=0.000000000 start_clipped=false") {
					t.Fatal("known actual zero became unknown or clipped")
				}
			case "zero_unknown":
				if !strings.Contains(want, "actual_segment_start=unknown start_clipped=unknown") {
					t.Fatal("unknown actual start borrowed selected zero")
				}
			case "clipped":
				if !strings.Contains(want, "measured_wait=[0.010000000,0.020000000)s") || !strings.Contains(want, "actual_segment_start=0.000000000 start_clipped=true") {
					t.Fatal("selected and actual start became interchangeable")
				}
			case "tolerance_tail":
				if !strings.Contains(want, "measured_wait=[0.010000000,0.020001000)s") || !strings.Contains(want, "selected_state_interval=[0.010000000,0.020000000)s wakeup_matches_selected_state_end=false") {
					t.Fatal("tolerance tail acquired scheduler-state coverage")
				}
			}
			if want == "" {
				if strings.Contains(report, ".measured_wait:") {
					t.Fatalf("nil/invalid optional display must not fabricate interval: %s", report)
				}
			} else if strings.Count(report, want) != 1 || strings.Count(report, ".measured_wait:") != 1 {
				t.Fatalf("shared interval must appear exactly once, want=%q\n%s", want, report)
			}
			if !strings.Contains(report, "latency_ms=20.000") || !strings.Contains(report, "latency_caliber=sleep_start_to_sched_wakeup") {
				t.Fatalf("optional display changed existing event/quantity: %s", report)
			}
			capped := renderStepBody(&Step{View: result.View, effMaxLines: 1}, stepOutcome{result: &result})
			if len(capped.lines) != 1 || capped.total != body.total || capped.total <= len(capped.lines) {
				t.Fatalf("display escaped existing line cap/accounting: full=%+v capped=%+v", body, capped)
			}
			after, _ := json.Marshal(result)
			if !bytes.Equal(before, after) {
				t.Fatal("renderer changed native payload or optional validity")
			}
		})
	}
}
