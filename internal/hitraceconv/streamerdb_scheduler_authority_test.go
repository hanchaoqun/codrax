package hitraceconv

import (
	"reflect"
	"strings"
	"testing"
)

func traceDBSchedulerAuthorityFixture(complete bool, lifecycle traceDBLifecycleIndex) traceDBSchedulerAuthority {
	index := newTraceDBThreadIndex(0, true)
	index.Processes[1] = traceDBProcess{IPID: 1, PID: 100, Name: "old-process"}
	index.Processes[2] = traceDBProcess{IPID: 2, PID: 100, Name: "new-process"}
	index.ByITID[1] = traceDBThread{ITID: 1, TID: 42, IPID: 1, Name: "old"}
	index.ByITID[2] = traceDBThread{ITID: 2, TID: 42, IPID: 2, Name: "new"}
	buildTraceDBThreadSecondaryIndexes(&index)
	collection := traceDBLifecycleCollection{
		Lifecycle: lifecycle, CreationComplete: complete, TerminalComplete: complete, ActivityComplete: complete,
	}
	return newTraceDBSchedulerAuthority(index, collection)
}

func traceDBTestCompleteSchedulerAuthority(index traceDBThreadIndex) traceDBSchedulerAuthority {
	return newTraceDBSchedulerAuthority(index, traceDBLifecycleCollection{
		CreationComplete: true, TerminalComplete: true, ActivityComplete: true,
	})
}

func TestTraceDBSchedulerAuthorityThreadAndProcessBoundaries(t *testing.T) {
	lifecycle := traceDBLifecycleIndex{
		ByTID: map[int64]traceDBLifecycleLane{
			42: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 2, NewIPID: 2}}},
		},
		ByPID: map[int64]traceDBLifecycleLane{
			100: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 2, NewIPID: 2}}},
		},
	}
	authority := traceDBSchedulerAuthorityFixture(true, lifecycle)
	if !authority.threadSourceIntervalAllows(1, 90, 100) ||
		authority.threadClosedEndpointAllows(1, 90, 100) ||
		!authority.threadPointAllows(2, 100) ||
		authority.threadPointAllows(1, 100) ||
		authority.threadSourceIntervalAllows(1, 90, 101) {
		t.Fatalf("thread/process cut boundary mismatch: %+v", authority)
	}

	processPoison := lifecycle
	processPoison.ByPID = map[int64]traceDBLifecycleLane{
		100: {PoisonPoints: []int64{95}},
	}
	processAuthority := traceDBSchedulerAuthorityFixture(true, processPoison)
	if processAuthority.threadSourceIntervalAllows(1, 90, 100) || !processAuthority.threadPointAllows(1, 90) {
		t.Fatalf("positive process lane was not independently enforced: %+v", processAuthority)
	}
}

func TestTraceDBSchedulerAuthorityCompletenessAndZeroValueFailClosed(t *testing.T) {
	incomplete := traceDBSchedulerAuthorityFixture(false, traceDBLifecycleIndex{})
	idle, ok := incomplete.schedulerSubjectFromExactITID(0, true)
	if !ok {
		t.Fatal("strict canonical idle identity was rejected")
	}
	var zero traceDBSchedulerAuthority
	if zero.threadPointAllows(1, 1) || zero.schedulerPointAllows(idle, 1) ||
		zero.schedulerSourceIntervalAllows(idle, 1, 2) {
		t.Fatal("zero-value scheduler authority failed open")
	}
	if incomplete.threadPointAllows(1, 1) || incomplete.threadSourceIntervalAllows(1, 1, 2) {
		t.Fatal("incomplete lifecycle authority allowed a non-idle subject")
	}
	if !incomplete.schedulerPointAllows(idle, 1) || !incomplete.schedulerSourceIntervalAllows(idle, 1, 2) {
		t.Fatal("exact scheduler idle identity should not depend on non-idle generation completeness")
	}
	missing, ok := incomplete.schedulerSubjectFromExactITID(0, false)
	if ok || incomplete.schedulerPointAllows(missing, 1) || incomplete.schedulerSourceIntervalAllows(missing, 1, 2) {
		t.Fatal("missing scheduler identity defaulted to the canonical idle bypass")
	}
}

