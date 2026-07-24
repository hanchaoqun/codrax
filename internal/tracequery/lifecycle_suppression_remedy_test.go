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
