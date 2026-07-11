package hitraceconv

import (
	"context"
	"strings"
	"testing"
)

func traceDBExportBlockedLifecycleFixture(
	t *testing.T,
	lifecycle traceDBLifecycleIndex,
	complete bool,
	rows ...string,
) TraceDBCoverage {
	t.Helper()
	statements := append(traceDBBlockedFixtureSchema(),
		"INSERT INTO process VALUES (8, 500, 'App-new')",
		"INSERT INTO thread VALUES (8, 562, 8, 'blocked-562-new', 1000, 0, 1)",
		"INSERT INTO data_dict VALUES (1, 'iowait')",
		"INSERT INTO data_dict VALUES (2, 'caller')",
		"INSERT INTO data_dict VALUES (10, 'schedule_timeout+0x10/0x20[kernel]')",
		"INSERT INTO args VALUES (1, 1, 0, 1, 110)",
		"INSERT INTO args VALUES (2, 2, 1, 10, 110)",
	)
	statements = append(statements, rows...)
	path := createTraceDBFixture(t, statements)
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	index, _, err := tdb.loadThreadIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	authority := newTraceDBSchedulerAuthority(index, traceDBLifecycleCollection{
		Lifecycle: lifecycle, CreationComplete: complete, TerminalComplete: complete, ActivityComplete: complete,
	})
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := exportTraceDBBlockedReasons(context.Background(), tdb, sink, authority)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.RowsEmitted != sink.stats.RowsAccepted {
		t.Fatalf("blocked sink/coverage mismatch: coverage=%+v sink=%+v", coverage, sink.stats)
	}
	return coverage
}

func traceDBBlockedLifecycleBaselineRows() []string {
	return []string{
		"INSERT INTO sched_slice VALUES (1, 900, 100, 4, 1, 'S', 40)",
		"INSERT INTO thread_state VALUES (1, 1000, 100, NULL, 1, 562, 500, 'S', 110)",
	}
}

func TestTraceDBBlockedLifecycleCandidateRequiresThreadAndProcessPoint(t *testing.T) {
	tests := []struct {
		name      string
		lifecycle traceDBLifecycleIndex
		complete  bool
	}{
		{
			name: "thread generation",
			lifecycle: traceDBLifecycleIndex{ByTID: map[int64]traceDBLifecycleLane{
				562: {Cuts: []traceDBLifecycleBoundary{{TS: 1000, NewITID: 8, NewIPID: 8}}},
			}},
			complete: true,
		},
		{
			name: "process generation",
			lifecycle: traceDBLifecycleIndex{ByPID: map[int64]traceDBLifecycleLane{
				500: {Cuts: []traceDBLifecycleBoundary{{TS: 1000, NewITID: 8, NewIPID: 8}}},
			}},
			complete: true,
		},
		{
			name:      "incomplete authority",
			lifecycle: traceDBLifecycleIndex{},
		},
		{
			name:      "global poison",
			lifecycle: traceDBLifecycleIndex{GlobalPoison: []int64{1000}},
			complete:  true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coverage := traceDBExportBlockedLifecycleFixture(t, test.lifecycle, test.complete,
				traceDBBlockedLifecycleBaselineRows()...)
			if coverage.RowsRead != 1 || coverage.RowsEmitted != 0 ||
				!strings.Contains(coverage.Skipped, "lifecycle_rejected_thread_state_candidate=1") {
				t.Fatalf("blocked candidate lifecycle failed open or misreported: %+v", coverage)
			}
		})
	}
}