func TestTraceDBSchedulerAuthorityIdleStillHonorsGlobalIntegrity(t *testing.T) {
	tests := []struct {
		name      string
		lifecycle traceDBLifecycleIndex
		point     bool
		interval  bool
	}{
		{name: "clean", lifecycle: traceDBLifecycleIndex{}, point: true, interval: true},
		{name: "point poison", lifecycle: traceDBLifecycleIndex{GlobalPoison: []int64{5}}, interval: false},
		{name: "interior poison", lifecycle: traceDBLifecycleIndex{GlobalPoison: []int64{6}}, point: true},
		{name: "global taint", lifecycle: traceDBLifecycleIndex{GlobalTaint: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authority := traceDBSchedulerAuthorityFixture(false, test.lifecycle)
			idle, ok := authority.schedulerSubjectFromExactITID(0, true)
			if !ok {
				t.Fatal("strict canonical idle identity was rejected")
			}
			if got := authority.schedulerPointAllows(idle, 5); got != test.point {
				t.Fatalf("idle point=%t, want %t", got, test.point)
			}
			if got := authority.schedulerSourceIntervalAllows(idle, 5, 7); got != test.interval {
				t.Fatalf("idle interval=%t, want %t", got, test.interval)
			}
		})
	}
}

func TestTraceDBSchedulerAuthorityEndpointShapesAndPIDZero(t *testing.T) {
	lifecycle := traceDBLifecycleIndex{
		GlobalPoison: []int64{10},
		ByTID: map[int64]traceDBLifecycleLane{
			77: {PoisonPoints: []int64{6}},
		},
	}
	authority := traceDBSchedulerAuthorityFixture(true, lifecycle)
	authority.identities.Processes[3] = traceDBProcess{IPID: 3, PID: 0, Name: "kernel"}
	authority.identities.ByITID[3] = traceDBThread{ITID: 3, TID: 77, IPID: 3, Name: "kernel-worker"}
	buildTraceDBThreadSecondaryIndexes(&authority.identities)
	idle, ok := authority.schedulerSubjectFromExactITID(0, true)
	if !ok {
		t.Fatal("strict canonical idle identity was rejected")
	}

	if !authority.schedulerSourceIntervalAllows(idle, 5, 10) {
		t.Fatal("half-open scheduler interval incorrectly included poison at its end")
	}
	if authority.schedulerPointAllows(idle, 10) {
		t.Fatal("scheduler point ignored global poison")
	}
	if authority.schedulerSourceIntervalAllows(idle, -1, 1) {
		t.Fatal("negative scheduler idle interval failed open")
	}
	if authority.schedulerSourceIntervalAllows(idle, -2, -1) ||
		authority.schedulerSourceIntervalAllows(idle, 5, 5) {
		t.Fatal("negative-end or empty scheduler idle interval failed open")
	}
	if !authority.threadPointAllows(3, 5) || !authority.threadSourceIntervalAllows(3, 5, 6) {
		t.Fatal("PID-zero process incorrectly required a process lifecycle lane")
	}
	if authority.threadPointAllows(3, 6) || authority.threadClosedEndpointAllows(3, 5, 6) {
		t.Fatal("PID-zero process bypassed its thread lifecycle lane")
	}
}

func TestTraceDBSchedulerAuthorityIdleRequiresUnambiguousCanonicalIdentity(t *testing.T) {
	authority := traceDBSchedulerAuthorityFixture(true, traceDBLifecycleIndex{})
	authority.identities.AmbiguousITID[0] = true
	if _, ok := authority.schedulerSubjectFromExactITID(0, true); ok {
		t.Fatal("ambiguous internal identity zero acquired the idle bypass")
	}
	delete(authority.identities.AmbiguousITID, 0)
	authority.identities.ByITID[0] = traceDBThread{ITID: 0, TID: 123, IPID: 0, Name: "not-idle"}
	if _, ok := authority.schedulerSubjectFromExactITID(0, true); ok {
		t.Fatal("conflicting materialized internal identity zero acquired the idle bypass")
	}
	authority.identities.ByITID[0] = traceDBThread{ITID: 0, TID: 0, IPID: 0, Name: "swapper"}
	if _, ok := authority.schedulerSubjectFromExactITID(0, true); !ok {
		t.Fatal("canonical materialized idle identity was rejected")
	}
	authority.identities.AmbiguousIPID[0] = true
	if _, ok := authority.schedulerSubjectFromExactITID(0, true); ok {
		t.Fatal("ambiguous internal process identity zero acquired the idle bypass")
	}
	delete(authority.identities.AmbiguousIPID, 0)
	authority.identities.Processes[0] = traceDBProcess{IPID: 0, PID: 123, Name: "not-idle"}
	if _, ok := authority.schedulerSubjectFromExactITID(0, true); ok {
		t.Fatal("conflicting materialized process identity zero acquired the idle bypass")
	}
	authority.identities.Processes[0] = traceDBProcess{IPID: 0, PID: 0, Name: "swapper"}
	if _, ok := authority.schedulerSubjectFromExactITID(0, true); !ok {
		t.Fatal("canonical materialized idle process identity was rejected")
	}
}

