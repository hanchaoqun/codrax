package tracequery

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// Use the public Run boundary and JSON before the optional fields exist: a
// missing receipt must be a semantic RED, not a proposed-API compile failure.
func b1638b2aMeasurementDomain(t *testing.T, value any, method string, pid int) map[string]any {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	domain, ok := object["measurement_domain"].(map[string]any)
	if !ok || domain["partition_id"] == nil || domain["partition_id"] == "" {
		t.Fatalf("native %s row lacks measurement_domain: %s", method, body)
	}
	if domain["version"] != float64(1) || domain["status"] != "constructed_partition" ||
		domain["method"] != method || domain["target_tid"] != float64(pid) {
		t.Fatalf("measurement receipt changed its bounded method/target meaning: %+v", domain)
	}
	return domain
}

func TestB1638B2ANativeStreamsPreserveEveryContribution(t *testing.T) {
	idx := b1638b2aRunningChurnIndex(t)
	q := Query{View: "window_stats", PID: 300, TimeStart: 5, TimeEnd: 5.07, TimeStartSet: true, TimeEndSet: true}
	thread := ThreadRef{Comm: "churny", PID: 300}
	wantRunning := map[int]*schedulerMeasurementRecorder{}
	for _, row := range []struct {
		cpu, startLine, endLine int
		start, end              float64
	}{{1, 2, 3, 5, 5.01}, {1, 5, 6, 5.03, 5.04}, {2, 8, 9, 5.06, 5.07}} {
		if wantRunning[row.cpu] == nil {
			wantRunning[row.cpu] = newSchedulerMeasurementRecorder(q, thread, "cpu_running_sweep", queryResultTimeWindow(q))
		}
		wantRunning[row.cpu].add(schedulerMeasurementSegment{
			Thread: thread, State: StateRunning, OriginalState: StateRunning,
			StartTs: row.start, EndTs: row.end, ActualStartTs: row.start, ActualEndTs: row.end,
			DurationMs: (row.end - row.start) * 1000, StartLine: row.startLine, EndLine: row.endLine,
			CPU: row.cpu, CPUKnown: true, Priority: 100, PriorityClass: "raw_scheduler_prio", Closure: string(EventSchedSwitch),
		})
	}
	wantChurn := newSchedulerMeasurementRecorder(q, thread, "state_churn_sweep", queryResultTimeWindow(q))
	times := []float64{5, 5.01, 5.02, 5.03, 5.04, 5.05, 5.06, 5.07}
	states := []ThreadState{StateRunning, StateSSleep, StateRunnable, StateRunning, StateSSleep, StateRunnable, StateRunning}
	for i, state := range states {
		closure := string(EventSchedSwitch)
		if state == StateSSleep {
			closure = string(EventSchedWakeup)
		}
		wantChurn.add(schedulerMeasurementSegment{
			Thread: thread, State: state, OriginalState: state,
			StartTs: times[i], EndTs: times[i+1], ActualStartTs: times[i], ActualEndTs: times[i+1],
			DurationMs: (times[i+1] - times[i]) * 1000, StartLine: i + 2, EndLine: i + 3, Closure: closure,
		})
	}
	// Repeat the actual producer: Go map traversal order between CPU streams
	// must never leak into a per-(TID, CPU) receipt.
	for attempt := 0; attempt < 12; attempt++ {
		result := Run(idx, q)
		if result.WindowStats == nil || len(result.WindowStats.TopRunning) != 2 || len(result.WindowStats.StateChurn) != 1 {
			t.Fatalf("native row roster changed: %+v", result.WindowStats)
		}
		for _, row := range result.WindowStats.TopRunning {
			want := wantRunning[row.CPU].finish()
			if want == nil || !reflect.DeepEqual(row.MeasurementDomain, want) {
				t.Fatalf("CPU %d receipt omitted/reordered a native contribution: got=%+v want=%+v", row.CPU, row.MeasurementDomain, want)
			}
		}
		if got, want := result.WindowStats.StateChurn[0].MeasurementDomain, wantChurn.finish(); want == nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("churn receipt is not its seven actual native contributions: got=%+v want=%+v", got, want)
		}
	}
}

