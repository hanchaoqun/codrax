package tracequery

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// B801-TRACETAIL1: an explicit query can extend past the last physical event,
// but that uncovered suffix is not measured scheduler time.  The trace is
// complete and monotonic, so LastTs is the authoritative artifact boundary.
func TestThreadTimelineDoesNotExtrapolateOpenStatePastArtifactTail(t *testing.T) {
	target := ThreadRef{Comm: "raw", PID: 21, TGID: 20}
	idx := &Index{
		Path:           "/trace/raw-fallback.systrace",
		Size:           256,
		FirstTs:        3.000,
		LastTs:         3.008,
		LineCount:      3,
		TimestampOrder: TraceTimestampOrderMonotonic,
		Events: []Event{
			{Type: EventSchedSwitch, Ts: 3.000, Line: 1, CPU: 5, PrevPID: 0, PrevComm: "idle/5", NextPID: 21, NextComm: "raw"},
			{Type: EventPerfSample, Ts: 3.003, Line: 2, CPU: 5, PID: 21, TGID: 20, Comm: "raw"},
			{Type: EventSchedSwitch, Ts: 3.008, Line: 3, CPU: 5, PrevPID: 21, PrevComm: "raw", PrevState: "R+", NextPID: 0, NextComm: "idle/5"},
		},
	}
	q := Query{PID: 21, TimeStart: 3.000, TimeEnd: 3.010, TimeStartSet: true, TimeEndSet: true}

	tl := ThreadTimeline(idx, q)
	if len(tl.Intervals) != 1 {
		t.Fatalf("only the physical 3.000..3.008 running interval is measurable: %+v", tl.Intervals)
	}
	got := tl.Intervals[0]
	if got.State != StateRunning || math.Abs(got.DurationMs-8) > 1e-6 || math.Abs(got.EndTs-3.008) > 1e-9 {
		t.Fatalf("artifact-tail interval drifted: %+v", got)
	}
	for _, interval := range tl.Intervals {
		if interval.State == StateRunnable {
			t.Fatalf("the uncovered 2ms suffix must not become runnable evidence: %+v", tl.Intervals)
		}
	}
	if !sliceContainsSubstring(tl.Caveats, "trace_artifact_tail_uncovered=true") ||
		!sliceContainsSubstring(tl.Caveats, "uncovered_ms=2.000") {
		t.Fatalf("typed artifact-tail caveat missing: %v", tl.Caveats)
	}

	account := buildTargetWindowStateAccount(idx, tl, true, target, TimeWindow{StartTs: 3.000, EndTs: 3.010}, nil)
	if account == nil || math.Abs(account.WindowMs-10) > 1e-6 || math.Abs(account.TotalMs-8) > 1e-6 || account.RunnableMs != 0 {
		t.Fatalf("requested window and measured account must remain distinct: %+v", account)
	}
}

func TestRootCauseRankDoesNotMintSchedulerSeatFromUncoveredArtifactTail(t *testing.T) {
	idx := &Index{
		Path:           "/trace/raw-fallback.systrace",
		Size:           256,
		FirstTs:        3.000,
		LastTs:         3.008,
		LineCount:      2,
		TimestampOrder: TraceTimestampOrderMonotonic,
		Events: []Event{
			{Type: EventSchedSwitch, Ts: 3.000, Line: 1, CPU: 5, PrevPID: 0, PrevComm: "idle/5", NextPID: 21, NextComm: "raw"},
			{Type: EventSchedSwitch, Ts: 3.008, Line: 2, CPU: 5, PrevPID: 21, PrevComm: "raw", PrevState: "R+", NextPID: 0, NextComm: "idle/5"},
		},
	}
	res := Run(idx, Query{View: "root_cause_rank", PID: 21, TimeStart: 3.000, TimeEnd: 3.010, TimeStartSet: true, TimeEndSet: true})
	if res.RootCauseRank == nil {
		t.Fatal("root cause result missing")
	}
	for _, item := range res.RootCauseRank.Items {
		if item.Type == "scheduler_latency" || item.Type == "runnable_wait" || strings.Contains(item.Type, "runnable") {
			t.Fatalf("uncovered suffix minted a scheduler/runnable root seat: %+v", item)
		}
	}
}

func TestStreamStateClusterDoesNotExtrapolatePastArtifactTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tail.systrace")
	content := "raw-21 (20) [005] .... 3.000000: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=raw next_pid=21 next_prio=52\n" +
		"raw-21 (20) [005] .... 3.008000: sched_switch: prev_comm=raw prev_pid=21 prev_prio=52 prev_state=R+ ==> next_comm=idle/5 next_pid=0 next_prio=120\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := StreamStateCluster(context.Background(), path, Query{
		PID: 21, TimeStart: 3.000, TimeEnd: 3.010, TimeStartSet: true, TimeEndSet: true,
	}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.WindowStats == nil {
		t.Fatal("stream state cluster missing window stats")
	}
	for _, row := range res.WindowStats.RunnableTop {
		if row.Thread.PID == 21 {
			t.Fatalf("streaming fallback minted runnable tail: %+v", row)
		}
	}
	if !sliceContainsSubstring(res.WindowStats.Caveats, "trace_artifact_tail_uncovered=true") {
		t.Fatalf("streaming fallback omitted typed tail caveat: %v", res.WindowStats.Caveats)
	}
}

func sliceContainsSubstring(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
