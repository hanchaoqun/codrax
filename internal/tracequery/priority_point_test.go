package tracequery

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"unsafe"
)

func priorityTestPoint(source string, ts float64, line int) priorityPhysicalPoint {
	return priorityPhysicalPoint{Source: source, Ts: ts, Line: line}
}

func TestPriorityPointAuthorityWakeupUsesExactAndStableProofs(t *testing.T) {
	idx := &Index{TimestampOrder: TraceTimestampOrderMonotonic, Events: []Event{
		{Line: 1, Ts: 5.000, Type: EventSchedSwitch, NextPID: 200, NextPrio: 20},
		{Line: 2, Ts: 5.005, Type: EventSchedWakeup, PID: 200, WakeePID: 100, WakeePrio: 159},
		{Line: 3, Ts: 5.006, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 20, PrevState: "S"},
	}}
	authority := newPriorityPointAuthority(idx)
	point, ok := authority.pointForEvent(idx.Events[1])
	if !ok {
		t.Fatal("wakeup point source was not resolved")
	}
	got := authority.wakeupRelationAtPoint(TraceFlavorHarmonyHitrace, 100, 200, point)
	if got.Target.Caliber != priorityCaliberExactAtPoint || got.Target.Priority != 159 ||
		got.Subject.Caliber != priorityCaliberClosedRangeStable || got.Subject.Priority != 20 ||
		got.Relation != "lower_priority_waker" || !got.hardEvidence() {
		t.Fatalf("exact wakee + stable waker did not mint the point relation: %+v", got)
	}
	start, _ := authority.pointForEvent(idx.Events[0])
	if after := authority.pointVerdictAt(200, start, priorityPointAfter); after.Caliber != priorityCaliberExactAtPoint || after.Priority != 20 {
		t.Fatalf("sched_switch next priority is not exact on its after side: %+v", after)
	}
	if before := authority.pointVerdictAt(200, start, priorityPointBefore); before.hardEvidence() {
		t.Fatalf("sched_switch next priority leaked onto its before side: %+v", before)
	}
}

func TestPriorityPointAuthorityNearestRemainsAdvisory(t *testing.T) {
	idx := &Index{TimestampOrder: TraceTimestampOrderMonotonic, Events: []Event{
		{Line: 10, Ts: 5.010, Type: EventSchedSwitch, NextPID: 200, NextPrio: 20},
	}}
	authority := newPriorityPointAuthority(idx)
	got := authority.pointVerdictAt(200, priorityTestPoint("compat:index", 5.000, 5), priorityPointAt)
	if got.Priority != 20 || got.Caliber != priorityCaliberAdvisoryNearest || got.hardEvidence() {
		t.Fatalf("future-only sample became point proof: %+v", got)
	}

	unknownOrder := &Index{Events: []Event{
		{Line: 1, Ts: 5.000, Type: EventSchedSwitch, NextPID: 200, NextPrio: 20},
		{Line: 3, Ts: 5.010, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 20, PrevState: "S"},
	}}
	if got := newPriorityPointAuthority(unknownOrder).pointVerdictAt(200, priorityTestPoint("compat:index", 5.005, 2), priorityPointAt); got.hardEvidence() || got.Caliber != priorityCaliberAdvisoryNearest {
		t.Fatalf("unknown timestamp order minted a closed stable range: %+v", got)
	}
}

func TestPriorityPointAuthorityBundleChildOrderProofIsSourceLocal(t *testing.T) {
	tests := []struct {
		name             string
		order            TraceTimestampOrder
		clockRegressions int
	}{
		{name: "regressed child", order: TraceTimestampOrderRegressed, clockRegressions: 1},
		{name: "unknown child", order: TraceTimestampOrderUnknown},
		{name: "clock regression contradicts monotonic label", order: TraceTimestampOrderMonotonic, clockRegressions: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			idx := &Index{
				// Composite events are canonically sorted. This public label must
				// not overwrite either child's physical ordering proof.
				TimestampOrder: TraceTimestampOrderMonotonic,
				TraceArtifacts: []TraceArtifactSource{
					{
						SourcePath: "/trace/regressed.systrace", VirtualLineBase: 0, LocalLineCount: 10,
						CausalCompatible: true, timestampOrder: tc.order, clockRegressions: tc.clockRegressions,
					},
					{
						SourcePath: "/trace/monotonic.systrace", VirtualLineBase: 100, LocalLineCount: 10,
						CausalCompatible: true, timestampOrder: TraceTimestampOrderMonotonic,
					},
				},
				Events: []Event{
					{Line: 1, Ts: 5.000, Type: EventSchedSwitch, NextPID: 200, NextPrio: 20},
					{Line: 101, Ts: 5.000, Type: EventSchedSwitch, NextPID: 300, NextPrio: 30},
					{Line: 3, Ts: 5.010, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 20, PrevState: "S"},
					{Line: 103, Ts: 5.010, Type: EventSchedSwitch, PrevPID: 300, PrevPrio: 30, PrevState: "S"},
				},
			}
			authority := newPriorityPointAuthority(idx)
			if got := authority.pointVerdictAt(200, priorityTestPoint("artifact:0", 5.000, 1), priorityPointAfter); got.Caliber != priorityCaliberExactAtPoint || got.Priority != 20 {
				t.Fatalf("source-local order guard discarded valid exact row evidence: %+v", got)
			}
			if got := authority.pointVerdictAt(200, priorityTestPoint("artifact:0", 5.005, 2), priorityPointAt); got.hardEvidence() || got.Caliber != priorityCaliberAdvisoryNearest {
				t.Fatalf("unproved child order minted a stable range from composite sorting: %+v", got)
			}
			if got := authority.pointVerdictAt(300, priorityTestPoint("artifact:1", 5.005, 102), priorityPointAt); got.Caliber != priorityCaliberClosedRangeStable || got.Priority != 30 {
				t.Fatalf("bad sibling order poisoned independently proved child range: %+v", got)
			}
		})
	}
}

func TestPriorityPointAuthorityMutationPoisonIsScopedOrGlobal(t *testing.T) {
	base := []Event{
		{Line: 1, Ts: 5.000, Type: EventSchedSwitch, NextPID: 200, NextPrio: 20},
		{Line: 2, Ts: 5.000, Type: EventSchedSwitch, NextPID: 300, NextPrio: 30},
		{Line: 3, Ts: 5.002, Type: EventPriorityMutation, Name: "sched_pi_setprio", WakeePID: 200},
		{Line: 4, Ts: 5.005, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 20, PrevState: "S"},
		{Line: 5, Ts: 5.005, Type: EventSchedSwitch, PrevPID: 300, PrevPrio: 30, PrevState: "S"},
	}
	idx := &Index{TimestampOrder: TraceTimestampOrderMonotonic, Events: base}
	authority := newPriorityPointAuthority(idx)
	point := priorityTestPoint("compat:index", 5.003, 3)
	if got := authority.pointVerdictAt(200, point, priorityPointAt); got.hardEvidence() || got.Caliber != priorityCaliberAdvisoryNearest {
		t.Fatalf("scoped PI mutation did not break subject range: %+v", got)
	}
	if got := authority.pointVerdictAt(300, point, priorityPointAt); got.Caliber != priorityCaliberClosedRangeStable || got.Priority != 30 {
		t.Fatalf("scoped PI mutation poisoned a healthy sibling: %+v", got)
	}

	globalEvents := append([]Event(nil), base...)
	globalEvents[2].WakeePID = 0
	global := newPriorityPointAuthority(&Index{TimestampOrder: TraceTimestampOrderMonotonic, Events: globalEvents})
	if got := global.pointVerdictAt(300, point, priorityPointAt); got.hardEvidence() {
		t.Fatalf("unscoped mutation failed to poison the source range: %+v", got)
	}
}

