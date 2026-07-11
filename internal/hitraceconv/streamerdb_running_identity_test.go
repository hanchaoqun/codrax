package hitraceconv

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func traceDBLoadRunningIdentityFixture(
	t *testing.T,
	threadStateDDL string,
	rows ...string,
) (map[int64][]traceDBRunningInterval, traceDBRunningIntegrity, TraceDBCoverage, traceDBThreadIndex) {
	t.Helper()
	return traceDBLoadRunningIdentityFixtureWithMutation(t, threadStateDDL, nil, rows...)
}

func traceDBLoadRunningIdentityFixtureWithMutation(
	t *testing.T,
	threadStateDDL string,
	mutate func(*traceDBThreadIndex),
	rows ...string,
) (map[int64][]traceDBRunningInterval, traceDBRunningIntegrity, TraceDBCoverage, traceDBThreadIndex) {
	t.Helper()
	statements := []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 100, 'user-a')",
		"INSERT INTO process VALUES (2, 200, 'user-b')",
		"INSERT INTO process VALUES (3, 0, 'kernel')",
		"INSERT INTO process VALUES (4, 400, 'user-c')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (1, 42, 1, 'thread-a', 0, 0, 1)",
		"INSERT INTO thread VALUES (2, 43, 2, 'thread-b', 0, 0, 1)",
		"INSERT INTO thread VALUES (3, 77, 3, 'kernel-worker', 0, 0, 1)",
		"INSERT INTO thread VALUES (4, 44, 4, 'thread-c', 0, 0, 1)",
		threadStateDDL,
	}
	statements = append(statements, rows...)
	path := createTraceDBFixture(t, statements)
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	identities, _, err := tdb.loadThreadIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if mutate != nil {
		mutate(&identities)
	}
	intervals, integrity, coverage, err := tdb.loadRunningIntervals(context.Background(), identities)
	if err != nil {
		t.Fatal(err)
	}
	return intervals, integrity, coverage, identities
}

func TestTraceDBRunningIdentityClaimsAcceptExactNullableIdleAndKernel(t *testing.T) {
	intervals, integrity, coverage, _ := traceDBLoadRunningIdentityFixture(t,
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid, pid)",
		"INSERT INTO thread_state VALUES (1, 10, 10, 1, 'Running', 42, 100)",
		"INSERT INTO thread_state VALUES (3, 20, 10, 2, 'Running', 77, 0)",
		"INSERT INTO thread_state VALUES (0, 30, 10, 0, 'Running', 0, 0)",
		"INSERT INTO thread_state VALUES (2, 40, 10, 3, 'Running', NULL, NULL)",
		"INSERT INTO thread_state VALUES (1, 50, 10, 4, 'Runnable', 999, 999)",
	)
	if integrity.GlobalTaint || len(integrity.TaintedITIDs) != 0 || coverage.RowsRead != 5 || coverage.RowsEmitted != 4 || coverage.Skipped != "" {
		t.Fatalf("valid/nullable Running identity claims changed: integrity=%+v coverage=%+v", integrity, coverage)
	}
	for _, want := range []struct {
		itid int64
		ts   int64
		cpu  int64
	}{{1, 10, 1}, {3, 20, 2}, {0, 30, 0}, {2, 40, 3}} {
		if cpu, ok := traceDBKnownCPUAt(intervals, want.itid, want.ts); !ok || cpu != want.cpu {
			t.Fatalf("Running witness (%d,%d)=(%d,%t), want CPU%d", want.itid, want.ts, cpu, ok, want.cpu)
		}
	}
	if !reflect.DeepEqual(coverage.ColumnsPresent, []string{"cpu", "dur", "itid", "pid", "state", "tid", "ts"}) ||
		!strings.Contains(coverage.FieldSources["subject_cross_check"], "UINT32_MAX missing sentinels") {
		t.Fatalf("Running identity provenance mismatch: %+v", coverage)
	}
}

