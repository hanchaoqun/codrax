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

// The Result schema pin sees the enclosing account's pointer type, not its
// nested fields. Review every new field's scalar/coordinate/bulk disposition
// here instead of re-signing an unchanged Result or adding a root-rank lane.
func TestB1607BDiagBinderInventoryFieldDisposition(t *testing.T) {
	for _, tc := range []struct {
		typ  reflect.Type
		want []string
	}{
		{reflect.TypeOf(tracequery.TargetWindowBinderWaitInventory{}), []string{
			"Thread|tracequery.ThreadRef|thread", "Window|tracequery.TimeWindow|window", "Scope|string|scope",
			"ScanStatus|string|scan_status", "OutputStatus|string|output_status", "TargetSleepCount|int|target_sleep_count",
			"ConfirmedCount|int|confirmed_count", "ConfirmedMs|float64|confirmed_ms",
			"UnresolvedCandidateCount|int|unresolved_candidate_count", "RemainingUnassociatedCount|int|remaining_unassociated_count",
			"Emitted|int|emitted", "HeadState|*tracequery.TimelineHeadState|head_state,omitempty",
			"CausalAttributionStatus|string|causal_attribution_status", "UnresolvedReasons|[]string|unresolved_reasons,omitempty",
			"Occurrences|[]tracequery.TargetWindowBinderWaitOccurrence|occurrences",
		}},
		{reflect.TypeOf(tracequery.TargetWindowBinderWaitOccurrence{}), []string{
			"Ordinal|int|ordinal", "Interval|tracequery.Interval|", "Peer|tracequery.ThreadRef|peer",
			"ClosureStatus|string|closure_status", "RequestTransactionID|int|request_transaction_id",
			"ReplyTransactionID|int|reply_transaction_id", "RequestSendLine|int|request_send_line",
			"RequestReceiveLine|int|request_receive_line", "ReplySendLine|int|reply_send_line",
			"ReplyReceiveLine|int|reply_receive_line", "ClosureLine|int|closure_line",
			"RequestSendTs|float64|request_send_ts", "RequestReceiveTs|float64|request_receive_ts",
			"ReplySendTs|float64|reply_send_ts", "ReplyReceiveTs|float64|reply_receive_ts", "ClosureTs|float64|closure_ts",
		}},
	} {
		var got []string
		for i := 0; i < tc.typ.NumField(); i++ {
			field := tc.typ.Field(i)
			if field.PkgPath == "" {
				got = append(got, field.Name+"|"+field.Type.String()+"|"+field.Tag.Get("json"))
			}
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s needs explicit scalar/coordinate/bulk detail review: got=%q want=%q", tc.typ.Name(), got, tc.want)
		}
	}
}

func b1607BDiagBinderLines(body stepBody) string {
	var lines []string
	for _, line := range body.lines {
		if strings.Contains(line, ".binder_wait_inventory") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func TestB1607BDiagBinderInventoryRetainsZeroAndUnresolvedBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name       string
		inventory  tracequery.TargetWindowBinderWaitInventory
		additional []string
	}{
		{"known zero", tracequery.TargetWindowBinderWaitInventory{ScanStatus: "complete", OutputStatus: "complete"}, nil},
		{"unresolved is not absent", tracequery.TargetWindowBinderWaitInventory{
			ScanStatus: "incomplete", OutputStatus: "complete", TargetSleepCount: 2,
			UnresolvedCandidateCount: 1, RemainingUnassociatedCount: 1,
			UnresolvedReasons: []string{"unknown_source_identity"},
			HeadState:         &tracequery.TimelineHeadState{Status: "unknown"},
		}, []string{"target_sleep_count=2", "unresolved_candidate_count=1", "remaining_unassociated_count=1", "unknown_source_identity", "head_state: status=unknown"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.inventory.Thread = tracequery.ThreadRef{PID: 41, TGID: 41, Comm: "target"}
			tc.inventory.Window = tracequery.TimeWindow{StartTs: 6793222, EndTs: 6793222.05}
			tc.inventory.Scope = "indexed_target_verified_closed_waits"
			tc.inventory.CausalAttributionStatus = "not_assessed"
			result := tracequery.Result{View: "window_stats", TargetWindowStates: &tracequery.TargetWindowStateAccount{BinderWaitInventory: &tc.inventory}}
			before, _ := json.Marshal(result)
			report := b1607BDiagBinderLines(renderStepBody(&Step{View: result.View, effMaxLines: 1000}, stepOutcome{result: &result}))
			for _, want := range append([]string{
				"scope=indexed_target_verified_closed_waits", "scan_status=" + tc.inventory.ScanStatus,
				"output_status=" + tc.inventory.OutputStatus, "confirmed_count=0", "confirmed_ms=0.000", "emitted=0",
				"causal_attribution_status=not_assessed",
			}, tc.additional...) {
				if !strings.Contains(report, want) {
					t.Fatalf("inventory lost explicit status/count %q:\n%s", want, report)
				}
			}
			if tc.name == "known zero" {
				for _, want := range []string{"target_sleep_count=0", "unresolved_candidate_count=0", "remaining_unassociated_count=0"} {
					if !strings.Contains(report, want) {
						t.Fatalf("complete empty census must preserve %q:\n%s", want, report)
					}
				}
			}
			after, _ := json.Marshal(result)
			if !bytes.Equal(before, after) || strings.Contains(report, "root_cause_rank=") || strings.Contains(report, "key_first") {
				t.Fatal("inventory display mutated data or acquired root/priority authority")
			}
		})
	}
}

