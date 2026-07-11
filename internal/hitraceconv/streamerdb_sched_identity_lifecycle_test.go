package hitraceconv

import (
	"context"
	"strings"
	"testing"
)

func traceDBLoadLifecycleSchedStarts(t *testing.T, authority traceDBSchedulerAuthority, rows ...string) (traceDBSchedStartIndex, TraceDBCoverage) {
	t.Helper()
	statements := []string{"CREATE TABLE sched_slice (itid, ts, cpu, priority)"}
	statements = append(statements, rows...)
	path := createTraceDBFixture(t, statements)
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	starts, coverage, err := tdb.loadSchedStarts(context.Background(), authority)
	if err != nil {
		t.Fatal(err)
	}
	return starts, coverage
}

func traceDBSchedStartLifecycleAuthority(complete bool, lifecycle traceDBLifecycleIndex) traceDBSchedulerAuthority {
	authority := traceDBSchedulerAuthorityFixture(complete, lifecycle)
	authority.identities.Processes[3] = traceDBProcess{IPID: 3, PID: 300, Name: "other-process"}
	authority.identities.ByITID[3] = traceDBThread{ITID: 3, TID: 77, IPID: 3, Name: "other-thread"}
	buildTraceDBThreadSecondaryIndexes(&authority.identities)
	return authority
}

func TestTraceDBSchedStartsLifecycleUsesPointThenClosedRange(t *testing.T) {
	t.Run("same identity cut endpoint", func(t *testing.T) {
		authority := traceDBSchedStartLifecycleAuthority(true, traceDBLifecycleIndex{
			ByTID: map[int64]traceDBLifecycleLane{
				42: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 1, NewIPID: 1}}},
			},
			ByPID: map[int64]traceDBLifecycleLane{
				100: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 1, NewIPID: 1}}},
			},
		})
		starts, _ := traceDBLoadLifecycleSchedStarts(t, authority,
			"INSERT INTO sched_slice VALUES (1, 100, 1, 40)",
			"INSERT INTO sched_slice VALUES (1, 110, 1, 41)",
		)
		if priority, known := traceDBNextSchedPriority(starts, 1, 100); !known || priority != 40 {
			t.Fatalf("candidate==query did not use point admission: priority=%d known=%t starts=%+v", priority, known, starts)
		}
		if _, known := traceDBNextSchedPriority(starts, 1, 90); known {
			t.Fatalf("closed query-to-candidate range ignored cut at candidate endpoint: %+v", starts)
		}
		if priority, known := traceDBNextSchedPriority(starts, 1, 101); !known || priority != 41 {
			t.Fatalf("cut before query poisoned later generation: priority=%d known=%t starts=%+v", priority, known, starts)
		}
	})

	t.Run("cut strictly between query and candidate", func(t *testing.T) {
		authority := traceDBSchedStartLifecycleAuthority(true, traceDBLifecycleIndex{
			ByTID: map[int64]traceDBLifecycleLane{
				42: {Cuts: []traceDBLifecycleBoundary{{TS: 105, NewITID: 1, NewIPID: 1}}},
			},
			ByPID: map[int64]traceDBLifecycleLane{
				100: {Cuts: []traceDBLifecycleBoundary{{TS: 105, NewITID: 1, NewIPID: 1}}},
			},
		})
		starts, _ := traceDBLoadLifecycleSchedStarts(t, authority,
			"INSERT INTO sched_slice VALUES (1, 110, 1, 41)",
		)
		if _, known := traceDBNextSchedPriority(starts, 1, 100); known {
			t.Fatalf("closed query-to-candidate range ignored interior generation cut: %+v", starts)
		}
		if priority, known := traceDBNextSchedPriority(starts, 1, 106); !known || priority != 41 {
			t.Fatalf("interior cut before query poisoned candidate: priority=%d known=%t starts=%+v", priority, known, starts)
		}
	})
}