func TestPriorityPointAuthorityNonPositiveSchedSwitchPoisonsOnlyOwningGeneration(t *testing.T) {
	for _, badPriority := range []int{0, -1} {
		t.Run(fmt.Sprintf("priority_%d", badPriority), func(t *testing.T) {
			idx := &Index{TimestampOrder: TraceTimestampOrderMonotonic, Events: []Event{
				{Line: 1, Ts: 5.000, Type: EventSchedSwitch, NextPID: 200, NextPrio: 20},
				{Line: 2, Ts: 5.000, Type: EventSchedSwitch, NextPID: 300, NextPrio: 30},
				{Line: 3, Ts: 5.005, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: badPriority, PrevState: "R", NextPID: 0, NextPrio: 0},
				// PID 0 remains the idle identity and must never acquire a poison scope.
				{Line: 4, Ts: 5.006, Type: EventSchedSwitch, PrevPID: 0, PrevPrio: badPriority, PrevState: "R", NextPID: 0, NextPrio: 0},
				{Line: 5, Ts: 5.010, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 20, PrevState: "S"},
				{Line: 6, Ts: 5.010, Type: EventSchedSwitch, PrevPID: 300, PrevPrio: 30, PrevState: "S"},
			}}
			authority := newPriorityPointAuthority(idx)
			if got := authority.pointVerdictAt(200, priorityTestPoint("compat:index", 5.007, 4), priorityPointAt); got.hardEvidence() {
				t.Fatalf("non-positive sched_switch value bridged a false stable range: %+v", got)
			}
			if got := authority.pointVerdictAt(300, priorityTestPoint("compat:index", 5.007, 4), priorityPointAt); got.Caliber != priorityCaliberClosedRangeStable || got.Priority != 30 {
				t.Fatalf("PID-scoped non-positive poison contaminated a healthy sibling: %+v", got)
			}
			found := false
			for _, mutation := range authority.mutations {
				if mutation.Reason != "nonpositive_sched_priority" {
					continue
				}
				if mutation.PID != 200 || mutation.Point.Source != "compat:index" || mutation.Point.Line != 3 || !mutation.GenerationScoped || !mutation.Generation.known {
					t.Fatalf("non-positive switch poison lost PID/source/generation identity: %+v", mutation)
				}
				found = true
			}
			if !found {
				t.Fatalf("non-positive sched_switch value minted no scoped poison: %+v", authority.mutations)
			}
			for scope := range authority.mutationsByScope {
				if scope.PID == 0 {
					t.Fatalf("idle PID 0 incorrectly acquired a priority poison scope: %+v", scope)
				}
			}
		})
	}

	t.Run("does not cross tid reuse", func(t *testing.T) {
		idx := &Index{
			TimestampOrder: TraceTimestampOrderMonotonic,
			Events: []Event{
				{Line: 1, Ts: 5.000, Type: EventSchedSwitch, NextPID: 200, NextPrio: 20},
				{Line: 2, Ts: 5.002, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 0, PrevState: "R"},
				{Line: 4, Ts: 5.004, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 20, PrevState: "S"},
				{Line: 6, Ts: 5.006, Type: EventSchedSwitch, NextPID: 200, NextPrio: 30},
				{Line: 9, Ts: 5.009, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 30, PrevState: "S"},
			},
			threadIncarnationFailures: []threadIncarnationConflict{{
				PID: 200, PreviousLine: 4, PreviousTs: 5.004,
				BoundaryLine: 5, BoundaryTs: 5.005, Signal: "sched_wakeup_new",
			}},
		}
		authority := newPriorityPointAuthority(idx)
		if got := authority.pointVerdictAt(200, priorityTestPoint("compat:index", 5.003, 3), priorityPointAt); got.hardEvidence() {
			t.Fatalf("non-positive value did not poison its owning generation: %+v", got)
		}
		if got := authority.pointVerdictAt(200, priorityTestPoint("compat:index", 5.007, 7), priorityPointAt); got.Caliber != priorityCaliberClosedRangeStable || got.Priority != 30 {
			t.Fatalf("old-generation non-positive poison contaminated reused TID: %+v", got)
		}
	})
}

func TestPriorityPointAuthoritySameRowNonPositiveCannotBeLaunderedByPositivePeer(t *testing.T) {
	for _, tc := range []struct {
		name               string
		prevPrio, nextPrio int
	}{
		{name: "bad_prev", prevPrio: 0, nextPrio: 20},
		{name: "bad_next", prevPrio: 20, nextPrio: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx := &Index{TimestampOrder: TraceTimestampOrderMonotonic, Events: []Event{
				{Line: 1, Ts: 5.000, Type: EventSchedSwitch, NextPID: 200, NextPrio: 20},
				{Line: 2, Ts: 5.005, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: tc.prevPrio, PrevState: "R", NextPID: 200, NextPrio: tc.nextPrio},
				{Line: 3, Ts: 5.010, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 20, PrevState: "S"},
			}}
			authority := newPriorityPointAuthority(idx)
			point := priorityTestPoint("compat:index", 5.005, 2)
			for _, side := range []priorityPointSide{priorityPointAt, priorityPointBefore, priorityPointAfter} {
				if got := authority.pointVerdictAt(200, point, side); got.hardEvidence() {
					t.Fatalf("same-row positive peer survived non-positive poison on side=%d: %+v", side, got)
				}
			}
			if len(authority.endpointsByPID[200]) != 2 {
				t.Fatalf("poisoned physical point survived endpoint normalization: %+v", authority.endpointsByPID[200])
			}
		})
	}
}

func TestPriorityPointAuthorityUnknownWakePriorityRemainsAbsenceNotPoison(t *testing.T) {
	idx := &Index{TimestampOrder: TraceTimestampOrderMonotonic, Events: []Event{
		{Line: 1, Ts: 5.000, Type: EventSchedSwitch, NextPID: 200, NextPrio: 20},
		{Line: 2, Ts: 5.005, Type: EventSchedWakeup, WakeePID: 200, WakeePrio: 0},
		{Line: 3, Ts: 5.010, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 20, PrevState: "S"},
	}}
	authority := newPriorityPointAuthority(idx)
	if got := authority.pointVerdictAt(200, priorityTestPoint("compat:index", 5.005, 2), priorityPointAt); got.Caliber != priorityCaliberClosedRangeStable || got.Priority != 20 {
		t.Fatalf("an absent/untrusted wake priority was misclassified as an explicit switch poison: %+v", got)
	}
}

