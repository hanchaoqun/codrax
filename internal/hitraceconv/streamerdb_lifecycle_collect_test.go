package hitraceconv

import (
	"context"
	"database/sql"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func traceDBLifecycleCollectorIndex(traceStart int64, traceStartKnown bool) traceDBThreadIndex {
	index := newTraceDBThreadIndex(traceStart, traceStartKnown)
	index.Processes[1] = traceDBProcess{IPID: 1, PID: 10, Name: "old-owner"}
	index.Processes[2] = traceDBProcess{IPID: 2, PID: 20, Name: "new-owner"}
	index.ByITID[1] = traceDBThread{ITID: 1, TID: 42, IPID: 1, Name: "old"}
	index.ByITID[2] = traceDBThread{ITID: 2, TID: 42, IPID: 2, Name: "new"}
	index.ThreadIDToITID[101] = 1
	index.ThreadIDToITID[102] = 2
	buildTraceDBThreadSecondaryIndexes(&index)
	return index
}

func traceDBLifecycleCollectorSchema() []string {
	return []string{
		"CREATE TABLE instant (ts, name, ref, ref_type)",
		"CREATE TABLE thread_state (ts, itid, state)",
		"CREATE TABLE sched_slice (ts, dur, itid, end_state)",
		"CREATE TABLE callstack (ts, itid, callid)",
		"CREATE TABLE syscall (ts, itid)",
		"CREATE TABLE native_hook (start_ts, itid)",
		"CREATE TABLE frame_slice (id, type, ts, itid)",
	}
}

func collectTraceDBLifecycleFixture(t *testing.T, index traceDBThreadIndex, statements []string) traceDBLifecycleCollection {
	t.Helper()
	path := createTraceDBFixture(t, statements)
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	collection, err := collectTraceDBLifecycle(context.Background(), tdb.db, index)
	if err != nil {
		t.Fatalf("collect lifecycle: %v", err)
	}
	return collection
}

func replaceTraceDBFixtureStatement(statements []string, old, replacement string) []string {
	out := append([]string(nil), statements...)
	for i, statement := range out {
		if statement == old {
			out[i] = replacement
			return out
		}
	}
	return out
}

func removeTraceDBFixtureStatement(statements []string, remove string) []string {
	out := make([]string, 0, len(statements))
	for _, statement := range statements {
		if statement != remove {
			out = append(out, statement)
		}
	}
	return out
}

func TestTraceDBLifecycleCollectorChoosesEarliestOfSixSourcesIndependentOfPhysicalOrder(t *testing.T) {
	load := func(t *testing.T, reverse bool) traceDBLifecycleCollection {
		t.Helper()
		statements := traceDBLifecycleCollectorSchema()
		rows := []string{
			"INSERT INTO thread_state VALUES (50,1,'X')",
			"INSERT INTO thread_state VALUES (70,2,'Running')",
			"INSERT INTO sched_slice VALUES (40,10,1,'X')",
			"INSERT INTO sched_slice VALUES (80,1,2,'R')",
			"INSERT INTO callstack VALUES (90,2,NULL)",
			"INSERT INTO syscall VALUES (60,2)",
			"INSERT INTO native_hook VALUES (65,2)",
			"INSERT INTO frame_slice VALUES (1,0,55,2)",
			"INSERT INTO frame_slice VALUES (2,0,75,2)",
		}
		if reverse {
			for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
				rows[left], rows[right] = rows[right], rows[left]
			}
		}
		return collectTraceDBLifecycleFixture(t, traceDBLifecycleCollectorIndex(0, true), append(statements, rows...))
	}

	forward := load(t, false)
	reverse := load(t, true)
	if !forward.CreationComplete || !forward.TerminalComplete || !forward.ActivityComplete {
		t.Fatalf("complete fixture lost authority: %+v", forward)
	}
	wantCut := []traceDBLifecycleBoundary{{TS: 55, NewITID: 2, NewIPID: 2}}
	if !reflect.DeepEqual(forward.Lifecycle.ByTID[42].Cuts, wantCut) ||
		!reflect.DeepEqual(forward.Lifecycle, reverse.Lifecycle) {
		t.Fatalf("six-source earliest cut depends on physical order:\nforward=%+v\nreverse=%+v", forward.Lifecycle, reverse.Lifecycle)
	}
	if !reflect.DeepEqual(forward.ActiveITIDs, map[int64]bool{1: true, 2: true}) {
		t.Fatalf("active registry diverged: %+v", forward.ActiveITIDs)
	}
	wantTables := []string{"callstack", "sched_slice", "thread_state", "syscall", "native_hook", "frame_slice"}
	if len(forward.ActiveCoverage) != len(wantTables) {
		t.Fatalf("active coverage count=%d, want %d: %+v", len(forward.ActiveCoverage), len(wantTables), forward.ActiveCoverage)
	}
	for i, table := range wantTables {
		if item := forward.ActiveCoverage[i]; item.Family != "resolver.active_thread" || item.Table != table {
			t.Fatalf("active coverage[%d]=%+v, want table %s", i, item, table)
		}
	}
}

