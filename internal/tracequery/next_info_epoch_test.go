package tracequery

import (
	"strconv"
	"strings"
	"testing"
)

func nextInfoEpochFixture(t *testing.T) (*Index, Query) {
	t.Helper()
	idx := buildTraceIndex(t, "next_info_epoch.systrace", `
       idle/4-0   (    0) [004] .... 0.900000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other next_pid=400 next_prio=120
        bg0-200   (  200) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=bg0 next_pid=200 next_prio=20
       ctrl-300   (  300) [001] .... 1.001000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=000
        app-100   (  100) [000] .... 1.011000: sched_switch: prev_comm=bg0 prev_pid=200 prev_prio=20 prev_state=R+ ==> next_comm=app next_pid=100 next_prio=52 next_info=1,10,2,0,0 cg=top-app
        app-100   (  100) [000] .... 1.012000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=bg0 next_pid=200 next_prio=20
       ctrl-300   (  300) [001] .... 1.020000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=000
        app-100   (  100) [000] .... 1.030000: sched_switch: prev_comm=bg0 prev_pid=200 prev_prio=20 prev_state=R+ ==> next_comm=app next_pid=100 next_prio=52 next_info=10,20,3,1,1,7,8,9 cg=top-app
        app-100   (  100) [000] .... 1.031000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
	`)
	q := Query{
		PID:             100,
		TimeStart:       1.000,
		TimeEnd:         1.040,
		TraceFlavorHint: TraceFlavorHarmonyHitrace,
		MinDurationMs:   0.05,
		Limit:           16,
	}
	return idx, q
}