func TestTraceDBBlockedLifecyclePredecessorUsesClosedThreadAndProcessRange(t *testing.T) {
	tests := []struct {
		name      string
		lifecycle traceDBLifecycleIndex
		wantEmit  bool
	}{
		{
			name: "thread cut at closed end",
			lifecycle: traceDBLifecycleIndex{ByTID: map[int64]traceDBLifecycleLane{
				562: {Cuts: []traceDBLifecycleBoundary{{TS: 1000, NewITID: 1, NewIPID: 1}}},
			}},
		},
		{
			name: "process cut at closed end",
			lifecycle: traceDBLifecycleIndex{ByPID: map[int64]traceDBLifecycleLane{
				500: {Cuts: []traceDBLifecycleBoundary{{TS: 1000, NewITID: 1, NewIPID: 1}}},
			}},
		},
		{
			name: "thread cut inside predecessor",
			lifecycle: traceDBLifecycleIndex{ByTID: map[int64]traceDBLifecycleLane{
				562: {Cuts: []traceDBLifecycleBoundary{{TS: 950, NewITID: 1, NewIPID: 1}}},
			}},
		},
		{
			name: "process cut inside predecessor",
			lifecycle: traceDBLifecycleIndex{ByPID: map[int64]traceDBLifecycleLane{
				500: {Cuts: []traceDBLifecycleBoundary{{TS: 950, NewITID: 1, NewIPID: 1}}},
			}},
		},
		{
			name:      "global poison inside predecessor",
			lifecycle: traceDBLifecycleIndex{GlobalPoison: []int64{950}},
		},
		{
			name: "thread cut aligned with predecessor start",
			lifecycle: traceDBLifecycleIndex{ByTID: map[int64]traceDBLifecycleLane{
				562: {Cuts: []traceDBLifecycleBoundary{{TS: 900, NewITID: 1, NewIPID: 1}}},
			}},
			wantEmit: true,
		},
		{
			name: "process cut aligned with predecessor start",
			lifecycle: traceDBLifecycleIndex{ByPID: map[int64]traceDBLifecycleLane{
				500: {Cuts: []traceDBLifecycleBoundary{{TS: 900, NewITID: 1, NewIPID: 1}}},
			}},
			wantEmit: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coverage := traceDBExportBlockedLifecycleFixture(t, test.lifecycle, true,
				traceDBBlockedLifecycleBaselineRows()...)
			if test.wantEmit {
				if coverage.RowsEmitted != 1 || strings.Contains(coverage.Skipped, "lifecycle_rejected") {
					t.Fatalf("start-aligned blocked generation was rejected: %+v", coverage)
				}
				return
			}
			if coverage.RowsEmitted != 0 ||
				!strings.Contains(coverage.Skipped, "lifecycle_rejected_prev_sched_slice_boundary=1") {
				t.Fatalf("closed predecessor lifecycle failed open or misreported: %+v", coverage)
			}
		})
	}
}

func TestTraceDBBlockedLifecycleGatesOnlyTheCandidateStartInstant(t *testing.T) {
	lifecycle := traceDBLifecycleIndex{ByTID: map[int64]traceDBLifecycleLane{
		562: {Cuts: []traceDBLifecycleBoundary{{TS: 1050, NewITID: 1, NewIPID: 1}}},
	}}
	coverage := traceDBExportBlockedLifecycleFixture(t, lifecycle, true,
		traceDBBlockedLifecycleBaselineRows()...)
	if coverage.RowsRead != 1 || coverage.RowsEmitted != 1 || strings.Contains(coverage.Skipped, "lifecycle_rejected") {
		t.Fatalf("blocked start marker was incorrectly treated as a thread_state duration interval: %+v", coverage)
	}
}

func TestTraceDBBlockedLifecycleInvalidPredecessorRemainsCohortBarrier(t *testing.T) {
	lifecycle := traceDBLifecycleIndex{ByTID: map[int64]traceDBLifecycleLane{
		562: {Cuts: []traceDBLifecycleBoundary{{TS: 950, NewITID: 1, NewIPID: 1}}},
	}}
	coverage := traceDBExportBlockedLifecycleFixture(t, lifecycle, true,
		"INSERT INTO sched_slice VALUES (1, 900, 100, 4, 1, 'S', 40)",
		"INSERT INTO sched_slice VALUES (2, 950, 50, 5, 1, 'S', 40)",
		"INSERT INTO thread_state VALUES (1, 1000, 100, NULL, 1, 562, 500, 'S', 110)",
	)
	if coverage.RowsEmitted != 0 ||
		!strings.Contains(coverage.Skipped, "lifecycle_rejected_prev_sched_slice_boundary=1") ||
		strings.Contains(coverage.Skipped, "ambiguous_prev_sched_slice_boundary") {
		t.Fatalf("invalid predecessor disappeared and let a valid sibling rescue the cohort: %+v", coverage)
	}
}

func TestTraceDBBlockedLifecyclePreservesPhysicalSourceDiagnosisPriority(t *testing.T) {
	key := traceDBBlockedBoundaryKey{ITID: 1, StateStart: 1000}
	index := traceDBBlockedBoundaryIndex{
		LifecycleRejected: map[traceDBBlockedBoundaryKey]bool{key: true},
		TaintedITIDs:      map[int64]bool{1: true},
	}
	if got := traceDBBlockedBoundaryIntegrityReason(index, key); got != "tainted_prev_sched_slice_lane" {
		t.Fatalf("lifecycle barrier masked physical source diagnosis: got %q", got)
	}
	delete(index.TaintedITIDs, 1)
	if got := traceDBBlockedBoundaryIntegrityReason(index, key); got != "lifecycle_rejected_prev_sched_slice_boundary" {
		t.Fatalf("pure lifecycle barrier diagnosis=%q", got)
	}
}