func TestTraceDBLifecycleCollectorCompletenessGateKeepsOnlyDirectEvidence(t *testing.T) {
	baseRows := []string{
		"INSERT INTO sched_slice VALUES (40,10,1,'X')",
		"INSERT INTO thread_state VALUES (50,1,'X')",
		"INSERT INTO frame_slice VALUES (1,0,60,2)",
	}
	tests := []struct {
		name            string
		statements      func() []string
		wantCreation    bool
		wantTerminal    bool
		wantActivity    bool
		wantDirectCut   bool
		wantLaneTainted bool
	}{
		{
			name: "missing creation authority",
			statements: func() []string {
				return append(removeTraceDBFixtureStatement(traceDBLifecycleCollectorSchema(),
					"CREATE TABLE instant (ts, name, ref, ref_type)"), baseRows...)
			},
			wantTerminal: true, wantActivity: true,
		},
		{
			name: "missing one terminal authority column",
			statements: func() []string {
				schema := replaceTraceDBFixtureStatement(traceDBLifecycleCollectorSchema(),
					"CREATE TABLE thread_state (ts, itid, state)", "CREATE TABLE thread_state (ts, itid)")
				rows := []string{
					"INSERT INTO sched_slice VALUES (40,10,1,'X')",
					"INSERT INTO thread_state VALUES (50,1)",
					"INSERT INTO frame_slice VALUES (1,0,60,2)",
				}
				return append(schema, rows...)
			},
			wantCreation: true, wantActivity: true,
		},
		{
			name: "missing activity timestamp keeps direct creation and taints lane",
			statements: func() []string {
				schema := replaceTraceDBFixtureStatement(traceDBLifecycleCollectorSchema(),
					"CREATE TABLE frame_slice (id, type, ts, itid)", "CREATE TABLE frame_slice (id, type, itid)")
				rows := []string{
					"INSERT INTO instant VALUES (100,'sched_wakeup_new',2,'itid')",
					"INSERT INTO sched_slice VALUES (40,10,1,'X')",
					"INSERT INTO thread_state VALUES (50,1,'X')",
					"INSERT INTO frame_slice VALUES (1,0,2)",
				}
				return append(schema, rows...)
			},
			wantCreation: true, wantTerminal: true, wantDirectCut: true, wantLaneTainted: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			collection := collectTraceDBLifecycleFixture(t, traceDBLifecycleCollectorIndex(0, true), test.statements())
			if collection.CreationComplete != test.wantCreation || collection.TerminalComplete != test.wantTerminal ||
				collection.ActivityComplete != test.wantActivity {
				t.Fatalf("completeness=(%t,%t,%t), want (%t,%t,%t)", collection.CreationComplete,
					collection.TerminalComplete, collection.ActivityComplete, test.wantCreation, test.wantTerminal, test.wantActivity)
			}
			cuts := collection.Lifecycle.ByTID[42].Cuts
			if test.wantDirectCut {
				if !reflect.DeepEqual(cuts, []traceDBLifecycleBoundary{{TS: 100, NewITID: 2, NewIPID: 2}}) {
					t.Fatalf("direct creation was lost or inferred cut leaked: %+v", cuts)
				}
			} else if len(cuts) != 0 {
				t.Fatalf("inferred cut escaped incomplete authority: %+v", cuts)
			}
			if collection.Lifecycle.ByTID[42].Tainted != test.wantLaneTainted {
				t.Fatalf("lane taint=%t, want %t: %+v", collection.Lifecycle.ByTID[42].Tainted, test.wantLaneTainted, collection.Lifecycle)
			}
		})
	}
}

func TestTraceDBLifecycleCollectorRequiresEveryActivitySourceProfile(t *testing.T) {
	tests := []struct {
		name        string
		oldDDL      string
		replacement string
	}{
		{name: "callstack timestamp", oldDDL: "CREATE TABLE callstack (ts, itid, callid)", replacement: "CREATE TABLE callstack (itid, callid)"},
		{name: "callstack identity", oldDDL: "CREATE TABLE callstack (ts, itid, callid)", replacement: "CREATE TABLE callstack (ts)"},
		{name: "sched timestamp", oldDDL: "CREATE TABLE sched_slice (ts, dur, itid, end_state)", replacement: "CREATE TABLE sched_slice (dur, itid, end_state)"},
		{name: "sched identity", oldDDL: "CREATE TABLE sched_slice (ts, dur, itid, end_state)", replacement: "CREATE TABLE sched_slice (ts, dur, end_state)"},
		{name: "thread-state timestamp", oldDDL: "CREATE TABLE thread_state (ts, itid, state)", replacement: "CREATE TABLE thread_state (itid, state)"},
		{name: "thread-state identity", oldDDL: "CREATE TABLE thread_state (ts, itid, state)", replacement: "CREATE TABLE thread_state (ts, state)"},
		{name: "syscall timestamp", oldDDL: "CREATE TABLE syscall (ts, itid)", replacement: "CREATE TABLE syscall (itid)"},
		{name: "syscall identity", oldDDL: "CREATE TABLE syscall (ts, itid)", replacement: "CREATE TABLE syscall (ts)"},
		{name: "native timestamp", oldDDL: "CREATE TABLE native_hook (start_ts, itid)", replacement: "CREATE TABLE native_hook (itid)"},
		{name: "native identity", oldDDL: "CREATE TABLE native_hook (start_ts, itid)", replacement: "CREATE TABLE native_hook (start_ts)"},
		{name: "frame timestamp", oldDDL: "CREATE TABLE frame_slice (id, type, ts, itid)", replacement: "CREATE TABLE frame_slice (id, type, itid)"},
		{name: "frame identity", oldDDL: "CREATE TABLE frame_slice (id, type, ts, itid)", replacement: "CREATE TABLE frame_slice (id, type, ts)"},
		{name: "frame XOR profile", oldDDL: "CREATE TABLE frame_slice (id, type, ts, itid)", replacement: "CREATE TABLE frame_slice (id, ts, itid)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			statements := replaceTraceDBFixtureStatement(traceDBLifecycleCollectorSchema(), test.oldDDL, test.replacement)
			collection := collectTraceDBLifecycleFixture(t, traceDBLifecycleCollectorIndex(0, true), statements)
			if collection.ActivityComplete {
				t.Fatalf("incomplete source profile gained aggregate authority: %+v", collection.LifecycleCoverage)
			}
			if len(collection.Lifecycle.ByTID[42].Cuts) != 0 {
				t.Fatalf("incomplete source profile minted inferred cut: %+v", collection.Lifecycle)
			}
		})
	}
}

func TestTraceDBLifecycleCollectorCreationTokensAndNamespaces(t *testing.T) {
	statements := append(traceDBLifecycleCollectorSchema(),
		"CREATE TABLE raw (ts, name, itid)",
		"INSERT INTO raw VALUES (100,'sched_wakeup_new',2)",
		"INSERT INTO instant VALUES (99,'sched_wakeup_new',2,'itid')",
		"INSERT INTO instant VALUES (105,' SCHED_WAKEUP_NEW ',2,'itid')",
		"INSERT INTO instant VALUES (106,x'73636865645f77616b6575705f6e6577',2,'itid')",
		"INSERT INTO instant VALUES (107,'sched_wakeup_new',2,'ipid')",
		"INSERT INTO instant VALUES (108,'sched_wakeup_new',0,'ipid')",
		"INSERT INTO instant VALUES (109,'sched_wakeup_new',0,'itid')",
		"INSERT INTO instant VALUES (110,'sched_wakeup_new',2,'itid')",
	)
	collection := collectTraceDBLifecycleFixture(t, traceDBLifecycleCollectorIndex(100, true), statements)
	lane := collection.Lifecycle.ByTID[42]
	if !reflect.DeepEqual(lane.Cuts, []traceDBLifecycleBoundary{{TS: 110, NewITID: 2, NewIPID: 2}}) {
		t.Fatalf("exact creation or pre-capture/raw exclusion mismatch: %+v", lane)
	}
	if !reflect.DeepEqual(lane.PoisonPoints, []int64{105, 106}) {
		t.Fatalf("near creation tokens were upgraded or erased: %+v", lane)
	}
	if !reflect.DeepEqual(collection.Lifecycle.GlobalPoison, []int64{107, 108}) {
		t.Fatalf("wrong ref namespace was localized by numeric coincidence: %+v", collection.Lifecycle)
	}
}

