package tracequery

import (
	"strings"
	"testing"
)

func TestRunPublishesActionableLifecycleSuppression(t *testing.T) {
	idx := &Index{
		Path:    "lifecycle.systrace",
		FirstTs: 1.000,
		LastTs:  1.500,
		threadIncarnationFailures: []threadIncarnationConflict{{
			PID:          42,
			PreviousLine: 10,
			PreviousTs:   1.100,
			BoundaryLine: 20,
			BoundaryTs:   1.200,
			Signal:       "sched_wakeup_new",
		}},
	}
	result := Run(idx, Query{
		View:         "event_search",
		PID:          42,
		LineStart:    1,
		LineEnd:      30,
		TimeStartSet: false,
		TimeEndSet:   false,
		Limit:        8,
	})
	if len(result.LifecycleSuppressions) != 1 {
		t.Fatalf("lifecycle suppression missing: %+v", result.LifecycleSuppressions)
	}
	got := result.LifecycleSuppressions[0]
	if got.ConflictTID != 42 || got.BoundaryLine != 20 || got.BoundaryTs != 1.200 ||
		!got.AffectsTarget || got.Scope != "target_and_global_pid_keyed_aggregates" {
		t.Fatalf("lifecycle boundary identity/scope drifted: %+v", got)
	}
	for _, want := range []string{"pid_tid_scheduler_aggregates", "thread_timeline", "wakeup_chain"} {
		if !containsExactString(got.AffectedLanes, want) {
			t.Fatalf("affected lanes missing %q: %+v", want, got)
		}
	}
	if !containsExactString(got.PreservedLanes, "cpu_busy_idle") {
		t.Fatalf("CPU-global lane must remain explicitly preserved: %+v", got)
	}
	for _, want := range []string{"pid=42,line_end=19", "pid=42,line_start=20"} {
		if !containsExactString(got.SuggestedQueries, want) {
			t.Fatalf("exact boundary remedy missing %q: %+v", want, got)
		}
	}
	if !containsSubstring(result.Caveats, "thread_incarnation_remedy=true") ||
		!containsSubstring(result.Caveats, "suggested_queries=pid=42,line_end=19|pid=42,line_start=20") {
		t.Fatalf("actionable lifecycle caveat missing: %v", result.Caveats)
	}
}

func TestLifecycleSuppressionWithdrawsFrameOwnershipButNotCPUGlobalLane(t *testing.T) {
	idx := &Index{
		threadIncarnationFailures: []threadIncarnationConflict{{
			PID:          42,
			PreviousLine: 10,
			PreviousTs:   1.100,
			BoundaryLine: 20,
			BoundaryTs:   1.200,
			Signal:       "sched_wakeup_new",
		}},
	}
	suppressions := traceLifecycleSuppressionsForQuery(idx, Query{
		View:      "frame_root_cause_bundle",
		PID:       42,
		LineStart: 1,
		LineEnd:   30,
	}, Result{
		View: "frame_root_cause_bundle",
		ThreadSelection: &ThreadSelectorResolution{
			NameCandidates: []ThreadRef{{PID: 33410, Comm: "ss.hm.ugc.aweme"}},
		},
	})
	if len(suppressions) != 1 {
		t.Fatalf("frame lifecycle suppression missing: %+v", suppressions)
	}
	got := suppressions[0]
	if got.FrameOwnershipStatus != "unavailable" ||
		!containsExactString(got.AffectedLanes, "frame_ownership") ||
		!containsExactString(got.PreservedLanes, "cpu_busy_idle") ||
		!containsExactString(got.CandidateSelectors, "pid=33410") ||
		!containsExactString(got.SuggestedQueries, "target_scope=process,pid=<confirmed_process_id>,span_name=<frame_marker>") {
		t.Fatalf("frame suppression authority/remedy drifted: %+v", got)
	}
	if caveat := traceLifecycleSuppressionCaveat(got); !strings.Contains(caveat, "frame_ownership_status=unavailable") {
		t.Fatalf("frame ownership withdrawal missing from caveat: %s", caveat)
	}
}

func containsExactString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestLifecycleSuppressionDisclosesEveryInWindowConflict(t *testing.T) {
	// LIFEMULTI (2026-07-24): the no_window.txt customer window carried TWO
	// identity boundaries (50173@69326.875412 / 50174@69326.876834) — the
	// first-wins disclosure hid the second. Every in-window boundary now
	// publishes its own suppression row, bounded by the roster cap.
	idx := &Index{
		Path:    "lifecycle.systrace",
		FirstTs: 69326.0,
		LastTs:  69328.0,
		threadIncarnationFailures: []threadIncarnationConflict{
			{PID: 50173, PreviousLine: 52000, PreviousTs: 69326.870000, BoundaryLine: 52108, BoundaryTs: 69326.875412, Signal: "sched_wakeup_new"},
			{PID: 50174, PreviousLine: 52200, PreviousTs: 69326.876000, BoundaryLine: 52300, BoundaryTs: 69326.876834, Signal: "sched_wakeup_new"},
		},
	}
	result := Run(idx, Query{
		View:         "event_search",
		PID:          32788,
		TimeStart:    69326.832743749,
		TimeEnd:      69327.060110624,
		TimeStartSet: true,
		TimeEndSet:   true,
		Limit:        8,
	})
	if len(result.LifecycleSuppressions) != 2 {
		t.Fatalf("both in-window conflicts must publish, got %+v", result.LifecycleSuppressions)
	}
	byTID := map[int]TraceLifecycleSuppression{}
	for _, s := range result.LifecycleSuppressions {
		byTID[s.ConflictTID] = s
	}
	if s, ok := byTID[50173]; !ok || s.BoundaryLine != 52108 || s.BoundaryTs != 69326.875412 {
		t.Fatalf("first boundary identity drifted: %+v", byTID)
	}
	if s, ok := byTID[50174]; !ok || s.BoundaryLine != 52300 || s.BoundaryTs != 69326.876834 {
		t.Fatalf("second boundary identity drifted: %+v", byTID)
	}
	if !containsSubstring(result.Caveats, "conflict_tid=50173") ||
		!containsSubstring(result.Caveats, "conflict_tid=50174") {
		t.Fatalf("both conflicts must reach the caveat face: %v", result.Caveats)
	}
}

func TestLifecycleSuppressionRosterIsBounded(t *testing.T) {
	idx := &Index{Path: "lifecycle.systrace", FirstTs: 1.0, LastTs: 2.0}
	for i := 0; i < traceLifecycleSuppressionMaxConflicts+2; i++ {
		idx.threadIncarnationFailures = append(idx.threadIncarnationFailures, threadIncarnationConflict{
			PID:          9000 + i,
			PreviousLine: 10 + i*10,
			PreviousTs:   1.100 + float64(i)*0.01,
			BoundaryLine: 15 + i*10,
			BoundaryTs:   1.105 + float64(i)*0.01,
			Signal:       "sched_wakeup_new",
		})
	}
	result := Run(idx, Query{
		View: "event_search", PID: 42,
		TimeStart: 1.0, TimeEnd: 2.0, TimeStartSet: true, TimeEndSet: true, Limit: 8,
	})
	if len(result.LifecycleSuppressions) != traceLifecycleSuppressionMaxConflicts {
		t.Fatalf("suppression roster must cap at %d, got %d", traceLifecycleSuppressionMaxConflicts, len(result.LifecycleSuppressions))
	}
}