func TestSchedulerHeadNonPositiveSchedSwitchPriorityCannotCarryAuthority(t *testing.T) {
	for _, tc := range []struct {
		name  string
		event Event
		pid   int
	}{
		{
			name: "prev", pid: 200,
			event: Event{Line: 1, Ts: 5, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 0, PrevState: "R", NextPID: 300, NextPrio: 30},
		},
		{
			name: "next", pid: 200,
			event: Event{Line: 1, Ts: 5, Type: EventSchedSwitch, PrevPID: 300, PrevPrio: 30, PrevState: "R", NextPID: 200, NextPrio: -1},
		},
		{
			name: "same_pid_bad_prev", pid: 200,
			event: Event{Line: 1, Ts: 5, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 0, PrevState: "R", NextPID: 200, NextPrio: 20},
		},
		{
			name: "same_pid_bad_next", pid: 200,
			event: Event{Line: 1, Ts: 5, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 20, PrevState: "R", NextPID: 200, NextPrio: 0},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := newSchedulerHeadSnapshot(6)
			applySchedulerHeadEvent(snapshot, tc.event)
			state, ok := snapshot.Threads[tc.pid]
			if !ok || !state.PriorityPoisoned || state.Priority > 0 || state.PriorityLine != tc.event.Line || state.PriorityTs != tc.event.Ts {
				t.Fatalf("non-positive switch priority escaped carry-in poison: %+v", state)
			}
			if _, ok := snapshot.Threads[0]; ok {
				t.Fatalf("idle PID 0 became a thread priority authority: %+v", snapshot.Threads[0])
			}
		})
	}
}

func TestPriorityPointAuthorityMalformedSchedulerPoisonScope(t *testing.T) {
	baseEvents := []Event{
		{Line: 1, Ts: 5.000, Type: EventSchedSwitch, NextPID: 200, NextPrio: 20},
		{Line: 2, Ts: 5.000, Type: EventSchedSwitch, NextPID: 300, NextPrio: 30},
		{Line: 5, Ts: 5.010, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 20, PrevState: "S"},
		{Line: 6, Ts: 5.010, Type: EventSchedSwitch, PrevPID: 300, PrevPrio: 30, PrevState: "S"},
	}
	t.Run("unique source PID poisons only its generation", func(t *testing.T) {
		idx := &Index{
			TimestampOrder: TraceTimestampOrderMonotonic, Events: append([]Event(nil), baseEvents...),
			schedulerRowIntegrityFailures: []schedulerRowIntegrityFailure{{
				EventName: "sched_wakeup", Line: 3, Ts: 5.005, PIDs: []int{200}, Fields: []string{"success_invalid"},
			}},
		}
		authority := newPriorityPointAuthority(idx)
		point := priorityTestPoint("compat:index", 5.006, 4)
		if got := authority.pointVerdictAt(200, point, priorityPointAt); got.hardEvidence() || got.Caliber != priorityCaliberAdvisoryNearest {
			t.Fatalf("unique malformed wake row did not poison its exact PID range: %+v", got)
		}
		if got := authority.pointVerdictAt(300, point, priorityPointAt); got.Caliber != priorityCaliberClosedRangeStable || got.Priority != 30 {
			t.Fatalf("PID-scoped malformed wake row poisoned healthy sibling PID: %+v", got)
		}
	})

	t.Run("ambiguous subject poisons the source point", func(t *testing.T) {
		idx := &Index{
			TimestampOrder: TraceTimestampOrderMonotonic, Events: append([]Event(nil), baseEvents...),
			schedulerRowIntegrityFailures: []schedulerRowIntegrityFailure{{
				EventName: "sched_switch", Line: 3, Ts: 5.005, PIDs: []int{200, 300},
				Fields: []string{"prev_state"},
			}},
		}
		authority := newPriorityPointAuthority(idx)
		point := priorityTestPoint("compat:index", 5.006, 4)
		for _, pid := range []int{200, 300} {
			if got := authority.pointVerdictAt(pid, point, priorityPointAt); got.hardEvidence() {
				t.Fatalf("ambiguous malformed switch failed to source-global poison pid=%d: %+v", pid, got)
			}
		}
	})

	t.Run("generation scope does not cross TID reuse", func(t *testing.T) {
		idx := &Index{
			TimestampOrder: TraceTimestampOrderMonotonic,
			Events: []Event{
				{Line: 1, Ts: 5.000, Type: EventSchedSwitch, NextPID: 200, NextPrio: 20},
				{Line: 4, Ts: 5.004, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 20, PrevState: "S"},
				{Line: 6, Ts: 5.006, Type: EventSchedSwitch, NextPID: 200, NextPrio: 30},
				{Line: 9, Ts: 5.009, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 30, PrevState: "S"},
			},
			threadIncarnationFailures: []threadIncarnationConflict{{
				PID: 200, PreviousLine: 4, PreviousTs: 5.004,
				BoundaryLine: 5, BoundaryTs: 5.005, Signal: "sched_wakeup_new",
			}},
			schedulerRowIntegrityFailures: []schedulerRowIntegrityFailure{{
				EventName: "sched_waking", Line: 2, Ts: 5.002, PIDs: []int{200}, Fields: []string{"success_invalid"},
			}},
		}
		authority := newPriorityPointAuthority(idx)
		if got := authority.pointVerdictAt(200, priorityTestPoint("compat:index", 5.003, 3), priorityPointAt); got.hardEvidence() {
			t.Fatalf("malformed row did not poison its owning generation: %+v", got)
		}
		if got := authority.pointVerdictAt(200, priorityTestPoint("compat:index", 5.007, 7), priorityPointAt); got.Caliber != priorityCaliberClosedRangeStable || got.Priority != 30 {
			t.Fatalf("old-generation malformed row poisoned reused TID generation: %+v", got)
		}
	})
}

func prioritySchedulerPoisonBundleIndex() *Index {
	return &Index{
		TimestampOrder: TraceTimestampOrderMonotonic,
		TraceArtifacts: []TraceArtifactSource{
			{
				SourcePath: "/trace/source-a.systrace", VirtualLineBase: 0, LocalLineCount: 10,
				CausalCompatible: true, timestampOrder: TraceTimestampOrderMonotonic,
			},
			{
				SourcePath: "/trace/source-b.systrace", VirtualLineBase: 100, LocalLineCount: 10,
				CausalCompatible: true, timestampOrder: TraceTimestampOrderMonotonic,
			},
		},
		Events: []Event{
			{Line: 1, Ts: 5.000, Type: EventSchedSwitch, NextPID: 200, NextPrio: 20},
			{Line: 5, Ts: 5.010, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 20, PrevState: "S"},
			{Line: 101, Ts: 5.000, Type: EventSchedSwitch, NextPID: 300, NextPrio: 30},
			{Line: 105, Ts: 5.010, Type: EventSchedSwitch, PrevPID: 300, PrevPrio: 30, PrevState: "S"},
		},
	}
}

