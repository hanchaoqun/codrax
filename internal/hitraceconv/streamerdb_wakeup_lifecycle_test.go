package hitraceconv

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

func traceDBWakeupLifecycleAuthority(lifecycle traceDBLifecycleIndex) traceDBSchedulerAuthority {
	index := newTraceDBThreadIndex(0, true)
	for ipid, process := range map[int64]traceDBProcess{
		1: {IPID: 1, PID: 100, Name: "target-old-process"},
		2: {IPID: 2, PID: 200, Name: "waker-old-process"},
		3: {IPID: 3, PID: 100, Name: "target-new-process"},
		4: {IPID: 4, PID: 200, Name: "waker-new-process"},
	} {
		index.Processes[ipid] = process
	}
	for itid, thread := range map[int64]traceDBThread{
		1: {ITID: 1, TID: 10, IPID: 1, Name: "target-old"},
		2: {ITID: 2, TID: 20, IPID: 2, Name: "waker-old"},
		3: {ITID: 3, TID: 10, IPID: 3, Name: "target-new"},
		4: {ITID: 4, TID: 20, IPID: 4, Name: "waker-new"},
	} {
		index.ByITID[itid] = thread
	}
	buildTraceDBThreadSecondaryIndexes(&index)
	return newTraceDBSchedulerAuthority(index, traceDBLifecycleCollection{
		Lifecycle: lifecycle, CreationComplete: true, TerminalComplete: true, ActivityComplete: true,
	})
}

func traceDBExportWakeupLifecycleFixture(t *testing.T, authority traceDBSchedulerAuthority, ref, waker int64, raws []traceDBRawWakeup) TraceDBCoverage {
	t.Helper()
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE instant (ts, name, ref, wakeup_from, ref_type)",
		"INSERT INTO instant VALUES (100, 'sched_wakeup', " + strconv.FormatInt(ref, 10) + ", " + strconv.FormatInt(waker, 10) + ", 'itid')",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	running := newTraceDBSchedulerRunningIndex(authority, map[int64][]traceDBRunningInterval{
		waker: {{Start: 90, End: 110, CPU: 2, PrefixMaxEnd: 110}},
	}, traceDBRunningIntegrity{TaintedITIDs: map[int64]bool{}}, nil)
	coverage, err := exportTraceDBWakeups(context.Background(), tdb, sink, authority, raws, traceDBSchedStartIndex{}, running)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.RowsEmitted != sink.stats.RowsAccepted {
		t.Fatalf("wakeup sink/coverage mismatch: coverage=%+v sink=%+v", coverage, sink.stats)
	}
	return coverage
}

func TestTraceDBWakeupLifecycleValidatesBothEndpointsAfterMatching(t *testing.T) {
	raw := []traceDBRawWakeup{{RowID: 1, TS: 100, Name: "sched_wakeup", TargetCPU: 7, ITID: 1}}
	tests := []struct {
		name      string
		authority traceDBSchedulerAuthority
		ref       int64
		waker     int64
		wantGap   string
	}{
		{
			name: "wakee thread cut",
			authority: traceDBWakeupLifecycleAuthority(traceDBLifecycleIndex{ByTID: map[int64]traceDBLifecycleLane{
				10: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 3, NewIPID: 3}}},
			}}),
			ref: 1, waker: 2, wantGap: "lifecycle_rejected_wakee_endpoint=1",
		},
		{
			name: "wakee process cut",
			authority: traceDBWakeupLifecycleAuthority(traceDBLifecycleIndex{ByPID: map[int64]traceDBLifecycleLane{
				100: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 3, NewIPID: 3}}},
			}}),
			ref: 1, waker: 2, wantGap: "lifecycle_rejected_wakee_endpoint=1",
		},
		{
			name: "waker thread cut",
			authority: traceDBWakeupLifecycleAuthority(traceDBLifecycleIndex{ByTID: map[int64]traceDBLifecycleLane{
				20: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 4, NewIPID: 4}}},
			}}),
			ref: 1, waker: 2, wantGap: "lifecycle_rejected_waker_endpoint=1",
		},
		{
			name: "waker process cut",
			authority: traceDBWakeupLifecycleAuthority(traceDBLifecycleIndex{ByPID: map[int64]traceDBLifecycleLane{
				200: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 4, NewIPID: 4}}},
			}}),
			ref: 1, waker: 2, wantGap: "lifecycle_rejected_waker_endpoint=1",
		},
		{
			name:      "global poison",
			authority: traceDBWakeupLifecycleAuthority(traceDBLifecycleIndex{GlobalPoison: []int64{100}}),
			ref:       1, waker: 2, wantGap: "lifecycle_rejected_wakee_endpoint=1",
		},
		{
			name:      "idle wakee forbidden",
			authority: traceDBWakeupLifecycleAuthority(traceDBLifecycleIndex{}),
			ref:       0, waker: 2, wantGap: "idle_wakee_endpoint_forbidden=1",
		},
		{
			name:      "idle waker forbidden",
			authority: traceDBWakeupLifecycleAuthority(traceDBLifecycleIndex{}),
			ref:       1, waker: 0, wantGap: "idle_waker_endpoint_forbidden=1",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidateRaw := append([]traceDBRawWakeup(nil), raw...)
			candidateRaw[0].ITID = test.ref
			coverage := traceDBExportWakeupLifecycleFixture(t, test.authority, test.ref, test.waker, candidateRaw)
			if coverage.RowsEmitted != 0 || !strings.Contains(coverage.Skipped, test.wantGap) {
				t.Fatalf("wakeup endpoint failed open or misreported: %+v", coverage)
			}
		})
	}

	newAuthority := traceDBWakeupLifecycleAuthority(traceDBLifecycleIndex{
		ByTID: map[int64]traceDBLifecycleLane{
			10: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 3, NewIPID: 3}}},
		},
		ByPID: map[int64]traceDBLifecycleLane{
			100: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 3, NewIPID: 3}}},
		},
	})
	newRaw := []traceDBRawWakeup{{RowID: 1, TS: 100, Name: "sched_wakeup", TargetCPU: 7, ITID: 3}}
	coverage := traceDBExportWakeupLifecycleFixture(t, newAuthority, 3, 2, newRaw)
	if coverage.RowsEmitted != 1 || !strings.Contains(coverage.Skipped, "priority_unknown_edges_preserved=1") {
		t.Fatalf("new-generation wakee at cut was rejected: %+v", coverage)
	}
}

func TestTraceDBWakeupLifecycleNeverPrecedesRawInstantMatching(t *testing.T) {
	authority := traceDBWakeupLifecycleAuthority(traceDBLifecycleIndex{GlobalPoison: []int64{100}})
	coverage := traceDBExportWakeupLifecycleFixture(t, authority, 1, 2, nil)
	if coverage.RowsEmitted != 0 || !strings.Contains(coverage.Skipped, "raw_instant_count_mismatch=1") ||
		strings.Contains(coverage.Skipped, "lifecycle_rejected") {
		t.Fatalf("lifecycle filtering ran before full raw/instant matching: %+v", coverage)
	}
}
