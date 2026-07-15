package tracequery

import (
	"fmt"
	"path/filepath"
	"testing"
)

func buildCrossSourceSchedulerHeadFixture(t *testing.T, boundary float64, sourceLines ...[]string) (*Index, []TraceArtifactSource) {
	t.Helper()
	sources := make([]TraceArtifactSource, 0, len(sourceLines))
	lineBase := 0
	for i, lines := range sourceLines {
		path := writeSchedulerCarryTrace(t, fmt.Sprintf("scheduler-source-%d.systrace", i), lines...)
		full, err := BuildIndex(t.Context(), path)
		if err != nil {
			t.Fatal(err)
		}
		if len(full.TraceArtifacts) != 1 {
			t.Fatalf("fixture source ledger drift for source %d: %+v", i, full.TraceArtifacts)
		}
		source := full.TraceArtifacts[0]
		source.VirtualLineBase = lineBase
		lineBase += source.LocalLineCount
		sources = append(sources, source)
	}
	idx := &Index{
		Path:             filepath.Join(t.TempDir(), "cross-source.tracebundle.json"),
		TraceArtifacts:   sources,
		LineCount:        lineBase,
		ScannedLineCount: lineBase,
		Windowed:         true,
		IndexTimeStart:   boundary,
		IndexTimeEnd:     boundary + 0.1,
		LastTs:           boundary + 0.1,
		TimestampOrder:   TraceTimestampOrderMonotonic,
	}
	if err := populateWindowSchedulerHead(t.Context(), idx, boundary); err != nil {
		t.Fatal(err)
	}
	return idx, sources
}

func requireCrossSourceSchedulerHeadFailClosed(t *testing.T, idx *Index, boundary float64) {
	t.Helper()
	head := schedulerHeadForQuery(idx, Query{TimeStart: boundary, TimeEnd: boundary + 0.1})
	if head == nil || head.Complete || !head.CrossSourceCPUUnproven ||
		head.Reason != schedulerHeadCrossSourceStateUnprovenReason {
		t.Fatalf("cross-source scheduler head did not fail closed: %+v", head)
	}
	if len(head.Threads) != 0 || len(head.CPUs) != 0 {
		t.Fatalf("failed cross-source head retained maps: threads=%+v cpus=%+v", head.Threads, head.CPUs)
	}
	coverage := schedulerHeadCoverageForWindow(idx, Query{TimeStart: boundary, TimeEnd: boundary + 0.1}, head)
	if coverage == nil || coverage.Status != "unknown" || coverage.Reason != schedulerHeadCrossSourceStateUnprovenReason {
		t.Fatalf("cross-source reason did not reach public coverage: %+v", coverage)
	}
}

func TestCrossSourceHeadFailsClosedForRedundantWakeupAgainstPriorRunning(t *testing.T) {
	const boundary = 1.0
	sourceA := []string{
		" idle-0 (0) [002] .... 0.100000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=120",
	}
	sourceB := []string{
		" waker-7 (7) [000] .... 0.200000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=000",
	}
	idx, sources := buildCrossSourceSchedulerHeadFixture(t, boundary, sourceA, sourceB)

	left, err := sourceSchedulerHeadSnapshot(t.Context(), sources[0], boundary)
	if err != nil {
		t.Fatal(err)
	}
	right, err := sourceSchedulerHeadSnapshot(t.Context(), sources[1], boundary)
	if err != nil {
		t.Fatal(err)
	}
	if !left.Complete || !right.Complete || len(left.Threads) == 0 || len(right.Threads) == 0 ||
		left.Threads[42].State != StateRunning || right.Threads[42].State != StateRunnable {
		t.Fatalf("fixture must expose the cross-source transition conflict: left=%+v right=%+v", left, right)
	}
	requireCrossSourceSchedulerHeadFailClosed(t, idx, boundary)

	combinedPath := writeSchedulerCarryTrace(t, "redundant-wakeup-single-source.systrace", append(sourceA, sourceB...)...)
	combined, err := BuildIndex(t.Context(), combinedPath)
	if err != nil {
		t.Fatal(err)
	}
	single := schedulerHeadFromEvents(combined, boundary)
	if !single.Complete || single.Threads[42].State != StateRunning || single.Threads[42].CPU != 2 {
		t.Fatalf("single-source canonical replay changed: %+v", single)
	}
}