func TestTraceDBLifecycleCollectorTerminalUsesCheckedEndAndCaptureDomain(t *testing.T) {
	t.Run("carry-in sched terminal uses checked end and ignores ts_end", func(t *testing.T) {
		schema := replaceTraceDBFixtureStatement(traceDBLifecycleCollectorSchema(),
			"CREATE TABLE sched_slice (ts, dur, itid, end_state)",
			"CREATE TABLE sched_slice (ts, dur, ts_end, itid, end_state)")
		statements := append(schema,
			"INSERT INTO sched_slice VALUES (90,20,999,1,'X')",
			"INSERT INTO frame_slice VALUES (1,0,120,2)",
		)
		collection := collectTraceDBLifecycleFixture(t, traceDBLifecycleCollectorIndex(100, true), statements)
		lane := collection.Lifecycle.ByTID[42]
		if !reflect.DeepEqual(lane.Terminals, []traceDBLifecycleBoundary{{TS: 110, NewITID: 1, NewIPID: 1}}) ||
			!reflect.DeepEqual(lane.Cuts, []traceDBLifecycleBoundary{{TS: 120, NewITID: 2, NewIPID: 2}}) {
			t.Fatalf("carry-in terminal did not use checked end: %+v", lane)
		}
	})

	t.Run("near string and blob tokens poison but never terminate", func(t *testing.T) {
		statements := append(traceDBLifecycleCollectorSchema(),
			"INSERT INTO thread_state VALUES (110,1,' x ')",
			"INSERT INTO thread_state VALUES (111,1,x'5a')",
			"INSERT INTO frame_slice VALUES (1,0,120,2)",
		)
		collection := collectTraceDBLifecycleFixture(t, traceDBLifecycleCollectorIndex(100, true), statements)
		lane := collection.Lifecycle.ByTID[42]
		if len(lane.Terminals) != 0 || len(lane.Cuts) != 0 || !reflect.DeepEqual(lane.PoisonPoints, []int64{110, 111}) {
			t.Fatalf("near terminal token gained authority or vanished: %+v", lane)
		}
	})

	t.Run("checked overflow taints lane and max endpoint remains exact", func(t *testing.T) {
		statements := append(traceDBLifecycleCollectorSchema(),
			"INSERT INTO sched_slice VALUES (9223372036854775807,1,1,'X')",
			"INSERT INTO sched_slice VALUES (9223372036854775806,1,1,'Z')",
		)
		collection := collectTraceDBLifecycleFixture(t, traceDBLifecycleCollectorIndex(0, true), statements)
		lane := collection.Lifecycle.ByTID[42]
		if !lane.Tainted || !reflect.DeepEqual(lane.Terminals,
			[]traceDBLifecycleBoundary{{TS: math.MaxInt64, NewITID: 1, NewIPID: 1}}) {
			t.Fatalf("overflow/max boundary handling mismatch: %+v", lane)
		}
	})

	t.Run("zero is a real timestamp", func(t *testing.T) {
		statements := append(traceDBLifecycleCollectorSchema(),
			"INSERT INTO thread_state VALUES (0,1,'X')",
			"INSERT INTO frame_slice VALUES (1,0,1,2)",
		)
		collection := collectTraceDBLifecycleFixture(t, traceDBLifecycleCollectorIndex(0, true), statements)
		lane := collection.Lifecycle.ByTID[42]
		if !reflect.DeepEqual(lane.Terminals, []traceDBLifecycleBoundary{{TS: 0, NewITID: 1, NewIPID: 1}}) ||
			!reflect.DeepEqual(lane.Cuts, []traceDBLifecycleBoundary{{TS: 1, NewITID: 2, NewIPID: 2}}) {
			t.Fatalf("timestamp zero was treated as absent: %+v", lane)
		}
	})

	t.Run("unknown capture start does not reject non-negative evidence", func(t *testing.T) {
		statements := append(traceDBLifecycleCollectorSchema(),
			"INSERT INTO thread_state VALUES (90,1,'X')",
			"INSERT INTO frame_slice VALUES (1,0,100,2)",
		)
		collection := collectTraceDBLifecycleFixture(t, traceDBLifecycleCollectorIndex(1000, false), statements)
		if !reflect.DeepEqual(collection.Lifecycle.ByTID[42].Cuts,
			[]traceDBLifecycleBoundary{{TS: 100, NewITID: 2, NewIPID: 2}}) {
			t.Fatalf("unknown capture start was used as a hard lower bound: %+v", collection.Lifecycle)
		}
	})
}