func TestTraceDBSchedulerAuthoritySameIdentityStillObservesGenerationCuts(t *testing.T) {
	lifecycle := traceDBLifecycleIndex{
		ByTID: map[int64]traceDBLifecycleLane{
			42: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 1, NewIPID: 1}}},
		},
		ByPID: map[int64]traceDBLifecycleLane{
			100: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 1, NewIPID: 1}}},
		},
	}
	authority := traceDBSchedulerAuthorityFixture(true, lifecycle)
	if !authority.threadPointAllows(1, 99) || !authority.threadPointAllows(1, 100) ||
		!authority.threadSourceIntervalAllows(1, 90, 100) ||
		!authority.threadSourceIntervalAllows(1, 100, 110) {
		t.Fatal("valid endpoint-aligned same-identity generation was rejected")
	}
	if authority.threadSourceIntervalAllows(1, 90, 101) ||
		authority.threadClosedEndpointAllows(1, 90, 100) {
		t.Fatal("same-identity generation cut was reduced to endpoint-only admission")
	}
}

func TestTraceDBSchedulerAuthorityProcessCutIsIndependentAndLaneLocal(t *testing.T) {
	lifecycle := traceDBLifecycleIndex{
		ByPID: map[int64]traceDBLifecycleLane{
			100: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 1, NewIPID: 1}}},
			300: {},
		},
		ByTID: map[int64]traceDBLifecycleLane{
			42: {Tainted: true},
			77: {},
		},
	}
	authority := traceDBSchedulerAuthorityFixture(true, lifecycle)
	authority.identities.Processes[3] = traceDBProcess{IPID: 3, PID: 300, Name: "other-process"}
	authority.identities.ByITID[3] = traceDBThread{ITID: 3, TID: 77, IPID: 3, Name: "other-thread"}
	buildTraceDBThreadSecondaryIndexes(&authority.identities)

	if authority.threadPointAllows(1, 90) {
		t.Fatal("matching tainted thread lane failed open")
	}
	if !authority.threadPointAllows(3, 90) {
		t.Fatal("unrelated thread/process lane was connected to another lane's taint")
	}

	delete(lifecycle.ByTID, 42)
	authority = traceDBSchedulerAuthorityFixture(true, lifecycle)
	if !authority.threadPointAllows(1, 90) || !authority.threadPointAllows(1, 101) {
		t.Fatal("same-IPID process cut should not reject valid endpoint points")
	}
	if authority.threadSourceIntervalAllows(1, 90, 101) ||
		authority.threadClosedEndpointAllows(1, 90, 100) {
		t.Fatal("independent same-IPID process generation cut was bypassed")
	}
}

func TestTraceDBSchedulerRunningIndexTaintsWholeCrossCutLane(t *testing.T) {
	lifecycle := traceDBLifecycleIndex{ByTID: map[int64]traceDBLifecycleLane{
		42: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 2, NewIPID: 2}}},
	}}
	authority := traceDBSchedulerAuthorityFixture(true, lifecycle)
	raw := map[int64][]traceDBRunningInterval{
		1: {
			{Start: 80, End: 90, CPU: 1, PrefixMaxEnd: 90},
			{Start: 90, End: 101, CPU: 1, PrefixMaxEnd: 101},
		},
		2: {{Start: 100, End: 110, CPU: 2, PrefixMaxEnd: 110}},
		0: {{Start: 100, End: 110, CPU: 0, PrefixMaxEnd: 110}},
	}
	coverage := TraceDBCoverage{RowsEmitted: 4}
	index := newTraceDBSchedulerRunningIndex(authority, raw,
		traceDBRunningIntegrity{TaintedITIDs: map[int64]bool{}}, &coverage)
	if _, ok := index.knownCPUAt(1, 85); ok {
		t.Fatal("one cross-cut Running row did not taint the whole ITID lane")
	}
	if cpu, ok := index.knownCPUAt(2, 105); !ok || cpu != 2 {
		t.Fatalf("valid new-generation Running lane lost: cpu=%d ok=%t", cpu, ok)
	}
	if cpu, ok := index.knownCPUAt(0, 105); !ok || cpu != 0 {
		t.Fatalf("canonical idle Running lane lost: cpu=%d ok=%t", cpu, ok)
	}
	if coverage.RowsEmitted != 2 || !strings.Contains(coverage.Skipped, "rejected_running_rows=2") ||
		!strings.Contains(coverage.Skipped, "total_tainted_itid_lanes=1") ||
		coverage.FieldSources["scheduler_lifecycle"] == "" {
		t.Fatalf("Running lifecycle coverage mismatch: %+v", coverage)
	}
}