func TestCrossSourceHeadFailsClosedWhenBlockedReasonRefinesOtherSourceDState(t *testing.T) {
	const boundary = 1.0
	sourceA := []string{
		" target-42 (42) [002] .... 0.100000: sched_switch: prev_comm=target prev_pid=42 prev_prio=120 prev_state=D ==> next_comm=other next_pid=9 next_prio=120",
	}
	sourceB := []string{
		" helper-8 (8) [001] .... 0.200000: sched_blocked_reason: pid=42 iowait=1 caller=f2fs_wait_on_block+0x10/0x20",
	}
	idx, sources := buildCrossSourceSchedulerHeadFixture(t, boundary, sourceA, sourceB)

	left, err := sourceSchedulerHeadSnapshot(t.Context(), sources[0], boundary)
	if err != nil {
		t.Fatal(err)
	}
	right, err := sourceSchedulerHeadSnapshot(t.Context(), sources[1], boundary)
	if err != nil {
		t.Fatal(err)
	}
	if !left.Complete || !right.Complete || left.schedulerEventCount != 1 || right.schedulerEventCount != 0 ||
		len(left.Threads) == 0 || len(right.Threads) != 0 || len(right.CPUs) != 0 || left.Threads[42].State != StateDSleep {
		t.Fatalf("optional refinement-only source must not become a base scheduler contributor: left=%+v right=%+v", left, right)
	}
	head := schedulerHeadForQuery(idx, Query{TimeStart: boundary, TimeEnd: boundary + 0.1})
	if head == nil || !head.Complete || head.Threads[42].State != StateDSleep {
		t.Fatalf("refinement-only sibling polluted the base D carry-in: %+v", head)
	}

	combinedPath := writeSchedulerCarryTrace(t, "blocked-reason-single-source.systrace",
		sourceA[0], sourceB[0])
	combined, err := BuildIndex(t.Context(), combinedPath)
	if err != nil {
		t.Fatal(err)
	}
	single := schedulerHeadFromEvents(combined, boundary)
	if !single.Complete || single.Threads[42].State != StateDSleep {
		t.Fatalf("closing-side blocked_reason must not mutate scheduler head state: %+v", single)
	}
}

func TestCrossSourceHeadIgnoresPhysicalSourceWithoutPrefixSchedulerRows(t *testing.T) {
	const boundary = 1.0
	sourceA := []string{
		" idle-0 (0) [002] .... 0.100000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=120",
	}
	sourceB := []string{
		" app-7 (7) [000] .... 0.200000: tracing_mark_write: B|7|ordinary_span",
	}
	idx, sources := buildCrossSourceSchedulerHeadFixture(t, boundary, sourceA, sourceB)

	nonScheduler, err := sourceSchedulerHeadSnapshot(t.Context(), sources[1], boundary)
	if err != nil {
		t.Fatal(err)
	}
	if !nonScheduler.Complete || nonScheduler.schedulerEventCount != 0 || len(nonScheduler.Threads) != 0 || len(nonScheduler.CPUs) != 0 {
		t.Fatalf("non-scheduler source was misclassified as a scheduler contributor: %+v", nonScheduler)
	}
	head := schedulerHeadForQuery(idx, Query{TimeStart: boundary, TimeEnd: boundary + 0.1})
	if head == nil || !head.Complete || head.CrossSourceCPUUnproven || head.Reason != "" ||
		head.Threads[42].State != StateRunning || head.Threads[42].CPU != 2 {
		t.Fatalf("a source without prefix scheduler rows degraded the valid single contributor: %+v", head)
	}
}