func TestTraceDBLifecycleCollectorMalformedLocalizationMatrix(t *testing.T) {
	tests := []struct {
		name                   string
		traceStart             int64
		rows                   []string
		wantSummaryGlobalTaint bool
		assert                 func(*testing.T, traceDBLifecycleIndex)
	}{
		{
			name: "known dual mismatch and known time is lane point",
			rows: []string{"INSERT INTO callstack VALUES (100,1,102)"},
			assert: func(t *testing.T, lifecycle traceDBLifecycleIndex) {
				if !reflect.DeepEqual(lifecycle.ByTID[42].PoisonPoints, []int64{100}) || len(lifecycle.GlobalPoison) != 0 || lifecycle.GlobalTaint {
					t.Fatalf("dual mismatch was not lane-local: %+v", lifecycle)
				}
			},
		},
		{
			name: "known identity and unknown time is lane taint",
			rows: []string{"INSERT INTO syscall VALUES ('bad',2)"},
			assert: func(t *testing.T, lifecycle traceDBLifecycleIndex) {
				if !lifecycle.ByTID[42].Tainted || lifecycle.GlobalTaint {
					t.Fatalf("known lane bad time escaped localization: %+v", lifecycle)
				}
			},
		},
		{
			name: "unknown identity and known time is global point",
			rows: []string{"INSERT INTO syscall VALUES (100,99)"},
			assert: func(t *testing.T, lifecycle traceDBLifecycleIndex) {
				if !reflect.DeepEqual(lifecycle.GlobalPoison, []int64{100}) || lifecycle.GlobalTaint {
					t.Fatalf("unknown lane known time was not global point: %+v", lifecycle)
				}
			},
		},
		{
			name:                   "unknown identity and unknown time is global taint",
			rows:                   []string{"INSERT INTO syscall VALUES ('bad','bad')"},
			wantSummaryGlobalTaint: true,
			assert: func(t *testing.T, lifecycle traceDBLifecycleIndex) {
				if !lifecycle.GlobalTaint {
					t.Fatalf("unplaceable unknown activity did not taint globally: %+v", lifecycle)
				}
			},
		},
		{
			name:       "known pre-capture domain skips even unknown identity",
			traceStart: 100,
			rows:       []string{"INSERT INTO syscall VALUES (90,99)"},
			assert: func(t *testing.T, lifecycle traceDBLifecycleIndex) {
				if lifecycle.GlobalTaint || len(lifecycle.GlobalPoison) != 0 {
					t.Fatalf("pre-capture row polluted capture domain: %+v", lifecycle)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			statements := append(traceDBLifecycleCollectorSchema(), test.rows...)
			collection := collectTraceDBLifecycleFixture(t, traceDBLifecycleCollectorIndex(test.traceStart, true), statements)
			test.assert(t, collection.Lifecycle)
			if test.wantSummaryGlobalTaint {
				summary := collection.LifecycleCoverage[0]
				if summary.FieldSources["global_taint"] != "true" || !strings.Contains(summary.Skipped, "global_taint=true") {
					t.Fatalf("ordinary global taint missing summary coverage: %+v", summary)
				}
			}
		})
	}
}

func TestTraceDBLifecycleCollectorIdleAndPublicTIDZeroStayOutOfGeneration(t *testing.T) {
	index := traceDBLifecycleCollectorIndex(0, true)
	index.ByITID[3] = traceDBThread{ITID: 3, TID: 0, IPID: 1, Name: "typed-idle"}
	index.ThreadIDToITID[103] = 3
	buildTraceDBThreadSecondaryIndexes(&index)
	statements := append(traceDBLifecycleCollectorSchema(),
		"INSERT INTO callstack VALUES (1,NULL,0)",
		"INSERT INTO callstack VALUES (2,0,0)",
		"INSERT INTO sched_slice VALUES (3,1,0,'X')",
		"INSERT INTO thread_state VALUES (4,0,'Z')",
		"INSERT INTO syscall VALUES (5,0)",
		"INSERT INTO native_hook VALUES (6,3)",
		"INSERT INTO frame_slice VALUES (1,0,7,0)",
	)
	collection := collectTraceDBLifecycleFixture(t, index, statements)
	if !reflect.DeepEqual(collection.ActiveITIDs, map[int64]bool{0: true, 3: true}) {
		t.Fatalf("active/lifecycle identity split mismatch: active=%+v coverage=%+v", collection.ActiveITIDs, collection.ActiveCoverage)
	}
	if collection.Lifecycle.GlobalTaint || len(collection.Lifecycle.GlobalPoison) != 0 || len(collection.Lifecycle.ByTID) != 0 {
		t.Fatalf("idle/public-TID-zero activity polluted positive generations: %+v", collection.Lifecycle)
	}
	for i, want := range []string{"callstack", "sched_slice", "thread_state", "syscall", "native_hook", "frame_slice"} {
		if item := collection.ActiveCoverage[i]; item.Table != want || item.Family != "resolver.active_thread" {
			t.Fatalf("active coverage order drift at %d: %+v", i, item)
		}
	}
	if got := collection.ActiveCoverage[1].ColumnsPresent; !reflect.DeepEqual(got, []string{"itid"}) {
		t.Fatalf("lifecycle timestamp leaked into active coverage: %+v", collection.ActiveCoverage[1])
	}
}

func TestTraceDBLifecycleCollectorUsesTableSpecificHighIdentityProfiles(t *testing.T) {
	const highITID = int64(4294967294)
	index := traceDBLifecycleCollectorIndex(0, true)
	index.ByITID[highITID] = traceDBThread{ITID: highITID, TID: 42, IPID: 2, Name: "high-new"}
	buildTraceDBThreadSecondaryIndexes(&index)
	statements := append(traceDBLifecycleCollectorSchema(),
		"INSERT INTO thread_state VALUES (50,1,'X')",
		"INSERT INTO sched_slice VALUES (40,10,1,'X')",
		"INSERT INTO syscall VALUES (60,-2)",
	)
	collection := collectTraceDBLifecycleFixture(t, index, statements)
	if !collection.ActiveITIDs[highITID] || !reflect.DeepEqual(collection.Lifecycle.ByTID[42].Cuts,
		[]traceDBLifecycleBoundary{{TS: 60, NewITID: highITID, NewIPID: 2}}) {
		t.Fatalf("signed high identity did not reach lifecycle authority: active=%+v lifecycle=%+v", collection.ActiveITIDs, collection.Lifecycle)
	}

	badStatements := append(traceDBLifecycleCollectorSchema(),
		"INSERT INTO thread_state VALUES (50,1,'X')",
		"INSERT INTO sched_slice VALUES (40,10,1,'X')",
		"INSERT INTO syscall VALUES (60,4294967294)",
	)
	bad := collectTraceDBLifecycleFixture(t, index, badStatements)
	if bad.ActiveITIDs[highITID] || len(bad.Lifecycle.ByTID[42].Cuts) != 0 || !reflect.DeepEqual(bad.Lifecycle.GlobalPoison, []int64{60}) {
		t.Fatalf("positive-high signed encoding gained authority: active=%+v lifecycle=%+v", bad.ActiveITIDs, bad.Lifecycle)
	}
}

func TestTraceDBLifecycleMalformedPointBudgetEscalatesToGlobalTaint(t *testing.T) {
	builder := newTraceDBLifecycleBuilder(traceDBLifecycleCollectorIndex(0, true))
	builder.malformedPointLimit = 2
	builder.addPoison(42, 1)
	builder.addGlobalPoison(2)
	builder.addPoison(42, 3)
	lifecycle := builder.finalize()
	if !builder.malformedPointBudgetExceeded || !lifecycle.GlobalTaint || len(lifecycle.GlobalPoison) != 0 ||
		len(lifecycle.ByTID[42].PoisonPoints) != 0 {
		t.Fatalf("malformed point budget did not fail closed and release points: builder=%+v lifecycle=%+v", builder, lifecycle)
	}
	coverage := traceDBLifecycleCollectionCoverage(traceDBLifecycleCollection{
		Lifecycle: lifecycle, MalformedPointBudgetExceeded: true,
	})
	if !strings.Contains(coverage.Skipped, "escalated_to_global_taint") ||
		coverage.FieldSources["malformed_point_budget_exceeded"] != "true" ||
		coverage.FieldSources["global_taint"] != "true" ||
		coverage.FieldSources["global_poison_points"] != "0" {
		t.Fatalf("budget escalation missing typed coverage: %+v", coverage)
	}
}

type traceDBLifecycleCountingQueryer struct {
	traceDBQueryer
	dataQueries []string
}

func (queryer *traceDBLifecycleCountingQueryer) recordDataQuery(query string) {
	upper := strings.ToUpper(strings.TrimSpace(query))
	if strings.HasPrefix(upper, "SELECT") && strings.Contains(upper, " FROM ") &&
		!strings.Contains(upper, "SQLITE_MASTER") {
		queryer.dataQueries = append(queryer.dataQueries, query)
	}
}

func (queryer *traceDBLifecycleCountingQueryer) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	queryer.recordDataQuery(query)
	return queryer.traceDBQueryer.QueryContext(ctx, query, args...)
}