func TestTraceDBSchedulerRunningIndexPreservesBaseIntegrity(t *testing.T) {
	authority := traceDBSchedulerAuthorityFixture(true, traceDBLifecycleIndex{})
	raw := map[int64][]traceDBRunningInterval{
		1: {{Start: 1, End: 2, CPU: 1, PrefixMaxEnd: 2}},
		2: {{Start: 1, End: 2, CPU: 2, PrefixMaxEnd: 2}},
	}
	index := newTraceDBSchedulerRunningIndex(authority, raw,
		traceDBRunningIntegrity{TaintedITIDs: map[int64]bool{1: true}}, nil)
	if _, ok := index.knownCPUAt(1, 1); ok {
		t.Fatal("base Running taint was bypassed")
	}
	if cpu, ok := index.knownCPUAt(2, 1); !ok || cpu != 2 {
		t.Fatalf("unrelated Running lane was connected: cpu=%d ok=%t index=%+v", cpu, ok, index)
	}
	if !reflect.DeepEqual(index.intervals[2], raw[2]) {
		t.Fatalf("valid Running intervals changed: %+v", index.intervals)
	}
}

func TestTraceDBSchedulerRunningIndexCompletenessAndEndpointShapes(t *testing.T) {
	raw := map[int64][]traceDBRunningInterval{
		1: {{Start: 5, End: 10, CPU: 1, PrefixMaxEnd: 10}},
		0: {{Start: 5, End: 10, CPU: 0, PrefixMaxEnd: 10}},
	}
	incomplete := traceDBSchedulerAuthorityFixture(false, traceDBLifecycleIndex{})
	index := newTraceDBSchedulerRunningIndex(incomplete, raw,
		traceDBRunningIntegrity{TaintedITIDs: map[int64]bool{}}, nil)
	if _, ok := index.knownCPUAt(1, 5); ok {
		t.Fatal("incomplete authority retained a non-idle Running lane")
	}
	if cpu, ok := index.knownCPUAt(0, 5); !ok || cpu != 0 {
		t.Fatalf("incomplete non-idle sources suppressed exact idle Running: cpu=%d ok=%t", cpu, ok)
	}

	endPoison := traceDBSchedulerAuthorityFixture(true, traceDBLifecycleIndex{GlobalPoison: []int64{10}})
	index = newTraceDBSchedulerRunningIndex(endPoison, raw,
		traceDBRunningIntegrity{TaintedITIDs: map[int64]bool{}}, nil)
	if cpu, ok := index.knownCPUAt(1, 9); !ok || cpu != 1 {
		t.Fatalf("half-open Running interval included poison at end: cpu=%d ok=%t", cpu, ok)
	}

	interiorPoison := traceDBSchedulerAuthorityFixture(true, traceDBLifecycleIndex{GlobalPoison: []int64{7}})
	index = newTraceDBSchedulerRunningIndex(interiorPoison, raw,
		traceDBRunningIntegrity{TaintedITIDs: map[int64]bool{}}, nil)
	if _, ok := index.knownCPUAt(1, 6); ok {
		t.Fatal("global poison inside a Running interval did not reject its lane")
	}
	if _, ok := index.knownCPUAt(0, 6); ok {
		t.Fatal("idle Running interval bypassed interior global poison")
	}
}