func TestB1638B2AChurnClipIOAndTailProvenance(t *testing.T) {
	q := Query{TimeStart: 1, TimeEnd: 2, TimeStartSet: true, TimeEndSet: true}
	thread := ThreadRef{Comm: "target", PID: 42}
	start := stateChurnOpen{thread: thread, state: StateDSleep, ts: .9, line: 10}
	for _, tc := range []struct {
		name    string
		endLine int
		closure string
	}{{"physical_close", 20, string(EventSchedWakeup)}, {"open_tail", 0, runnableCPUContinuityBoundaryWindowEnd}} {
		t.Run(tc.name, func(t *testing.T) {
			accs := map[string]*stateChurnAcc{}
			// The existing classifier selects this observed I/O marker, while
			// its display fallback may replace a missing close line with 19.
			reasons := map[int][]Event{42: {{Type: EventSchedBlockedReason, WakeePID: 42, Ts: 1.5, Line: 19,
				IOWait: 1, BlockedReasonIOWaitKnown: true, Reason: "wait_on_page"}}}
			addStateChurnInterval(nil, accs, start, 2.1, tc.endLine, q, reasons, tc.closure)
			acc := accs[threadKey(thread)]
			if acc == nil || acc.fragmentCount != 1 || acc.ioWaitMs != 1000 || acc.dStateMs != 0 || acc.lineEnd != firstPositive(tc.endLine, 19) {
				t.Fatalf("fixture must retain its existing final IO lane and display close: %+v", acc)
			}
			want := newSchedulerMeasurementRecorder(q, thread, "state_churn_sweep", queryResultTimeWindow(q))
			want.add(schedulerMeasurementSegment{Thread: thread, State: StateIOWait, OriginalState: StateDSleep,
				StartTs: 1, EndTs: 2, ActualStartTs: .9, ActualEndTs: 2.1, DurationMs: 1000,
				StartLine: 10, EndLine: tc.endLine, Closure: tc.closure, IO: true, IOMarked: true, Caller: "wait_on_page"})
			if got := acc.measurement.finish(); got == nil || !reflect.DeepEqual(got, want.finish()) {
				t.Fatalf("receipt lost final/original state, clipping or physical closure: got=%+v want=%+v", got, want.finish())
			}
			if _, ok := buildStateChurnSummary(acc, 1); ok {
				t.Fatal("a receipt must not bypass the existing churn fragment/switch gate")
			}
		})
	}
}

func TestB1638B2AActualRunScopeAndCapsRemainNative(t *testing.T) {
	idx := b1638b2aRunningChurnIndex(t)
	q := Query{View: "window_stats", PID: 300, TimeStart: 5, TimeEnd: 5.07, TimeStartSet: true, TimeEndSet: true}
	base := Run(idx, q)
	check := func(t *testing.T, result Result, same bool) {
		t.Helper()
		if result.WindowStats == nil || len(result.WindowStats.TopRunning) != 2 || len(result.WindowStats.StateChurn) != 1 {
			t.Fatalf("scope/presentation settings altered the fixture's eligible roster: %+v", result.WindowStats)
		}
		for i, row := range result.WindowStats.TopRunning {
			prior := base.WindowStats.TopRunning[i]
			if row.CPU != prior.CPU || row.DurationMs != prior.DurationMs || row.LineStart != prior.LineStart || row.LineEnd != prior.LineEnd {
				t.Fatalf("receipt changed original running values: got=%+v prior=%+v", row, prior)
			}
			if row.MeasurementDomain == nil || (row.MeasurementDomain.PartitionID == prior.MeasurementDomain.PartitionID) != same {
				t.Fatalf("native source identity did not respect this scope: got=%+v prior=%+v", row.MeasurementDomain, prior.MeasurementDomain)
			}
		}
		row, prior := result.WindowStats.StateChurn[0], base.WindowStats.StateChurn[0]
		if row.TotalMs != prior.TotalMs || row.FragmentCount != prior.FragmentCount || row.StateSwitches != prior.StateSwitches || row.Summary != prior.Summary {
			t.Fatalf("receipt changed original churn values/summary: got=%+v prior=%+v", row, prior)
		}
		if row.MeasurementDomain == nil || (row.MeasurementDomain.PartitionID == prior.MeasurementDomain.PartitionID) != same {
			t.Fatalf("churn source identity did not respect this scope: got=%+v prior=%+v", row.MeasurementDomain, prior.MeasurementDomain)
		}
	}
	t.Run("display_caps", func(t *testing.T) {
		limited := q
		limited.Limit, limited.MaxDepth, limited.MaxBranches, limited.MaxChainNodes = 1, 1, 1, 1
		check(t, Run(idx, limited), true)
	})
	t.Run("explicit_line_scope_even_with_equal_values", func(t *testing.T) {
		filtered := q
		filtered.LineStart, filtered.LineEnd = 2, 9
		result := Run(idx, filtered)
		check(t, result, false)
		for _, row := range result.WindowStats.TopRunning {
			if row.MeasurementDomain.QueryLineStart != 2 || row.MeasurementDomain.QueryLineEnd != 9 {
				t.Fatalf("native line filter missing: %+v", row.MeasurementDomain)
			}
		}
	})
	t.Run("EOF_does_not_rewrite_requested_bounds", func(t *testing.T) {
		beyond := q
		beyond.TimeEnd = 5.1
		result := Run(idx, beyond)
		check(t, result, false)
		for _, row := range result.WindowStats.TopRunning {
			if row.MeasurementDomain.WindowEndTs != 5.07 {
				t.Fatalf("busy sweep receipt must use its existing EOF-bounded end: %+v", row.MeasurementDomain)
			}
		}
		if result.WindowStats.StateChurn[0].MeasurementDomain.WindowEndTs != 5.1 {
			t.Fatal("churn method was incorrectly assigned the running method's different window")
		}
	})
}