func TestTraceDBRunningIdentityMissingSentinelsAreNoClaim(t *testing.T) {
	const missing = int64(maxTraceDBInternalID + 1)
	intervals, integrity, coverage, _ := traceDBLoadRunningIdentityFixture(t,
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid, pid)",
		"INSERT INTO thread_state VALUES (1, 10, 10, 1, 'Running', 4294967295, 4294967295)",
		"INSERT INTO thread_state VALUES (1, 20, 10, 1, 'Running', 4294967295, 100)",
		"INSERT INTO thread_state VALUES (1, 30, 10, 1, 'Running', 42, 4294967295)",
		"INSERT INTO thread_state VALUES (1, 40, 10, 1, 'Running', NULL, 100)",
		"INSERT INTO thread_state VALUES (1, 50, 10, 1, 'Running', 42, NULL)",
		"INSERT INTO thread_state VALUES (0, 60, 10, 0, 'Running', 4294967295, 4294967295)",
		"INSERT INTO thread_state VALUES (3, 70, 10, 2, 'Running', 77, 0)",
		"INSERT INTO thread_state VALUES (999, 80, 10, 3, 'Running', 4294967295, 4294967295)",
	)
	if integrity.GlobalTaint || len(integrity.TaintedITIDs) != 0 || coverage.RowsEmitted != 8 || coverage.Skipped != "" {
		t.Fatalf("upstream missing identity sentinels were treated as malformed claims: integrity=%+v coverage=%+v", integrity, coverage)
	}
	if value, claimed, valid := traceDBRunningPublicIdentityClaim(true, missing); !valid || claimed || value != 0 {
		t.Fatalf("UINT32_MAX claim=(%d,%t,%t), want missing/valid", value, claimed, valid)
	}
	if value, claimed, valid := traceDBRunningPublicIdentityClaim(true, int64(0)); !valid || !claimed || value != 0 {
		t.Fatalf("zero claim=(%d,%t,%t), want exact claimed zero", value, claimed, valid)
	}
	for _, endpoint := range []struct {
		itid int64
		ts   int64
		cpu  int64
	}{{1, 10, 1}, {1, 20, 1}, {1, 30, 1}, {1, 40, 1}, {1, 50, 1}, {0, 60, 0}, {3, 70, 2}, {999, 80, 3}} {
		if cpu, ok := traceDBKnownCPUAt(intervals, endpoint.itid, endpoint.ts); !ok || cpu != endpoint.cpu {
			t.Fatalf("sentinel/NULL endpoint (%d,%d)=(%d,%t), want CPU%d", endpoint.itid, endpoint.ts, cpu, ok, endpoint.cpu)
		}
	}
}

func TestTraceDBRunningIdentityNamesRemainDisplayOnly(t *testing.T) {
	_, integrity, coverage, _ := traceDBLoadRunningIdentityFixtureWithMutation(t,
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid, pid)",
		func(index *traceDBThreadIndex) {
			thread := index.ByITID[1]
			thread.Name = "renamed-thread\nwith-display-noise"
			index.ByITID[1] = thread
			process := index.Processes[1]
			process.Name = "renamed-process"
			index.Processes[1] = process
		},
		"INSERT INTO thread_state VALUES (1, 10, 10, 1, 'Running', 42, 100)",
	)
	if integrity.GlobalTaint || len(integrity.TaintedITIDs) != 0 || coverage.RowsEmitted != 1 {
		t.Fatalf("display rename affected hard Running identity: integrity=%+v coverage=%+v", integrity, coverage)
	}
}

