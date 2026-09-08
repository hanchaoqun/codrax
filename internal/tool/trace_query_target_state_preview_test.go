package tool

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1625TargetOutsideTopNTrace(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("# tracer: nop\n")
	for cpu := 0; cpu < 10; cpu++ {
		fmt.Fprintf(&b, " idle-0 (0) [%03d] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=busy%d next_pid=%d next_prio=120\n", cpu+20, cpu, 200+cpu)
	}
	b.WriteString(" idle-0 (0) [012] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=120\n")
	b.WriteString(" target-42 (42) [012] .... 1.001000: sched_switch: prev_comm=target prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n")
	b.WriteString(" worker-88 (88) [004] .... 1.010000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=004\n")
	b.WriteString(" idle-0 (0) [004] .... 1.011000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=120\n")
	b.WriteString(" target-42 (42) [004] .... 1.013000: sched_switch: prev_comm=target prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n")
	for cpu := 0; cpu < 10; cpu++ {
		fmt.Fprintf(&b, " busy%d-%d (%d) [%03d] .... 1.020000: sched_switch: prev_comm=busy%d prev_pid=%d prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n", cpu, 200+cpu, 200+cpu, cpu+20, cpu, 200+cpu)
	}
	path := filepath.Join(t.TempDir(), "customer.systrace")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func b1625Execute(t *testing.T, capture, view string, pid int, start, end float64) (types.ToolResult, tracequery.Result, string) {
	t.Helper()
	work := filepath.Join(t.TempDir(), ".codrax", "blob", "session")
	ctx := &types.BusContext{RepoRoot: filepath.Dir(capture), WorkDir: work, Mutable: types.NewMutableState("target account")}
	p, _ := json.Marshal(map[string]any{"source": "path", "path": capture, "view": view, "pid": pid, "time_start": start, "time_end": end})
	result, err := (&TraceQuery{}).Execute(ctx, p)
	if err != nil || !result.Success {
		t.Fatalf("actual trace query failed: %v %+v", err, result)
	}
	// Each actual execution owns an otherwise empty work directory. Read its
	// complete JSON rather than reconstructing target values from text/TopN.
	files, err := filepath.Glob(filepath.Join(work, "trace-query-result-*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("expected one actual JSON payload: %v %v", files, err)
	}
	payload, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var decoded tracequery.Result
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	full := result.Summary
	// Small summaries keep their full text inline and use the JSON payload as
	// RawRef. Only an independently offloaded summary is a text continuation.
	if result.RawRef != "" && result.RawRef != files[0] {
		raw, err := os.ReadFile(result.RawRef)
		if err != nil {
			t.Fatal(err)
		}
		full = string(raw)
	}
	return result, decoded, full
}

func b1625StateLine(t *testing.T, text string) string {
	t.Helper()
	var found []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "- target_window_states ") {
			found = append(found, line)
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected one target state account, got %d; text head:\n%.5000s", len(found), text)
	}
	return found[0]
}

func TestB1625ActualWindowStatsPublishesTargetOutsideGlobalTopN(t *testing.T) {
	capture := b1625TargetOutsideTopNTrace(t)
	result, wire, text := b1625Execute(t, capture, "window_stats", 42, 1, 1.020)
	account := wire.TargetWindowStates
	if account == nil || wire.WindowStats == nil || len(wire.WindowStats.TopRunning) != 8 {
		t.Fatalf("fixture did not construct account and bounded global top8: %+v", wire)
	}
	for _, row := range wire.WindowStats.TopRunning {
		if row.Thread.PID == 42 {
			t.Fatal("target must be below global TopN in this regression")
		}
	}
	if math.Abs(account.RunningMs-3) > 1e-6 || math.Abs(account.RunnableMs-1) > 1e-6 || math.Abs(account.SleepMs-16) > 1e-6 {
		t.Fatalf("unexpected actual scheduler partition: %+v", account)
	}
	line := b1625StateLine(t, text)
	var state *types.ObservationRecord
	for i := range result.Observations {
		if result.Observations[i].Predicate == "target_window_states" {
			state = &result.Observations[i]
			break
		}
	}
	if state == nil || line != "- "+state.Summary+fmt.Sprintf(" lines=%d-%d", account.LineStart, account.LineEnd) {
		t.Fatalf("text not the existing typed account: line=%q record=%+v", line, state)
	}
	if strings.Index(text, line) > strings.Index(text, "target_d_io_wait_occurrence_roster") {
		t.Fatal("target account must precede long wait/rank detail")
	}
	if !strings.Contains(result.Summary, line) {
		t.Fatal("ordinary tool head preview lost target account")
	}
	for _, want := range []string{"target_cpu_running_roster target-42 status=complete assignment=complete emitted=2 total=2 known=3.000ms unknown=0.000ms overflow=0.000ms", "target_cpu_running cpu=4 running=2.000ms", "target_cpu_running cpu=12 running=1.000ms"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing exact uncapped target CPU fact %q", want)
		}
	}
	if strings.Index(text, "target_cpu_running cpu=4 ") > strings.Index(text, "target_cpu_running cpu=12 ") {
		t.Fatal("target CPU order changed")
	}
}