func (queryer *traceDBLifecycleCountingQueryer) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	queryer.recordDataQuery(query)
	return queryer.traceDBQueryer.QueryRowContext(ctx, query, args...)
}

type traceDBLifecycleFailingSchemaQueryer struct {
	traceDBQueryer
}

func (queryer traceDBLifecycleFailingSchemaQueryer) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(query)), "PRAGMA TABLE_INFO") &&
		strings.Contains(strings.ToLower(query), "frame_slice") {
		return nil, errors.New("injected frame schema failure")
	}
	return queryer.traceDBQueryer.QueryContext(ctx, query, args...)
}

func (queryer traceDBLifecycleFailingSchemaQueryer) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return queryer.traceDBQueryer.QueryRowContext(ctx, query, args...)
}

func TestTraceDBLifecycleCollectorSchemaFailureIsFailAtomicWithCoverage(t *testing.T) {
	path := createTraceDBFixture(t, traceDBLifecycleCollectorSchema())
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	collection, err := collectTraceDBLifecycle(context.Background(),
		traceDBLifecycleFailingSchemaQueryer{traceDBQueryer: tdb.db}, traceDBLifecycleCollectorIndex(0, true))
	if err == nil || !strings.Contains(err.Error(), "injected frame schema failure") {
		t.Fatalf("schema failure was hidden: %v", err)
	}
	if len(collection.ActiveITIDs) != 0 {
		t.Fatalf("preflight failure leaked partial active state: %+v", collection.ActiveITIDs)
	}
	wantTables := []string{"callstack", "sched_slice", "thread_state", "syscall", "native_hook", "frame_slice"}
	if len(collection.ActiveCoverage) != len(wantTables) {
		t.Fatalf("preflight failure lost coverage: %+v", collection.ActiveCoverage)
	}
	for i, table := range wantTables {
		if item := collection.ActiveCoverage[i]; item.Table != table || item.RowsEmitted != 0 {
			t.Fatalf("fail-atomic coverage[%d]=%+v, want unswept %s", i, item, table)
		}
	}
	if item := collection.ActiveCoverage[len(collection.ActiveCoverage)-1]; item.Error == "" {
		t.Fatalf("failed schema item lost typed error: %+v", item)
	}
}

func TestSplitTraceDBLifecycleCoveragePreservesBothOrders(t *testing.T) {
	items := []TraceDBCoverage{
		{Family: "resolver.active_thread", Table: "callstack"},
		{Family: "resolver.lifecycle", Table: "__authority__"},
		{Family: "scheduler", Table: "sched_slice"},
		{Family: "resolver.lifecycle.activity", Table: "frame_slice"},
		{Family: "counter", Table: "measure"},
	}
	regular, lifecycle := splitTraceDBLifecycleCoverage(items)
	if !reflect.DeepEqual(regular, []TraceDBCoverage{items[0], items[2], items[4]}) ||
		!reflect.DeepEqual(lifecycle, []TraceDBCoverage{items[1], items[3]}) {
		t.Fatalf("coverage deferral changed relative order: regular=%+v lifecycle=%+v", regular, lifecycle)
	}
}

func TestTraceDBLifecycleCollectorUsesSuppliedReadTransactionAndBoundedScans(t *testing.T) {
	statements := append(traceDBLifecycleCollectorSchema(),
		"INSERT INTO thread_state VALUES (50,1,'X')",
		"INSERT INTO sched_slice VALUES (40,10,1,'X')",
		"INSERT INTO frame_slice VALUES (1,0,60,2)",
	)
	path := createTraceDBFixture(t, statements)
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	tx, err := tdb.db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	counting := &traceDBLifecycleCountingQueryer{traceDBQueryer: tx}
	collection, collectErr := collectTraceDBLifecycle(ctx, counting, traceDBLifecycleCollectorIndex(0, true))
	rollbackErr := tx.Rollback()
	if collectErr != nil {
		t.Fatalf("collector bypassed supplied read transaction or failed: %v", collectErr)
	}
	if rollbackErr != nil {
		t.Fatal(rollbackErr)
	}
	if !reflect.DeepEqual(collection.Lifecycle.ByTID[42].Cuts,
		[]traceDBLifecycleBoundary{{TS: 60, NewITID: 2, NewIPID: 2}}) {
		t.Fatalf("read transaction collector result mismatch: %+v", collection.Lifecycle)
	}

	counts := map[string]int{}
	for _, query := range counting.dataQueries {
		upper := strings.ToUpper(query)
		for _, forbidden := range []string{" WHERE ", "DISTINCT", "COALESCE", "CAST(", "COUNT(", " ORDER BY "} {
			if strings.Contains(upper, forbidden) {
				t.Fatalf("collector data query hides or repairs physical rows with %q: %s", forbidden, query)
			}
		}
		for _, table := range []string{"instant", "thread_state", "sched_slice", "callstack", "syscall", "native_hook", "frame_slice"} {
			if strings.Contains(query, `FROM "`+table+`"`) {
				counts[table]++
			}
		}
	}
	wantCounts := map[string]int{"instant": 1, "thread_state": 2, "sched_slice": 2, "callstack": 1, "syscall": 1, "native_hook": 1, "frame_slice": 1}
	if !reflect.DeepEqual(counts, wantCounts) {
		t.Fatalf("collector scan count reopened a legacy third pass: got=%+v want=%+v queries=%+v", counts, wantCounts, counting.dataQueries)
	}
}