func TestTraceDBRunningIdentityIdleUsesCanonicalAuthority(t *testing.T) {
	tests := []struct {
		name       string
		ddl        string
		row        string
		mutate     func(*traceDBThreadIndex)
		wantAccept bool
	}{
		{
			name:       "unmaterialized canonical idle",
			ddl:        "CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid, pid)",
			row:        "INSERT INTO thread_state VALUES (0, 10, 10, 0, 'Running', 0, 0)",
			wantAccept: true,
		},
		{
			name: "exact materialized canonical idle",
			ddl:  "CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid, pid)",
			row:  "INSERT INTO thread_state VALUES (0, 10, 10, 0, 'Running', 0, 0)",
			mutate: func(index *traceDBThreadIndex) {
				index.ByITID[0] = traceDBThread{ITID: 0, TID: 0, IPID: 0}
				index.Processes[0] = traceDBProcess{IPID: 0, PID: 0}
			},
			wantAccept: true,
		},
		{
			name: "ambiguous itid zero",
			ddl:  "CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid, pid)",
			row:  "INSERT INTO thread_state VALUES (0, 10, 10, 0, 'Running', 0, 0)",
			mutate: func(index *traceDBThreadIndex) {
				index.AmbiguousITID[0] = true
			},
		},
		{
			name: "ambiguous ipid zero",
			ddl:  "CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid, pid)",
			row:  "INSERT INTO thread_state VALUES (0, 10, 10, 0, 'Running', 0, 0)",
			mutate: func(index *traceDBThreadIndex) {
				index.AmbiguousIPID[0] = true
			},
		},
		{
			name: "conflicting materialized thread zero",
			ddl:  "CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid, pid)",
			row:  "INSERT INTO thread_state VALUES (0, 10, 10, 0, 'Running', 0, 0)",
			mutate: func(index *traceDBThreadIndex) {
				index.ByITID[0] = traceDBThread{ITID: 0, TID: 1, IPID: 0}
			},
		},
		{
			name: "conflicting materialized process zero",
			ddl:  "CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid, pid)",
			row:  "INSERT INTO thread_state VALUES (0, 10, 10, 0, 'Running', 0, 0)",
			mutate: func(index *traceDBThreadIndex) {
				index.Processes[0] = traceDBProcess{IPID: 0, PID: 1}
			},
		},
		{
			name: "conflict cannot hide behind absent public claims",
			ddl:  "CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
			row:  "INSERT INTO thread_state VALUES (0, 10, 10, 0, 'Running')",
			mutate: func(index *traceDBThreadIndex) {
				index.AmbiguousITID[0] = true
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			intervals, integrity, coverage, identities := traceDBLoadRunningIdentityFixtureWithMutation(t, test.ddl, test.mutate, test.row)
			authority := traceDBTestCompleteSchedulerAuthority(identities)
			_, schedulerAccepts := authority.schedulerSubjectFromExactITID(0, true)
			if schedulerAccepts != test.wantAccept {
				t.Fatalf("scheduler canonical-idle parity=%t, want %t", schedulerAccepts, test.wantAccept)
			}
			if test.wantAccept {
				if integrity.GlobalTaint || integrity.TaintedITIDs[0] || coverage.RowsEmitted != 1 {
					t.Fatalf("canonical idle rejected: integrity=%+v coverage=%+v", integrity, coverage)
				}
				if cpu, ok := traceDBKnownCPUAt(intervals, 0, 10); !ok || cpu != 0 {
					t.Fatalf("canonical idle CPU=(%d,%t), want CPU0", cpu, ok)
				}
				return
			}
			if integrity.GlobalTaint || !integrity.TaintedITIDs[0] || coverage.RowsEmitted != 0 ||
				!strings.Contains(coverage.Skipped, "ambiguous_idle_identity=1") {
				t.Fatalf("conflicting idle identity did not taint lane zero: integrity=%+v coverage=%+v", integrity, coverage)
			}
		})
	}
}

func TestTraceDBRunningIdentityClaimMismatchTaintsOnlyItsITIDLane(t *testing.T) {
	intervals, integrity, coverage, identities := traceDBLoadRunningIdentityFixture(t,
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid, pid)",
		"INSERT INTO thread_state VALUES (1, 10, 10, 1, 'Running', 42, 100)",
		"INSERT INTO thread_state VALUES (1, 20, 10, 1, 'Running', 43, 100)",
		"INSERT INTO thread_state VALUES (2, 30, 10, 2, 'Running', 43, 100)",
		"INSERT INTO thread_state VALUES (4, 40, 10, 4, 'Running', 44, 400)",
	)
	if integrity.GlobalTaint || !integrity.TaintedITIDs[1] || !integrity.TaintedITIDs[2] || integrity.TaintedITIDs[4] ||
		coverage.RowsEmitted != 2 || !strings.Contains(coverage.Skipped, "tid_claim_mismatch=1") ||
		!strings.Contains(coverage.Skipped, "pid_claim_mismatch=1") {
		t.Fatalf("Running claim mismatch taint locality/accounting mismatch: integrity=%+v coverage=%+v", integrity, coverage)
	}
	authority := traceDBTestCompleteSchedulerAuthority(identities)
	typedCoverage := coverage
	typed := newTraceDBSchedulerRunningIndex(authority, intervals, integrity, &typedCoverage)
	if _, status := typed.lookupCPUAt(1, 10); status != traceDBSchedulerRunningSourceTainted {
		t.Fatalf("valid sibling rescued a mismatched Running lane: status=%d", status)
	}
	if _, status := typed.lookupCPUAt(2, 30); status != traceDBSchedulerRunningSourceTainted {
		t.Fatalf("PID-mismatched Running lane status=%d, want source tainted", status)
	}
	if cpu, status := typed.lookupCPUAt(4, 40); status != traceDBSchedulerRunningKnown || cpu != 4 {
		t.Fatalf("unrelated Running lane was connected: cpu=%d status=%d", cpu, status)
	}
}

