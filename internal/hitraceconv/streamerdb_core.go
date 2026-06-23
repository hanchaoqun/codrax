package hitraceconv

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type traceDB struct {
	db   *sql.DB
	path string
}

type traceDBValue struct {
	Valid bool
	Text  string
}

type traceDBProcess struct {
	IPID int64
	PID  int64
	Name string
}

type traceDBThread struct {
	ITID         int64
	TID          int64
	IPID         int64
	Name         string
	StartTS      int64
	IsMainThread bool
	SwitchCount  int64
}

type traceDBThreadIndex struct {
	ByITID     map[int64]traceDBThread
	ByTID      map[int64]traceDBThread
	Processes  map[int64]traceDBProcess
	ByProcess  map[int64][]traceDBThread
	TraceStart int64
}

type traceDBInstantKey struct {
	TS   int64
	Name string
}

type traceDBSchedStart struct {
	TS       int64
	CPU      int64
	Priority int64
}

type traceDBRunningInterval struct {
	Start int64
	End   int64
	CPU   int64
}

func openTraceDB(ctx context.Context, path string) (*traceDB, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, fmt.Errorf("trace DB path is required")
	}
	db, err := sql.Open("sqlite", sqliteReadOnlyDSN(trimmed))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &traceDB{db: db, path: trimmed}, nil
}

func sqliteReadOnlyDSN(path string) string {
	uriPath := strings.TrimSpace(path)
	if abs, err := filepath.Abs(uriPath); err == nil {
		uriPath = abs
	}
	return sqliteReadOnlyDSNFromURIPath(uriPath)
}

func sqliteReadOnlyDSNFromURIPath(path string) string {
	uriPath := sqliteFileURIPath(path)
	u := url.URL{Scheme: "file", Path: uriPath}
	q := u.Query()
	q.Set("mode", "ro")
	u.RawQuery = q.Encode()
	return u.String()
}

func sqliteFileURIPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "."
	}
	path = strings.ReplaceAll(path, "\\", "/")
	if sqliteLooksLikeWindowsDrivePath(path) {
		return "/" + path
	}
	if strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}

