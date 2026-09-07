package tracediag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// The old Result pin fingerprints only the nested pointer type. This local
// closed carrier census requires explicit diagnostic treatment of future new
// fields without re-signing unrelated, unchanged Result/bundle hashes.
func TestB1607DiagInventoryFieldDisposition(t *testing.T) {
	for _, tc := range []struct {
		typ  reflect.Type
		want []string
	}{
		{reflect.TypeOf(tracequery.TargetWindowSleepInventory{}), []string{
			"Thread|thread", "Window|window", "Scope|scope", "ScanStatus|scan_status", "OutputStatus|output_status",
			"Total|total", "Emitted|emitted", "TotalMs|total_ms", "SleepMs|sleep_ms", "DStateMs|d_state_ms", "IOWaitMs|io_wait_ms",
			"HeadState|head_state,omitempty", "StateClosureStatus|state_closure_status", "BinderAssociationStatus|binder_association_status",
			"CausalAttributionStatus|causal_attribution_status", "Occurrences|occurrences",
		}},
		{reflect.TypeOf(tracequery.TargetWindowSleepOccurrence{}), []string{"Ordinal|ordinal", "Interval|"}},
	} {
		var got []string
		for i := 0; i < tc.typ.NumField(); i++ {
			field := tc.typ.Field(i)
			if field.PkgPath == "" {
				got = append(got, field.Name+"|"+field.Tag.Get("json"))
			}
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s needs explicit zero/coordinate/bulk detail review: got=%q want=%q", tc.typ.Name(), got, tc.want)
		}
	}
}