func TestTraceDBRunningIdentityAuthorityFailuresAreExact(t *testing.T) {
	tests := []struct {
		name   string
		ddl    string
		row    string
		mutate func(*traceDBThreadIndex)
		reason string
	}{
		{
			name: "ambiguous thread",
			ddl:  "CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid)",
			row:  "INSERT INTO thread_state VALUES (1, 10, 10, 1, 'Running', 42)",
			mutate: func(index *traceDBThreadIndex) {
				index.AmbiguousITID[1] = true
			},
			reason: "ambiguous_claim_subject=1",
		},
		{
			name: "missing thread",
			ddl:  "CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid)",
			row:  "INSERT INTO thread_state VALUES (1, 10, 10, 1, 'Running', 42)",
			mutate: func(index *traceDBThreadIndex) {
				delete(index.ByITID, 1)
			},
			reason: "unresolved_claim_subject=1",
		},
		{
			name: "ambiguous process",
			ddl:  "CREATE TABLE thread_state (itid, ts, dur, cpu, state, pid)",
			row:  "INSERT INTO thread_state VALUES (1, 10, 10, 1, 'Running', 100)",
			mutate: func(index *traceDBThreadIndex) {
				index.AmbiguousIPID[1] = true
			},
			reason: "ambiguous_claim_process=1",
		},
		{
			name: "missing process",
			ddl:  "CREATE TABLE thread_state (itid, ts, dur, cpu, state, pid)",
			row:  "INSERT INTO thread_state VALUES (1, 10, 10, 1, 'Running', 100)",
			mutate: func(index *traceDBThreadIndex) {
				delete(index.Processes, 1)
			},
			reason: "unresolved_claim_process=1",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, integrity, coverage, _ := traceDBLoadRunningIdentityFixtureWithMutation(t, test.ddl, test.mutate, test.row)
			if integrity.GlobalTaint || !integrity.TaintedITIDs[1] || coverage.RowsEmitted != 0 ||
				!strings.Contains(coverage.Skipped, test.reason) {
				t.Fatalf("authority failure %q was not isolated: integrity=%+v coverage=%+v", test.reason, integrity, coverage)
			}
		})
	}

	t.Run("tid only does not require process", func(t *testing.T) {
		_, integrity, coverage, _ := traceDBLoadRunningIdentityFixtureWithMutation(t,
			"CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid)",
			func(index *traceDBThreadIndex) { delete(index.Processes, 1) },
			"INSERT INTO thread_state VALUES (1, 10, 10, 1, 'Running', 42)",
		)
		if integrity.GlobalTaint || len(integrity.TaintedITIDs) != 0 || coverage.RowsEmitted != 1 {
			t.Fatalf("TID-only claim improperly required a process: integrity=%+v coverage=%+v", integrity, coverage)
		}
	})
}

func TestTraceDBRunningIdentityClaimBoundsAndStorageClasses(t *testing.T) {
	intervals, integrity, coverage, _ := traceDBLoadRunningIdentityFixtureWithMutation(t,
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid, pid)",
		func(index *traceDBThreadIndex) {
			thread := index.ByITID[4]
			thread.TID = math.MaxInt32
			index.ByITID[4] = thread
			process := index.Processes[4]
			process.PID = math.MaxInt32
			index.Processes[4] = process
		},
		"INSERT INTO thread_state VALUES (4, 10, 10, 4, 'Running', 2147483647, 2147483647)",
		"INSERT INTO thread_state VALUES (1, 20, 10, 1, 'Running', X'2A', 100)",
		"INSERT INTO thread_state VALUES (2, 30, 10, 2, 'Running', 43, 2147483648)",
	)
	if integrity.GlobalTaint || !integrity.TaintedITIDs[1] || !integrity.TaintedITIDs[2] || integrity.TaintedITIDs[4] ||
		coverage.RowsEmitted != 1 || !strings.Contains(coverage.Skipped, "invalid_tid_claim=1") ||
		!strings.Contains(coverage.Skipped, "invalid_pid_claim=1") {
		t.Fatalf("public identity storage/bounds mismatch: integrity=%+v coverage=%+v", integrity, coverage)
	}
	if cpu, ok := traceDBKnownCPUAt(intervals, 4, 10); !ok || cpu != 4 {
		t.Fatalf("MaxInt32 exact public claims lost: cpu=%d ok=%t", cpu, ok)
	}
}