func sqliteLooksLikeWindowsDrivePath(path string) bool {
	if len(path) < 2 || path[1] != ':' {
		return false
	}
	c := path[0]
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func (tdb *traceDB) close() error {
	if tdb == nil || tdb.db == nil {
		return nil
	}
	return tdb.db.Close()
}

func (tdb *traceDB) tableExists(ctx context.Context, table string) (bool, error) {
	var one int
	err := tdb.db.QueryRowContext(ctx,
		"SELECT 1 FROM sqlite_master WHERE type='table' AND name=?1 LIMIT 1",
		table,
	).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (tdb *traceDB) columnNames(ctx context.Context, table string) ([]string, error) {
	rows, err := tdb.db.QueryContext(ctx, "PRAGMA table_info("+quoteSQLiteIdent(table)+")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

func (tdb *traceDB) columnExists(ctx context.Context, table, column string) (bool, error) {
	columns, err := tdb.columnNames(ctx, table)
	if err != nil {
		return false, err
	}
	for _, item := range columns {
		if item == column {
			return true, nil
		}
	}
	return false, nil
}

func (tdb *traceDB) rowCount(ctx context.Context, table string) (int, error) {
	var count int
	err := tdb.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM "+quoteSQLiteIdent(table)).Scan(&count)
	return count, err
}

func (tdb *traceDB) inspectCoverage(ctx context.Context, family, table string, requiredColumns []string) (item TraceDBCoverage, err error) {
	start := time.Now()
	defer func() {
		traceDBSetCoverageElapsed(&item, start)
	}()
	item = TraceDBCoverage{Family: family, Table: table, Role: traceDBCoverageRole(family, table)}
	found, err := tdb.tableExists(ctx, table)
	if err != nil {
		item.Error = err.Error()
		return item, err
	}
	item.Found = found
	if !found {
		item.Skipped = "missing table"
		return item, nil
	}
	columns, err := tdb.columnNames(ctx, table)
	if err != nil {
		item.Error = err.Error()
		return item, err
	}
	columnSet := make(map[string]bool, len(columns))
	for _, column := range columns {
		columnSet[column] = true
	}
	for _, column := range requiredColumns {
		if columnSet[column] {
			item.ColumnsPresent = append(item.ColumnsPresent, column)
		} else {
			item.ColumnsMissing = append(item.ColumnsMissing, column)
		}
	}
	sort.Strings(item.ColumnsPresent)
	sort.Strings(item.ColumnsMissing)
	count, err := tdb.rowCount(ctx, table)
	if err != nil {
		item.Error = err.Error()
		return item, err
	}
	item.RowsRead = count
	if len(item.ColumnsMissing) > 0 {
		item.Skipped = "missing required columns: " + strings.Join(item.ColumnsMissing, ",")
	}
	return item, nil
}

func traceDBCoverageRole(family, table string) string {
	family = strings.TrimSpace(family)
	table = strings.TrimSpace(table)
	switch {
	case family == "resolver" || strings.HasPrefix(family, "resolver."):
		return "resolver_index"
	case family == "sorter" && table == "__systrace_rows__":
		return "systrace_text_output"
	case family == "perf" && table == "__perftrace_rows__":
		return "perftrace_text_output"
	case family == "trace_cross_validation":
		return "tracequery_cross_validation"
	default:
		return "query_ready_export"
	}
}

func (tdb *traceDB) traceStart(ctx context.Context) (int64, TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "resolver", "trace_range", []string{"start_ts"})
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return 0, coverage, err
	}
	var start int64
	err = tdb.db.QueryRowContext(ctx, "SELECT start_ts FROM trace_range LIMIT 1").Scan(&start)
	if err == sql.ErrNoRows {
		coverage.Skipped = "empty table"
		return 0, coverage, nil
	}
	if err != nil {
		coverage.Error = err.Error()
		return 0, coverage, err
	}
	return start, coverage, nil
}

func (tdb *traceDB) loadThreadIndex(ctx context.Context) (traceDBThreadIndex, []TraceDBCoverage, error) {
	var coverage []TraceDBCoverage
	traceStart, traceStartCoverage, err := tdb.traceStart(ctx)
	coverage = append(coverage, traceStartCoverage)
	if err != nil {
		return traceDBThreadIndex{}, coverage, err
	}
	processCoverage, err := tdb.inspectCoverage(ctx, "resolver", "process", []string{"ipid", "pid", "name"})
	coverage = append(coverage, processCoverage)
	if err != nil {
		return traceDBThreadIndex{}, coverage, err
	}
	threadCoverage, err := tdb.inspectCoverage(ctx, "resolver", "thread", []string{"itid", "tid", "ipid", "name", "start_ts", "is_main_thread", "switch_count"})
	coverage = append(coverage, threadCoverage)
	if err != nil {
		return traceDBThreadIndex{}, coverage, err
	}
	index := traceDBThreadIndex{
		ByITID:     map[int64]traceDBThread{},
		ByTID:      map[int64]traceDBThread{},
		Processes:  map[int64]traceDBProcess{},
		ByProcess:  map[int64][]traceDBThread{},
		TraceStart: traceStart,
	}
	if processCoverage.Found && len(processCoverage.ColumnsMissing) == 0 {
		rows, err := tdb.db.QueryContext(ctx, "SELECT ipid, pid, COALESCE(name, '') FROM process WHERE pid IS NOT NULL")
		if err != nil {
			return index, coverage, err
		}
		defer rows.Close()
		for rows.Next() {
			var item traceDBProcess
			if err := rows.Scan(&item.IPID, &item.PID, &item.Name); err != nil {
				return index, coverage, err
			}
			index.Processes[item.IPID] = item
		}
		if err := rows.Err(); err != nil {
			return index, coverage, err
		}
	}
	if !threadCoverage.Found || len(threadCoverage.ColumnsMissing) > 0 {
		return index, coverage, nil
	}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT itid, tid, COALESCE(ipid, 0), COALESCE(name, ''),
		       COALESCE(start_ts, ?1), COALESCE(is_main_thread, 0),
		       COALESCE(switch_count, 0)
		FROM thread
		WHERE tid IS NOT NULL AND tid > 0
	`, traceStart)
	if err != nil {
		return index, coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var mainFlag int64
		var item traceDBThread
		if err := rows.Scan(&item.ITID, &item.TID, &item.IPID, &item.Name, &item.StartTS, &mainFlag, &item.SwitchCount); err != nil {
			return index, coverage, err
		}
		item.IsMainThread = mainFlag != 0
		index.ByITID[item.ITID] = item
		index.ByTID[item.TID] = item
		index.ByProcess[item.IPID] = append(index.ByProcess[item.IPID], item)
	}
	if err := rows.Err(); err != nil {
		return index, coverage, err
	}
	return index, coverage, nil
}

func (tdb *traceDB) loadArgsets(ctx context.Context) (map[int64]map[string]traceDBValue, []TraceDBCoverage, error) {
	argsCoverage, err := tdb.inspectCoverage(ctx, "resolver", "args", []string{"argset", "key", "datatype", "value"})
	if err != nil {
		return nil, []TraceDBCoverage{argsCoverage}, err
	}
	dictCoverage, err := tdb.inspectCoverage(ctx, "resolver", "data_dict", []string{"id", "data"})
	coverage := []TraceDBCoverage{argsCoverage, dictCoverage}
	if err != nil {
		return nil, coverage, err
	}
	out := map[int64]map[string]traceDBValue{}
	if !argsCoverage.Found || !dictCoverage.Found || len(argsCoverage.ColumnsMissing) > 0 || len(dictCoverage.ColumnsMissing) > 0 {
		return out, coverage, nil
	}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT a.argset, key_dict.data AS key_name, COALESCE(a.datatype, 0),
		       a.value, value_dict.data AS value_text
		FROM args a
		LEFT JOIN data_dict key_dict ON key_dict.id = a.key
		LEFT JOIN data_dict value_dict ON value_dict.id = a.value
	`)
	if err != nil {
		return out, coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var argset sql.NullInt64
		var key sql.NullString
		var datatype sql.NullInt64
		var numeric sql.NullInt64
		var text sql.NullString
		if err := rows.Scan(&argset, &key, &datatype, &numeric, &text); err != nil {
			return out, coverage, err
		}
		if !argset.Valid || !key.Valid || strings.TrimSpace(key.String) == "" {
			continue
		}
		value := traceDBValue{}
		if datatype.Valid && datatype.Int64 == 1 && text.Valid {
			value = traceDBValue{Valid: true, Text: text.String}
		} else if numeric.Valid {
			value = traceDBValue{Valid: true, Text: fmt.Sprintf("%d", numeric.Int64)}
		}
		if !value.Valid {
			continue
		}
		if out[argset.Int64] == nil {
			out[argset.Int64] = map[string]traceDBValue{}
		}
		out[argset.Int64][key.String] = value
	}
	return out, coverage, rows.Err()
}

func (tdb *traceDB) loadRawEventCPUs(ctx context.Context) (map[traceDBInstantKey]int64, TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "resolver", "raw", []string{"ts", "name", "cpu"})
	out := map[traceDBInstantKey]int64{}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return out, coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, "SELECT ts, name, cpu FROM raw WHERE ts IS NOT NULL AND name IS NOT NULL AND cpu IS NOT NULL")
	if err != nil {
		return out, coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var ts, cpu int64
		var name string
		if err := rows.Scan(&ts, &name, &cpu); err != nil {
			return out, coverage, err
		}
		key := traceDBInstantKey{TS: ts, Name: name}
		if _, ok := out[key]; !ok {
			out[key] = cpu
		}
	}
	return out, coverage, rows.Err()
}