func TestB1625ActualWindowsZeroComponentsAndBundleSingleAccount(t *testing.T) {
	capture := b1625TargetOutsideTopNTrace(t)
	for _, tc := range []struct {
		name, view                 string
		start, end, running, sleep float64
	}{
		{"early", "window_stats", 1, 1.005, 1, 4},
		{"late", "window_stats", 1.009, 1.020, 2, 8},
		{"zero-running", "window_stats", 1.002, 1.009, 0, 7},
		{"bundle", "frame_root_cause_bundle", 1, 1.020, 3, 16},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, wire, text := b1625Execute(t, capture, tc.view, 42, tc.start, tc.end)
			account := traceQueryTargetWindowStatesAccount(wire)
			if account == nil || math.Abs(account.RunningMs-tc.running) > 1e-6 || math.Abs(account.SleepMs-tc.sleep) > 1e-6 {
				t.Fatalf("wrong actual account: %+v", account)
			}
			line := b1625StateLine(t, text)
			for _, want := range []string{fmt.Sprintf("running=%.3fms", tc.running), fmt.Sprintf("sleep=%.3fms", tc.sleep), fmt.Sprintf("window=%.6f..%.6f", tc.start, tc.end), "d_state=0.000ms io_wait=0.000ms", "sleep_mechanism=unproven"} {
				if !strings.Contains(line, want) {
					t.Errorf("missing same-window field %q: %s", want, line)
				}
			}
			if tc.running == 0 && strings.Contains(text, "target_cpu_running_roster") {
				t.Fatal("measured zero running fabricated CPU assignment")
			}
			if tc.view == "frame_root_cause_bundle" && strings.Index(text, line) < strings.Index(text, "## Frame root cause bundle") {
				t.Fatal("bundle account moved from existing bundle location")
			}
		})
	}
}

func TestB1625ActualUnavailableAndLifecycleDoNotBorrowGlobalAccount(t *testing.T) {
	for _, lifecycle := range []bool{false, true} {
		t.Run(fmt.Sprintf("lifecycle=%t", lifecycle), func(t *testing.T) {
			capture := b1625TargetOutsideTopNTrace(t)
			pid := 999
			if lifecycle {
				body, err := os.ReadFile(capture)
				if err != nil {
					t.Fatal(err)
				}
				body = []byte(strings.Replace(string(body), "sched_wakeup: comm=target", "sched_wakeup_new: comm=target", 1))
				if err := os.WriteFile(capture, body, 0o644); err != nil {
					t.Fatal(err)
				}
				pid = 42
			}
			result, wire, text := b1625Execute(t, capture, "window_stats", pid, 1, 1.020)
			if wire.TargetWindowStates != nil {
				t.Fatalf("unavailable target manufactured account: %+v", wire.TargetWindowStates)
			}
			if strings.Contains(text, "- target_window_states ") || strings.Contains(text, "- target_cpu_running_roster ") {
				t.Fatal("missing target borrowed global/process rows or fabricated zeros")
			}
			if lifecycle && (len(wire.LifecycleSuppressions) == 0 || !strings.Contains(text, "lifecycle_suppression conflict_tid=42")) {
				t.Fatal("actual lifecycle withdrawal no longer disclosed")
			}
			for _, row := range result.Observations {
				if row.Predicate == "target_window_states" || row.Predicate == "target_cpu_running" {
					t.Fatalf("withdrawn target has typed authority: %+v", row)
				}
			}
		})
	}
}