func TestB1607BDiagBinderOccurrenceKeepsFullIntervalWithFourEndpoints(t *testing.T) {
	row := tracequery.TargetWindowBinderWaitOccurrence{
		Ordinal: 1,
		Interval: tracequery.Interval{
			StartTs: 6793222.001050, EndTs: 6793222.001150, DurationMs: .100,
			ActualStartTs: 6793222.001, ActualEndTs: 6793222.0012,
			StartLine: 3, EndLine: 6, State: tracequery.StateSSleep,
			Summary: "unchanged original scheduler summary",
		},
		Peer: tracequery.ThreadRef{PID: 2, TGID: 2, Comm: "peer"}, ClosureStatus: "verified_reply_wakeup",
		RequestTransactionID: 100, ReplyTransactionID: 101,
		RequestSendLine: 1, RequestReceiveLine: 4, ReplySendLine: 5, ReplyReceiveLine: 8, ClosureLine: 6,
		RequestSendTs: 6793222.0009, RequestReceiveTs: 6793222.00105,
		ReplySendTs: 6793222.00119, ReplyReceiveTs: 6793222.00125, ClosureTs: 6793222.0012,
	}
	var lines []string
	walkDetail(reflect.ValueOf(tracequery.TargetWindowBinderWaitInventory{Occurrences: []tracequery.TargetWindowBinderWaitOccurrence{row}}), "target_window_states.binder_wait_inventory", func(line string) { lines = append(lines, line) }, 0)
	var occurrenceLines []string
	for _, line := range lines {
		if strings.Contains(line, ".occurrences[0]") {
			occurrenceLines = append(occurrenceLines, line)
		}
	}
	if len(occurrenceLines) != 1 {
		t.Fatalf("one verified wait must retain its full interval and closure on one detail line: %q", occurrenceLines)
	}
	for _, want := range []string{
		"ordinal=1", "start_ts=6793222.001050", "end_ts=6793222.001150", "duration_ms=0.100",
		"actual_start_ts=6793222.001000", "actual_end_ts=6793222.001200", "start_line=3", "end_line=6",
		"summary=unchanged original scheduler summary", "closure_status=verified_reply_wakeup",
		"request_transaction_id=100", "reply_transaction_id=101", "request_send_line=1", "request_receive_line=4",
		"reply_send_line=5", "reply_receive_line=8", "closure_line=6", "request_send_ts=6793222.000900",
		"request_receive_ts=6793222.001050", "reply_send_ts=6793222.001190", "reply_receive_ts=6793222.001250", "closure_ts=6793222.001200",
	} {
		if !strings.Contains(occurrenceLines[0], want) {
			t.Errorf("confirmed occurrence lost %q:\n%s", want, occurrenceLines[0])
		}
	}
	if strings.Contains(occurrenceLines[0], "e+06") {
		t.Fatal("large physical trace coordinates became scientific notation")
	}
}

func TestB1607BDiagBinderRequiredZeroCoordinatesStayExplicit(t *testing.T) {
	row := tracequery.TargetWindowBinderWaitOccurrence{
		Interval:      tracequery.Interval{State: tracequery.StateSSleep, StartTs: 0, EndTs: .001, DurationMs: 1},
		RequestSendTs: 0, RequestSendLine: 1,
	}
	tokens := binderInventoryScalarTokens(reflect.ValueOf(row)) + " " + binderInventoryScalarTokens(reflect.ValueOf(row.Interval))
	if !strings.Contains(tokens, "request_send_ts=0.000000") || !strings.Contains(tokens, "start_ts=0.000000") {
		t.Fatalf("the serialized origin must not disappear: %s", tokens)
	}
	if strings.Contains(tokens, "actual_start_ts=") || strings.Contains(tokens, "cpu=0") {
		t.Fatalf("optional absent/unknown coordinates must not be manufactured: %s", tokens)
	}
}

