package hitraceconv

import (
	"context"
	"errors"
	"fmt"
	"unsafe"

	"modernc.org/libc"
	"modernc.org/libc/sys/types"
	sqlite3 "modernc.org/sqlite/lib"
)

const (
	defaultTraceDBSQLiteHardHeapBytes int64 = 256 << 20
	defaultTraceDBSQLiteCacheKiB            = 8 << 10
)

var traceDBSQLiteBudgetGate = make(chan struct{}, 1)

var (
	errTraceDBSQLiteHeapBudgetExceeded = errors.New("sealed SQLite heap budget exceeded")
	errTraceDBSQLiteBudgetAuthority    = errors.New("trace DB SQLite budget authority unavailable")
)

var traceDBSQLiteMemoryAccountingErr = enableTraceDBSQLiteMemoryAccounting()

// modernc SQLite is compiled with SQLITE_DEFAULT_MEMSTATUS=0. SQLite's hard
// heap limit is only enforceable when allocation accounting is enabled, so
// configure it before this package can open its first connection. This package
// is the repository's sole modernc SQLite owner.
func enableTraceDBSQLiteMemoryAccounting() error {
	tls := libc.NewTLS()
	if tls == nil {
		return newTraceDBSQLiteBudgetAuthorityError("sealed_sqlite_heap_accounting_tls_unavailable")
	}
	defer tls.Close()
	args := libc.Xmalloc(tls, types.Size_t(unsafe.Sizeof(uintptr(0))))
	if args == 0 {
		return newTraceDBSQLiteBudgetAuthorityError("sealed_sqlite_heap_accounting_args_unavailable")
	}
	defer libc.Xfree(tls, args)
	if code := sqlite3.Xsqlite3_config(tls, sqlite3.SQLITE_CONFIG_MEMSTATUS, libc.VaList(args, int32(1))); code != sqlite3.SQLITE_OK {
		return newTraceDBSQLiteBudgetAuthorityError(fmt.Sprintf("sealed_sqlite_heap_accounting_config_failed_%d", code))
	}
	return nil
}

func newTraceDBSQLiteBudgetAuthorityError(reason string) error {
	return fmt.Errorf("%w: %w", errTraceDBSQLiteBudgetAuthority, &traceDBOutputInvariantError{Reason: reason})
}

type traceDBSQLiteHeapBudgetError struct {
	cause      error
	limitBytes int64
}

func (err *traceDBSQLiteHeapBudgetError) Error() string {
	return fmt.Sprintf("%s: code=sealed_sqlite_heap_budget_exceeded limit_bytes=%d: %v", errTraceDBSQLiteHeapBudgetExceeded, err.limitBytes, err.cause)
}

func (err *traceDBSQLiteHeapBudgetError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func (err *traceDBSQLiteHeapBudgetError) Is(target error) bool {
	return target == errTraceDBSQLiteHeapBudgetExceeded
}

type traceDBSQLiteBudgetLease struct {
	held bool
}

func acquireTraceDBSQLiteBudgetLease(ctx context.Context) (*traceDBSQLiteBudgetLease, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case traceDBSQLiteBudgetGate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	lease := &traceDBSQLiteBudgetLease{held: true}
	if err := applyTraceDBSQLiteHardHeapLimit(); err != nil {
		lease.release()
		return nil, err
	}
	return lease, nil
}

func (lease *traceDBSQLiteBudgetLease) release() {
	if lease == nil || !lease.held {
		return
	}
	lease.held = false
	<-traceDBSQLiteBudgetGate
}

func applyTraceDBSQLiteHardHeapLimit() error {
	if traceDBSQLiteMemoryAccountingErr != nil {
		return traceDBSQLiteMemoryAccountingErr
	}
	tls := libc.NewTLS()
	if tls == nil {
		return newTraceDBSQLiteBudgetAuthorityError("sealed_sqlite_heap_budget_tls_unavailable")
	}
	defer tls.Close()
	current := int64(sqlite3.Xsqlite3_hard_heap_limit64(tls, -1))
	if current < 0 {
		return newTraceDBSQLiteBudgetAuthorityError("sealed_sqlite_heap_budget_query_failed")
	}
	if current == 0 || current > defaultTraceDBSQLiteHardHeapBytes {
		if prior := int64(sqlite3.Xsqlite3_hard_heap_limit64(tls, sqlite3.Tsqlite3_int64(defaultTraceDBSQLiteHardHeapBytes))); prior < 0 {
			return newTraceDBSQLiteBudgetAuthorityError("sealed_sqlite_heap_budget_apply_failed")
		}
		current = int64(sqlite3.Xsqlite3_hard_heap_limit64(tls, -1))
	}
	if current <= 0 || current > defaultTraceDBSQLiteHardHeapBytes {
		return newTraceDBSQLiteBudgetAuthorityError("sealed_sqlite_heap_budget_not_applied")
	}
	return nil
}

func normalizeTraceDBSQLiteHeapBudgetError(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errTraceDBSQLiteHeapBudgetExceeded) {
		return err
	}
	if traceDBSQLitePrimaryErrorCode(err) != sqlite3.SQLITE_NOMEM {
		return err
	}
	return &traceDBSQLiteHeapBudgetError{cause: err, limitBytes: traceDBSQLiteCurrentHardHeapLimit()}
}

func traceDBSQLiteCurrentHardHeapLimit() int64 {
	tls := libc.NewTLS()
	if tls == nil {
		return defaultTraceDBSQLiteHardHeapBytes
	}
	defer tls.Close()
	current := int64(sqlite3.Xsqlite3_hard_heap_limit64(tls, -1))
	if current <= 0 || current > defaultTraceDBSQLiteHardHeapBytes {
		return defaultTraceDBSQLiteHardHeapBytes
	}
	return current
}