func b1607DiagSleepFixture(t *testing.T, count int) (string, *tracequery.Index) {
	t.Helper()
	const base = 6793222.0
	var trace strings.Builder
	fmt.Fprintf(&trace, "idle-0 (0) [000] .... %.6f: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120\n", base)
	for i := 0; i < count; i++ {
		start := base + .001 + float64(i)*.001
		state := "S"
		if i%3 != 0 {
			state = "D"
		}
		fmt.Fprintf(&trace, "target-41 (41) [000] .... %.6f: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=%s ==> next_comm=waker next_pid=2 next_prio=120\n", start, state)
		if i%3 == 2 {
			fmt.Fprintf(&trace, "waker-2 (2) [000] .... %.6f: sched_blocked_reason: pid=41 iowait=1 caller=io_schedule\n", start+.00005)
		}
		fmt.Fprintf(&trace, "waker-2 (2) [000] .... %.6f: sched_wakeup: comm=target pid=41 prio=120 target_cpu=000\n", start+.0002)
		fmt.Fprintf(&trace, "waker-2 (2) [000] .... %.6f: sched_switch: prev_comm=waker prev_pid=2 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120\n", start+.0003)
	}
	fmt.Fprintf(&trace, "target-41 (41) [000] .... %.6f: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120\n", base+.045)
	path := filepath.Join(t.TempDir(), "sleep.ftrace")
	if err := os.WriteFile(path, []byte(trace.String()), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return path, idx
}

func b1607DiagSleepLines(body stepBody) string {
	var lines []string
	for _, line := range body.lines {
		if strings.Contains(line, ".sleep_inventory") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func TestB1607DiagActualNestedInventoryKeepsCoordinatesAndEngineCap(t *testing.T) {
	_, idx := b1607DiagSleepFixture(t, 39)
	for _, view := range []string{"window_stats", "wakeup_chain", "root_cause_rank", "frame_root_cause_bundle"} {
		t.Run(view, func(t *testing.T) {
			result := tracequery.Run(idx, tracequery.Query{View: view, PID: 41, TimeStart: 6793222, TimeEnd: 6793222.05})
			before, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			body := renderStepBody(&Step{View: view, effMaxLines: 10000}, stepOutcome{result: &result})
			report := b1607DiagSleepLines(body)
			for _, want := range []string{
				"scope=constructed_target_timeline", "scan_status=complete", "output_status=incomplete",
				"total=39", "emitted=32", "total_ms=7.800", "sleep_ms=2.600", "d_state_ms=2.600", "io_wait_ms=2.600",
				"state_closure_status=not_assessed", "binder_association_status=not_assessed", "causal_attribution_status=not_assessed",
				"ordinal=1", "start_ts=6793222.001000", "end_ts=6793222.001200", "duration_ms=0.200", "ordinal=32",
			} {
				if !strings.Contains(report, want) {
					t.Fatalf("actual nested inventory omitted %q:\n%s", want, report)
				}
			}
			if strings.Contains(report, "ordinal=33") || strings.Contains(report, "e+06") || strings.Contains(report, "root_cause_rank=") {
				t.Fatalf("diagnostics changed engine cap/coordinate form/authority:\n%s", report)
			}
			after, err := json.Marshal(result)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("diagnostic rendering mutated the engine result")
			}
		})
	}
}

func TestB1607DiagActualZeroSleepAndClippedDualLedger(t *testing.T) {
	_, idx := b1607DiagSleepFixture(t, 0)
	result := tracequery.Run(idx, tracequery.Query{View: "window_stats", PID: 41, TimeStart: 6793222, TimeEnd: 6793222.04})
	report := b1607DiagSleepLines(renderStepBody(&Step{View: result.View, effMaxLines: 1000}, stepOutcome{result: &result}))
	for _, want := range []string{"total=0", "emitted=0", "total_ms=0.000", "sleep_ms=0.000", "d_state_ms=0.000", "io_wait_ms=0.000", "output_status=complete"} {
		if !strings.Contains(report, want) {
			t.Fatalf("a measured zero census must remain explicit, not absent %q:\n%s", want, report)
		}
	}
	_, idx = b1607DiagSleepFixture(t, 1)
	result = tracequery.Run(idx, tracequery.Query{View: "wakeup_chain", PID: 41, TimeStart: 6793222.00105, TimeEnd: 6793222.00115})
	report = b1607DiagSleepLines(renderStepBody(&Step{View: result.View, effMaxLines: 1000}, stepOutcome{result: &result}))
	for _, want := range []string{"start_ts=6793222.001050", "end_ts=6793222.001150", "actual_start_ts=6793222.001000", "actual_end_ts=6793222.001200", "duration_ms=0.100"} {
		if !strings.Contains(report, want) {
			t.Fatalf("clamped and original boundaries lost %q:\n%s", want, report)
		}
	}
}

func TestB1607DiagActualReportCapDoesNotChangeInventoryEnumeration(t *testing.T) {
	path, idx := b1607DiagSleepFixture(t, 39)
	result := tracequery.Run(idx, tracequery.Query{View: "window_stats", PID: 41, TimeStart: 6793222, TimeEnd: 6793222.05})
	full := renderStepBody(&Step{View: result.View, effMaxLines: 10000}, stepOutcome{result: &result})
	small := renderStepBody(&Step{View: result.View, effMaxLines: 12}, stepOutcome{result: &result})
	if len(small.lines) != 12 || small.total != full.total || small.total <= len(small.lines) {
		t.Fatalf("report line accounting changed enumeration: small=%+v full_total=%d", small, full.total)
	}
	if result.TargetWindowStates == nil || result.TargetWindowStates.SleepInventory.Total != 39 || result.TargetWindowStates.SleepInventory.Emitted != 32 {
		t.Fatal("presentation cap changed engine census or engine return cap")
	}
	scriptPath := filepath.Join(t.TempDir(), "sleep.yaml")
	script := "version: 1\ndefaults: { window: \"6793222.000000..6793222.050000\" }\nsteps:\n  - label: sleep\n    view: window_stats\n    pid: 41\n    max_lines: 12\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	var report bytes.Buffer
	failed, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: path, Now: fixedNow}, &report)
	if err != nil || failed != 0 {
		t.Fatalf("actual diagnostic report failed: failed=%d err=%v\n%s", failed, err, report.String())
	}
	if !strings.Contains(report.String(), "按帽截断至 12") || !strings.Contains(report.String(), "[步骤状态摘要]") || !strings.Contains(report.String(), "输出行=12/") {
		t.Fatalf("report cap must disclose its own distinct output loss:\n%s", report.String())
	}
}