func b1607BDiagBinderFixture(t *testing.T, count int, omitReplyReceipt bool) (string, *tracequery.Index) {
	t.Helper()
	const base = 6793222.0
	var trace strings.Builder
	fmt.Fprintf(&trace, "idle-0 (0) [000] .... %.6f: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120\n", base)
	for i := 0; i < count; i++ {
		start := base + .001 + float64(i)*.002
		requestID := 10000 + 2*i
		fmt.Fprintf(&trace, "target-41 (41) [000] .... %.6f: binder_transaction: transaction=%d dest_node=1 dest_proc=2 dest_thread=2 reply=0 flags=0x10 code=0x19\n", start-.00005, requestID)
		fmt.Fprintf(&trace, "target-41 (41) [000] .... %.6f: sched_wakeup: comm=peer pid=2 prio=120 target_cpu=000\n", start-.000025)
		fmt.Fprintf(&trace, "peer-2 (2) [000] .... %.6f: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=peer next_pid=2 next_prio=120\n", start)
		fmt.Fprintf(&trace, "peer-2 (2) [000] .... %.6f: binder_transaction_received: transaction=%d\n", start+.00002, requestID)
		fmt.Fprintf(&trace, "peer-2 (2) [000] .... %.6f: binder_transaction: transaction=%d dest_node=0 dest_proc=41 dest_thread=41 reply=1 flags=0x0 code=0x0\n", start+.00019, requestID+1)
		fmt.Fprintf(&trace, "peer-2 (2) [000] .... %.6f: sched_wakeup: comm=target pid=41 prio=120 target_cpu=000\n", start+.0002)
		fmt.Fprintf(&trace, "peer-2 (2) [000] .... %.6f: sched_switch: prev_comm=peer prev_pid=2 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120\n", start+.00022)
		if !omitReplyReceipt {
			fmt.Fprintf(&trace, "target-41 (41) [000] .... %.6f: binder_transaction_received: transaction=%d\n", start+.00025, requestID+1)
		}
	}
	fmt.Fprintf(&trace, "target-41 (41) [000] .... %.6f: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120\n", base+.09)
	path := filepath.Join(t.TempDir(), "binder.ftrace")
	if err := os.WriteFile(path, []byte(trace.String()), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return path, idx
}

func b1607BDiagBinderAccount(t *testing.T, result tracequery.Result) *tracequery.TargetWindowBinderWaitInventory {
	t.Helper()
	account := result.TargetWindowStates
	if result.FrameRootCauseBundle != nil {
		account = result.FrameRootCauseBundle.TargetWindowStates
	}
	if account == nil || account.BinderWaitInventory == nil {
		t.Fatal("actual Run did not publish the independent Binder inventory")
	}
	return account.BinderWaitInventory
}

func TestB1607BDiagActualFourViewsKeepIndependentCensusAndReturnCap(t *testing.T) {
	_, idx := b1607BDiagBinderFixture(t, 39, false)
	for _, view := range []string{"window_stats", "wakeup_chain", "root_cause_rank", "frame_root_cause_bundle"} {
		t.Run(view, func(t *testing.T) {
			result := tracequery.Run(idx, tracequery.Query{View: view, PID: 41, TimeStart: 6793222, TimeEnd: 6793222.09, MinDurationMs: 100, MaxBranches: 1, MaxDepth: 1, MaxChainNodes: 1, Limit: 1})
			inventory := b1607BDiagBinderAccount(t, result)
			if inventory.ConfirmedCount != 39 || inventory.Emitted != 32 || len(inventory.Occurrences) != 32 {
				t.Fatalf("fixture must exercise complete scan versus engine return cap: %+v", inventory)
			}
			before, _ := json.Marshal(result)
			report := b1607BDiagBinderLines(renderStepBody(&Step{View: view, effMaxLines: 10000}, stepOutcome{result: &result}))
			for _, want := range []string{
				"scan_status=complete", "output_status=incomplete", "target_sleep_count=39", "confirmed_count=39",
				"confirmed_ms=7.800", "unresolved_candidate_count=0", "remaining_unassociated_count=0", "emitted=32",
				"causal_attribution_status=not_assessed", "ordinal=1 ", "ordinal=32 ",
				"closure_status=verified_reply_wakeup",
				"start_ts=6793222.001000", "end_ts=6793222.001200", "duration_ms=0.200",
				"request_send_ts=6793222.000950", "request_receive_ts=6793222.001020",
				"reply_send_ts=6793222.001190", "reply_receive_ts=6793222.001250", "closure_ts=6793222.001200",
			} {
				if !strings.Contains(report, want) {
					t.Fatalf("actual nested inventory lost %q:\n%s", want, report)
				}
			}
			if strings.Count(report, ".occurrences[") != 32 || strings.Contains(report, "ordinal=33 ") ||
				strings.Contains(report, "e+06") || strings.Contains(report, "root_cause_rank=") {
				t.Fatalf("diagnostics changed row count, coordinates or authority:\n%s", report)
			}
			after, _ := json.Marshal(result)
			if !bytes.Equal(before, after) {
				t.Fatal("diagnostic rendering mutated actual engine data")
			}
		})
	}
}

func TestB1607BDiagActualZeroUnresolvedAndClippedCoordinates(t *testing.T) {
	for _, tc := range []struct {
		name       string
		count      int
		omitReply  bool
		start, end float64
		want       []string
	}{
		{"zero", 0, false, 6793222, 6793222.09, []string{"target_sleep_count=0", "confirmed_count=0", "confirmed_ms=0.000", "emitted=0"}},
		{"missing receipt", 1, true, 6793222, 6793222.09, []string{"target_sleep_count=1", "confirmed_count=0", "unresolved_candidate_count=1", "emitted=0"}},
		{"clipped", 1, false, 6793222.00105, 6793222.00115, []string{
			"confirmed_count=1", "confirmed_ms=0.100", "start_ts=6793222.001050", "end_ts=6793222.001150",
			"actual_start_ts=6793222.001000", "actual_end_ts=6793222.001200", "closure_ts=6793222.001200",
			"reply_receive_ts=6793222.001250",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, idx := b1607BDiagBinderFixture(t, tc.count, tc.omitReply)
			result := tracequery.Run(idx, tracequery.Query{View: "window_stats", PID: 41, TimeStart: tc.start, TimeEnd: tc.end})
			inventory := b1607BDiagBinderAccount(t, result)
			report := b1607BDiagBinderLines(renderStepBody(&Step{View: result.View, effMaxLines: 1000}, stepOutcome{result: &result}))
			for _, want := range tc.want {
				if !strings.Contains(report, want) {
					t.Fatalf("actual inventory lost %q:\n%s", want, report)
				}
			}
			if tc.omitReply {
				if len(inventory.UnresolvedReasons) == 0 {
					t.Fatal("fixture must retain the unresolved cause rather than claim no Binder waits")
				}
				for _, reason := range inventory.UnresolvedReasons {
					if !strings.Contains(report, reason) {
						t.Fatalf("unresolved cause %q disappeared:\n%s", reason, report)
					}
				}
			}
		})
	}
}

func TestB1607BDiagActualReportCapDoesNotChangeBinderScan(t *testing.T) {
	path, idx := b1607BDiagBinderFixture(t, 39, false)
	result := tracequery.Run(idx, tracequery.Query{View: "window_stats", PID: 41, TimeStart: 6793222, TimeEnd: 6793222.09})
	full := renderStepBody(&Step{View: result.View, effMaxLines: 10000}, stepOutcome{result: &result})
	small := renderStepBody(&Step{View: result.View, effMaxLines: 12}, stepOutcome{result: &result})
	if len(small.lines) != 12 || small.total != full.total || small.total <= len(small.lines) {
		t.Fatalf("report cap accounting changed: small=%+v full_total=%d", small, full.total)
	}
	inventory := b1607BDiagBinderAccount(t, result)
	if inventory.ConfirmedCount != 39 || inventory.Emitted != 32 {
		t.Fatal("report cap changed engine scan or engine return accounting")
	}
	scriptPath := filepath.Join(t.TempDir(), "binder.yaml")
	script := "version: 1\ndefaults: { window: \"6793222.000000..6793222.090000\" }\nsteps:\n  - label: binder\n    view: window_stats\n    pid: 41\n    max_lines: 12\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	var report bytes.Buffer
	failed, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: path, Now: fixedNow}, &report)
	if err != nil || failed != 0 {
		t.Fatalf("actual diagnostic run failed: failed=%d err=%v\n%s", failed, err, report.String())
	}
	if !strings.Contains(report.String(), "按帽截断至 12") || !strings.Contains(report.String(), "输出行=12/") {
		t.Fatalf("report must separately disclose its own trimming:\n%s", report.String())
	}
}