func TestB1638B2AUnnormalizedAndExplicitZeroBoundsStayHonest(t *testing.T) {
	idx := buildTraceIndex(t, "running_churn_zero.systrace", strings.ReplaceAll(deadTailChurnTrace, "5.", "0."))
	q := Query{View: "window_stats", PID: 300, TimeStart: 0, TimeEnd: .07, TimeStartSet: true, TimeEndSet: true}
	known := Run(idx, q)
	if known.WindowStats == nil || len(known.WindowStats.TopRunning) != 1 || len(known.WindowStats.StateChurn) != 1 {
		t.Fatalf("explicit zero must retain its original rows: %+v", known.WindowStats)
	}
	for _, value := range []struct {
		row    any
		method string
	}{{known.WindowStats.TopRunning[0], "cpu_running_sweep"}, {known.WindowStats.StateChurn[0], "state_churn_sweep"}} {
		if domain := b1638b2aMeasurementDomain(t, value.row, value.method, 300); domain["window_start_ts"] != float64(0) {
			t.Fatalf("explicit zero start disappeared: %+v", domain)
		}
	}
	q.TimeStartSet = false
	unknown := ComputeWindowStats(idx, q) // Deliberately bypass Run's proven index-bound backfill.
	if len(unknown.TopRunning) != 1 || len(unknown.StateChurn) != 1 ||
		unknown.TopRunning[0].DurationMs != known.WindowStats.TopRunning[0].DurationMs || unknown.StateChurn[0].TotalMs != known.WindowStats.StateChurn[0].TotalMs {
		t.Fatal("unknown receipt scope must not erase or recompute native values")
	}
	if unknown.TopRunning[0].MeasurementDomain != nil || unknown.StateChurn[0].MeasurementDomain != nil {
		t.Fatal("an unset zero start must not become a known measurement scope")
	}
	positive := b1638b2aRunningChurnIndex(t)
	noEnd := ComputeWindowStats(positive, Query{PID: 300, TimeStart: 5, TimeStartSet: true})
	if len(noEnd.TopRunning) != 2 || len(noEnd.StateChurn) != 1 {
		t.Fatalf("existing unnormalized no-end rows changed: %+v", noEnd)
	}
	for _, row := range noEnd.TopRunning {
		if row.MeasurementDomain != nil {
			t.Fatal("busy loop with an unset scheduler end must not invent a receipt window")
		}
	}
	if domain := noEnd.StateChurn[0].MeasurementDomain; domain == nil || domain.WindowEndTs != positive.LastTs {
		t.Fatalf("churn must retain its own existing last-event end fallback: %+v", domain)
	}
}

func b1638b2aRunningChurnIndex(t *testing.T) *Index {
	t.Helper()
	lines := strings.Split(deadTailChurnTrace, "\n")
	for i, line := range lines {
		if strings.Contains(line, "5.060000:") || strings.Contains(line, "5.070000:") {
			lines[i] = strings.Replace(line, "[001]", "[002]", 1)
		}
	}
	return buildTraceIndex(t, "running_churn_measurement.systrace", strings.Join(lines, "\n"))
}

func TestB1638B2AActualRunRunningAndChurnReceipts(t *testing.T) {
	idx := b1638b2aRunningChurnIndex(t)
	before, _ := json.Marshal(idx.Events)
	q := Query{View: "window_stats", PID: 300, TimeStart: 5, TimeEnd: 5.07, TimeStartSet: true, TimeEndSet: true}
	result := Run(idx, q)
	if result.WindowStats == nil {
		t.Fatal("fixture must publish actual window statistics")
	}
	t.Run("running", func(t *testing.T) {
		seen := map[int]string{}
		for _, row := range result.WindowStats.TopRunning {
			if row.Thread.PID != 300 {
				continue
			}
			wantMs := map[int]float64{1: 20, 2: 10}[row.CPU]
			if wantMs == 0 || !near(row.DurationMs, wantMs, .000001) {
				t.Fatalf("running CPU buckets changed their actual booked time: %+v", row)
			}
			domain := b1638b2aMeasurementDomain(t, row, "cpu_running_sweep", 300)
			seen[row.CPU] = domain["partition_id"].(string)
		}
		if len(seen) != 2 || seen[1] == seen[2] {
			t.Fatalf("each CPU must retain its own nonempty native measurement: %+v", seen)
		}
	})
	t.Run("churn", func(t *testing.T) {
		assertChurnRowWithoutDeadTail(t, "Run[5,5.07]", result.WindowStats.StateChurn)
		found := false
		for _, row := range result.WindowStats.StateChurn {
			if row.Thread.PID == 300 {
				found = true
				b1638b2aMeasurementDomain(t, row, "state_churn_sweep", 300)
			}
		}
		if !found {
			t.Fatal("actual Run did not publish the eligible churn row")
		}
	})
	after, _ := json.Marshal(idx.Events)
	if !bytes.Equal(before, after) {
		t.Fatal("measurement metadata mutated the input events")
	}
}
