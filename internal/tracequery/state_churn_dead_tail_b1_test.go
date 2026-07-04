package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// state_churn_dead_tail_b1_test.go — §7.11 B-1 sequel pins (2026-07-04
// review, F1): the churn open gate rejects StateStopped/StateDead exactly
// like Unknown. B-1 typed T/t/X/Z as non-Unknown so interval faces book them
// honestly, but the churn five-state lanes skip them — so an OPEN dead
// segment contributed only fragments/switches/maxSegment, and a thread's
// exit tail (dead until window end) tripped the maxSegment>=70%-of-total
// suppression gate and silently killed a REAL churn row.
//
// Review counterexample shape (two-window overlay): thread churny-300
// fragments running/sleep/runnable 7 times over [5.0, 5.07] then exits
// (prev_state=Z). With the window cut before any meaningful dead tail the
// churn row exists; widening the window to [5.0, 5.5] swallowed a 430ms dead
// tail segment (430 >= 0.70*70ms total) and the row VANISHED pre-fix. Post-
// fix both windows show the row and fragments never include the dead tail.

const deadTailChurnTrace = `
   starter-9 (    9) [001] .... 5.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=churny next_pid=300 next_prio=100
  churny-300 (  300) [001] .... 5.010000: sched_switch: prev_comm=churny prev_pid=300 prev_prio=100 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
    waker-10 (   10) [000] .... 5.020000: sched_wakeup: comm=churny pid=300 prio=100 target_cpu=001
  churny-300 (  300) [001] .... 5.030000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=churny next_pid=300 next_prio=100
  churny-300 (  300) [001] .... 5.040000: sched_switch: prev_comm=churny prev_pid=300 prev_prio=100 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
    waker-10 (   10) [000] .... 5.050000: sched_wakeup: comm=churny pid=300 prio=100 target_cpu=001
  churny-300 (  300) [001] .... 5.060000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=churny next_pid=300 next_prio=100
  churny-300 (  300) [001] .... 5.070000: sched_switch: prev_comm=churny prev_pid=300 prev_prio=100 prev_state=Z ==> next_comm=idle/1 next_pid=0 next_prio=120
`

func assertChurnRowWithoutDeadTail(t *testing.T, window string, items []ThreadStateChurnSummary) {
	t.Helper()
	var row *ThreadStateChurnSummary
	for i := range items {
		if items[i].Thread.PID == 300 {
			row = &items[i]
			break
		}
	}
	if row == nil {
		t.Fatalf("window %s: churn row for churny-300 missing (dead tail suppressed a real churn row): %+v", window, items)
	}
	// 7 live fragments (3 running, 2 sleep, 2 runnable); the dead tail is
	// NOT a fragment and never becomes maxSegment.
	if row.FragmentCount != 7 || row.StateSwitches != 6 {
		t.Fatalf("window %s: fragments/switches must count live segments only (dead tail excluded), got %+v", window, row)
	}
	if row.TotalMs < 69 || row.TotalMs > 71 {
		t.Fatalf("window %s: five-state total must stay ~70ms, got %+v", window, row)
	}
	if row.MaxSegmentMs >= row.TotalMs*0.70 {
		t.Fatalf("window %s: max segment must stay a live 10ms slice, got %+v", window, row)
	}
	if row.DominantState != string(StateRunning) {
		t.Fatalf("window %s: dominant state must be running, got %+v", window, row)
	}
}

func TestStateChurnDeadTailDoesNotSuppressChurnRow(t *testing.T) {
	idx := buildTraceIndex(t, "dead_tail_churn.systrace", deadTailChurnTrace)
	// Window A ends right after the exit: no meaningful dead tail either way.
	assertChurnRowWithoutDeadTail(t, "[5.0,5.075]",
		computeStateChurnSummaries(idx, Query{TimeStart: 5.0, TimeEnd: 5.075}, 8))
	// Window B swallows the 430ms dead tail — pre-fix the open dead segment
	// closed at window end, became maxSegment (430 >= 0.70*70) and the row
	// vanished. The two windows MUST agree.
	assertChurnRowWithoutDeadTail(t, "[5.0,5.5]",
		computeStateChurnSummaries(idx, Query{TimeStart: 5.0, TimeEnd: 5.5}, 8))
}

func TestStreamStateClusterDeadTailExcludedFromChurnAccumulators(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dead_tail_churn_stream.systrace")
	if err := os.WriteFile(path, []byte(strings.TrimLeft(deadTailChurnTrace, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := StreamStateCluster(context.Background(), path, Query{
		PID:       300,
		TimeStart: 5.0,
		TimeEnd:   5.5,
	}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if res.WindowStats == nil {
		t.Fatalf("expected stream window stats")
	}
	assertChurnRowWithoutDeadTail(t, "stream[5.0,5.5]", res.WindowStats.StateChurn)
}