func TestTraceDBSchedStartsLifecycleThreadAndProcessGatesAreIndependent(t *testing.T) {
	tests := []struct {
		name      string
		lifecycle traceDBLifecycleIndex
	}{
		{
			name: "thread cut only",
			lifecycle: traceDBLifecycleIndex{ByTID: map[int64]traceDBLifecycleLane{
				42: {Cuts: []traceDBLifecycleBoundary{{TS: 105, NewITID: 1, NewIPID: 1}}},
			}},
		},
		{
			name: "process cut only",
			lifecycle: traceDBLifecycleIndex{ByPID: map[int64]traceDBLifecycleLane{
				100: {Cuts: []traceDBLifecycleBoundary{{TS: 105, NewITID: 1, NewIPID: 1}}},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authority := traceDBSchedStartLifecycleAuthority(true, test.lifecycle)
			starts, _ := traceDBLoadLifecycleSchedStarts(t, authority,
				"INSERT INTO sched_slice VALUES (1, 110, 1, 41)",
				"INSERT INTO sched_slice VALUES (3, 110, 3, 43)",
			)
			if _, known := traceDBNextSchedPriority(starts, 1, 100); known {
				t.Fatalf("matching lifecycle lane failed open: %+v", starts)
			}
			if priority, known := traceDBNextSchedPriority(starts, 3, 100); !known || priority != 43 {
				t.Fatalf("unrelated lifecycle lane was connected: priority=%d known=%t starts=%+v", priority, known, starts)
			}
		})
	}
}

func TestTraceDBSchedStartsLifecycleRejectionRemainsNearestBarrier(t *testing.T) {
	authority := traceDBSchedStartLifecycleAuthority(true, traceDBLifecycleIndex{GlobalPoison: []int64{100}})
	starts, coverage := traceDBLoadLifecycleSchedStarts(t, authority,
		"INSERT INTO sched_slice VALUES (1, 100, 1, 40)",
		"INSERT INTO sched_slice VALUES (1, 110, 1, 41)",
	)
	if _, known := traceDBNextSchedPriority(starts, 1, 90); known {
		t.Fatalf("lifecycle-invalid nearest candidate was deleted in favor of a later point: %+v", starts)
	}
	if priority, known := traceDBNextSchedPriority(starts, 1, 101); !known || priority != 41 {
		t.Fatalf("barrier before query poisoned later point: priority=%d known=%t starts=%+v", priority, known, starts)
	}
	if coverage.RowsEmitted != 1 || !strings.Contains(coverage.Skipped, "lifecycle_rejected_key_cohorts=1") ||
		coverage.FieldSources["lifecycle"] == "" || !strings.Contains(coverage.FieldSources["lookup"], "closed") {
		t.Fatalf("sched-start lifecycle accounting/provenance mismatch: %+v", coverage)
	}
}

func TestTraceDBSchedStartsIncompleteAuthorityOnlyAllowsExactIdlePoint(t *testing.T) {
	authority := traceDBSchedStartLifecycleAuthority(false, traceDBLifecycleIndex{})
	starts, coverage := traceDBLoadLifecycleSchedStarts(t, authority,
		"INSERT INTO sched_slice VALUES (0, 100, 0, -1)",
		"INSERT INTO sched_slice VALUES (1, 100, 1, 40)",
	)
	if priority, known := traceDBNextSchedPriority(starts, 0, 100); !known || priority != -1 {
		t.Fatalf("exact idle sched point was tied to non-idle lifecycle completeness: priority=%d known=%t starts=%+v", priority, known, starts)
	}
	if _, known := traceDBNextSchedPriority(starts, 0, 99); known {
		t.Fatalf("idle sched point borrowed a closed interval across time: %+v", starts)
	}
	if _, known := traceDBNextSchedPriority(starts, 1, 100); known {
		t.Fatalf("incomplete lifecycle authority admitted a non-idle priority: %+v", starts)
	}
	if coverage.RowsEmitted != 1 || !strings.Contains(coverage.Skipped, "lifecycle_rejected_key_cohorts=1") {
		t.Fatalf("incomplete sched-start authority accounting mismatch: %+v", coverage)
	}

	poisoned := traceDBSchedStartLifecycleAuthority(false, traceDBLifecycleIndex{GlobalPoison: []int64{100}})
	starts, _ = traceDBLoadLifecycleSchedStarts(t, poisoned,
		"INSERT INTO sched_slice VALUES (0, 100, 0, -1)",
	)
	if _, known := traceDBNextSchedPriority(starts, 0, 100); known {
		t.Fatalf("idle sched point bypassed global poison: %+v", starts)
	}
}