func TestTraceDBLifecycleCollectorSQLAndProductionAuthorityAreStructurallyPinned(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source")
	}
	dir := filepath.Dir(current)
	collectorPath := filepath.Join(dir, "streamerdb_lifecycle_collect.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, collectorPath, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err != nil || !strings.Contains(strings.ToUpper(value), "SELECT") {
			return true
		}
		upper := strings.ToUpper(value)
		if strings.Contains(upper, "SQLITE_MASTER") {
			return true
		}
		for _, forbidden := range []string{" WHERE ", "DISTINCT", "COALESCE", "CAST(", "COUNT(", " ORDER BY "} {
			if strings.Contains(upper, forbidden) {
				t.Fatalf("collector SQL literal contains forbidden repair/filter %q: %q", forbidden, value)
			}
		}
		return true
	})

	collectorBody, err := os.ReadFile(collectorPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"BeginTx(", "loadActiveThreadIDs(", "tdb.db"} {
		if strings.Contains(string(collectorBody), forbidden) {
			t.Fatalf("collector bypassed supplied queryer through %q", forbidden)
		}
	}
	productionBody, err := os.ReadFile(filepath.Join(dir, "streamerdb_export_sched.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(productionBody), "collectTraceDBLifecycle(") != 1 ||
		strings.Contains(string(productionBody), "loadActiveThreadIDs(") {
		t.Fatalf("production has multiple lifecycle/legacy active authorities:\n%s", productionBody)
	}
	type productionSite struct {
		file     string
		function string
	}
	calls := map[string][]productionSite{}
	composites := map[string][]productionSite{}
	targetCalls := map[string]bool{
		"newTraceDBSchedulerAuthority":       true,
		"loadSchedStarts":                    true,
		"loadRunningIntervals":               true,
		"loadSchedulerRunningIndex":          true,
		"loadExtendedLegacyRunningIntervals": true,
		"newTraceDBSchedulerRunningIndex":    true,
		"lookupCPUAt":                        true,
		"exportTraceDBWakeups":               true,
		"exportTraceDBBlockedReasons":        true,
		"loadTraceDBBlockedCandidates":       true,
		"loadTraceDBBlockedSchedBoundaries":  true,
		"resolveThreadSubject":               true,
		"threadPointAllows":                  true,
		"threadClosedEndpointAllows":         true,
		"queryTraceDBSchedSliceRows":         true,
		"scanTraceDBSchedSourceRow":          true,
		"traceDBStrictInternalID":            true,
		"schedulerSubjectFromExactITID":      true,
		"schedulerPointAllows":               true,
		"schedulerNextPointAllows":           true,
		"schedulerSourceIntervalAllows":      true,
		"validateTraceDBSchedLifecycle":      true,
	}
	authorityConsumers := map[string]bool{
		"exportTraceDBSchedSwitch":          true,
		"auditTraceDBSchedSwitchRows":       true,
		"scanTraceDBSchedSourceRow":         true,
		"loadSchedStarts":                   true,
		"loadSchedulerRunningIndex":         true,
		"exportTraceDBWakeups":              true,
		"exportTraceDBBlockedReasons":       true,
		"loadTraceDBBlockedCandidates":      true,
		"loadTraceDBBlockedSchedBoundaries": true,
	}
	authorityIdentityConsumers := map[string]bool{
		"scanTraceDBSchedSourceRow":         true,
		"loadSchedStarts":                   true,
		"traceDBNextSchedMeta":              true,
		"exportTraceDBWakeups":              true,
		"exportTraceDBBlockedReasons":       true,
		"loadTraceDBBlockedCandidates":      true,
		"loadTraceDBBlockedSchedBoundaries": true,
	}
	strictScannerAssignments := 0
	strictSchedStartAssignments := 0
	wakeupEndpointCalls := map[string]int{}
	blockedEndpointCalls := map[string]int{}
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse production file %s: %v", path, err)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			site := productionSite{file: filepath.Base(path), function: function.Name.Name}
			authorityParams := 0
			runningIndexParams := 0
			for _, field := range function.Type.Params.List {
				if ident, ok := field.Type.(*ast.Ident); ok {
					switch ident.Name {
					case "traceDBSchedulerAuthority":
						authorityParams += len(field.Names)
					case "traceDBSchedulerRunningIndex":
						runningIndexParams += len(field.Names)
					case "traceDBThreadIndex":
						if authorityConsumers[function.Name.Name] {
							t.Fatalf("scheduler authority consumer %s accepts a raw thread index", function.Name.Name)
						}
					}
				}
			}
			if authorityConsumers[function.Name.Name] {
				if authorityParams != 1 {
					t.Fatalf("scheduler consumer %s authority params=%d, want 1", function.Name.Name, authorityParams)
				}
			}
			if function.Name.Name == "exportTraceDBWakeups" && runningIndexParams != 1 {
				t.Fatalf("wakeup exporter typed Running params=%d, want 1", runningIndexParams)
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				switch typed := node.(type) {
				case *ast.AssignStmt:
					if (function.Name.Name == "scanTraceDBSchedSourceRow" || function.Name.Name == "loadSchedStarts") && typed.Tok == token.DEFINE &&
						len(typed.Lhs) == 2 && len(typed.Rhs) == 1 {
						leftITID, leftITIDOK := typed.Lhs[0].(*ast.Ident)
						leftExact, leftExactOK := typed.Lhs[1].(*ast.Ident)
						call, callOK := typed.Rhs[0].(*ast.CallExpr)
						callee, calleeOK := func() (*ast.Ident, bool) {
							if !callOK {
								return nil, false
							}
							ident, ok := call.Fun.(*ast.Ident)
							return ident, ok
						}()
						argument, argumentOK := func() (*ast.Ident, bool) {
							if !callOK || len(call.Args) != 1 {
								return nil, false
							}
							ident, ok := call.Args[0].(*ast.Ident)
							return ident, ok
						}()
						if leftITIDOK && leftExactOK && calleeOK && argumentOK && leftITID.Name == "itid" &&
							leftExact.Name == "itidOK" && callee.Name == "traceDBStrictInternalID" {
							switch {
							case function.Name.Name == "scanTraceDBSchedSourceRow" && argument.Name == "rawITID":
								strictScannerAssignments++
							case function.Name.Name == "loadSchedStarts" && argument.Name == "itidRaw":
								strictSchedStartAssignments++
							}
						}
					}
				case *ast.SelectorExpr:
					if authorityIdentityConsumers[function.Name.Name] &&
						typed.Sel.Name == "identities" {
						t.Fatalf("scheduler lifecycle consumer %s reopened raw authority identities", function.Name.Name)
					}
					if function.Name.Name == "exportTraceDBWakeups" &&
						(typed.Sel.Name == "RunningTaintedITID" || typed.Sel.Name == "RunningGlobalTaint") {
						t.Fatalf("wakeup exporter reopened legacy Running side field %s", typed.Sel.Name)
					}
				case *ast.CallExpr:
					name := ""
					switch callee := typed.Fun.(type) {
					case *ast.Ident:
						name = callee.Name
					case *ast.SelectorExpr:
						name = callee.Sel.Name
					}
					if targetCalls[name] {
						calls[name] = append(calls[name], site)
					}
					if (name == "exportTraceDBBlockedReasons" && function.Name.Name == "exportTraceDBSchedulerFamilies") ||
						((name == "loadTraceDBBlockedCandidates" || name == "loadTraceDBBlockedSchedBoundaries") &&
							function.Name.Name == "exportTraceDBBlockedReasons") {
						argumentPosition := 2
						if name == "exportTraceDBBlockedReasons" {
							argumentPosition = 3
						}
						if len(typed.Args) <= argumentPosition {
							t.Fatalf("blocked authority call %s has %d arguments", name, len(typed.Args))
						}
						authority, ok := typed.Args[argumentPosition].(*ast.Ident)
						if !ok || authority.Name != "authority" {
							t.Fatalf("blocked authority call %s does not pass the shared authority", name)
						}
					}
					if function.Name.Name == "loadTraceDBBlockedCandidates" &&
						(name == "resolveThreadSubject" || name == "threadPointAllows") {
						method, methodOK := typed.Fun.(*ast.SelectorExpr)
						receiver, receiverOK := func() (*ast.Ident, bool) {
							if !methodOK {
								return nil, false
							}
							ident, ok := method.X.(*ast.Ident)
							return ident, ok
						}()
						rowField := func(argument ast.Expr, field string) bool {
							selector, ok := argument.(*ast.SelectorExpr)
							if !ok || selector.Sel.Name != field {
								return false
							}
							owner, ok := selector.X.(*ast.Ident)
							return ok && owner.Name == "row"
						}
						if !receiverOK || receiver.Name != "authority" || len(typed.Args) == 0 ||
							!rowField(typed.Args[0], "ITID") {
							t.Fatalf("blocked candidate %s does not use authority with row.ITID", name)
						}
						if name == "resolveThreadSubject" {
							if len(typed.Args) != 1 {
								t.Fatalf("blocked candidate resolve args=%d, want 1", len(typed.Args))
							}
							blockedEndpointCalls["resolve:row.ITID"]++
						} else {
							if len(typed.Args) != 2 || !rowField(typed.Args[1], "TS") {
								t.Fatalf("blocked candidate point does not use row.TS")
							}
							blockedEndpointCalls["point:row.ITID,row.TS"]++
						}
					}
					if function.Name.Name == "loadTraceDBBlockedSchedBoundaries" && name == "threadClosedEndpointAllows" {
						method, methodOK := typed.Fun.(*ast.SelectorExpr)
						receiver, receiverOK := func() (*ast.Ident, bool) {
							if !methodOK {
								return nil, false
							}
							ident, ok := method.X.(*ast.Ident)
							return ident, ok
						}()
						want := []string{"itid", "ts", "end"}
						if !receiverOK || receiver.Name != "authority" || len(typed.Args) != len(want) {
							t.Fatalf("blocked predecessor closed gate does not use shared authority")
						}
						for i, argument := range typed.Args {
							ident, ok := argument.(*ast.Ident)
							if !ok || ident.Name != want[i] {
								t.Fatalf("blocked predecessor closed gate arg %d is not %s", i, want[i])
							}
						}
						blockedEndpointCalls["closed:itid,ts,end"]++
					}
					if function.Name.Name == "exportTraceDBWakeups" &&
						(name == "traceDBKnownCPUAt" || name == "traceDBCPUAt") {
						t.Fatalf("wakeup exporter bypassed typed Running lookup through %s", name)
					}
					if function.Name.Name == "exportTraceDBWakeups" &&
						(name == "resolveThreadSubject" || name == "threadPointAllows") {
						method, methodOK := typed.Fun.(*ast.SelectorExpr)
						receiver, receiverOK := func() (*ast.Ident, bool) {
							if !methodOK {
								return nil, false
							}
							ident, ok := method.X.(*ast.Ident)
							return ident, ok
						}()
						endpoint := func(argument ast.Expr) (string, bool) {
							selector, ok := argument.(*ast.SelectorExpr)
							if !ok {
								return "", false
							}
							owner, ok := selector.X.(*ast.Ident)
							return selector.Sel.Name, ok && owner.Name == "instant"
						}
						if !receiverOK || receiver.Name != "authority" || len(typed.Args) == 0 {
							t.Fatalf("wakeup %s call does not use the shared authority and typed endpoint", name)
						}
						field, fieldOK := endpoint(typed.Args[0])
						if !fieldOK || (field != "Ref" && field != "WakeupFrom") {
							t.Fatalf("wakeup %s call uses an unexpected endpoint", name)
						}
						if name == "resolveThreadSubject" {
							if len(typed.Args) != 1 {
								t.Fatalf("wakeup resolve endpoint args=%d, want 1", len(typed.Args))
							}
							wakeupEndpointCalls["resolve:"+field]++
						} else {
							if len(typed.Args) != 2 {
								t.Fatalf("wakeup point endpoint args=%d, want 2", len(typed.Args))
							}
							timestamp, timestampOK := endpoint(typed.Args[1])
							if !timestampOK || timestamp != "TS" {
								t.Fatalf("wakeup point endpoint does not use instant.TS")
							}
							wakeupEndpointCalls["point:"+field]++
						}
					}
					if name == "schedulerSubjectFromExactITID" &&
						(function.Name.Name == "scanTraceDBSchedSourceRow" || function.Name.Name == "loadSchedStarts") {
						if len(typed.Args) != 2 {
							t.Fatalf("scheduler subject factory args=%d in %s, want 2", len(typed.Args), function.Name.Name)
						}
						itid, itidOK := typed.Args[0].(*ast.Ident)
						exact, exactOK := typed.Args[1].(*ast.Ident)
						if !itidOK || !exactOK || itid.Name != "itid" || exact.Name != "itidOK" {
							t.Fatalf("%s did not pass the strict decoder bit into subject factory", function.Name.Name)
						}
					}
				case *ast.CompositeLit:
					if ident, ok := typed.Type.(*ast.Ident); ok &&
						(ident.Name == "traceDBSchedulerAuthority" || ident.Name == "traceDBSchedulerSubject") {
						composites[ident.Name] = append(composites[ident.Name], site)
					}
				}
				return true
			})
		}
	}
	assertCallSites := func(name string, want map[string]int) {
		t.Helper()
		got := map[string]int{}
		for _, site := range calls[name] {
			got[site.function]++
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("production call sites for %s=%v, want %v (sites=%+v)", name, got, want, calls[name])
		}
	}
	assertCallSites("newTraceDBSchedulerAuthority", map[string]int{"exportTraceDBSchedulerFamilies": 1})
	assertCallSites("loadSchedStarts", map[string]int{"exportTraceDBSchedulerFamilies": 1})
	assertCallSites("loadRunningIntervals", map[string]int{
		"loadExtendedLegacyRunningIntervals": 1,
		"loadSchedulerRunningIndex":          1,
	})
	assertCallSites("loadSchedulerRunningIndex", map[string]int{"exportTraceDBSchedulerFamilies": 1})
	assertCallSites("loadExtendedLegacyRunningIntervals", map[string]int{"exportTraceDBExtendedFamilies": 1})
	assertCallSites("newTraceDBSchedulerRunningIndex", map[string]int{
		"exportTraceDBExtendedFamilies": 1,
		"loadSchedulerRunningIndex":     1,
	})
	assertCallSites("lookupCPUAt", map[string]int{
		"exportTraceDBRawFtraceFamilies": 1,
		"exportTraceDBWakeups":           1,
		"knownCPUAt":                     1,
		"prepareTraceDBCallstackRow":     2,
		"prepareTraceDBFrameSliceRow":    2,
		"prepareTraceDBNativeHookEvent":  1,
		"traceDBResolvePerfSampleCPU":    1,
	})
	assertCallSites("exportTraceDBWakeups", map[string]int{"exportTraceDBSchedulerFamilies": 1})
	assertCallSites("exportTraceDBBlockedReasons", map[string]int{"exportTraceDBSchedulerFamilies": 1})
	assertCallSites("loadTraceDBBlockedCandidates", map[string]int{"exportTraceDBBlockedReasons": 1})
	assertCallSites("loadTraceDBBlockedSchedBoundaries", map[string]int{"exportTraceDBBlockedReasons": 1})
	wantWakeupEndpoints := map[string]int{
		"resolve:Ref":        1,
		"resolve:WakeupFrom": 1,
		"point:Ref":          1,
		"point:WakeupFrom":   1,
	}
	if !reflect.DeepEqual(wakeupEndpointCalls, wantWakeupEndpoints) {
		t.Fatalf("wakeup endpoint gates=%v, want %v", wakeupEndpointCalls, wantWakeupEndpoints)
	}
	wantBlockedEndpoints := map[string]int{
		"resolve:row.ITID":      1,
		"point:row.ITID,row.TS": 1,
		"closed:itid,ts,end":    1,
	}
	if !reflect.DeepEqual(blockedEndpointCalls, wantBlockedEndpoints) {
		t.Fatalf("blocked endpoint gates=%v, want %v", blockedEndpointCalls, wantBlockedEndpoints)
	}
	assertCallSites("resolveThreadSubject", map[string]int{
		"exportTraceDBCallstack":                 1,
		"exportTraceDBWakeups":                   2,
		"loadTraceDBBlockedCandidates":           1,
		"prepareTraceDBCallstackRow":             1,
		"prepareTraceDBFrameSliceRow":            1,
		"prepareTraceDBNativeHookEvent":          1,
		"scanTraceDBSchedSourceRow":              1,
		"threadSubject":                          1,
		"traceDBCallstackExactEmitterCandidates": 1,
		"traceDBRawPairingOwner":                 2,
		"traceDBResolveRawSubject":               2,
	})
	assertCallSites("threadPointAllows", map[string]int{
		"exportTraceDBPerfSamples":        1,
		"exportTraceDBWakeups":            2,
		"loadTraceDBBlockedCandidates":    1,
		"prepareTraceDBCallstackRow":      2,
		"prepareTraceDBNativeHookEvent":   1,
		"schedulerPointAllows":            1,
		"traceDBAdmitRawCanonicalSubject": 1,
	})
	assertCallSites("threadClosedEndpointAllows", map[string]int{
		"loadTraceDBBlockedSchedBoundaries": 1,
		"prepareTraceDBCallstackRow":        1,
		"prepareTraceDBFrameSliceRow":       1,
		"schedulerNextPointAllows":          1,
	})
	assertCallSites("queryTraceDBSchedSliceRows", map[string]int{"auditTraceDBSchedSwitchRows": 1, "exportTraceDBSchedSwitch": 1})
	assertCallSites("scanTraceDBSchedSourceRow", map[string]int{"auditTraceDBSchedSwitchRows": 1, "exportTraceDBSchedSwitch": 1})
	assertCallSites("schedulerSubjectFromExactITID", map[string]int{"loadSchedStarts": 1, "newTraceDBSchedulerRunningIndex": 1, "scanTraceDBSchedSourceRow": 1, "traceDBRawPairingOwner": 1, "traceDBResolveRawSubject": 1})
	assertCallSites("schedulerPointAllows", map[string]int{"loadSchedStarts": 1, "schedulerNextPointAllows": 1, "traceDBResolveRawSubject": 1, "validateTraceDBSchedLifecycle": 2})
	assertCallSites("schedulerNextPointAllows", map[string]int{"traceDBNextSchedMeta": 1})
	assertCallSites("schedulerSourceIntervalAllows", map[string]int{"newTraceDBSchedulerRunningIndex": 1, "validateTraceDBSchedLifecycle": 1})
	assertCallSites("validateTraceDBSchedLifecycle", map[string]int{"scanTraceDBSchedSourceRow": 2})
	strictDecoderCalls := 0
	for _, site := range calls["traceDBStrictInternalID"] {
		if site.function == "scanTraceDBSchedSourceRow" {
			strictDecoderCalls++
		}
	}
	if strictDecoderCalls != 1 {
		t.Fatalf("sched scanner strict ITID decoder calls=%d, want 1", strictDecoderCalls)
	}
	if strictScannerAssignments != 1 {
		t.Fatalf("sched scanner strict ITID decoder assignments=%d, want 1", strictScannerAssignments)
	}
	if strictSchedStartAssignments != 1 {
		t.Fatalf("sched-start strict ITID decoder assignments=%d, want 1", strictSchedStartAssignments)
	}
	for kind, allowed := range map[string]string{
		"traceDBSchedulerAuthority": "newTraceDBSchedulerAuthority",
		"traceDBSchedulerSubject":   "schedulerSubjectFromExactITID",
	} {
		if len(composites[kind]) == 0 {
			t.Fatalf("production %s constructor is missing", kind)
		}
		for _, site := range composites[kind] {
			if site.function != allowed {
				t.Fatalf("production %s literal escaped %s: %+v", kind, allowed, site)
			}
		}
	}
	for _, required := range []string{"defer func()", "coverage = append(coverage, lifecycleCoverage...)", "lifecycleCoverage = lifecycle.LifecycleCoverage"} {
		if !strings.Contains(string(productionBody), required) {
			t.Fatalf("scheduler early returns can drop lifecycle coverage; missing %q", required)
		}
	}
}