func TestB1625SharedStateTextKeepsUnknownCPUAndTypedPayload(t *testing.T) {
	_, wire, _ := b1625Execute(t, b1625TargetOutsideTopNTrace(t), "window_stats", 42, 1, 1.020)
	if wire.WindowStats == nil || wire.WindowStats.ProcessDomainCensus == nil || len(wire.WindowStats.TopRunning) == 0 {
		t.Fatal("fixture must retain actual process census and global CPU rows")
	}
	// Existing global/process/census rows remain present. Their known CPU
	// assignments must not fill this independently unknown target dimension.
	account := *wire.TargetWindowStates
	account.RunningByCPU = nil
	account.RunningCPUKnownMs = 0
	account.RunningCPUUnknownMs = account.RunningMs
	account.RunningCPURosterTotal = 0
	account.RunningCPURosterEmitted = 0
	account.RunningCPUAssignmentStatus = "unavailable"
	wire.TargetWindowStates = &account
	for _, partial := range []bool{false, true} {
		if partial {
			account.RunningByCPU = []tracequery.TargetWindowCPURunning{{CPU: 0, RunningMs: 1, SegmentCount: 1, StartTs: 1, EndTs: 1.001, LineStart: 12, LineEnd: 13}}
			account.RunningCPUKnownMs = 1
			account.RunningCPUUnknownMs = 2
			account.RunningCPURosterTotal = 1
			account.RunningCPURosterEmitted = 1
			account.RunningCPUAssignmentStatus = "partial"
		}
		before, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		at := time.Unix(1, 0).UTC()
		observations := traceQueryTypedObservations(wire, "path", "payload", "raw", "scope", at)
		text := traceQuerySummary(wire, traceQueryParams{View: "window_stats"}, "path", "payload")
		b1625StateLine(t, text)
		want := "assignment=unavailable emitted=0 total=0 known=0.000ms unknown=3.000ms"
		if partial {
			want = "assignment=partial emitted=1 total=1 known=1.000ms unknown=2.000ms"
		}
		if !strings.Contains(text, want) {
			t.Errorf("unknown CPU amount lost: %q", want)
		}
		if got := strings.Count(text, "  target_cpu_running cpu="); got != account.RunningCPURosterEmitted {
			t.Errorf("guessed CPU rows: got %d want %d", got, account.RunningCPURosterEmitted)
		}
		if partial && !strings.Contains(text, "target_cpu_running cpu=0 running=1.000ms") {
			t.Fatal("known CPU zero mistaken for unknown")
		}
		after, _ := json.Marshal(wire)
		if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(observations, traceQueryTypedObservations(wire, "path", "payload", "raw", "scope", at)) {
			t.Fatal("text renderer mutated typed evidence, engine fields or provenance")
		}
		if again := traceQuerySummary(wire, traceQueryParams{View: "window_stats"}, "path", "payload"); again != text {
			t.Fatal("text rendering not idempotent")
		}
		// Both routes must publish precisely the same account bytes. The
		// bundle keeps its own location; the generic route gains a head copy.
		var bundle strings.Builder
		writeTraceFrameRootCauseBundleSummary(&bundle, &tracequery.FrameRootCauseBundle{Target: account.Thread, Window: account.Window, TargetWindowStates: &account})
		if b1625StateLine(t, bundle.String()) != b1625StateLine(t, text) {
			t.Fatal("generic/bundle state formatter drift")
		}
	}
	for _, missing := range []*tracequery.TargetWindowStateAccount{nil, {}, {TotalMs: math.NaN()}} {
		var b strings.Builder
		writeTraceTargetWindowStateAccount(&b, missing)
		if b.Len() != 0 {
			t.Fatal("old positive-total admission changed")
		}
	}
	wire.TargetWindowStates = nil
	withoutTarget := traceQuerySummary(wire, traceQueryParams{View: "window_stats"}, "path", "payload")
	if !strings.Contains(withoutTarget, "process_domain_census") || strings.Contains(withoutTarget, "- target_window_states ") || strings.Contains(withoutTarget, "- target_cpu_running_roster ") {
		t.Fatal("process census filled a missing target account or was discarded")
	}
	// A bundle can expose both copies in JSON; only its authoritative copy is
	// rendered once. No query-window or subject is elected by the new writer.
	wire.FrameRootCauseBundle = &tracequery.FrameRootCauseBundle{Target: account.Thread, Window: account.Window, TargetWindowStates: &account}
	wire.TargetWindowStates = &tracequery.TargetWindowStateAccount{Thread: tracequery.ThreadRef{PID: 999, Comm: "not-selected"}, TotalMs: 9, RunningMs: 9}
	text := traceQuerySummary(wire, traceQueryParams{View: "window_stats"}, "path", "payload")
	if line := b1625StateLine(t, text); strings.Contains(line, "not-selected") {
		t.Fatal("existing bundle account selection changed")
	}
}