func (tdb *traceDB) loadSchedStarts(ctx context.Context) (map[int64][]traceDBSchedStart, TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "resolver", "sched_slice", []string{"itid", "ts", "cpu", "priority"})
	out := map[int64][]traceDBSchedStart{}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return out, coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT itid, ts, cpu, COALESCE(priority, 120)
		FROM sched_slice
		WHERE itid IS NOT NULL AND ts IS NOT NULL AND cpu IS NOT NULL
		ORDER BY itid, ts
	`)
	if err != nil {
		return out, coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var itid int64
		var item traceDBSchedStart
		if err := rows.Scan(&itid, &item.TS, &item.CPU, &item.Priority); err != nil {
			return out, coverage, err
		}
		out[itid] = append(out[itid], item)
	}
	return out, coverage, rows.Err()
}

func traceDBNextSchedMeta(starts map[int64][]traceDBSchedStart, itid int64, ts, defaultCPU, defaultPrio int64) (int64, int64) {
	entries := starts[itid]
	idx := sort.Search(len(entries), func(i int) bool { return entries[i].TS >= ts })
	if idx >= len(entries) {
		return defaultCPU, defaultPrio
	}
	return entries[idx].CPU, entries[idx].Priority
}

func (tdb *traceDB) loadRunningIntervals(ctx context.Context) (map[int64][]traceDBRunningInterval, TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "resolver", "thread_state", []string{"itid", "ts", "dur", "cpu", "state"})
	out := map[int64][]traceDBRunningInterval{}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return out, coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT itid, ts, COALESCE(dur, 0), cpu, COALESCE(state, '')
		FROM thread_state
		WHERE itid IS NOT NULL AND ts IS NOT NULL AND cpu IS NOT NULL
		ORDER BY itid, ts
	`)
	if err != nil {
		return out, coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var itid int64
		var dur int64
		var state string
		var item traceDBRunningInterval
		if err := rows.Scan(&itid, &item.Start, &dur, &item.CPU, &state); err != nil {
			return out, coverage, err
		}
		if !traceDBThreadStateIsRunning(state) || dur <= 0 {
			continue
		}
		item.End = item.Start + dur
		out[itid] = append(out[itid], item)
		coverage.RowsEmitted++
	}
	return out, coverage, rows.Err()
}