func TestPriorityPointAuthoritySchedulerPoisonBundleAndCapScope(t *testing.T) {
	assertSourceVerdict := func(t *testing.T, authority *priorityPointAuthority, pid int, source string, line int, wantHard bool) {
		t.Helper()
		got := authority.pointVerdictAt(pid, priorityTestPoint(source, 5.005, line), priorityPointAt)
		if got.hardEvidence() != wantHard {
			t.Fatalf("pid=%d source=%s hard=%t want=%t verdict=%+v", pid, source, got.hardEvidence(), wantHard, got)
		}
	}

	t.Run("source-global malformed row preserves healthy sibling artifact", func(t *testing.T) {
		idx := prioritySchedulerPoisonBundleIndex()
		idx.schedulerRowIntegrityFailures = []schedulerRowIntegrityFailure{{
			EventName: "sched_switch", Line: 3, Ts: 5.005, AffectsAllPIDs: true,
			Fields: []string{"parser_rejected_row"}, SourcePath: "/trace/source-a.systrace",
		}}
		authority := newPriorityPointAuthority(idx)
		assertSourceVerdict(t, authority, 200, "artifact:0", 3, false)
		assertSourceVerdict(t, authority, 300, "artifact:1", 103, true)
	})

	t.Run("zero coordinate disables only a proven source", func(t *testing.T) {
		idx := prioritySchedulerPoisonBundleIndex()
		idx.schedulerRowIntegrityFailures = []schedulerRowIntegrityFailure{{
			EventName: "sched_wakeup_new", Line: 0, Ts: 0, PIDs: []int{200},
			Fields: []string{"pid_invalid"}, SourcePath: "/trace/source-a.systrace",
		}}
		authority := newPriorityPointAuthority(idx)
		assertSourceVerdict(t, authority, 200, "artifact:0", 3, false)
		assertSourceVerdict(t, authority, 300, "artifact:1", 103, true)
	})

	t.Run("unbound zero coordinate fails closed globally", func(t *testing.T) {
		idx := prioritySchedulerPoisonBundleIndex()
		idx.schedulerRowIntegrityFailures = []schedulerRowIntegrityFailure{{
			EventName: "sched_wakeup", Line: 0, Ts: 0, AffectsAllPIDs: true, Fields: []string{"pid_invalid"},
		}}
		authority := newPriorityPointAuthority(idx)
		assertSourceVerdict(t, authority, 200, "artifact:0", 3, false)
		assertSourceVerdict(t, authority, 300, "artifact:1", 103, false)
	})

	t.Run("unbound explicit source cannot be overridden by a valid virtual line", func(t *testing.T) {
		idx := prioritySchedulerPoisonBundleIndex()
		idx.schedulerRowIntegrityFailures = []schedulerRowIntegrityFailure{{
			EventName: "sched_switch", Line: 3, Ts: 5.005, PIDs: []int{200},
			Fields: []string{"parser_rejected_row"}, SourcePath: "/trace/not-a-bundle-child.systrace",
		}}
		authority := newPriorityPointAuthority(idx)
		assertSourceVerdict(t, authority, 200, "artifact:0", 3, false)
		assertSourceVerdict(t, authority, 300, "artifact:1", 103, false)
	})

	t.Run("source-local cap preserves healthy sibling artifact", func(t *testing.T) {
		idx := prioritySchedulerPoisonBundleIndex()
		idx.schedulerRowIntegrityFailures = make([]schedulerRowIntegrityFailure, schedulerRowIntegrityFailureCap)
		for i := range idx.schedulerRowIntegrityFailures {
			idx.schedulerRowIntegrityFailures[i] = schedulerRowIntegrityFailure{EventName: "sched_migrate_task", Line: i + 1, Ts: 4 + float64(i)/1000}
		}
		appendSchedulerRowIntegrityFailure(idx, schedulerRowIntegrityFailure{
			EventName: "sched_wakeup", SourcePath: "/trace/source-a.systrace",
			Line: 3, Ts: 5.005, PIDs: []int{200}, Fields: []string{"success_invalid"},
		})
		if !idx.schedulerRowIntegrityFailuresCapped || idx.schedulerRowIntegrityOverflowGlobal || len(idx.schedulerRowIntegrityOverflowSources) != 1 {
			t.Fatalf("source-local scheduler cap scope drifted: capped=%t global=%t sources=%v",
				idx.schedulerRowIntegrityFailuresCapped, idx.schedulerRowIntegrityOverflowGlobal, idx.schedulerRowIntegrityOverflowSources)
		}
		authority := newPriorityPointAuthority(idx)
		assertSourceVerdict(t, authority, 200, "artifact:0", 3, false)
		assertSourceVerdict(t, authority, 300, "artifact:1", 103, true)
	})

	t.Run("legacy unscoped cap fails closed globally", func(t *testing.T) {
		idx := prioritySchedulerPoisonBundleIndex()
		idx.schedulerRowIntegrityFailuresCapped = true
		authority := newPriorityPointAuthority(idx)
		assertSourceVerdict(t, authority, 200, "artifact:0", 3, false)
		assertSourceVerdict(t, authority, 300, "artifact:1", 103, false)
	})
}

func TestPriorityPointAuthorityInvalidMutationCoordinateWithdrawsRanges(t *testing.T) {
	for _, test := range []struct {
		name  string
		event Event
	}{
		{name: "zero line", event: Event{Line: 0, Ts: 5.005, Type: EventPriorityMutation, Name: "sched_pi_setprio", WakeePID: 200}},
		{name: "nonfinite timestamp", event: Event{Line: 3, Ts: math.NaN(), Type: EventPriorityMutation, Name: "sched_pi_setprio", WakeePID: 200}},
	} {
		t.Run(test.name, func(t *testing.T) {
			idx := &Index{TimestampOrder: TraceTimestampOrderMonotonic, Events: []Event{
				{Line: 1, Ts: 5.000, Type: EventSchedSwitch, NextPID: 200, NextPrio: 20},
				test.event,
				{Line: 5, Ts: 5.010, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 20, PrevState: "S"},
			}}
			if got := newPriorityPointAuthority(idx).pointVerdictAt(200, priorityTestPoint("compat:index", 5.006, 4), priorityPointAt); got.hardEvidence() {
				t.Fatalf("invalid mutation coordinate retained hard range authority: %+v", got)
			}
		})
	}
}

