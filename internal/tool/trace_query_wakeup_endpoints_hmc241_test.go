package tool

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC241WakeupRowLocalPublicProcessIdentity(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_event_participants/wakeups.systrace")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "event_search", "event_types": []string{"sched_wakeup"}, "time_start": 1.0, "time_end": 1.02})
	count := 0
	for _, record := range result.Observations {
		if record.Predicate != "scheduler_wakeup_event" {
			continue
		}
		count++
		f, ok := types.DecodeRuntimeWakeupEvent(record)
		if !ok {
			t.Fatalf("not a valid row-local event: %+v", record)
		}
		want := 10
		if f.Waker.PID == 13 {
			want = 30
		}
		if f.Waker.TGID != want || f.Wakee.TGID != 0 {
			t.Fatalf("row-local source lost emitter or borrowed wakee process: %+v", f)
		}
		if record.ProvenanceLane != types.ObservationProvenanceArtifactSpan || record.Unit != "event" || record.Value != "1" {
			t.Fatalf("identity display upgraded row authority: %+v", record)
		}
		if !strings.Contains(record.Summary, "被唤醒线程所属进程 TGID=未记录") {
			t.Fatalf("unknown process disappeared: %s", record.Summary)
		}
	}
	if count != 5 {
		t.Fatalf("inclusive native search must retain all five events, got %d", count)
	}
}

func TestHMC241WakeupRowLocalProcessBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name                                   string
		tid, tgid, wakee, wantWaker, wantWakee int
	}{
		{"known_distinct", 11, 10, 42, 10, 0},
		{"same_row_same_thread", 11, 10, 11, 10, 10},
		{"zero", 11, 0, 42, 0, 0},
		{"negative", 11, -1, 42, 0, 0},
		{"overflow", 11, types.RuntimeTargetMaxPID + 1, 42, 0, 0},
		{"invalid_thread", types.RuntimeTargetMaxPID + 1, 10, 42, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := tracequery.Event{Type: tracequery.EventSchedWakeup, Comm: "same", PID: tc.tid, TGID: tc.tgid, WakeeComm: "same", WakeePID: tc.wakee, Line: 3, Ts: 1.003}
			before, _ := json.Marshal(e)
			ref := types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "/fixture.trace", QueryScopeID: "q", PayloadRef: "p", QueryWindowKnown: true, QueryWindowStartTs: 1, QueryWindowEndTs: 1.02}
			coverage := &tracequery.EventSearchCoverage{ScanScope: &types.TraceEventSearchScanScope{TimeStartApplied: true, TimeStart: 1, TimeEndApplied: true, TimeEnd: 1.02, ObservedBasis: "parsed_events", ObservedCount: 1}}
			rows := traceQuerySchedulerWakeEventObservations([]tracequery.EventView{{Event: e}}, ref, "scope", "now", coverage)
			if len(rows) != 1 {
				t.Fatalf("expected one native record, got %d", len(rows))
			}
			f, ok := types.DecodeRuntimeWakeupEvent(rows[0])
			if !ok || f.Waker.TGID != tc.wantWaker || f.Wakee.TGID != tc.wantWakee {
				t.Fatalf("row process mismatch valid=%t: %+v", ok, f)
			}
			after, _ := json.Marshal(e)
			if string(before) != string(after) {
				t.Fatal("display rewrote the source event")
			}
		})
	}
}

func TestHMC241WakeupProcessPrecisionDoesNotMergeDisplayLanes(t *testing.T) {
	record := types.ObservationRecord{SourceRef: types.ObservationSourceRef{Path: "/same.trace", QueryScopeID: "q", PayloadRef: "p", TimeDomain: "trace_seconds"}, SupportRefs: []string{"/same.trace:3"}}
	unknown := tracequery.ThreadRef{Comm: "worker", PID: 11}
	knownA, knownB := unknown, unknown
	knownA.TGID, knownB.TGID = 10, 20
	nodes := map[string]bool{}
	for _, thread := range []tracequery.ThreadRef{unknown, knownA, knownB} {
		node := runtimeWakeupDisplayNode(record, thread, "unused")
		if nodes[node] {
			t.Fatalf("different published process identities merged: %+v", thread)
		}
		nodes[node] = true
		if node != runtimeWakeupDisplayNode(record, thread, "other-fallback") {
			t.Fatal("same exact observed identity split")
		}
	}
}
