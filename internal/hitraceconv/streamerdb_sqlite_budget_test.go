package hitraceconv

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"modernc.org/libc"
	sqlite3 "modernc.org/sqlite/lib"
)

const traceDBSQLiteBudgetChildMode = "CODRAX_SQLITE_BUDGET_CHILD_MODE"

func TestTraceDBSQLiteBudgetLeaseCancellationAndRelease(t *testing.T) {
	first, err := acquireTraceDBSQLiteBudgetLease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if second, err := acquireTraceDBSQLiteBudgetLease(canceled); second != nil || !errors.Is(err, context.Canceled) {
		first.release()
		t.Fatalf("canceled waiter acquired lease=%v err=%v", second, err)
	}
	first.release()
	first.release()

	second, err := acquireTraceDBSQLiteBudgetLease(context.Background())
	if err != nil {
		t.Fatalf("released budget slot remained unavailable: %v", err)
	}
	second.release()
}

func TestTraceDBSQLiteBudgetSubprocessContracts(t *testing.T) {
	if mode := os.Getenv(traceDBSQLiteBudgetChildMode); mode != "" {
		switch mode {
		case "preserve_lower_limit":
			runTraceDBSQLitePreserveLowerLimitChild(t)
		case "sorter_budget":
			runTraceDBSQLiteSorterBudgetChild(t)
		default:
			t.Fatalf("unknown SQLite budget child mode %q", mode)
		}
		return
	}

	for _, mode := range []string{"preserve_lower_limit", "sorter_budget"} {
		t.Run(mode, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestTraceDBSQLiteBudgetSubprocessContracts$")
			command.Env = append(os.Environ(), traceDBSQLiteBudgetChildMode+"="+mode)
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("SQLite budget %s subprocess failed: %v\n%s", mode, err, output)
			}
		})
	}
}

func runTraceDBSQLitePreserveLowerLimitChild(t *testing.T) {
	const lowerLimit = int64(192 << 20)
	setTraceDBSQLiteHardHeapLimitForTest(t, lowerLimit)
	lease, err := acquireTraceDBSQLiteBudgetLease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	lease.release()
	if got := queryTraceDBSQLiteHardHeapLimitForTest(t); got <= 0 || got > lowerLimit {
		t.Fatalf("budget acquisition raised existing lower hard limit: got=%d want<=%d", got, lowerLimit)
	}
}

func runTraceDBSQLiteSorterBudgetChild(t *testing.T) {
	source := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
	db, err := sql.Open("sqlite", source)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	statement, err := tx.Prepare("INSERT INTO sched_slice(ts, dur, cpu, end_state, priority, itid) VALUES (?, 1, 0, ?, 20, 2)")
	if err != nil {
		tx.Rollback()
		db.Close()
		t.Fatal(err)
	}
	const rows = 30000
	payload := strings.Repeat("x", 2048)
	for index := 0; index < rows; index++ {
		value := fmt.Sprintf("%08d-%s", rows-index, payload)
		if _, err := statement.Exec(2_000_000+rows-index, value); err != nil {
			statement.Close()
			tx.Rollback()
			db.Close()
			t.Fatal(err)
		}
	}
	if err := statement.Close(); err != nil {
		tx.Rollback()
		db.Close()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	parent := t.TempDir()
	privateDir, err := newPrivateConversionDir(parent, "codrax-sqlite-budget-*")
	if err != nil {
		t.Fatal(err)
	}
	dbPath, err := privateDir.ChildPath(sealedTraceDBVirtualName)
	if err != nil {
		privateDir.FinalizeCleanup()
		t.Fatal(err)
	}
	copyTestFile(t, source, dbPath)
	sealed, err := privateDir.AdoptRegularChild(sealedTraceDBVirtualName, true)
	if err != nil {
		privateDir.FinalizeCleanup()
		t.Fatal(err)
	}
	defer finishSealedTraceDBTestFixture(t, privateDir, sealed)

	const fixtureLimit = int64(8 << 20)
	setTraceDBSQLiteHardHeapLimitForTest(t, fixtureLimit)
	probe, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	var oversized int64
	probeErr := probe.QueryRow("SELECT length(randomblob(16777216))").Scan(&oversized)
	if closeErr := probe.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if traceDBSQLitePrimaryErrorCode(probeErr) != sqlite3.SQLITE_NOMEM {
		t.Fatalf("SQLite hard heap probe err=%T %v value=%d", probeErr, probeErr, oversized)
	}
	output := filepath.Join(parent, "must-not-publish.systrace")
	ledger, err := newConversionFileLedger(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := ledger.releaseOwnedAuthorities(); err != nil {
			t.Errorf("release SQLite budget fixture ledger: %v", err)
		}
	}()
	_, exportErr := exportTraceDBToSystraceFromSealedWithLedger(context.Background(), sealed, dbPath, output, ledger)
	if !errors.Is(exportErr, errTraceDBSQLiteHeapBudgetExceeded) {
		t.Fatalf("oversized sealed sorter error=%T %v, want typed heap budget", exportErr, exportErr)
	}
	if !strings.Contains(exportErr.Error(), "limit_bytes=8388608") {
		t.Fatalf("heap-budget error lost effective lower limit: %v", exportErr)
	}
	if _, statErr := os.Stat(output); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("heap-budget failure published output: stat=%v", statErr)
	}

	groupDB, err := openTraceDBFromSealed(context.Background(), sealed, dbPath)
	if err != nil {
		t.Fatalf("sealed DB did not recover after ORDER BY budget failure: %v", err)
	}
	groupRows, groupErr := groupDB.db.QueryContext(context.Background(), `
		SELECT end_state, count(*)
		FROM sched_slice
		GROUP BY end_state
		ORDER BY end_state`)
	if groupErr == nil {
		for groupRows.Next() {
			var state string
			var count int64
			if scanErr := groupRows.Scan(&state, &count); scanErr != nil {
				groupErr = scanErr
				break
			}
		}
		if rowsErr := groupRows.Err(); groupErr == nil {
			groupErr = rowsErr
		}
		if closeErr := groupRows.Close(); groupErr == nil {
			groupErr = closeErr
		}
	}
	groupErr = normalizeTraceDBSQLiteHeapBudgetError(groupErr)
	groupErr = traceDBJoinPreservingSingle(groupErr, groupDB.close())
	if !errors.Is(groupErr, errTraceDBSQLiteHeapBudgetExceeded) {
		t.Fatalf("oversized sealed GROUP BY error=%T %v, want typed heap budget", groupErr, groupErr)
	}
}

func setTraceDBSQLiteHardHeapLimitForTest(t *testing.T, limit int64) {
	t.Helper()
	tls := libc.NewTLS()
	if tls == nil {
		t.Fatal("SQLite test TLS unavailable")
	}
	defer tls.Close()
	if prior := int64(sqlite3.Xsqlite3_hard_heap_limit64(tls, sqlite3.Tsqlite3_int64(limit))); prior < 0 {
		t.Fatalf("set SQLite hard heap limit %d returned %d", limit, prior)
	}
	if got := int64(sqlite3.Xsqlite3_hard_heap_limit64(tls, -1)); got <= 0 || got > limit {
		t.Fatalf("SQLite hard heap limit got=%d want<=%d", got, limit)
	}
}

func queryTraceDBSQLiteHardHeapLimitForTest(t *testing.T) int64 {
	t.Helper()
	tls := libc.NewTLS()
	if tls == nil {
		t.Fatal("SQLite test TLS unavailable")
	}
	defer tls.Close()
	return int64(sqlite3.Xsqlite3_hard_heap_limit64(tls, -1))
}