func TestTraceDBBlockedLifecycleForbidsIdleCandidate(t *testing.T) {
	coverage := traceDBExportBlockedLifecycleFixture(t, traceDBLifecycleIndex{}, true,
		"INSERT INTO sched_slice VALUES (1, 900, 100, 4, 0, 'S', 40)",
		"INSERT INTO thread_state VALUES (1, 1000, 100, NULL, 0, 562, 500, 'S', 110)",
	)
	if coverage.RowsRead != 1 || coverage.RowsEmitted != 0 ||
		!strings.Contains(coverage.Skipped, "idle_blocked_candidate_forbidden=1") {
		t.Fatalf("idle identity acquired blocked-reason authority: %+v", coverage)
	}
}

func TestTraceDBBlockedLifecycleRequiresPositiveCanonicalProcess(t *testing.T) {
	coverage := traceDBExportBlockedLifecycleFixture(t, traceDBLifecycleIndex{}, true,
		"UPDATE process SET pid=0 WHERE ipid=1",
		"INSERT INTO sched_slice VALUES (1, 900, 100, 4, 1, 'S', 40)",
		"INSERT INTO thread_state VALUES (1, 1000, 100, NULL, 1, 562, 500, 'S', 110)",
	)
	if coverage.RowsRead != 1 || coverage.RowsEmitted != 0 ||
		!strings.Contains(coverage.Skipped, "missing_process_identity=1") {
		t.Fatalf("non-idle blocked candidate acquired authority from PID zero: %+v", coverage)
	}
}

func TestTraceDBBlockedLifecycleRejectionIsLaneLocal(t *testing.T) {
	lifecycle := traceDBLifecycleIndex{ByTID: map[int64]traceDBLifecycleLane{
		562: {Cuts: []traceDBLifecycleBoundary{{TS: 1000, NewITID: 8, NewIPID: 8}}},
	}}
	coverage := traceDBExportBlockedLifecycleFixture(t, lifecycle, true,
		"INSERT INTO sched_slice VALUES (1, 900, 100, 4, 1, 'S', 40)",
		"INSERT INTO sched_slice VALUES (2, 1900, 100, 5, 2, 'S', 40)",
		"INSERT INTO thread_state VALUES (1, 1000, 100, NULL, 1, 562, 500, 'S', 110)",
		"INSERT INTO thread_state VALUES (2, 2000, 100, NULL, 2, 563, 500, 'S', 110)",
	)
	if coverage.RowsRead != 2 || coverage.RowsEmitted != 1 ||
		!strings.Contains(coverage.Skipped, "lifecycle_rejected_thread_state_candidate=1") {
		t.Fatalf("matching lifecycle rejection spread to an unrelated thread lane: %+v", coverage)
	}
}

func TestTraceDBBlockedLifecycleAcceptsNewGenerationAtPredecessorStart(t *testing.T) {
	lifecycle := traceDBLifecycleIndex{
		ByTID: map[int64]traceDBLifecycleLane{
			562: {Cuts: []traceDBLifecycleBoundary{{TS: 1000, NewITID: 8, NewIPID: 8}}},
		},
		ByPID: map[int64]traceDBLifecycleLane{
			500: {Cuts: []traceDBLifecycleBoundary{{TS: 1000, NewITID: 8, NewIPID: 8}}},
		},
	}
	coverage := traceDBExportBlockedLifecycleFixture(t, lifecycle, true,
		"INSERT INTO sched_slice VALUES (1, 1000, 100, 4, 8, 'S', 40)",
		"INSERT INTO thread_state VALUES (1, 1100, 100, NULL, 8, 562, 500, 'S', 110)",
	)
	if coverage.RowsRead != 1 || coverage.RowsEmitted != 1 ||
		strings.Contains(coverage.Skipped, "lifecycle_rejected") || coverage.FieldSources["lifecycle"] == "" {
		t.Fatalf("valid new blocked generation at predecessor start was rejected or undisclosed: %+v", coverage)
	}
}