func TestTraceDBSchedulerRunningIndexSameIdentityCutAndLaneLocality(t *testing.T) {
	lifecycle := traceDBLifecycleIndex{
		ByTID: map[int64]traceDBLifecycleLane{
			42: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 1, NewIPID: 1}}},
		},
		ByPID: map[int64]traceDBLifecycleLane{
			100: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 1, NewIPID: 1}}},
		},
	}
	authority := traceDBSchedulerAuthorityFixture(true, lifecycle)
	authority.identities.Processes[3] = traceDBProcess{IPID: 3, PID: 300, Name: "other-process"}
	authority.identities.ByITID[3] = traceDBThread{ITID: 3, TID: 77, IPID: 3, Name: "other-thread"}
	buildTraceDBThreadSecondaryIndexes(&authority.identities)

	aligned := map[int64][]traceDBRunningInterval{
		1: {
			{Start: 90, End: 100, CPU: 1, PrefixMaxEnd: 100},
			{Start: 100, End: 110, CPU: 1, PrefixMaxEnd: 110},
		},
	}
	index := newTraceDBSchedulerRunningIndex(authority, aligned,
		traceDBRunningIntegrity{TaintedITIDs: map[int64]bool{}}, nil)
	if cpu, ok := index.knownCPUAt(1, 99); !ok || cpu != 1 {
		t.Fatalf("old Running generation ending at cut was rejected: cpu=%d ok=%t", cpu, ok)
	}
	if cpu, ok := index.knownCPUAt(1, 100); !ok || cpu != 1 {
		t.Fatalf("new Running generation starting at cut was rejected: cpu=%d ok=%t", cpu, ok)
	}

	crossing := map[int64][]traceDBRunningInterval{
		1: {{Start: 90, End: 101, CPU: 1, PrefixMaxEnd: 101}},
		3: {{Start: 90, End: 101, CPU: 3, PrefixMaxEnd: 101}},
	}
	index = newTraceDBSchedulerRunningIndex(authority, crossing,
		traceDBRunningIntegrity{TaintedITIDs: map[int64]bool{}}, nil)
	if _, ok := index.knownCPUAt(1, 95); ok {
		t.Fatal("same-identity cross-cut Running row did not taint its whole lane")
	}
	if cpu, ok := index.knownCPUAt(3, 95); !ok || cpu != 3 {
		t.Fatalf("unrelated Running lane was connected to generation cut: cpu=%d ok=%t", cpu, ok)
	}
}

func TestTraceDBSchedulerRunningIndexPIDZeroAndGlobalTaint(t *testing.T) {
	authority := traceDBSchedulerAuthorityFixture(true, traceDBLifecycleIndex{})
	authority.identities.Processes[3] = traceDBProcess{IPID: 3, PID: 0, Name: "kernel"}
	authority.identities.ByITID[3] = traceDBThread{ITID: 3, TID: 77, IPID: 3, Name: "kernel-worker"}
	buildTraceDBThreadSecondaryIndexes(&authority.identities)
	raw := map[int64][]traceDBRunningInterval{
		3: {{Start: 5, End: 10, CPU: 3, PrefixMaxEnd: 10}},
		0: {{Start: 5, End: 10, CPU: 0, PrefixMaxEnd: 10}},
	}
	index := newTraceDBSchedulerRunningIndex(authority, raw,
		traceDBRunningIntegrity{TaintedITIDs: map[int64]bool{}}, nil)
	if cpu, ok := index.knownCPUAt(3, 6); !ok || cpu != 3 {
		t.Fatalf("PID-zero Running lane incorrectly required process lifecycle: cpu=%d ok=%t", cpu, ok)
	}

	authority.lifecycle.ByTID = map[int64]traceDBLifecycleLane{77: {Tainted: true}}
	index = newTraceDBSchedulerRunningIndex(authority, raw,
		traceDBRunningIntegrity{TaintedITIDs: map[int64]bool{}}, nil)
	if _, ok := index.knownCPUAt(3, 6); ok {
		t.Fatal("PID-zero Running lane bypassed matching thread taint")
	}
	if cpu, ok := index.knownCPUAt(0, 6); !ok || cpu != 0 {
		t.Fatalf("thread-lane taint incorrectly spread to idle: cpu=%d ok=%t", cpu, ok)
	}

	authority.lifecycle.GlobalTaint = true
	index = newTraceDBSchedulerRunningIndex(authority, raw,
		traceDBRunningIntegrity{TaintedITIDs: map[int64]bool{}}, nil)
	if _, ok := index.knownCPUAt(3, 6); ok {
		t.Fatal("global lifecycle taint allowed positive Running identity")
	}
	if _, ok := index.knownCPUAt(0, 6); ok {
		t.Fatal("global lifecycle taint allowed idle Running identity")
	}
}