func TestTraceDBRunningIdentityTaintCannotBeRescuedInEitherOrder(t *testing.T) {
	tests := []struct {
		name string
		rows []string
	}{
		{
			name: "valid then mismatched",
			rows: []string{
				"INSERT INTO thread_state VALUES (1, 10, 20, 1, 'Running', 42, 100)",
				"INSERT INTO thread_state VALUES (1, 20, 20, 1, 'Running', 43, 100)",
			},
		},
		{
			name: "mismatched then valid",
			rows: []string{
				"INSERT INTO thread_state VALUES (1, 20, 20, 1, 'Running', 43, 100)",
				"INSERT INTO thread_state VALUES (1, 10, 20, 1, 'Running', 42, 100)",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows := append([]string{}, test.rows...)
			rows = append(rows, "INSERT INTO thread_state VALUES (4, 10, 20, 4, 'Running', 44, 400)")
			intervals, integrity, _, identities := traceDBLoadRunningIdentityFixture(t,
				"CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid, pid)", rows...)
			identities.RunningTaintedITID = integrity.TaintedITIDs
			identities.RunningGlobalTaint = integrity.GlobalTaint
			if _, status := traceDBExtendedRunningCPUAt(identities, intervals, 1, 15); status != traceDBExtendedRunningSourceTainted {
				t.Fatalf("mismatched sibling rescued lane in %s order: status=%d", test.name, status)
			}
			if cpu, status := traceDBExtendedRunningCPUAt(identities, intervals, 4, 15); status != traceDBExtendedRunningKnown || cpu != 4 {
				t.Fatalf("unrelated lane connected in %s order: cpu=%d status=%d", test.name, cpu, status)
			}
		})
	}

	t.Run("unplaceable itid globally taints every lookup", func(t *testing.T) {
		intervals, integrity, _, identities := traceDBLoadRunningIdentityFixture(t,
			"CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid, pid)",
			"INSERT INTO thread_state VALUES (CAST(1 AS TEXT), 10, 20, 1, 'Running', 42, 100)",
			"INSERT INTO thread_state VALUES (4, 10, 20, 4, 'Running', 44, 400)",
		)
		identities.RunningTaintedITID = integrity.TaintedITIDs
		identities.RunningGlobalTaint = integrity.GlobalTaint
		if !integrity.GlobalTaint {
			t.Fatalf("invalid ITID did not taint globally: %+v", integrity)
		}
		if _, status := traceDBExtendedRunningCPUAt(identities, intervals, 4, 15); status != traceDBExtendedRunningSourceTainted {
			t.Fatalf("global poison was rescued by a valid lane: status=%d", status)
		}
	})
}

func TestTraceDBRunningIdentityClaimsStrictStorageAndTaintPlacement(t *testing.T) {
	_, integrity, coverage, _ := traceDBLoadRunningIdentityFixture(t,
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid, pid)",
		"INSERT INTO thread_state VALUES (1, 10, 10, 1, 'Running', CAST(42 AS TEXT), 100)",
		"INSERT INTO thread_state VALUES (2, 20, 10, 2, 'Running', 43, CAST(200 AS REAL))",
		"INSERT INTO thread_state VALUES (3, 30, 10, 3, 'Running', 77, -1)",
		"INSERT INTO thread_state VALUES (0, 40, 10, 0, 'Running', 42, 0)",
		"INSERT INTO thread_state VALUES (999, 50, 10, 5, 'Running', 999, 999)",
		"INSERT INTO thread_state VALUES (CAST(1 AS TEXT), 60, 10, 6, 'Running', 42, 100)",
		"INSERT INTO thread_state VALUES (CAST(1 AS TEXT), 70, 10, 7, 'Runnable', 42, 100)",
	)
	for _, itid := range []int64{0, 1, 2, 3, 999} {
		if !integrity.TaintedITIDs[itid] {
			t.Fatalf("Running claim failure did not taint ITID %d: %+v", itid, integrity)
		}
	}
	if !integrity.GlobalTaint {
		t.Fatalf("unplaceable potential Running identity did not taint globally: %+v", integrity)
	}
	for _, reason := range []string{
		"invalid_tid_claim=1", "invalid_pid_claim=2", "idle_tid_claim_mismatch=1",
		"unresolved_claim_subject=1", "invalid_scalar_or_interval=1",
	} {
		if !strings.Contains(coverage.Skipped, reason) {
			t.Fatalf("Running strict claim coverage missing %q: %+v", reason, coverage)
		}
	}
	if coverage.RowsEmitted != 0 {
		t.Fatalf("invalid Running claims emitted intervals: %+v", coverage)
	}
}