func TestPriorityPointAuthorityRelationScopePriorityClosureParity(t *testing.T) {
	mutationCases := []struct {
		name           string
		row            string
		wantRelation   bool
		wantScopedRows int
	}{
		{
			name:         "unrelated PID mutation is pruned without co-poison",
			row:          "       boost-9 (9) [003] .... 1.003000: sched_pi_setprio: comm=noise pid=300 oldprio=30 newprio=40",
			wantRelation: true,
		},
		{
			name:           "relevant PID mutation is retained and withdraws range",
			row:            "       boost-9 (9) [003] .... 1.003000: sched_pi_setprio: comm=worker pid=200 oldprio=20 newprio=40",
			wantScopedRows: 1,
		},
		{
			name:           "PID zero global mutation is retained and withdraws range",
			row:            "      binder-9 (9) [003] .... 1.003000: binder_set_priority: pid=200 old_prio=20 new_prio=40",
			wantScopedRows: 1,
		},
	}
	for _, tc := range mutationCases {
		t.Run(tc.name, func(t *testing.T) {
			trace := strings.Join([]string{
				"       idle-0 (0) [001] .... 1.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=159",
				"       idle-0 (0) [002] .... 1.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20",
				"       idle-0 (0) [003] .... 1.002000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=noise next_pid=300 next_prio=30",
				tc.row,
				"     worker-200 (200) [002] .... 1.005000: sched_wakeup: comm=app pid=100 prio=159 target_cpu=001",
				"     worker-200 (200) [002] .... 1.006000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120",
				"        app-100 (100) [001] .... 1.010000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120",
				"      noise-300 (300) [003] .... 1.011000: sched_switch: prev_comm=noise prev_pid=300 prev_prio=30 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120",
			}, "\n") + "\n"
			path := filepath.Join(t.TempDir(), "priority-relation-scope.systrace")
			if err := os.WriteFile(path, []byte(trace), 0o644); err != nil {
				t.Fatal(err)
			}
			base := BuildOptions{
				TimeStart: 0.999, TimeEnd: 1.012, TimeStartSet: true, TimeEndSet: true,
				AllowWindowedParse: true, ScopePID: 100,
			}
			full, err := BuildIndexWithOptions(context.Background(), path, base)
			if err != nil {
				t.Fatalf("full index: %v", err)
			}
			scopedOptions := base
			scopedOptions.RelationScoped = true
			scoped, err := BuildIndexWithOptions(context.Background(), path, scopedOptions)
			if err != nil {
				t.Fatalf("relation-scoped index: %v", err)
			}
			if !scoped.RelationScoped || !scoped.relationScopePriorityComplete || !scoped.relationScopeTIDs[100] || !scoped.relationScopeTIDs[200] {
				t.Fatalf("parser did not publish exact priority closure token: scoped=%t complete=%t tids=%v",
					scoped.RelationScoped, scoped.relationScopePriorityComplete, scoped.relationScopeTIDs)
			}
			mutationRows := 0
			for _, event := range scoped.Events {
				if event.Type == EventPriorityMutation {
					mutationRows++
				}
			}
			if mutationRows != tc.wantScopedRows {
				t.Fatalf("relation-scoped mutation closure rows=%d want=%d events=%+v", mutationRows, tc.wantScopedRows, scoped.Events)
			}
			verdict := func(idx *Index) priorityPointRelationVerdict {
				authority := newPriorityPointAuthority(idx)
				for _, event := range idx.Events {
					if event.Type == EventSchedWakeup && event.WakeePID == 100 {
						point, ok := authority.pointForEvent(event)
						if !ok {
							t.Fatalf("wakeup point source missing: %+v", event)
						}
						return authority.wakeupRelationAtPoint(TraceFlavorHarmonyHitrace, 100, 200, point)
					}
				}
				t.Fatalf("wakeup row absent")
				return priorityPointRelationVerdict{}
			}
			fullVerdict, scopedVerdict := verdict(full), verdict(scoped)
			if fullVerdict.hardEvidence() != tc.wantRelation || scopedVerdict.hardEvidence() != tc.wantRelation ||
				fullVerdict.Relation != scopedVerdict.Relation || fullVerdict.Subject.Caliber != scopedVerdict.Subject.Caliber {
				t.Fatalf("full/scoped priority verdict drift: want_relation=%t full=%+v scoped=%+v", tc.wantRelation, fullVerdict, scopedVerdict)
			}
		})
	}
}

func TestRelationScopePriorityMergeRequiresEveryEligibleSchedulerVote(t *testing.T) {
	healthy := &Index{RelationScoped: true, relationScopePriorityComplete: true}
	incomplete := &Index{RelationScoped: true, relationScopePriorityComplete: false}
	notScoped := &Index{RelationScoped: false, relationScopePriorityComplete: true}

	t.Run("healthy scheduler survives non-voting siblings", func(t *testing.T) {
		var merge relationScopePriorityMerge
		merge.observeAdmitted(TraceArtifactSource{Kind: "systrace", CausalCompatible: true}, healthy)
		merge.observeAdmitted(TraceArtifactSource{Kind: "perftrace", CausalCompatible: true}, incomplete)
		merge.observeAdmitted(TraceArtifactSource{Kind: "systrace", CausalCompatible: false}, incomplete)
		merge.observeAdmitted(TraceArtifactSource{Kind: "systrace", CausalCompatible: true}, notScoped)
		if !merge.complete() || merge.voters != 1 {
			t.Fatalf("perf/isolated/unscoped siblings changed the one eligible vote: %+v", merge)
		}
	})

	t.Run("one incomplete eligible scheduler fails the AND", func(t *testing.T) {
		var merge relationScopePriorityMerge
		merge.observeAdmitted(TraceArtifactSource{Kind: "systrace", CausalCompatible: true}, healthy)
		merge.observeAdmitted(TraceArtifactSource{Kind: "ftrace", CausalCompatible: true}, incomplete)
		if merge.complete() || merge.voters != 2 {
			t.Fatalf("incomplete eligible scheduler did not withdraw composite closure: %+v", merge)
		}
	})

	t.Run("no eligible voter cannot mint closure", func(t *testing.T) {
		var merge relationScopePriorityMerge
		merge.observeAdmitted(TraceArtifactSource{Kind: "perftrace", CausalCompatible: true}, healthy)
		merge.observeAdmitted(TraceArtifactSource{Kind: "systrace", CausalCompatible: false}, healthy)
		if merge.complete() || merge.voters != 0 {
			t.Fatalf("non-voters minted composite closure: %+v", merge)
		}
	})
}

