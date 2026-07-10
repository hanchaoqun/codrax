package tracequery

// Wave-2 PERF+PROV audit batch pins (§29.25 处置委托 2026-07-10).
//
// #21/#22 one-parse-per-line hot loop, #3/#23 bounded duration tracker with
// amortized resets, #24 per-index duration-order memo, #25 pairing hygiene,
// #36 dual-coordinate witnesses, #37 typed raw-unavailable caveats, #39 child
// caveat preservation, #40 one-systrace authority in the finalizer, #41
// composite-vs-direct interface-join consistency, #35 direct perftrace
// capability disclosure.

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- #21: shared-scan hot loop -------------------------------------------

// TestSafeParseLineScan_PanicDegradesToTypedCounter pins the recover seam of
// the memoized parse used by the main loop: a per-line panic must degrade to
// the typed counter, never kill the build.
func TestSafeParseLineScan_PanicDegradesToTypedCounter(t *testing.T) {
	idx := &Index{}
	orig := parseLineScanFn
	parseLineScanFn = func(*lineScan, *stringInterner) (Event, bool) { panic("malformed input") }
	defer func() { parseLineScanFn = orig }()
	var scan lineScan
	scan.reset(1, "any line")
	ev, ok := safeParseLineScan(&scan, nil, idx)
	if ok {
		t.Fatalf("panicked parse must report not-ok, got %+v", ev)
	}
	if idx.ParseLinePanics != 1 {
		t.Fatalf("panic must increment the typed counter, got %d", idx.ParseLinePanics)
	}
}

// TestLineScanTimestampMatchesParseLineTimestamp pins the byte-identical
// window-gate grammar contract: the shared memo must accept and reject the
// exact same timestamps as parseLineTimestamp, or windowed pruning would
// drift between the memoized main loop and every standalone consumer.
func TestLineScanTimestampMatchesParseLineTimestamp(t *testing.T) {
	lines := []string{
		"  app-42 (42) [001] d..3 100.123456: sched_switch: prev_comm=a prev_pid=1 prev_prio=1 prev_state=S ==> next_comm=b next_pid=2 next_prio=2",
		"  app-42 (42) [001] d..3 100: print: B|42|x",
		"free prose mentioning 123.456: which is not a header",
		"  app-42 (42) [001] d..3 99999999999999999999.0: print: B|42|overflow",
		"# tracer: nop",
		"",
		"  weird-comm-with-42.5-inside-7 (7) [000] d..3 55.5: print: C|7|v|1",
	}
	for _, line := range lines {
		wantTs, wantOK := parseLineTimestamp(line)
		var scan lineScan
		scan.reset(1, line)
		gotTs, gotOK := scan.timestamp()
		if gotTs != wantTs || gotOK != wantOK {
			t.Fatalf("lineScan.timestamp diverges from parseLineTimestamp on %q: got (%v,%v) want (%v,%v)", line, gotTs, gotOK, wantTs, wantOK)
		}
		// Memoized second read must be stable.
		gotTs2, gotOK2 := scan.timestamp()
		if gotTs2 != gotTs || gotOK2 != gotOK {
			t.Fatalf("memoized timestamp unstable on %q", line)
		}
	}
}