func TestTraceDBRunningIdentityClaimsAreIndependentlyOptional(t *testing.T) {
	t.Run("tid only", func(t *testing.T) {
		_, integrity, coverage, _ := traceDBLoadRunningIdentityFixture(t,
			"CREATE TABLE thread_state (itid, ts, dur, cpu, state, tid)",
			"INSERT INTO thread_state VALUES (1, 10, 10, 1, 'Running', 42)",
			"INSERT INTO thread_state VALUES (2, 20, 10, 2, 'Running', 42)",
		)
		if integrity.GlobalTaint || integrity.TaintedITIDs[1] || !integrity.TaintedITIDs[2] || coverage.RowsEmitted != 1 {
			t.Fatalf("tid-only Running claims mismatch: integrity=%+v coverage=%+v", integrity, coverage)
		}
	})
	t.Run("pid only", func(t *testing.T) {
		_, integrity, coverage, _ := traceDBLoadRunningIdentityFixture(t,
			"CREATE TABLE thread_state (itid, ts, dur, cpu, state, pid)",
			"INSERT INTO thread_state VALUES (1, 10, 10, 1, 'Running', 100)",
			"INSERT INTO thread_state VALUES (2, 20, 10, 2, 'Running', 100)",
		)
		if integrity.GlobalTaint || integrity.TaintedITIDs[1] || !integrity.TaintedITIDs[2] || coverage.RowsEmitted != 1 {
			t.Fatalf("pid-only Running claims mismatch: integrity=%+v coverage=%+v", integrity, coverage)
		}
	})
	t.Run("legacy absent columns", func(t *testing.T) {
		_, integrity, coverage, _ := traceDBLoadRunningIdentityFixture(t,
			"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
			"INSERT INTO thread_state VALUES (999, 10, 10, 1, 'Running')",
		)
		if integrity.GlobalTaint || len(integrity.TaintedITIDs) != 0 || coverage.RowsEmitted != 1 {
			t.Fatalf("absent optional identity columns became a mandatory claim: integrity=%+v coverage=%+v", integrity, coverage)
		}
	})
}

