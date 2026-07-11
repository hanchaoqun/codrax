package hitraceconv

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestTraceDBRunningConsumerScopesSeparateSchedulerFromExtendedLegacy(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
		"INSERT INTO thread_state VALUES (1, 90, 11, 1, 'Running')",
		"INSERT INTO thread_state VALUES (3, 90, 20, 3, 'Running')",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()

	authority := traceDBSchedStartLifecycleAuthority(true, traceDBLifecycleIndex{
		ByTID: map[int64]traceDBLifecycleLane{
			42: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 1, NewIPID: 1}}},
		},
		ByPID: map[int64]traceDBLifecycleLane{
			100: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 1, NewIPID: 1}}},
		},
	})
	scheduler, schedulerCoverage, err := tdb.loadSchedulerRunningIndex(context.Background(), authority)
	if err != nil {
		t.Fatal(err)
	}
	if _, status := scheduler.lookupCPUAt(1, 95); status != traceDBSchedulerRunningLifecycleRejected {
		t.Fatalf("scheduler Running cross-cut status=%d, want lifecycle rejected", status)
	}
	if cpu, status := scheduler.lookupCPUAt(3, 95); status != traceDBSchedulerRunningKnown || cpu != 3 {
		t.Fatalf("scheduler Running unrelated lane=(cpu=%d,status=%d), want known CPU3", cpu, status)
	}
	if schedulerCoverage.RowsRead != 2 || schedulerCoverage.RowsEmitted != 1 ||
		schedulerCoverage.FieldSources["running_consumer_scope"] != "scheduler_lifecycle_gated" ||
		!strings.Contains(schedulerCoverage.Skipped, "lifecycle_rejected_itid_lanes=1") {
		t.Fatalf("scheduler Running scope/accounting mismatch: %+v", schedulerCoverage)
	}

	baseIntervals, baseIntegrity, baseCoverage, err := tdb.loadRunningIntervals(context.Background(), authority.identities)
	if err != nil {
		t.Fatal(err)
	}
	legacyIntervals, legacyIntegrity, legacyCoverage, err := tdb.loadExtendedLegacyRunningIntervals(context.Background(), authority.identities)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(legacyIntervals, baseIntervals) || !reflect.DeepEqual(legacyIntegrity, baseIntegrity) ||
		legacyCoverage.RowsRead != baseCoverage.RowsRead || legacyCoverage.RowsEmitted != baseCoverage.RowsEmitted {
		t.Fatalf("extended legacy Running facade changed the strict loader: base=%+v %+v %+v legacy=%+v %+v %+v",
			baseIntervals, baseIntegrity, baseCoverage, legacyIntervals, legacyIntegrity, legacyCoverage)
	}
	if cpu, known := traceDBKnownCPUAt(legacyIntervals, 1, 95); !known || cpu != 1 {
		t.Fatalf("extended legacy Running unexpectedly adopted scheduler lifecycle gate: cpu=%d known=%t", cpu, known)
	}
	if legacyCoverage.FieldSources["running_consumer_scope"] != "extended_legacy_r1b_b_open" ||
		!strings.Contains(legacyCoverage.FieldSources["generation_admission"], "R1b-B") {
		t.Fatalf("extended legacy Running scope was not disclosed: %+v", legacyCoverage)
	}
}

func TestTraceDBSchedulerRunningFacadePreservesBaseTaintStatus(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
		"INSERT INTO thread_state VALUES (1, 90, 20, 4096, 'Running')",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	authority := traceDBSchedStartLifecycleAuthority(true, traceDBLifecycleIndex{})
	scheduler, coverage, err := tdb.loadSchedulerRunningIndex(context.Background(), authority)
	if err != nil {
		t.Fatal(err)
	}
	if _, status := scheduler.lookupCPUAt(1, 95); status != traceDBSchedulerRunningSourceTainted {
		t.Fatalf("malformed Running CPU status=%d, want source tainted", status)
	}
	if coverage.RowsRead != 1 || coverage.RowsEmitted != 0 ||
		coverage.FieldSources["running_consumer_scope"] != "scheduler_lifecycle_gated" ||
		!strings.Contains(coverage.Skipped, "potential Running") {
		t.Fatalf("base Running taint coverage changed: %+v", coverage)
	}
}