func TestTraceBundlePropagatesRelationScopePriorityClosureFromSchedulerChild(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "priority.systrace")
	perf := filepath.Join(dir, "priority.perftrace")
	bundle := filepath.Join(dir, "priority.tracebundle.json")
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(strings.TrimSpace(body)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(primary, `
       idle-0 (0) [001] .... 1.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=159
       idle-0 (0) [002] .... 1.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [002] .... 1.005000: sched_wakeup: comm=app pid=100 prio=159 target_cpu=001
     worker-200 (200) [002] .... 1.006000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.010000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`)
	write(perf, `app-100 (100) [001] .... 1.004000: perf_sample: cpu=1 pid=100 tid=100 period=1 event=cpu-cycles symbol=App dso=libapp.so source=test`)
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"priority.systrace",
  "artifacts":[
    {"type":"systrace","path":"priority.systrace"},
    {"type":"perftrace","path":"priority.perftrace","perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true}}
  ],
  "perf_clock_alignments":[
    {"artifact_path":"priority.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","confidence":"same_domain","calibrated":false}
  ]
}`)

	idx, err := BuildIndexWithOptions(context.Background(), bundle, BuildOptions{
		TimeStart: 0.999, TimeEnd: 1.011, TimeStartSet: true, TimeEndSet: true,
		AllowWindowedParse: true, RelationScoped: true, ScopePID: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !idx.RelationScoped || !idx.relationScopePriorityComplete ||
		!idx.relationScopeTIDs[100] || !idx.relationScopeTIDs[200] {
		t.Fatalf("bundle lost its admitted scheduler child's closure: scoped=%t complete=%t tids=%v artifacts=%+v",
			idx.RelationScoped, idx.relationScopePriorityComplete, idx.relationScopeTIDs, idx.TraceArtifacts)
	}
	authority := newPriorityPointAuthority(idx)
	for _, event := range idx.Events {
		if event.Type != EventSchedWakeup || event.WakeePID != 100 {
			continue
		}
		point, ok := authority.pointForEvent(event)
		if !ok {
			t.Fatalf("bundle wakeup lost physical source: %+v", event)
		}
		if verdict := authority.wakeupRelationAtPoint(TraceFlavorHarmonyHitrace, 100, 200, point); !verdict.hardEvidence() || verdict.Relation != "lower_priority_waker" {
			t.Fatalf("composite closure did not authorize the primary child's exact relation: %+v", verdict)
		}
		return
	}
	t.Fatal("primary scheduler wakeup was not admitted")
}

func TestPriorityPointAuthorityUsesPhysicalLineAtSameTimestamp(t *testing.T) {
	idx := &Index{TimestampOrder: TraceTimestampOrderMonotonic, Events: []Event{
		{Line: 10, Ts: 5.000, Type: EventSchedSwitch, NextPID: 200, NextPrio: 20},
		{Line: 20, Ts: 5.000, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 20, PrevState: "S"},
	}}
	point := priorityTestPoint("compat:index", 5.000, 15)
	if got := newPriorityPointAuthority(idx).pointVerdictAt(200, point, priorityPointAt); got.Caliber != priorityCaliberClosedRangeStable {
		t.Fatalf("same-ts physical-line interval was not proved: %+v", got)
	}
	idx.Events = append(idx.Events, Event{Line: 15, Ts: 5.000, Type: EventPriorityMutation, Name: "binder_set_priority"})
	if got := newPriorityPointAuthority(idx).pointVerdictAt(200, point, priorityPointAt); got.hardEvidence() {
		t.Fatalf("same-ts intervening global mutation did not break the range: %+v", got)
	}
}

func TestPriorityPointAuthoritySlicesCrossCPURelationByStableRange(t *testing.T) {
	idx := &Index{TimestampOrder: TraceTimestampOrderMonotonic, Events: []Event{
		{Line: 1, Ts: 5.000, Type: EventSchedSwitch, PrevPID: 100, PrevPrio: 159, PrevState: "S"},
		{Line: 2, Ts: 5.000, Type: EventSchedSwitch, NextPID: 200, NextPrio: 140},
		{Line: 3, Ts: 5.004, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 140, PrevState: "S"},
		{Line: 4, Ts: 5.005, Type: EventSchedSwitch, NextPID: 200, NextPrio: 159},
		{Line: 5, Ts: 5.009, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 159, PrevState: "S"},
		{Line: 6, Ts: 5.010, Type: EventSchedWakeup, WakeePID: 100, WakeePrio: 159},
	}}
	intervals := []Interval{
		{Thread: ThreadRef{PID: 200}, State: StateRunning, CPU: 7, CPUKnown: true, StartTs: 5.001, EndTs: 5.004, DurationMs: 3, StartLine: 2, EndLine: 3},
		{Thread: ThreadRef{PID: 200}, State: StateRunning, CPU: 7, CPUKnown: true, StartTs: 5.005, EndTs: 5.009, DurationMs: 4, StartLine: 4, EndLine: 5},
	}
	authority := newPriorityPointAuthority(idx)
	slices := authority.lowerPriorityRelationSlices(TraceFlavorHarmonyHitrace, 100, 200, 1, intervals)
	if len(slices) != 1 || slices[0].TargetPriority != 159 || slices[0].DependencyPriority != 140 ||
		slices[0].Relation != "lower_priority_dependency" || slices[0].Interval.CPU != 7 ||
		!near(slices[0].Interval.DurationMs, 3, 0.000001) {
		t.Fatalf("relation-scoped cross-CPU slice wrong: %+v", slices)
	}
	if generic := authority.lowerPriorityRelationSlices(TraceFlavorGenericFtrace, 100, 200, 1, intervals); len(generic) != 0 {
		t.Fatalf("generic raw priorities minted a numeric relation: %+v", generic)
	}

	relationScoped := &Index{
		TimestampOrder:    idx.TimestampOrder,
		Events:            append([]Event(nil), idx.Events...),
		RelationScoped:    true,
		relationScopeTIDs: map[int]bool{100: true, 200: true},
	}
	if got := newPriorityPointAuthority(relationScoped).lowerPriorityRelationSlices(TraceFlavorHarmonyHitrace, 100, 200, 1, intervals); len(got) != 0 {
		t.Fatalf("hand-built relation index without parser closure token claimed completeness: %+v", got)
	}

	wrongSubject := intervals[0]
	wrongSubject.Thread.PID = 300
	if got := authority.lowerPriorityRelationSlices(TraceFlavorHarmonyHitrace, 100, 200, 1, []Interval{wrongSubject}); len(got) != 0 {
		t.Fatalf("an interval for another TID authorized a dependency relation: %+v", got)
	}
}

func TestPriorityIntervalProvenanceBindsWakeupLineToIntervalStart(t *testing.T) {
	idx := &Index{
		TimestampOrder: TraceTimestampOrderMonotonic,
		threadIncarnationFailures: []threadIncarnationConflict{{
			PID: 200, PreviousLine: 1, PreviousTs: 5.001,
			BoundaryLine: 5, BoundaryTs: 5.005, Signal: "sched_wakeup_new",
		}},
	}
	authority := newPriorityPointAuthority(idx)
	interval := Interval{
		Thread: ThreadRef{PID: 200}, State: StateRunnable,
		StartTs: 5.001, EndTs: 5.009, DurationMs: 8,
		StartLine: 1, WakeupLine: 2,
	}
	source, generation, ok := authority.intervalPriorityProvenance(interval, 200)
	if !ok || source != "compat:index" || !generation.contains(interval.StartTs, interval.WakeupLine) {
		t.Fatalf("opening wakeup coordinate did not bind to the interval-start generation: source=%q generation=%+v ok=%t", source, generation, ok)
	}
	if generation.contains(interval.EndTs, interval.WakeupLine) {
		t.Fatalf("fixture did not separate start/end generations: %+v", generation)
	}
}

func TestPriorityPointAuthorityCarriesOnlyRealUnpoisonedHeadEndpoint(t *testing.T) {
	windowed := &Index{
		Windowed: true, TimestampOrder: TraceTimestampOrderMonotonic,
		Events: []Event{{Line: 2, Ts: 5.010, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 20, PrevState: "S"}},
	}
	head := newSchedulerHeadSnapshot(5.000)
	head.Complete = true
	head.Threads[200] = schedulerHeadThread{
		Thread: ThreadRef{PID: 200}, Priority: 20,
		PriorityTs: 4.900, PriorityLine: 1,
	}
	windowed.setSchedulerHead(head)
	q := Query{TimeStart: 5.000, TimeEnd: 5.020}
	point := priorityTestPoint("compat:index", 5.005, 1)
	withHead := newPriorityPointAuthorityForQuery(windowed, q)
	if got := withHead.pointVerdictAt(200, point, priorityPointAt); got.Caliber != priorityCaliberClosedRangeStable || got.Start.Ts != 4.900 || got.Start.Line != 1 {
		t.Fatalf("real head endpoint did not close the windowed range: %+v", got)
	}
	if got := newPriorityPointAuthority(windowed).pointVerdictAt(200, point, priorityPointAt); got.hardEvidence() {
		t.Fatalf("windowed event subset fabricated an opening endpoint: %+v", got)
	}

	poisoned := newSchedulerHeadSnapshot(5.000)
	poisoned.Complete = true
	poisoned.Threads[200] = schedulerHeadThread{
		Thread: ThreadRef{PID: 200}, Priority: 20, PriorityPoisoned: true,
		PriorityTs: 4.900, PriorityLine: 1,
	}
	windowed.setSchedulerHead(poisoned)
	if got := newPriorityPointAuthorityForQuery(windowed, q).pointVerdictAt(200, point, priorityPointAt); got.hardEvidence() {
		t.Fatalf("poisoned head priority was injected as an endpoint: %+v", got)
	}

	conflictingWindow := &Index{
		Windowed: true, TimestampOrder: TraceTimestampOrderMonotonic,
		Events: []Event{
			{Line: 1, Ts: 4.900, Type: EventSchedSwitch, NextPID: 200, NextPrio: 30},
			{Line: 2, Ts: 5.010, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 20, PrevState: "S"},
		},
	}
	conflicting := newSchedulerHeadSnapshot(5.000)
	conflicting.Complete = true
	conflicting.Threads[200] = schedulerHeadThread{
		Thread: ThreadRef{PID: 200}, Priority: 20,
		PriorityTs: 4.900, PriorityLine: 1,
	}
	conflictingWindow.setSchedulerHead(conflicting)
	conflictAuthority := newPriorityPointAuthorityForQuery(conflictingWindow, q)
	if got := conflictAuthority.pointVerdictAt(200, priorityTestPoint("compat:index", 4.900, 1), priorityPointAt); got.hardEvidence() {
		t.Fatalf("conflicting carried and in-trace endpoints elected a hard value: %+v", got)
	}
	if got := conflictAuthority.pointVerdictAt(200, point, priorityPointAt); got.hardEvidence() {
		t.Fatalf("conflicting carried endpoint closed a stable range: %+v", got)
	}
}

func TestPriorityPointAuthorityCancellationPublishesNoPartialLedger(t *testing.T) {
	events := make([]Event, 0, 1024)
	for i := 0; i < 1024; i++ {
		events = append(events, Event{
			Line: i + 1, Ts: 5 + float64(i)/1e6,
			Type: EventSchedSwitch, NextPID: 200, NextPrio: 20,
		})
	}
	idx := &Index{TimestampOrder: TraceTimestampOrderMonotonic, Events: events}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runCancel := newRunCancelState(ctx)
	authority := newPriorityPointAuthorityWithCancel(idx, runCancel)
	if authority.buildComplete || len(authority.endpointsByPID) != 0 || len(authority.stableByPID) != 0 || len(authority.mutations) != 0 {
		t.Fatalf("pre-fired construction retained a partial ledger: %+v", authority)
	}
	if got := authority.pointVerdictAt(200, priorityTestPoint("compat:index", 5.0001, 100), priorityPointAt); got.Caliber != priorityCaliberUnknown || got.Priority != 0 {
		t.Fatalf("incomplete authority published a point verdict: %+v", got)
	}

	midBuildContext := &priorityCancelAfterChecksContext{
		Context: context.Background(), done: make(chan struct{}), cancelAfter: 2,
	}
	largeEvents := make([]Event, 0, 2*runCancelSampleMask)
	for i := 0; i < cap(largeEvents); i++ {
		largeEvents = append(largeEvents, Event{
			Line: i + 1, Ts: 6 + float64(i)/1e6,
			Type: EventSchedSwitch, NextPID: 300, NextPrio: 140,
		})
	}
	midBuild := newPriorityPointAuthorityWithCancel(
		&Index{TimestampOrder: TraceTimestampOrderMonotonic, Events: largeEvents},
		newRunCancelState(midBuildContext),
	)
	if midBuild.buildComplete || len(midBuild.endpointsByPID) != 0 || len(midBuild.stableByPID) != 0 || len(midBuild.mutationsByScope) != 0 {
		t.Fatalf("mid-build cancellation retained a partial ledger: %+v", midBuild)
	}
}

func TestPriorityPointAuthorityRelationCancellationDiscardsPartialSlices(t *testing.T) {
	idx := &Index{TimestampOrder: TraceTimestampOrderMonotonic, Events: []Event{
		{Line: 1, Ts: 5.000, Type: EventSchedSwitch, NextPID: 100, NextPrio: 159},
		{Line: 2, Ts: 5.000, Type: EventSchedSwitch, NextPID: 200, NextPrio: 20},
		{Line: 3, Ts: 5.010, Type: EventSchedSwitch, PrevPID: 100, PrevPrio: 159, PrevState: "S"},
		{Line: 4, Ts: 5.010, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 20, PrevState: "S"},
	}}
	authority := newPriorityPointAuthority(idx)
	ctx := &priorityCancelAfterChecksContext{
		Context: context.Background(), done: make(chan struct{}), cancelAfter: 4,
	}
	authority.cancel = newRunCancelState(ctx)
	// Arrange for cancellation after one relation slice has been appended:
	// entry sample #1; range-intersection tick; post-intersection sample #2;
	// interval tick samples #3; post-build sample #4 fires and discards out.
	authority.cancel.units = runCancelSampleMask - 1
	intervals := []Interval{{
		Thread: ThreadRef{PID: 200}, State: StateRunning,
		StartTs: 5.001, EndTs: 5.009, DurationMs: 8, StartLine: 2, EndLine: 4,
	}}
	if got := authority.lowerPriorityRelationSlices(TraceFlavorHarmonyHitrace, 100, 200, 1, intervals); got != nil {
		t.Fatalf("canceled relation builder published partial slices: %+v", got)
	}
	if !ctx.canceled || !authority.cancel.fired() {
		t.Fatalf("cancellation fixture did not fire: checks=%d state=%+v", ctx.checks, authority.cancel)
	}
}

type priorityCancelAfterChecksContext struct {
	context.Context
	done        chan struct{}
	checks      int
	cancelAfter int
	canceled    bool
}

func (c *priorityCancelAfterChecksContext) Done() <-chan struct{} { return c.done }

func (c *priorityCancelAfterChecksContext) Err() error {
	c.checks++
	if !c.canceled && c.checks >= c.cancelAfter {
		close(c.done)
		c.canceled = true
	}
	if c.canceled {
		return context.Canceled
	}
	return nil
}

func TestPriorityPointAuthorityLargeInputRetainsLinearEndpoints(t *testing.T) {
	const count = 32 * 1024
	events := make([]Event, 0, count)
	for i := 0; i < count; i++ {
		events = append(events, Event{
			Line: i + 1, Ts: 5 + float64(i)/1e6,
			Type: EventSchedSwitch, NextPID: 200, NextPrio: 140,
		})
	}
	authority := newPriorityPointAuthority(&Index{TimestampOrder: TraceTimestampOrderMonotonic, Events: events})
	if !authority.buildComplete || len(authority.endpointsByPID) != 1 || len(authority.endpointsByPID[200]) != count {
		t.Fatalf("large authority endpoint account drifted: complete=%t pids=%d endpoints=%d",
			authority.buildComplete, len(authority.endpointsByPID), len(authority.endpointsByPID[200]))
	}
	// Equal adjacent endpoints coalesce into one range; retained range state is
	// O(PID/runs), not a second O(events) mirror.
	if len(authority.stableByPID[200]) != 1 || len(authority.mutations) != 0 {
		t.Fatalf("large authority range compaction drifted: ranges=%d mutations=%d", len(authority.stableByPID[200]), len(authority.mutations))
	}
}

func TestPriorityPointAuthorityManyRangesLookupAndPoisonRetentionAreLedgerBounded(t *testing.T) {
	const rangeCount = 4 * 1024
	events := make([]Event, 0, rangeCount*2)
	for i := 0; i < rangeCount; i++ {
		priority := 20
		if i%2 != 0 {
			priority = 30
		}
		line := i*3 + 1
		start := 5 + float64(i)/1000
		events = append(events,
			Event{Line: line, Ts: start, Type: EventSchedSwitch, NextPID: 200, NextPrio: priority},
			Event{Line: line + 2, Ts: start + 0.0005, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: priority, PrevState: "S"},
		)
	}
	authority := newPriorityPointAuthority(&Index{TimestampOrder: TraceTimestampOrderMonotonic, Events: events})
	if !authority.buildComplete || len(authority.stableByPID[200]) != rangeCount {
		t.Fatalf("many-range ledger shape drifted: complete=%t ranges=%d", authority.buildComplete, len(authority.stableByPID[200]))
	}
	// Repeated probes across a large dynamic range ledger guard both exact
	// binary lookup and nearest binary bracketing. A structural AST pin guards
	// the complexity class itself; this fixture guards behavior at scale.
	for i := 0; i < rangeCount; i += 4 {
		line := i*3 + 1
		start := 5 + float64(i)/1000
		if got := authority.pointVerdictAt(200, priorityTestPoint("compat:index", start+0.00025, line+1), priorityPointAt); got.Caliber != priorityCaliberClosedRangeStable || got.Priority != 20 {
			t.Fatalf("binary stable-range lookup drift at range %d: %+v", i, got)
		}
		if got := authority.rangeVerdict(200,
			priorityTestPoint("compat:index", start+0.0001, line),
			priorityTestPoint("compat:index", start+0.0004, line+2)); got.Caliber != priorityCaliberClosedRangeStable || got.Priority != 20 {
			t.Fatalf("binary covering-range lookup drift at range %d: %+v", i, got)
		}
		if got := authority.pointVerdictAt(200, priorityTestPoint("compat:index", start+0.00065, line+2), priorityPointAt); got.Caliber != priorityCaliberAdvisoryNearest || got.Priority != 20 {
			t.Fatalf("binary nearest lookup drift at gap %d: %+v", i, got)
		}
	}

	const mutationCount = 8 * 1024
	mutations := make([]Event, 0, mutationCount)
	for i := 0; i < mutationCount; i++ {
		mutations = append(mutations, Event{
			Line: i + 1, Ts: 10 + float64(i)/1e6, Type: EventPriorityMutation,
			Name: "sched_pi_setprio", WakeePID: 200,
		})
	}
	poison := newPriorityPointAuthority(&Index{TimestampOrder: TraceTimestampOrderMonotonic, Events: mutations})
	indexedMutations := 0
	for scope, points := range poison.mutationsByScope {
		if scope.Source == "compat:index" && scope.PID == 200 {
			indexedMutations += len(points)
		}
	}
	if !poison.buildComplete || len(poison.mutations) != mutationCount || indexedMutations != mutationCount {
		t.Fatalf("poison witness ledger is not exactly input-bounded: complete=%t mutations=%d indexed=%d",
			poison.buildComplete, len(poison.mutations), indexedMutations)
	}
}

func TestPriorityPointAuthorityAllocationAndRetainedSlope(t *testing.T) {
	const (
		smallCount                  = 16 * 1024
		largeCount                  = 32 * 1024
		maxTotalAllocBytesPerEvent  = uint64(2 * 1024)
		maxRetainedBytesPerEventEst = uint64(768)
	)
	// Warm one-time runtime/compiler paths outside the measurement.
	_ = newPriorityPointAuthority(&Index{TimestampOrder: TraceTimestampOrderMonotonic, Events: []Event{
		{Line: 1, Ts: 1, Type: EventSchedSwitch, NextPID: 200, NextPrio: 20},
	}})
	previousGCPercent := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(previousGCPercent)

	smallAlloc, smallRetained := measurePriorityAuthorityResources(smallCount)
	largeAlloc, largeRetained := measurePriorityAuthorityResources(largeCount)
	smallAllocPerEvent := smallAlloc / smallCount
	largeAllocPerEvent := largeAlloc / largeCount
	smallRetainedPerEvent := smallRetained / smallCount
	largeRetainedPerEvent := largeRetained / largeCount
	t.Logf("TQ-PRIORITY-POINT-AUTHORITY resource evidence: events=%d total_alloc=%d bytes total_alloc_per_event=%d retained_estimate=%d bytes retained_per_event_estimate=%d",
		smallCount, smallAlloc, smallAllocPerEvent, smallRetained, smallRetainedPerEvent)
	t.Logf("TQ-PRIORITY-POINT-AUTHORITY resource evidence: events=%d total_alloc=%d bytes total_alloc_per_event=%d retained_estimate=%d bytes retained_per_event_estimate=%d",
		largeCount, largeAlloc, largeAllocPerEvent, largeRetained, largeRetainedPerEvent)
	if largeAllocPerEvent > maxTotalAllocBytesPerEvent {
		t.Fatalf("priority authority allocation=%d bytes/event exceeds conservative %d-byte budget", largeAllocPerEvent, maxTotalAllocBytesPerEvent)
	}
	if largeRetainedPerEvent > maxRetainedBytesPerEventEst {
		t.Fatalf("priority authority retained estimate=%d bytes/event exceeds conservative %d-byte budget", largeRetainedPerEvent, maxRetainedBytesPerEventEst)
	}
	// Doubling the event ledger may cross allocator bucket boundaries, so the
	// slope ratchet leaves 33% + 128 bytes/event headroom while still rejecting
	// a hidden quadratic sidecar.
	if largeAllocPerEvent > smallAllocPerEvent+smallAllocPerEvent/3+128 {
		t.Fatalf("priority authority allocation slope is super-linear: small=%d large=%d bytes/event", smallAllocPerEvent, largeAllocPerEvent)
	}
	if largeRetainedPerEvent > smallRetainedPerEvent+smallRetainedPerEvent/3+128 {
		t.Fatalf("priority authority retained slope is super-linear: small=%d large=%d bytes/event", smallRetainedPerEvent, largeRetainedPerEvent)
	}
}

func measurePriorityAuthorityResources(count int) (uint64, uint64) {
	events := make([]Event, 0, count)
	for i := 0; i < count; i++ {
		events = append(events, Event{
			Line: i + 1, Ts: 5 + float64(i)/1e6,
			Type: EventSchedSwitch, NextPID: 200, NextPrio: 20,
		})
	}
	idx := &Index{TimestampOrder: TraceTimestampOrderMonotonic, Events: events}
	// Generation metadata is a separate Index-level ledger; prebuild it so the
	// measured bytes are the priority authority itself.
	ensureThreadGenerationMetadata(idx)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	authority := newPriorityPointAuthority(idx)
	runtime.ReadMemStats(&after)
	retained := priorityAuthorityRetainedBytesEstimate(authority)
	runtime.KeepAlive(authority)
	return after.TotalAlloc - before.TotalAlloc, retained
}

func priorityAuthorityRetainedBytesEstimate(authority *priorityPointAuthority) uint64 {
	if authority == nil {
		return 0
	}
	// Conservative reachable-payload estimate. Map bucket/runtime metadata is
	// intentionally covered by the separate TotalAlloc ratchet above; this
	// account counts every retained dense backing array, including the mutation
	// ledger and its search-index copy.
	retained := uint64(unsafe.Sizeof(*authority))
	for _, endpoints := range authority.endpointsByPID {
		retained += uint64(cap(endpoints)) * uint64(unsafe.Sizeof(priorityEndpoint{}))
	}
	for _, ranges := range authority.stableByPID {
		retained += uint64(cap(ranges)) * uint64(unsafe.Sizeof(priorityStableRange{}))
	}
	retained += uint64(cap(authority.mutations)) * uint64(unsafe.Sizeof(priorityMutation{}))
	for _, points := range authority.mutationsByScope {
		retained += uint64(cap(points)) * uint64(unsafe.Sizeof(priorityPhysicalPoint{}))
	}
	for _, sources := range authority.sourcesByPID {
		retained += uint64(cap(sources)) * uint64(unsafe.Sizeof(""))
	}
	retained += uint64(len(authority.rangeSourceComplete)) *
		(uint64(unsafe.Sizeof("")) + uint64(unsafe.Sizeof(false)))
	return retained
}