func TestTraceDBRunningIdentityCrossCheckIsStructurallyPinned(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source")
	}
	dir := filepath.Dir(current)
	parse := func(name string) *ast.File {
		t.Helper()
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		return file
	}
	isIdent := func(expression ast.Expr, name string) bool {
		ident, ok := expression.(*ast.Ident)
		return ok && ident.Name == name
	}
	isSelector := func(expression ast.Expr, receiver, field string) bool {
		selector, ok := expression.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != field {
			return false
		}
		return isIdent(selector.X, receiver)
	}

	core := parse("streamerdb_core.go")
	claimCalls := 0
	strictITIDCalls := 0
	genericITIDCalls := 0
	failCalls := 0
	coverageErrorAssignments := 0
	for _, declaration := range core.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "loadRunningIntervals" || function.Body == nil {
			continue
		}
		identityParams := 0
		for _, field := range function.Type.Params.List {
			if ident, ok := field.Type.(*ast.Ident); ok && ident.Name == "traceDBThreadIndex" {
				identityParams += len(field.Names)
			}
		}
		if identityParams != 1 {
			t.Fatalf("Running loader identity params=%d, want 1", identityParams)
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			if assignment, ok := node.(*ast.AssignStmt); ok {
				for _, lhs := range assignment.Lhs {
					if isSelector(lhs, "coverage", "Error") {
						coverageErrorAssignments++
					}
				}
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			callee, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			if callee.Name == "traceDBStrictInternalID" && len(call.Args) == 1 && isIdent(call.Args[0], "itidRaw") {
				strictITIDCalls++
			}
			if callee.Name == "traceDBStrictSQLiteInt" && len(call.Args) == 1 && isIdent(call.Args[0], "itidRaw") {
				genericITIDCalls++
			}
			if callee.Name == "fail" {
				failCalls++
			}
			if callee.Name != "traceDBRunningSubjectClaimReason" {
				return true
			}
			want := []string{"identities", "itid", "hasTID", "tidRaw", "hasPID", "pidRaw"}
			if len(call.Args) != len(want) {
				t.Fatalf("Running claim gate args=%d, want %d", len(call.Args), len(want))
			}
			for i, argument := range call.Args {
				ident, ok := argument.(*ast.Ident)
				if !ok || ident.Name != want[i] {
					t.Fatalf("Running claim gate arg %d is not %s", i, want[i])
				}
			}
			claimCalls++
			return true
		})
	}
	if claimCalls != 1 {
		t.Fatalf("Running subject claim calls=%d, want 1", claimCalls)
	}
	if strictITIDCalls != 2 || genericITIDCalls != 0 {
		t.Fatalf("Running ITID decoder calls strict=%d generic=%d, want 2/0", strictITIDCalls, genericITIDCalls)
	}
	if failCalls != 5 || coverageErrorAssignments != 1 {
		t.Fatalf("Running post-inspect failure chokepoint calls=%d coverage.Error assignments=%d, want 5/1", failCalls, coverageErrorAssignments)
	}

	callers := map[string]map[string]int{}
	canonicalIdleCallers := map[string]int{}
	extendedLookupCallers := map[string]int{}
	knownCPUCallers := map[string]int{}
	indexAuthorityAssignments := 0
	extendedGuardSelectors := map[string]int{}
	runningHandoffAssignments := map[string]int{}
	var lastRunningHandoff, firstExtendedRunningDispatch token.Pos
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if assignment, ok := node.(*ast.AssignStmt); ok && function.Name.Name == "exportTraceDBExtendedFamilies" &&
					assignment.Tok == token.DEFINE && len(assignment.Lhs) == 1 && len(assignment.Rhs) == 1 &&
					isIdent(assignment.Lhs[0], "index") && isSelector(assignment.Rhs[0], "authority", "identities") {
					indexAuthorityAssignments++
				}
				if assignment, ok := node.(*ast.AssignStmt); ok && function.Name.Name == "exportTraceDBExtendedFamilies" &&
					len(assignment.Lhs) == 1 && len(assignment.Rhs) == 1 {
					switch {
					case isSelector(assignment.Lhs[0], "index", "RunningTaintedITID") && isSelector(assignment.Rhs[0], "runningIntegrity", "TaintedITIDs"):
						runningHandoffAssignments["lane"]++
						if assignment.End() > lastRunningHandoff {
							lastRunningHandoff = assignment.End()
						}
					case isSelector(assignment.Lhs[0], "index", "RunningGlobalTaint") && isSelector(assignment.Rhs[0], "runningIntegrity", "GlobalTaint"):
						runningHandoffAssignments["global"]++
						if assignment.End() > lastRunningHandoff {
							lastRunningHandoff = assignment.End()
						}
					}
				}
				if selector, ok := node.(*ast.SelectorExpr); ok && function.Name.Name == "traceDBExtendedRunningCPUAt" && isIdent(selector.X, "index") &&
					(selector.Sel.Name == "RunningGlobalTaint" || selector.Sel.Name == "RunningTaintedITID") {
					extendedGuardSelectors[selector.Sel.Name]++
				}
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := ""
				switch callee := call.Fun.(type) {
				case *ast.Ident:
					name = callee.Name
				case *ast.SelectorExpr:
					name = callee.Sel.Name
				}
				if name == "loadRunningIntervals" || name == "loadExtendedLegacyRunningIntervals" {
					if callers[name] == nil {
						callers[name] = map[string]int{}
					}
					callers[name][function.Name.Name]++
				}
				if name == "traceDBCanonicalIdleIdentityExact" {
					canonicalIdleCallers[function.Name.Name]++
				}
				if name == "traceDBExtendedRunningCPUAt" {
					extendedLookupCallers[function.Name.Name]++
				}
				if name == "traceDBKnownCPUAt" {
					knownCPUCallers[function.Name.Name]++
				}
				if name == "exportTraceDBRawFtraceFamilies" && function.Name.Name == "exportTraceDBExtendedFamilies" {
					firstExtendedRunningDispatch = call.Pos()
				}
				switch {
				case name == "loadRunningIntervals" && function.Name.Name == "loadSchedulerRunningIndex":
					if len(call.Args) != 2 || !isIdent(call.Args[0], "ctx") || !isSelector(call.Args[1], "authority", "identities") {
						t.Fatalf("scheduler Running loader did not receive authority.identities")
					}
				case name == "loadRunningIntervals" && function.Name.Name == "loadExtendedLegacyRunningIntervals":
					if len(call.Args) != 2 || !isIdent(call.Args[0], "ctx") || !isIdent(call.Args[1], "identities") {
						t.Fatalf("extended Running facade did not forward its identities argument")
					}
				case name == "loadExtendedLegacyRunningIntervals" && function.Name.Name == "exportTraceDBExtendedFamilies":
					if len(call.Args) != 2 || !isIdent(call.Args[0], "ctx") || !isIdent(call.Args[1], "index") {
						t.Fatalf("extended export did not pass its authority-derived index")
					}
				case name == "traceDBCanonicalIdleIdentityExact" && function.Name.Name == "schedulerSubjectIsExact":
					if len(call.Args) != 1 || !isSelector(call.Args[0], "authority", "identities") {
						t.Fatalf("scheduler idle gate did not use authority.identities")
					}
				case name == "traceDBCanonicalIdleIdentityExact" && function.Name.Name == "exportTraceDBThreadRegistrations":
					if len(call.Args) != 1 || !isIdent(call.Args[0], "index") {
						t.Fatalf("registration idle gate did not use the shared identity index")
					}
				case name == "traceDBCanonicalIdleIdentityExact" && function.Name.Name == "traceDBRunningSubjectClaimReason":
					if len(call.Args) != 1 || !isIdent(call.Args[0], "identities") {
						t.Fatalf("Running idle gate did not use loader identities")
					}
				}
				return true
			})
		}
	}
	wantCallers := map[string]map[string]int{
		"loadRunningIntervals": {
			"loadExtendedLegacyRunningIntervals": 1,
			"loadSchedulerRunningIndex":          1,
		},
		"loadExtendedLegacyRunningIntervals": {"exportTraceDBExtendedFamilies": 1},
	}
	if !reflect.DeepEqual(callers, wantCallers) {
		t.Fatalf("Running loader call graph=%v, want %v", callers, wantCallers)
	}
	if !reflect.DeepEqual(canonicalIdleCallers, map[string]int{
		"exportTraceDBThreadRegistrations": 1,
		"schedulerSubjectIsExact":          1,
		"traceDBRunningSubjectClaimReason": 1,
	}) {
		t.Fatalf("canonical idle authority callers=%v", canonicalIdleCallers)
	}
	if indexAuthorityAssignments != 1 {
		t.Fatalf("extended authority-derived index assignments=%d, want 1", indexAuthorityAssignments)
	}
	if !reflect.DeepEqual(runningHandoffAssignments, map[string]int{"global": 1, "lane": 1}) ||
		lastRunningHandoff == 0 || firstExtendedRunningDispatch == 0 || lastRunningHandoff >= firstExtendedRunningDispatch {
		t.Fatalf("extended Running integrity handoff=%v last=%d first-dispatch=%d", runningHandoffAssignments, lastRunningHandoff, firstExtendedRunningDispatch)
	}
	if !reflect.DeepEqual(extendedGuardSelectors, map[string]int{"RunningGlobalTaint": 1, "RunningTaintedITID": 1}) {
		t.Fatalf("extended Running lookup source guards=%v", extendedGuardSelectors)
	}
	if !reflect.DeepEqual(extendedLookupCallers, map[string]int{
		"exportTraceDBRawFtraceFamilies": 1,
		"prepareTraceDBFrameSliceRow":    2,
		"prepareTraceDBNativeHookEvent":  1,
	}) {
		t.Fatalf("extended Running lookup callers=%v", extendedLookupCallers)
	}
	if !reflect.DeepEqual(knownCPUCallers, map[string]int{
		"lookupCPUAt":                 1,
		"traceDBExtendedRunningCPUAt": 1,
	}) {
		t.Fatalf("raw Running CPU lookup callers=%v", knownCPUCallers)
	}
}