func traceDBThreadStateIsRunning(state string) bool {
	return strings.EqualFold(strings.TrimSpace(state), "Running")
}

func traceDBCPUAt(intervals map[int64][]traceDBRunningInterval, itid, ts, defaultCPU int64) int64 {
	entries := intervals[itid]
	idx := sort.Search(len(entries), func(i int) bool { return entries[i].Start > ts })
	if idx == 0 {
		return defaultCPU
	}
	entry := entries[idx-1]
	if entry.Start <= ts && ts < entry.End {
		return entry.CPU
	}
	return defaultCPU
}

func (tdb *traceDB) loadActiveThreadIDs(ctx context.Context) (map[int64]bool, []TraceDBCoverage, error) {
	sources := []struct {
		table  string
		column string
	}{
		{"callstack", "callid"},
		{"sched_slice", "itid"},
		{"thread_state", "itid"},
		{"syscall", "itid"},
		{"native_hook", "itid"},
		{"frame_slice", "itid"},
	}
	out := map[int64]bool{}
	var coverage []TraceDBCoverage
	for _, source := range sources {
		item, err := tdb.inspectCoverage(ctx, "resolver.active_thread", source.table, []string{source.column})
		coverage = append(coverage, item)
		if err != nil {
			return out, coverage, err
		}
		if !item.Found || len(item.ColumnsMissing) > 0 {
			continue
		}
		rows, err := tdb.db.QueryContext(ctx,
			"SELECT DISTINCT "+quoteSQLiteIdent(source.column)+" FROM "+quoteSQLiteIdent(source.table)+" WHERE "+quoteSQLiteIdent(source.column)+" IS NOT NULL",
		)
		if err != nil {
			return out, coverage, err
		}
		for rows.Next() {
			var itid int64
			if err := rows.Scan(&itid); err != nil {
				_ = rows.Close()
				return out, coverage, err
			}
			out[itid] = true
		}
		if err := rows.Close(); err != nil {
			return out, coverage, err
		}
	}
	return out, coverage, nil
}

func quoteSQLiteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