func TestNextInfoDynamicMasksStayEpochScopedAndAttributeExactRunnable(t *testing.T) {
	idx, q := nextInfoEpochFixture(t)
	stats := ComputeWindowStats(idx, q)
	var got *CPUConstraintSummary
	for i := range stats.CPUConstraints {
		if stats.CPUConstraints[i].Thread.PID == 100 {
			got = &stats.CPUConstraints[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("expected next_info summary: %+v", stats.CPUConstraints)
	}
	if got.EpochTotal != 2 || got.EpochEmitted != 2 || !got.EpochComplete {
		t.Fatalf("two versioned snapshots must remain explicit: %+v", got)
	}
	if len(got.AllowedCPUs) != 0 || len(got.ExcludedCPUs) != 0 {
		t.Fatalf("changing masks must never flatten into one simultaneous allowed/excluded set: %+v", got)
	}
	if got.RestrictionProof != CPUConstraintRestrictionProofEpochScoped ||
		got.RestrictionEpochCount != 2 || !near(got.RestrictedRunnableWaitMs, 20, 0.001) {
		t.Fatalf("root authority must be the exact restricted epoch∩runnable account: %+v", got)
	}
	first, second := got.Epochs[0], got.Epochs[1]
	if first.FieldCount != 5 || len(first.ExtensionFields) != 0 ||
		!near(first.RunnableWaitMs, 10, 0.001) || first.AllowedCPUs[0] != 0 {
		t.Fatalf("five-field epoch drifted: %+v", first)
	}
	if second.FieldCount != 8 || strings.Join(second.ExtensionFields, ",") != "8,9" ||
		!near(second.RunnableWaitMs, 10, 0.001) || second.AllowedCPUs[0] != 4 {
		t.Fatalf("eight-field append-only epoch/tail drifted: %+v", second)
	}
	if !second.LoadKnown || second.Load != 20 || !second.SchedGroupKnown || second.SchedGroup != 3 ||
		!second.ICESBoostKnown || !second.ICESBoost || !second.SMTExpelKnown || second.SMTExpel != 1 ||
		!second.CGIDKnown || second.CGID != 7 {
		t.Fatalf("known rich prefix fields must stay typed on their own epoch: %+v", second)
	}

	rank := BuildRootCauseRank(idx, q)
	for _, item := range rank.Items {
		if item.Type == "cpu_affinity_or_cpuset" && item.Thread.PID == 100 {
			if !near(item.RunnableMs, 20, 0.001) || len(item.CPUConstraintAllowedCPUs) != 0 {
				t.Fatalf("projection owner must use epoch-attributed value without a union mask: %+v", item)
			}
			if !strings.Contains(item.Summary, "restriction_epochs=2") ||
				!strings.Contains(item.Summary, "restricted_runnable=20.000ms") ||
				!strings.Contains(item.Summary, "f=8") ||
				!strings.Contains(item.Summary, "tail=8,9") {
				t.Fatalf("epoch construction must remain auditable on the rank handoff: %q", item.Summary)
			}
			return
		}
	}
	t.Fatalf("exact restricted runnable epoch account did not mint a root contender: %+v", rank.Items)
}

func TestNextInfoMetadataWithoutRunnableJoinStaysContextOnly(t *testing.T) {
	idx := buildTraceIndex(t, "next_info_epoch_context.systrace", `
       idle/4-0   (    0) [004] .... 0.900000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other next_pid=400 next_prio=120
       idle/0-0   (    0) [000] .... 1.010000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52 next_info=1,10,2,0,0
        app-100   (  100) [000] .... 1.011000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
	`)
	q := Query{PID: 100, TimeStart: 1, TimeEnd: 1.02, TraceFlavorHint: TraceFlavorHarmonyHitrace, MinDurationMs: 0.05}
	stats := ComputeWindowStats(idx, q)
	if len(stats.CPUConstraints) != 1 || stats.CPUConstraints[0].EpochTotal != 1 {
		t.Fatalf("authoritative metadata must remain visible as context: %+v", stats.CPUConstraints)
	}
	if stats.CPUConstraints[0].RestrictedRunnableWaitMs != 0 {
		t.Fatalf("no wakeup→sched-in runnable segment means no causal value: %+v", stats.CPUConstraints[0])
	}
	rank := BuildRootCauseRank(idx, q)
	for _, item := range rank.Items {
		if item.Type == "cpu_affinity_or_cpuset" && item.Thread.PID == 100 {
			t.Fatalf("metadata-only next_info must not hard-mint a root cause: %+v", item)
		}
	}
}

func TestCPUConstraintMixedProofsUnionSameRunnableSegment(t *testing.T) {
	idx := buildTraceIndex(t, "next_info_mixed_epoch.systrace", `
       idle/4-0   (    0) [004] .... 0.900000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other next_pid=400 next_prio=120
        bg0-200   (  200) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=bg0 next_pid=200 next_prio=20
       ctrl-300   (  300) [001] .... 1.000500: sched_setaffinity: comm=app pid=100 mask=0x1 cpuset=top-app target_cpu=0 policy=bind
       ctrl-300   (  300) [001] .... 1.001000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=000
        app-100   (  100) [000] .... 1.011000: sched_switch: prev_comm=bg0 prev_pid=200 prev_prio=20 prev_state=R+ ==> next_comm=app next_pid=100 next_prio=52 next_info=1,10,2,0,0 cg=top-app
        app-100   (  100) [000] .... 1.012000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
	`)
	q := Query{PID: 100, TimeStart: 1, TimeEnd: 1.02, TraceFlavorHint: TraceFlavorHarmonyHitrace, MinDurationMs: 0.05}
	stats := ComputeWindowStats(idx, q)
	if len(stats.CPUConstraints) != 1 {
		t.Fatalf("expected one per-thread summary: %+v", stats.CPUConstraints)
	}
	got := stats.CPUConstraints[0]
	if got.EpochTotal != 2 || got.AllowedCPUsAuthority != CPUConstraintAllowedCPUsAuthorityMixedPrecise {
		t.Fatalf("both explicit and kernel snapshot epochs must remain visible: %+v", got)
	}
	if !near(got.Epochs[0].RunnableWaitMs, 10, 0.001) ||
		!near(got.Epochs[1].RunnableWaitMs, 10, 0.001) {
		t.Fatalf("each proof view should retain the exact segment it witnessed: %+v", got.Epochs)
	}
	if !near(got.RestrictedRunnableWaitMs, 10, 0.001) {
		t.Fatalf("the causal owner must union the same physical segment once, got %.6f", got.RestrictedRunnableWaitMs)
	}
}

func TestCPUConstraintEpochDisplayCapNeverCapsAccounting(t *testing.T) {
	idx := &Index{TimestampOrder: TraceTimestampOrderMonotonic}
	idx.Events = append(idx.Events, Event{
		Line: 1, Ts: 0.9, CPU: 4, Type: EventSchedSwitch,
		NextPID: 400, NextComm: "other",
	})
	var segments []runnableWaitSegment
	for i := 0; i < 18; i++ {
		ts := 1 + float64(i+1)*0.001
		idx.Events = append(idx.Events, Event{
			Line: 2 + i, Ts: ts, CPU: 0, Type: EventSchedSwitch,
			NextPID: 100, NextComm: "app",
			NextInfo:            "1," + strconv.Itoa(i) + ",2,0,0",
			NextInfoAffinity:    "1",
			NextInfoAllowedCPUs: []int{0},
			NextInfoLoad:        int32(i),
			NextInfoLoadKnown:   true,
			NextInfoGroup:       2,
			NextInfoGroupKnown:  true,
			NextInfoBoostKnown:  true,
			NextInfoExpelKnown:  true,
		})
		// The raw load token above is deliberately unique to create 18 rich
		// snapshots. Typed fields are pre-populated because this unit test
		// exercises accounting/cap behavior, not parse admission.
		segments = append(segments, runnableWaitSegment{
			thread:  ThreadRef{PID: 100, Comm: "app"},
			startTs: ts - 0.0005, endTs: ts, durationMs: 0.5,
			cpu: 0, cpuKnown: true,
		})
	}
	eventIndexes := make([]int, len(idx.Events))
	for i := range eventIndexes {
		eventIndexes[i] = i
	}
	got := computeCPUConstraintEpochAccounting(
		idx.Events,
		eventIndexes,
		Query{TimeStart: 1, TimeEnd: 1.02},
		segments,
		railCPUAttributionUniverse(idx.Events),
		map[int]string{0: "small", 4: "big"},
		coreCapabilityMap{},
		nil,
	)[100]
	if got.total != 18 || len(got.epochs) != cpuConstraintEpochDisplayCap {
		t.Fatalf("epoch display cap drifted: total=%d emitted=%d", got.total, len(got.epochs))
	}
	if !got.allowedUniform || got.restrictionEpochCount != 18 {
		t.Fatalf("full-roster mask/proof accounting must survive the display cap: %+v", got)
	}
	if !near(got.restrictedRunnableWaitMs, 9, 0.001) {
		t.Fatalf("all 18 half-ms segments must contribute before display truncation, got %.6f", got.restrictedRunnableWaitMs)
	}
}