// TestWindowedAuditLedgersSurviveSharedScan is the #21 behavior pin: with the
// one-parse-per-line loop, every physical-row audit family must still mint
// its witness — including from gate-skipped PREFIX rows whose tracker state
// and carry-in poison the windowed correctness contract depends on.
func TestWindowedAuditLedgersSurviveSharedScan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audits.systrace")
	body := strings.Join([]string{
		// Prefix (before the 10.0..11.0 window): trace-span rollback carry-in
		// poison — recorded despite being gate-skipped.
		"  app-10 (10) [000] d..3 9.000000: print: B|10|PrefixSpan",
		"  app-10 (10) [000] d..3 8.900000: print: E|10",
		// Prefix malformed trace-mark payload (non-numeric span pid) — the
		// carry-in poison must survive the skip lane.
		"  app-10 (10) [000] d..3 9.100000: print: B|abc|Bad",
		// In-window rows:
		"  app-10 (10) [000] d..3 10.100000: sched_switch: prev_comm=app prev_pid=10 prev_prio=100 prev_state=S ==> next_comm=w next_pid=11 next_prio=120",
		// Malformed scheduler row (missing next_pid).
		"  app-10 (10) [000] d..3 10.150000: sched_switch: prev_comm=app prev_pid=10 prev_prio=100 prev_state=S ==> next_comm=w next_prio=120",
		// Malformed interrupt endpoint (bad vec).
		"  <idle>-0 (-----) [001] d..3 10.200000: softirq_entry: vec=banana",
		// Malformed workqueue endpoint (invalid work pointer).
		"  kworker-33 (33) [002] d..3 10.250000: workqueue_execute_start: work struct=nope function=f",
		// Invalid cpu_id on a cpu_frequency row.
		"  <idle>-0 (-----) [001] d..3 10.300000: cpu_frequency: state=1000000 cpu_id=zap",
		// Incarnation reuse: tid 11 seen above, then created anew.
		"  app-10 (10) [000] d..3 10.400000: sched_wakeup_new: comm=w pid=11 prio=120 target_cpu=001",
		// Scheduler order rollback on cpu lane (in-window).
		"  app-10 (10) [000] d..3 10.500000: sched_switch: prev_comm=w prev_pid=11 prev_prio=120 prev_state=S ==> next_comm=app next_pid=10 next_prio=100",
		"  app-10 (10) [000] d..3 10.450000: sched_switch: prev_comm=app prev_pid=10 prev_prio=100 prev_state=S ==> next_comm=w next_pid=11 next_prio=120",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := BuildOptions{AllowWindowedParse: true, TimeStart: 10.0, TimeStartSet: true, TimeEnd: 11.0, TimeEndSet: true}
	idx, err := BuildIndexWithOptions(context.Background(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !idx.Windowed {
		t.Fatal("fixture must build a windowed index")
	}
	// (1) Prefix trace-span rollback carried in.
	spanRollback := false
	for _, failure := range idx.durationOrderFailures {
		if failure.Family == durationOrderTraceSpan && failure.Line == 2 && failure.CurrentTs == 8.9 {
			spanRollback = true
		}
	}
	if !spanRollback {
		t.Fatalf("gate-skipped prefix trace-span rollback must stay recorded: %+v", idx.durationOrderFailures)
	}
	// (2) Prefix malformed trace-mark row.
	if len(idx.traceMarkIntegrityFailures) == 0 {
		t.Fatal("prefix malformed trace-mark row must mint its witness")
	}
	// (3) Malformed scheduler row.
	foundSchedRow := false
	for _, failure := range idx.schedulerRowIntegrityFailures {
		if failure.EventName == "sched_switch" && failure.Line == 5 {
			foundSchedRow = true
		}
	}
	if !foundSchedRow {
		t.Fatalf("malformed sched_switch row must mint scheduler_row witness: %+v", idx.schedulerRowIntegrityFailures)
	}
	// (4) Malformed interrupt endpoint.
	foundIRQ := false
	// (5) Malformed workqueue endpoint.
	foundWq := false
	for _, failure := range idx.durationOrderFailures {
		if failure.Family == durationOrderSoftIRQ && failure.Issue == "endpoint_parse_incomplete" {
			foundIRQ = true
		}
		if failure.Family == durationOrderWorkqueue && failure.Issue == "endpoint_parse_incomplete" {
			foundWq = true
		}
	}
	if !foundIRQ || !foundWq {
		t.Fatalf("interrupt/workqueue endpoint witnesses missing: irq=%v wq=%v %+v", foundIRQ, foundWq, idx.durationOrderFailures)
	}
	// (6) cpu_input witness.
	foundCPU := false
	for _, failure := range idx.cpuInputIntegrityFailures {
		if failure.Field == "cpu_id" {
			foundCPU = true
		}
	}
	if !foundCPU {
		t.Fatalf("invalid cpu_id must mint cpu_input witness: %+v", idx.cpuInputIntegrityFailures)
	}
	// (7) Incarnation conflict.
	foundIncarnation := false
	for _, conflict := range idx.threadIncarnationFailures {
		if conflict.PID == 11 && conflict.Signal == "sched_wakeup_new" {
			foundIncarnation = true
		}
	}
	if !foundIncarnation {
		t.Fatalf("tid reuse must mint incarnation conflict: %+v", idx.threadIncarnationFailures)
	}
	// (8) Scheduler order rollback.
	if len(idx.schedulerOrderFailures) == 0 {
		t.Fatal("in-window scheduler lane rollback must mint order witness")
	}
}

// TestWindowedColdBuildCostBoundedByLineScanFloor is the #21 regression
// tripwire (复核 F2 rework: the earlier "windowed ≤ 3× full parse" ratio was a
// false defense — the 29× regression inflated BOTH paths, so their ratio
// stayed ≈1 and the pin passed on the broken tree). The anchor here is a
// CALIBRATION floor the defended regression class cannot inflate: one
// read+trim+single-header-match pass over the same file, measured in the same
// run on the same machine. A cold windowed build legitimately pays ~1 header
// match + 1 parseKV + 1 Event parse + tracker feeds per line ≈ 2.4× the
// floor (measured 2026-07-11, M5 Max); the defended class — per-line REPEATED
// header regexes / repeated Event parses across the five audits (the shipped
// 29× shape measured ≈10× the floor) — lands at ≥4× and fails the bound.
// A single accidental extra regex (≈3.2×) stays below it and is benchmark
// territory, not tripwire territory.
func TestWindowedColdBuildCostBoundedByLineScanFloor(t *testing.T) {
	if testing.Short() {
		t.Skip("timing tripwire skipped in -short")
	}
	path := writeSyntheticTrace(t, 60000)
	measure := func(f func()) time.Duration {
		best := time.Duration(1 << 62)
		for i := 0; i < 3; i++ {
			start := time.Now()
			f()
			if d := time.Since(start); d < best {
				best = d
			}
		}
		return best
	}
	floor := measure(func() {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		r := bufio.NewReaderSize(f, 256*1024)
		for {
			line, err := r.ReadString('\n')
			if len(line) > 0 {
				trimmed := strings.TrimRight(line, "\r\n")
				_ = ftraceLineRE.FindStringSubmatch(trimmed)
			}
			if err != nil {
				break
			}
		}
	})
	late := 100.0 + float64(60000-2000)*0.0001
	opts := BuildOptions{AllowWindowedParse: true, TimeStart: late, TimeStartSet: true, TimeEnd: late + 0.1, TimeEndSet: true}
	windowed := measure(func() {
		resetAnchorCaches()
		if _, err := BuildIndexWithOptions(context.Background(), path, opts); err != nil {
			t.Fatal(err)
		}
	})
	t.Logf("line-scan floor=%v cold windowed=%v ratio=%.2f (budget 4.0)", floor, windowed, float64(windowed)/float64(floor))
	if windowed > 4*floor {
		t.Fatalf("cold windowed build %v exceeds 4× the single-header-match line-scan floor %v (ratio %.2f) — the per-line audits are re-running header matches or Event parses again (the #21 29× regression class)", windowed, floor, float64(windowed)/float64(floor))
	}
}

// TestWindowGateKeepsInclusiveTimeStartBoundary pins the inclusive window
// head with zero padding: an event exactly at time_start belongs to the
// window (timeInWindow includes both endpoints; the gate must not drop it).
// Default tool-layer padding normally masks an off-by-one here — this pin
// removes the mask.
func TestWindowGateKeepsInclusiveTimeStartBoundary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "boundary.systrace")
	body := strings.Join([]string{
		" app-10 (10) [000] d..3 9.900000: sched_switch: prev_comm=a prev_pid=10 prev_prio=1 prev_state=S ==> next_comm=b next_pid=11 next_prio=2",
		" app-10 (10) [000] d..3 10.000000: sched_switch: prev_comm=b prev_pid=11 prev_prio=2 prev_state=S ==> next_comm=a next_pid=10 next_prio=1",
		" app-10 (10) [000] d..3 10.100000: sched_switch: prev_comm=a prev_pid=10 prev_prio=1 prev_state=S ==> next_comm=b next_pid=11 next_prio=2",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := BuildOptions{AllowWindowedParse: true, TimeStart: 10.0, TimeStartSet: true, TimeEnd: 10.5, TimeEndSet: true}
	idx, err := BuildIndexWithOptions(context.Background(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ev := range idx.Events {
		if ev.Ts == 10.0 {
			found = true
		}
		if ev.Ts == 9.9 {
			t.Fatalf("pre-window event must stay outside a zero-padding window: %+v", ev)
		}
	}
	if !found {
		t.Fatalf("event exactly at time_start must be retained (inclusive boundary): %+v", idx.Events)
	}
}

// --- #22: token prescreens must not change validator verdicts -------------

func TestInterruptPrescreenPreservesVerdicts(t *testing.T) {
	cases := []struct {
		line string
		want bool // non-nil violation expected
	}{
		{"  <idle>-0 (-----) [001] d..3 10.0: softirq_entry: vec=banana", true},
		{"  <idle>-0 (-----) [001] d..3 10.0: irq_handler_exit: irq=-2 ret=handled", true},
		{"  <idle>-0 (-----) [001] d..3 10.0: ipi_entry: ", true},
		{"  <idle>-0 (-----) [001] d..3 10.0: softirq_entry: vec=9 [action=RCU]", false},
		{"  app-1 (1) [000] d..3 10.0: sched_switch: prev_comm=a prev_pid=1 prev_prio=1 prev_state=S ==> next_comm=b next_pid=2 next_prio=2", false},
		{"  app-1 (1) [000] d..3 10.0: print: B|1|x", false},
	}
	for _, tc := range cases {
		got := interruptEndpointValidationFailure(1, tc.line) != nil
		if got != tc.want {
			t.Fatalf("interrupt verdict flipped for %q: got %v want %v", tc.line, got, tc.want)
		}
	}
	// Every mintable name must pass the prescreen.
	for _, name := range []string{"irq_handler_entry", "irq_handler_exit", "softirq_entry", "softirq_exit", "ipi_entry", "ipi_exit"} {
		if !interruptEndpointRawCandidate("  x-1 (1) [000] d..3 1.0: " + name + ": ") {
			t.Fatalf("prescreen must admit %s rows", name)
		}
	}
}

func TestDurationEndpointFallbackPrescreenKeepsFallbackReachable(t *testing.T) {
	// Header pid is non-numeric, so ftraceLineRE rejects the row; only the
	// fallback grammar can audit it. The prescreen must keep that lane alive.
	line := "  kworker/u16:5-badpid [002] d..3 10.250000: workqueue_execute_start: work struct=nope"
	failure := durationEndpointRawValidationFailure(7, line)
	if failure == nil || failure.Family != durationOrderWorkqueue {
		t.Fatalf("fallback-grammar workqueue audit lost: %+v", failure)
	}
	if !durationEndpointFallbackCandidate(line) {
		t.Fatal("prescreen must admit workqueue_execute_ rows")
	}
	if durationEndpointFallbackCandidate("  app-1 (1) [000] d..3 10.0: sched_switch: prev_pid=1") {
		t.Fatal("prescreen must reject non-endpoint rows")
	}
}

// --- #3/#23: bounded tracker + amortized resets ---------------------------

func TestDurationTrackerLaneBudgetFailsClosed(t *testing.T) {
	tracker := newDurationOrderTracker()
	for i := 0; i < durationOrderTrackerLaneBudget+50; i++ {
		ev := Event{
			Line: i + 1, Ts: 1.0 + float64(i)*0.001, Type: EventTraceMark, PID: 42,
			SpanAction: "C", SpanName: fmt.Sprintf("counter_%d", i), SpanValue: "1",
			FieldText: fmt.Sprintf("C|42|counter_%d|1", i),
		}
		tracker.observeAll(ev)
	}
	if got := len(tracker.familyLanes[durationOrderTraceCounter]); got > durationOrderTrackerLaneBudget {
		t.Fatalf("counter lanes exceed budget: %d", got)
	}
	if !tracker.capped[durationOrderTraceCounter] {
		t.Fatal("budget overflow must mark the family capped (fail-close), not drop lanes silently")
	}
	if tracker.capped[durationOrderTraceSpan] {
		t.Fatal("unrelated families must stay uncapped")
	}
}

func TestDurationTrackerBudgetSurfacesAsOrderAuditTruncated(t *testing.T) {
	events := make([]Event, 0, durationOrderTrackerLaneBudget+10)
	for i := 0; i < durationOrderTrackerLaneBudget+10; i++ {
		events = append(events, Event{
			Line: i + 1, Ts: 1.0 + float64(i)*0.001, Type: EventTraceMark, PID: 42,
			SpanAction: "C", SpanName: fmt.Sprintf("c%d", i), SpanValue: "1",
			FieldText: fmt.Sprintf("C|42|c%d|1", i),
		})
	}
	idx := &Index{Events: events, TimestampOrder: TraceTimestampOrderUnknown}
	failures := durationOrderFailuresForQuery(idx, Query{})
	violation := failures[durationOrderTraceCounter]
	if violation == nil || violation.Lane != "order_audit_truncated" {
		t.Fatalf("budget overflow must fail-close the family via order_audit_truncated: %+v", violation)
	}
}

func TestDurationTrackerResetPIDClearsOnlyOwnedLanes(t *testing.T) {
	tracker := newDurationOrderTracker()
	open := func(pid int, work string) {
		tracker.observeAll(Event{
			Line: 1, Ts: 1.0, Type: EventWorkqueue, PID: pid,
			Name: "workqueue_execute_start", FieldText: "work struct=0xdeadbeef" + work,
		})
	}
	// Two owners with open workqueue lanes (distinct pointers).
	open(10, "00")
	open(20, "11")
	if len(tracker.last) != 2 {
		t.Fatalf("expected 2 open lanes, got %d", len(tracker.last))
	}
	tracker.resetPID(10)
	if len(tracker.last) != 1 {
		t.Fatalf("resetPID(10) must clear exactly pid 10's lane: %d lanes left", len(tracker.last))
	}
	if len(tracker.ownerLanes[10]) != 0 {
		t.Fatal("inverse owner index must drop the cleared pid")
	}
	if len(tracker.ownerLanes[20]) != 1 {
		t.Fatal("unrelated owner's inverse index must survive")
	}
}

func TestDurationTrackerResetTraceMarkPIDUsesStackMembership(t *testing.T) {
	tracker := newDurationOrderTracker()
	asyncS := func(pid int, name string, cookie string) {
		tracker.observeAll(Event{
			Line: 1, Ts: 1.0, Type: EventTraceMark, PID: pid,
			SpanAction: "S", SpanName: name, SpanValue: cookie,
			FieldText: fmt.Sprintf("S|%d|%s|%s", pid, name, cookie),
		})
	}
	asyncS(10, "sharedLane", "7") // pid 10 opens
	asyncS(20, "otherLane", "9")  // pid 20 opens a different lane
	if len(tracker.traceSpanOwners) != 2 {
		t.Fatalf("expected 2 async lanes, got %d", len(tracker.traceSpanOwners))
	}
	tracker.resetTraceMarkPID(10)
	if len(tracker.traceSpanOwners) != 1 {
		t.Fatalf("reset must clear exactly the lane whose stack holds pid 10: %d lanes", len(tracker.traceSpanOwners))
	}
	for lane := range tracker.traceSpanOwners {
		if !strings.Contains(lane.key, "otherLane") {
			t.Fatalf("wrong lane survived: %+v", lane)
		}
	}
	if len(tracker.spanOwnerLanes[10]) != 0 {
		t.Fatal("span-owner inverse index must drop pid 10")
	}
}

// TestDurationTrackerInverseIndexesStayConsistent pins the bookkeeping
// invariants under a mixed open/close/sample/reset sequence: familyLanes
// mirrors the domain of last; ownerLanes mirrors owners; spanOwnerLanes
// counts match the surviving stacks.
func TestDurationTrackerInverseIndexesStayConsistent(t *testing.T) {
	tracker := newDurationOrderTracker()
	feed := []Event{
		{Line: 1, Ts: 1.0, Type: EventTraceMark, PID: 10, SpanAction: "B", FieldText: "B|10|a"},
		{Line: 2, Ts: 1.1, Type: EventTraceMark, PID: 10, SpanAction: "S", SpanName: "x", SpanValue: "1", FieldText: "S|10|x|1"},
		{Line: 3, Ts: 1.2, Type: EventTraceMark, PID: 11, SpanAction: "S", SpanName: "x", SpanValue: "1", FieldText: "S|11|x|1"},
		{Line: 4, Ts: 1.3, Type: EventWorkqueue, PID: 12, Name: "workqueue_execute_start", FieldText: "work struct=0xdeadbeef22"},
		{Line: 5, Ts: 1.4, Type: EventTraceMark, PID: 10, SpanAction: "E", FieldText: "E|10"},
		{Line: 6, Ts: 1.5, Type: EventTraceMark, PID: 99, SpanAction: "F", SpanName: "x", SpanValue: "1", FieldText: "F|99|x|1"},
		{Line: 7, Ts: 1.6, Type: EventWorkqueue, PID: 12, Name: "workqueue_execute_end", FieldText: "work struct=0xdeadbeef22"},
		{Line: 8, Ts: 1.7, Type: EventTraceMark, PID: 42, SpanAction: "C", SpanName: "ctr", SpanValue: "5", FieldText: "C|42|ctr|5"},
	}
	for _, ev := range feed {
		tracker.observeAll(ev)
	}
	tracker.resetPID(11)
	// Invariant checks.
	familyCount := 0
	for _, lanes := range tracker.familyLanes {
		familyCount += len(lanes)
		for lane := range lanes {
			if _, ok := tracker.last[lane]; !ok {
				t.Fatalf("familyLanes holds lane absent from last: %+v", lane)
			}
		}
	}
	if familyCount != len(tracker.last) {
		t.Fatalf("familyLanes cardinality %d != last %d", familyCount, len(tracker.last))
	}
	ownerCount := 0
	for owner, lanes := range tracker.ownerLanes {
		ownerCount += len(lanes)
		for lane := range lanes {
			if tracker.owners[lane] != owner {
				t.Fatalf("ownerLanes[%d] holds lane owned by %d", owner, tracker.owners[lane])
			}
		}
	}
	if ownerCount != len(tracker.owners) {
		t.Fatalf("ownerLanes cardinality %d != owners %d", ownerCount, len(tracker.owners))
	}
	spanCounts := map[int]map[durationOrderLane]int{}
	for lane, stack := range tracker.traceSpanOwners {
		for _, owner := range stack {
			byLane := spanCounts[owner]
			if byLane == nil {
				byLane = map[durationOrderLane]int{}
				spanCounts[owner] = byLane
			}
			byLane[lane]++
		}
	}
	for owner, wantByLane := range spanCounts {
		for lane, want := range wantByLane {
			if got := tracker.spanOwnerLanes[owner][lane]; got != want {
				t.Fatalf("spanOwnerLanes[%d][%v]=%d want %d", owner, lane, got, want)
			}
		}
	}
	for owner, byLane := range tracker.spanOwnerLanes {
		for lane, got := range byLane {
			if want := spanCounts[owner][lane]; got != want {
				t.Fatalf("stale spanOwnerLanes[%d][%v]=%d want %d", owner, lane, got, want)
			}
		}
	}
}

// --- #24: per-index memo ---------------------------------------------------

func TestDurationOrderFailuresForQueryMemoizedPerIndex(t *testing.T) {
	events := []Event{
		{Line: 1, Ts: 2.0, Type: EventTraceMark, PID: 10, SpanAction: "B", FieldText: "B|10|s"},
		{Line: 2, Ts: 1.5, Type: EventTraceMark, PID: 10, SpanAction: "E", FieldText: "E|10"},
	}
	idx := &Index{Events: events, TimestampOrder: TraceTimestampOrderRegressed}
	first := durationOrderFailuresForQuery(idx, Query{})
	if first[durationOrderTraceSpan] == nil {
		t.Fatalf("rollback must be detected: %+v", first)
	}
	// Mutating Events after the first call must NOT change the verdict —
	// proof that the scan ran once and is served from the per-index memo.
	idx.Events = nil
	second := durationOrderFailuresForQuery(idx, Query{})
	if second[durationOrderTraceSpan] == nil {
		t.Fatal("memoized scan must survive event-slice mutation (scan re-ran instead of memo)")
	}
	// The q filter still applies post-memo: a bounded query that excludes the
	// rollback (both timestamps past TimeEnd is the only exclusion arm).
	filtered := durationOrderFailuresForQuery(idx, Query{TimeEnd: 1.0})
	if filtered[durationOrderTraceSpan] != nil {
		t.Fatalf("post-memo relevance filter lost: %+v", filtered[durationOrderTraceSpan])
	}
}

// TestMemoCapDoesNotFalseCloseInWindowWitness (复核 F3): with the old
// 64-witness memo cap, 64+ violations sitting entirely AFTER the query window
// pushed the single in-window violation past the cap and collapsed the whole
// family into order_audit_truncated. The 1024 memo cap keeps the precise
// witness reachable; capped fail-close survives past the larger cap.
func TestMemoCapDoesNotFalseCloseInWindowWitness(t *testing.T) {
	var all []Event
	line := 1
	// 70 post-window rollbacks on distinct sync lanes (pids 1000+i), minted
	// BEFORE the in-window witness in scan order so they flood the memo cap
	// first (the exact false-close shape).
	for i := 0; i < 70; i++ {
		pid := 1000 + i
		all = append(all,
			Event{Line: line, Ts: 100.0, Type: EventTraceMark, PID: pid, SpanAction: "B", FieldText: "B|x"},
			Event{Line: line + 1, Ts: 99.0, Type: EventTraceMark, PID: pid, SpanAction: "E", FieldText: "E"},
		)
		line += 2
	}
	// One in-window rollback (workqueue lane), scanned LAST.
	all = append(all,
		Event{Line: line, Ts: 5.0, Type: EventWorkqueue, PID: 7, Name: "workqueue_execute_start", FieldText: "work struct=0xdeadbeef77"},
		Event{Line: line + 1, Ts: 4.5, Type: EventWorkqueue, PID: 7, Name: "workqueue_execute_end", FieldText: "work struct=0xdeadbeef77"},
	)
	idx := &Index{Events: all, TimestampOrder: TraceTimestampOrderRegressed}
	failures := durationOrderFailuresForQuery(idx, Query{TimeStart: 4.0, TimeEnd: 6.0})
	violation := failures[durationOrderWorkqueue]
	if violation == nil {
		t.Fatal("in-window workqueue rollback must surface")
	}
	if violation.Lane == "order_audit_truncated" {
		t.Fatalf("post-window witness flood must not collapse the in-window witness into a family truncation: %+v", violation)
	}
	if violation.CurrentTs != 4.5 || violation.PreviousTs != 5.0 {
		t.Fatalf("precise witness expected: %+v", violation)
	}
}

func TestFrequencyOrderIntegrityConsumesSameMemo(t *testing.T) {
	events := []Event{
		{Line: 1, Ts: 2.0, Type: EventCPUFrequency, CPU: 3, Frequency: 100, Name: "cpu_frequency", CPUForField: 3, CPUForFieldValid: true, CPUForFieldPresent: true},
		{Line: 2, Ts: 1.0, Type: EventCPUFrequency, CPU: 3, Frequency: 200, Name: "cpu_frequency", CPUForField: 3, CPUForFieldValid: true, CPUForFieldPresent: true},
	}
	idx := &Index{Events: events, TimestampOrder: TraceTimestampOrderRegressed}
	first := frequencyOrderIntegrityForQuery(idx, Query{})
	if !first.frequencyUnsafe(3) {
		t.Fatalf("cpu3 frequency rollback must fail-close: %+v", first)
	}
	idx.Events = nil
	second := frequencyOrderIntegrityForQuery(idx, Query{})
	if !second.frequencyUnsafe(3) {
		t.Fatal("frequency integrity must consume the per-index memo")
	}
}

// --- #25: pairing hygiene ---------------------------------------------------

// TestResetTraceMarkSyncPairingStateScopesToSource: the reset invalidates
// only the named physical source and emitter (a lifecycle/malformed row in
// one attachment must never erase an identically numbered emitter's open
// state in another attachment). Rebase note (origin/main 4092164e): the async
// half of the original #25 pin was structurally superseded by the typed
// traceMarkAsyncPairer (per-source/per-owner cohort FSM with its own
// lifecycle-cut tests); only the sync-stack scoping remains this pin's
// responsibility.
func TestResetTraceMarkSyncPairingStateScopesToSource(t *testing.T) {
	stacks := map[string][]Event{
		traceMarkSyncPairingKey("srcA", 10): {{PID: 10}},
		traceMarkSyncPairingKey("srcA", 11): {{PID: 11}},
		traceMarkSyncPairingKey("srcB", 10): {{PID: 10}},
	}
	resetTraceMarkSyncPairingState("srcA", 10, stacks)
	if _, ok := stacks[traceMarkSyncPairingKey("srcA", 10)]; ok {
		t.Fatal("srcA/pid10 sync stack must be cleared")
	}
	if _, ok := stacks[traceMarkSyncPairingKey("srcA", 11)]; !ok {
		t.Fatal("srcA/pid11 must survive a pid10 reset")
	}
	if _, ok := stacks[traceMarkSyncPairingKey("srcB", 10)]; !ok {
		t.Fatal("srcB sync stack must survive a srcA reset")
	}
	resetTraceMarkSyncPairingState("", 10, stacks)
	resetTraceMarkSyncPairingState("srcB", -1, stacks)
	if _, ok := stacks[traceMarkSyncPairingKey("srcB", 10)]; !ok {
		t.Fatal("empty-source/negative-pid resets must be no-ops")
	}
}

func TestResolveArtifactSourceIndexMatchesSpanResolution(t *testing.T) {
	sources := []TraceArtifactSource{
		{SourcePath: "a", CausalCompatible: true, LocalLineCount: 10, VirtualLineBase: 0},
		{SourcePath: "iso", CausalCompatible: false, LocalLineCount: 10, VirtualLineBase: 100},
		{SourcePath: "b", CausalCompatible: true, LocalLineCount: 5, VirtualLineBase: 200},
		// Overlapping (hand-built pathological) source to exercise ambiguity.
		{SourcePath: "overlap", CausalCompatible: true, LocalLineCount: 3, VirtualLineBase: 202},
	}
	for _, line := range []int{-1, 0, 1, 10, 11, 100, 105, 200, 201, 202, 203, 205, 206, 300} {
		spans := resolveTraceArtifactSpans(sources, line, line)
		wantOK := len(spans) == 1
		wantPath := ""
		if wantOK {
			wantPath = spans[0].SourcePath
		}
		gotIdx, gotOK := resolveTraceArtifactSourceIndexForLine(sources, line)
		if gotOK != wantOK {
			t.Fatalf("line %d: ok mismatch got %v want %v (spans=%+v)", line, gotOK, wantOK, spans)
		}
		if gotOK && sources[gotIdx].SourcePath != wantPath {
			t.Fatalf("line %d: path mismatch got %q want %q", line, sources[gotIdx].SourcePath, wantPath)
		}
	}
}

func TestBlockPairingLaneDeletionKeepsVerdicts(t *testing.T) {
	idx := &Index{Path: "t.systrace"}
	add := func(line int, ts float64, name string) {
		typ := EventBlockIssue
		if strings.HasSuffix(name, "complete") {
			typ = EventBlockComplete
		}
		idx.Events = append(idx.Events, Event{
			Line: line, Ts: ts, Type: typ, Name: name, PID: 9,
			BlockIOFields: &BlockIOFields{Dev: "8,0", Op: "R", Sector: 100, Len: 8, IdentityParsed: true, IdentityValid: true},
		})
	}
	// Pair 1 closes, identical identity reopens and closes again, then an
	// ambiguous cohort (two concurrent starts) closes suppressed.
	add(1, 1.0, "block_rq_issue")
	add(2, 1.1, "block_rq_complete")
	add(3, 2.0, "block_rq_issue")
	add(4, 2.2, "block_rq_complete")
	add(5, 3.0, "block_rq_issue")
	add(6, 3.1, "block_rq_issue")
	add(7, 3.2, "block_rq_complete")
	add(8, 3.3, "block_rq_complete")
	out := computeBlockIOLatencies(idx, Query{}, 8)
	if len(out.latencies) != 2 {
		t.Fatalf("two clean pairs expected after lane recycling: %+v", out.latencies)
	}
	var ambiguous int
	for _, summary := range out.summaries {
		ambiguous += summary.AmbiguousCohortCount
	}
	if ambiguous != 1 {
		t.Fatalf("depth-2 cohort must stay ambiguous: %+v", out.summaries)
	}
}

// --- #36: dual-coordinate witnesses ----------------------------------------

func TestRebasedWitnessesRenderLocalLine(t *testing.T) {
	dir := t.TempDir()
	perftrace := filepath.Join(dir, "cap.perftrace")
	systrace := filepath.Join(dir, "cap.systrace")
	bundle := filepath.Join(dir, "cap.tracebundle.json")
	// Perftrace declared FIRST so the systrace child gets a non-zero
	// VirtualLineBase (declaration order is merge order).
	perfBody := " app-20 (20) [001] .... 10.001000: perf_sample: cpu=1 pid=20 tid=20 period=9000 event=cpu-cycles symbol=X dso=l.so source=test\n"
	sysBody := strings.Join([]string{
		" app-10 (10) [000] d..3 10.100000: print: B|10|Span",
		" app-10 (10) [000] d..3 10.050000: print: E|10",
		" app-10 (10) [000] d..3 10.200000: sched_switch: prev_comm=app prev_pid=10 prev_prio=100 prev_state=S ==> next_comm=w next_prio=120",
		"",
	}, "\n")
	if err := os.WriteFile(perftrace, []byte(perfBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(systrace, []byte(sysBody), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"version":"t","artifacts":[
  {"type":"perftrace","path":"cap.perftrace","perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true}},
  {"type":"systrace","path":"cap.systrace"}
],"perf_clock_alignments":[{"artifact_path":"cap.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","calibrated":false}]}`
	if err := os.WriteFile(bundle, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	var systraceBase int
	for _, source := range idx.TraceArtifacts {
		if strings.HasSuffix(source.SourcePath, ".systrace") {
			systraceBase = source.VirtualLineBase
		}
	}
	if systraceBase == 0 {
		t.Fatalf("fixture must give the systrace a non-zero virtual base: %+v", idx.TraceArtifacts)
	}
	foundDuration := false
	for i := range idx.durationOrderFailures {
		failure := &idx.durationOrderFailures[i]
		if failure.Family != durationOrderTraceSpan || failure.SourcePath == "" {
			continue
		}
		foundDuration = true
		if failure.LocalLine != 2 || failure.Line != systraceBase+2 {
			t.Fatalf("duration witness coordinates: %+v (base=%d)", failure, systraceBase)
		}
		reason := failure.reason()
		if !strings.Contains(reason, fmt.Sprintf("line=%d", failure.Line)) ||
			!strings.Contains(reason, "local_line=2") ||
			!strings.Contains(reason, "source=") {
			t.Fatalf("duration witness must render both coordinates: %q", reason)
		}
	}
	if !foundDuration {
		t.Fatalf("trace-span rollback witness missing: %+v", idx.durationOrderFailures)
	}
	foundRow := false
	for i := range idx.schedulerRowIntegrityFailures {
		failure := &idx.schedulerRowIntegrityFailures[i]
		if failure.SourcePath == "" {
			continue
		}
		foundRow = true
		if failure.LocalLine != 3 || failure.Line != systraceBase+3 {
			t.Fatalf("scheduler row witness coordinates: %+v (base=%d)", failure, systraceBase)
		}
		if reason := failure.reason(); !strings.Contains(reason, "local_line=3") {
			t.Fatalf("scheduler row witness must render local_line: %q", reason)
		}
	}
	if !foundRow {
		t.Fatalf("scheduler row witness missing: %+v", idx.schedulerRowIntegrityFailures)
	}
}

func TestSingleFileWitnessStaysTerse(t *testing.T) {
	violation := durationOrderViolation{
		Family: durationOrderTraceSpan, Lane: "l", LaneKey: "k",
		PreviousTs: 2, CurrentTs: 1, Line: 7, SourcePath: "/tmp/x.systrace",
	}
	if reason := violation.reason(); strings.Contains(reason, "local_line=") {
		t.Fatalf("physical-coordinate witness must not grow a local_line marker: %q", reason)
	}
	rebased := violation
	rebased.LocalLine = 7
	rebased.Line = 107
	if reason := rebased.reason(); !strings.Contains(reason, "local_line=7") || !strings.Contains(reason, "line=107") {
		t.Fatalf("rebased witness must carry both coordinates: %q", reason)
	}
	// Base-0 artifact: coordinates coincide, marker stays off.
	coinciding := violation
	coinciding.LocalLine = 7
	if reason := coinciding.reason(); strings.Contains(reason, "local_line=") {
		t.Fatalf("coinciding coordinates must stay terse: %q", reason)
	}
}

func TestIncarnationWitnessRendersLocalCoordinates(t *testing.T) {
	conflict := threadIncarnationConflict{
		PID: 11, Signal: "sched_wakeup_new",
		PreviousLine: 105, BoundaryLine: 109, BoundaryTs: 10.4,
		SourcePath: "/tmp/cap.systrace", LocalPreviousLine: 5, LocalBoundaryLine: 9,
	}
	reason := conflict.reason()
	if !strings.Contains(reason, "previous_line=105") || !strings.Contains(reason, "boundary_line=109") ||
		!strings.Contains(reason, "local_previous_line=5") || !strings.Contains(reason, "local_boundary_line=9") {
		t.Fatalf("rebased incarnation witness must carry both coordinate systems: %q", reason)
	}
	physical := conflict
	physical.LocalPreviousLine, physical.LocalBoundaryLine = 105, 109
	physical.PreviousLine, physical.BoundaryLine = 105, 109
	if reason := physical.reason(); strings.Contains(reason, "local_boundary_line=") {
		t.Fatalf("physical-coordinate incarnation witness must stay terse: %q", reason)
	}
}

// --- #37: typed raw-unavailable caveats -------------------------------------

func TestRawUnavailableCaveatsAreReasonSpecific(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "raw.systrace")
	body := " app-10 (10) [000] d..3 10.0: sched_switch: prev_comm=a prev_pid=10 prev_prio=1 prev_state=S ==> next_comm=b next_pid=11 next_prio=2\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	// (a) Artifact deleted while the in-process index stays alive → open
	// failure must NOT be described as an identity mismatch.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	res := Run(idx, Query{View: "event_search", Limit: 5})
	if len(res.Events) == 0 || res.Events[0].RawUnavailableReason != "artifact_open_failed" {
		t.Fatalf("expected artifact_open_failed rows: %+v", res.Events)
	}
	if !containsSubstring(res.Caveats, "raw_artifact_open_failed=true") {
		t.Fatalf("open failure needs its own caveat: %v", res.Caveats)
	}
	if containsSubstring(res.Caveats, "raw_artifact_identity_mismatch") {
		t.Fatalf("open failure must not claim an identity mismatch: %v", res.Caveats)
	}
	// (b) Artifact replaced with different bytes → identity mismatch keeps
	// its historical wording.
	if err := os.WriteFile(path, []byte(body+body), 0o644); err != nil {
		t.Fatal(err)
	}
	res = Run(idx, Query{View: "event_search", Limit: 5})
	if len(res.Events) == 0 || res.Events[0].RawUnavailableReason != "artifact_identity_changed" {
		t.Fatalf("expected artifact_identity_changed rows: %+v", res.Events)
	}
	if !containsSubstring(res.Caveats, "raw_artifact_identity_mismatch=true") {
		t.Fatalf("identity change must keep the identity caveat: %v", res.Caveats)
	}
	// (c) clock_inverse_unsafe describes a precision limit, not withheld raw
	// text.
	fake := Result{View: "event_search", Events: []EventView{{RawUnavailableReason: "clock_inverse_unsafe"}}}
	caveats := resultCaveats(idx, Query{View: "event_search"}, fake)
	if !containsSubstring(caveats, "raw_source_ts_inverse_unsafe=true") {
		t.Fatalf("clock inverse arm needs its own caveat: %v", caveats)
	}
	if containsSubstring(caveats, "raw_artifact_identity_mismatch") {
		t.Fatalf("clock inverse arm must not claim identity mismatch: %v", caveats)
	}
}

// --- #39: child caveats survive composite merge ----------------------------

func TestCompositeMergePreservesChildCaveats(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "cap.systrace")
	perftrace := filepath.Join(dir, "cap.perftrace")
	bundle := filepath.Join(dir, "cap.tracebundle.json")
	// Two same-named threads in different processes make the thread selector
	// ambiguous, so the relation-scope seed resolution fails with a caveat.
	sysBody := strings.Join([]string{
		" worker-11 (100) [000] d..3 10.000000: sched_switch: prev_comm=worker prev_pid=11 prev_prio=100 prev_state=S ==> next_comm=x next_pid=1 next_prio=1",
		" worker-22 (200) [000] d..3 10.100000: sched_switch: prev_comm=worker prev_pid=22 prev_prio=100 prev_state=S ==> next_comm=x next_pid=1 next_prio=1",
		"",
	}, "\n")
	if err := os.WriteFile(systrace, []byte(sysBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(perftrace, []byte(" app-20 (20) [001] .... 10.001: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=X dso=l.so source=test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"version":"t","systrace":"cap.systrace","artifacts":[
  {"type":"systrace","path":"cap.systrace"},
  {"type":"perftrace","path":"cap.perftrace","perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true}}
],"perf_clock_alignments":[{"artifact_path":"cap.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","calibrated":false}]}`
	if err := os.WriteFile(bundle, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := BuildOptions{
		AllowWindowedParse: true,
		TimeStart:          9.0, TimeStartSet: true, TimeEnd: 11.0, TimeEndSet: true,
		RelationScoped: true, ScopeThread: "worker",
	}
	idx, err := BuildIndexWithOptions(context.Background(), bundle, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !containsSubstring(idx.Caveats, "relation_scope_thread_ambiguous") {
		t.Fatalf("child relation-scope disclosure lost in composite merge: %v", idx.Caveats)
	}
}

// --- #40: one-systrace authority enforced at the finalizer ------------------

func TestPathListLaneInheritsOneSystraceAuthority(t *testing.T) {
	specs := traceArtifactSpecsForPaths([]string{"/tmp/a.systrace", "/tmp/b.systrace"})
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if !specs[0].source.CausalCompatible {
		t.Fatalf("first systrace keeps causal authority: %+v", specs[0].source)
	}
	second := specs[1].source
	if second.CausalCompatible || second.ClockAlignment != TraceClockAlignmentIsolated {
		t.Fatalf("second systrace must be force-isolated by the finalizer: %+v", second)
	}
	if !strings.Contains(second.IsolationReason, "only one systrace causal authority") {
		t.Fatalf("isolation reason must name the authority rule: %q", second.IsolationReason)
	}
	// The bundle lane keeps its historical behavior (regression guard for
	// the rule's relocation into finalizeTraceArtifactSpecs).
	single := traceArtifactSpecsForPaths([]string{"/tmp/a.systrace", "/tmp/a.perftrace"})
	for _, spec := range single {
		if spec.source.Kind == "systrace" && !spec.source.CausalCompatible {
			t.Fatalf("lone systrace must stay authoritative: %+v", spec.source)
		}
	}
}

// --- #41: composite vs direct interface-join consistency --------------------

func TestInterfaceJoinConsistentAcrossEntryPointsUnderRegression(t *testing.T) {
	body := ` app-10 (10) [000] d..3 5.000000: print: B|10|transact[com.test.IFoo:3]
 app-10 (10) [000] d..3 4.500000: binder_transaction: transaction=77 dest_node=1 dest_proc=99 dest_thread=0 reply=0 flags=0x10 code=0x3
 app-10 (10) [000] d..3 4.600000: print: E|10
 server-99 (99) [001] d..3 5.100000: binder_transaction_received: transaction=77
`
	dirA := t.TempDir()
	direct := filepath.Join(dirA, "direct.systrace")
	if err := os.WriteFile(direct, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	dirB := t.TempDir()
	child := filepath.Join(dirB, "capture.systrace")
	if err := os.WriteFile(child, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(dirB, "wrapped.tracebundle.json")
	if err := os.WriteFile(bundle, []byte(`{"version":"t","systrace":"capture.systrace","artifacts":[{"type":"systrace","path":"capture.systrace"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	idxDirect, err := BuildIndex(context.Background(), direct)
	if err != nil {
		t.Fatal(err)
	}
	idxBundle, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	for name, idx := range map[string]*Index{"direct": idxDirect, "bundle": idxBundle} {
		res := BuildIPCGraph(idx, Query{})
		if len(res.Edges) != 1 {
			t.Fatalf("%s: binder edge itself must survive: %+v", name, res.Edges)
		}
		// The trace_span lane order is untrustworthy (physical B/E rollback),
		// so BOTH entry points must withhold the interface join — before this
		// pin, the direct lane published IFoo while the composite lane
		// silently published "" for the same capture.
		if res.Edges[0].Interface != "" {
			t.Fatalf("%s: interface join must fail closed under a trace_span rollback: %+v", name, res.Edges[0])
		}
		if !containsSubstring(res.Caveats, "trace_mark_interface_join_fail_closed=true") {
			t.Fatalf("%s: fail-close needs its typed caveat: %v", name, res.Caveats)
		}
	}
}

// TestInterfaceJoinSurvivesHealthyTrace guards the fail-close gate's
// precision: without a trace_span rollback the interface join must keep
// publishing.
func TestInterfaceJoinSurvivesHealthyTrace(t *testing.T) {
	body := ` app-10 (10) [000] d..3 4.000000: print: B|10|transact[com.test.IFoo:3]
 app-10 (10) [000] d..3 4.500000: binder_transaction: transaction=77 dest_node=1 dest_proc=99 dest_thread=0 reply=0 flags=0x10 code=0x3
 app-10 (10) [000] d..3 4.600000: print: E|10
 server-99 (99) [001] d..3 5.100000: binder_transaction_received: transaction=77
`
	dir := t.TempDir()
	direct := filepath.Join(dir, "healthy.systrace")
	if err := os.WriteFile(direct, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), direct)
	if err != nil {
		t.Fatal(err)
	}
	res := BuildIPCGraph(idx, Query{})
	if len(res.Edges) != 1 || res.Edges[0].Interface != "com.test.IFoo:3" {
		t.Fatalf("healthy trace must keep the verbatim interface join: %+v", res.Edges)
	}
}

// --- #35: direct perftrace capability disclosure ----------------------------

func TestDirectPerftraceCarriesCapabilityUnattestedCaveat(t *testing.T) {
	dir := t.TempDir()
	perftrace := filepath.Join(dir, "solo.perftrace")
	if err := os.WriteFile(perftrace, []byte(" app-20 (20) [001] .... 10.001: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=X dso=l.so source=test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), perftrace)
	if err != nil {
		t.Fatal(err)
	}
	if !containsSubstring(idx.Caveats, "perftrace_capability_unattested=true") {
		t.Fatalf("direct perftrace must disclose unattested capability: %v", idx.Caveats)
	}
	res := Run(idx, Query{View: "event_search", Limit: 5})
	if !containsSubstring(res.Caveats, "perftrace_capability_unattested=true") {
		t.Fatalf("disclosure must reach query results: %v", res.Caveats)
	}
	// The disclosure is perftrace-lane-scoped: a direct systrace never
	// carries it.
	plain := filepath.Join(dir, "plain.systrace")
	if err := os.WriteFile(plain, []byte(" app-10 (10) [000] d..3 10.0: sched_switch: prev_comm=a prev_pid=10 prev_prio=1 prev_state=S ==> next_comm=b next_pid=11 next_prio=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plainIdx, err := BuildIndex(context.Background(), plain)
	if err != nil {
		t.Fatal(err)
	}
	if containsSubstring(plainIdx.Caveats, "perftrace_capability_unattested") {
		t.Fatalf("direct systrace must not carry the perftrace disclosure: %v", plainIdx.Caveats)
	}
	// Bundle children must NOT carry it: their rows pass the capability
	// admission gate instead.
	systrace := filepath.Join(dir, "cap.systrace")
	child := filepath.Join(dir, "cap.perftrace")
	bundle := filepath.Join(dir, "cap.tracebundle.json")
	if err := os.WriteFile(systrace, []byte(" app-10 (10) [000] d..3 10.0: sched_switch: prev_comm=a prev_pid=10 prev_prio=1 prev_state=S ==> next_comm=b next_pid=11 next_prio=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(child, []byte(" app-20 (20) [001] .... 10.001: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=X dso=l.so source=test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"version":"t","systrace":"cap.systrace","artifacts":[
  {"type":"systrace","path":"cap.systrace"},
  {"type":"perftrace","path":"cap.perftrace","perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true}}
],"perf_clock_alignments":[{"artifact_path":"cap.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","calibrated":false}]}`
	if err := os.WriteFile(bundle, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	bundleIdx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if containsSubstring(bundleIdx.Caveats, "perftrace_capability_unattested") {
		t.Fatalf("bundle children pass the admission gate and must not carry the direct-lane caveat: %v", bundleIdx.Caveats)
	}
}
