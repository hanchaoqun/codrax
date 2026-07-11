package hitraceconv

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

// Mirrors tracequery.maxTraceCPUIndex. Keep the converter boundary identical
// so malformed SQL CPUs are rejected before they can be minted into systrace.
const maxTraceDBCPUIndex int64 = 4095

// Trace Streamer uses uint32 append indexes for internal identities and
// reserves UINT32_MAX as the missing sentinel.  Index zero is valid (the
// canonical idle/swapper thread in the scheduler projection).
const maxTraceDBInternalID int64 = 1<<32 - 2

type traceDB struct {
	db   *sql.DB
	path string
}

type traceDBValue struct {
	Valid    bool
	Text     string
	Datatype int64
}

type traceDBArgsetIndex struct {
	Sets        map[int64]map[string]traceDBValue
	Present     map[int64]bool
	Invalid     map[int64]bool
	InvalidKeys map[int64]map[string]bool
}

type traceDBProcess struct {
	IPID             int64
	PID              int64
	Name             string
	RegistrationHint traceDBTimestampMetadata
}

type traceDBThread struct {
	ITID             int64
	TID              int64
	IPID             int64
	Name             string
	RegistrationHint traceDBTimestampMetadata
	ObservedEndHint  traceDBTimestampMetadata
	IsMainThread     bool
	SwitchCount      int64
}

// traceDBTimestampMetadata is deliberately not a lifetime interval. Current
// Trace Streamer does not populate thread.start_ts on its normal creation
// paths, process.start_ts is a path-dependent first observation, and
// thread.end_ts can be overwritten by exit/free and later TID reuse. Keep the
// tri-state provenance so registration/coverage can be honest without letting
// these values participate in hard identity or generation selection.
type traceDBTimestampMetadata struct {
	Value   int64
	Known   bool
	Tainted bool
}

type traceDBThreadIndex struct {
	ByITID             map[int64]traceDBThread
	AmbiguousITID      map[int64]bool
	ThreadIDToITID     map[int64]int64
	AmbiguousThreadID  map[int64]bool
	HasThreadIDColumn  bool
	ByTIDCandidates    map[int64][]traceDBThread
	Processes          map[int64]traceDBProcess
	AmbiguousIPID      map[int64]bool
	ProcessIDToIPID    map[int64]int64
	AmbiguousProcessID map[int64]bool
	HasProcessIDColumn bool
	ByProcess          map[int64][]traceDBThread
	TraceStart         int64
	TraceStartKnown    bool
	RunningTaintedITID map[int64]bool
	RunningGlobalTaint bool
}

type traceDBRunningIntegrity struct {
	TaintedITIDs map[int64]bool
	GlobalTaint  bool
}

type traceDBRawWakeup struct {
	RowID     int64
	TS        int64
	Name      string
	TargetCPU int64
	ITID      int64
}

type traceDBSchedStart struct {
	TS       int64
	CPU      int64
	Priority int64
	Known    bool
}

type traceDBSchedStartIndex struct {
	ByITID         map[int64][]traceDBSchedStart
	TaintedITIDs   map[int64]bool
	GlobalBarriers []int64
	GlobalTaint    bool
}

type traceDBRunningInterval struct {
	Start        int64
	End          int64
	CPU          int64
	PrefixMaxEnd int64
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
		"SELECT 1 FROM sqlite_master WHERE type='table' AND name=?1 COLLATE NOCASE LIMIT 1",
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
		if sqliteASCIIIdentifierEqual(item, column) {
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
		columnSet[sqliteASCIIIdentifierFold(column)] = true
	}
	for _, column := range requiredColumns {
		if columnSet[sqliteASCIIIdentifierFold(column)] {
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

func sqliteASCIIIdentifierEqual(left, right string) bool {
	return sqliteASCIIIdentifierFold(left) == sqliteASCIIIdentifierFold(right)
}

func sqliteASCIIIdentifierFold(value string) string {
	bytes := []byte(value)
	for i, b := range bytes {
		if b >= 'A' && b <= 'Z' {
			bytes[i] = b + ('a' - 'A')
		}
	}
	return string(bytes)
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
	coverage.FieldSources = map[string]string{
		"start_ts": "exactly one non-negative SQLite INTEGER row; zero is a valid timestamp",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return 0, coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, "SELECT start_ts FROM trace_range")
	if err != nil {
		coverage.Error = err.Error()
		return 0, coverage, err
	}
	defer rows.Close()
	var value any
	rowCount := 0
	for rows.Next() {
		var raw any
		if err := rows.Scan(&raw); err != nil {
			coverage.Error = err.Error()
			return 0, coverage, err
		}
		rowCount++
		if rowCount == 1 {
			value = raw
			continue
		}
		// Singleton validity is already disproven. Stop without materializing an
		// unbounded malformed trace_range table.
		break
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return 0, coverage, err
	}
	if rowCount != 1 || coverage.RowsRead != 1 {
		observed := coverage.RowsRead
		if rowCount > observed {
			observed = rowCount
		}
		coverage.Skipped = fmt.Sprintf("expected exactly one trace_range row, got %d", observed)
		return 0, coverage, nil
	}
	start, ok := traceDBStrictSQLiteInt(value)
	if !ok || start < 0 {
		coverage.Skipped = "trace_range.start_ts rejected: expected non-negative SQLite INTEGER"
		return 0, coverage, nil
	}
	coverage.RowsEmitted = 1
	return start, coverage, nil
}

func (tdb *traceDB) loadThreadIndex(ctx context.Context) (traceDBThreadIndex, []TraceDBCoverage, error) {
	var coverage []TraceDBCoverage
	traceStart, traceStartCoverage, err := tdb.traceStart(ctx)
	coverage = append(coverage, traceStartCoverage)
	if err != nil {
		return traceDBThreadIndex{}, coverage, err
	}
	processCoverage, err := tdb.inspectCoverage(ctx, "resolver", "process", []string{"ipid", "pid"})
	coverage = append(coverage, processCoverage)
	if err != nil {
		return traceDBThreadIndex{}, coverage, err
	}
	threadCoverage, err := tdb.inspectCoverage(ctx, "resolver", "thread", []string{"itid", "tid", "ipid", "is_main_thread", "switch_count"})
	coverage = append(coverage, threadCoverage)
	if err != nil {
		return traceDBThreadIndex{}, coverage, err
	}
	index := newTraceDBThreadIndex(traceStart, traceStartCoverage.RowsEmitted == 1 && traceStartCoverage.Skipped == "")
	if processCoverage.Found {
		index.HasProcessIDColumn, err = tdb.columnExists(ctx, "process", "id")
		if err != nil {
			return index, coverage, err
		}
		if index.HasProcessIDColumn {
			coverage[1].ColumnsPresent = appendTraceDBCoverageColumn(coverage[1].ColumnsPresent, "id")
			sort.Strings(coverage[1].ColumnsPresent)
		}
		coverage[1].FieldSources = map[string]string{
			"source_profile": map[bool]string{true: "full current profile: process.id/ipid alias parity required", false: "id-less compatibility profile"}[index.HasProcessIDColumn],
		}
	}
	if threadCoverage.Found {
		index.HasThreadIDColumn, err = tdb.columnExists(ctx, "thread", "id")
		if err != nil {
			return index, coverage, err
		}
		if index.HasThreadIDColumn {
			coverage[2].ColumnsPresent = appendTraceDBCoverageColumn(coverage[2].ColumnsPresent, "id")
			sort.Strings(coverage[2].ColumnsPresent)
		}
		coverage[2].FieldSources = map[string]string{
			"source_profile": map[bool]string{true: "full current profile: thread.id/itid alias parity required", false: "id-less compatibility profile"}[index.HasThreadIDColumn],
		}
	}
	if processCoverage.Found && len(processCoverage.ColumnsMissing) == 0 {
		if err := tdb.loadStrictProcessIndex(ctx, &index, &coverage[1]); err != nil {
			return index, coverage, err
		}
	}
	if threadCoverage.Found && len(threadCoverage.ColumnsMissing) == 0 {
		if err := tdb.loadStrictThreadIndex(ctx, &index, &coverage[2]); err != nil {
			return index, coverage, err
		}
	}
	return index, coverage, nil
}

func (tdb *traceDB) loadArgsets(ctx context.Context) (traceDBArgsetIndex, []TraceDBCoverage, error) {
	argsCoverage, err := tdb.inspectCoverage(ctx, "resolver", "args", []string{"argset", "key", "datatype", "value"})
	if err != nil {
		return traceDBArgsetIndex{}, []TraceDBCoverage{argsCoverage}, err
	}
	dictCoverage, err := tdb.inspectCoverage(ctx, "resolver", "data_dict", []string{"id", "data"})
	coverage := []TraceDBCoverage{argsCoverage, dictCoverage}
	if err != nil {
		return traceDBArgsetIndex{}, coverage, err
	}
	out := traceDBArgsetIndex{
		Sets: map[int64]map[string]traceDBValue{}, Present: map[int64]bool{}, Invalid: map[int64]bool{},
		InvalidKeys: map[int64]map[string]bool{},
	}
	if !argsCoverage.Found || !dictCoverage.Found || len(argsCoverage.ColumnsMissing) > 0 || len(dictCoverage.ColumnsMissing) > 0 {
		return out, coverage, nil
	}
	dict := map[int64]string{}
	dictInvalid := map[int64]bool{}
	// data_dict and args are logical sets.  Their correctness cannot depend on
	// SQLite's optional rowid (WITHOUT ROWID is a valid schema variant), and all
	// duplicate typed keys are poisoned below regardless of scan order.
	dictRows, err := tdb.db.QueryContext(ctx, `SELECT id, data FROM data_dict ORDER BY id, data`)
	if err != nil {
		return out, coverage, err
	}
	invalidDictRows := 0
	for dictRows.Next() {
		var idRaw, dataRaw any
		if err := dictRows.Scan(&idRaw, &dataRaw); err != nil {
			_ = dictRows.Close()
			return out, coverage, err
		}
		id, idOK := traceDBStrictSQLiteInt(idRaw)
		data, dataOK := traceDBStrictArgText(dataRaw, true)
		if !idOK || id < 0 || !dataOK {
			invalidDictRows++
			if idOK && id >= 0 {
				dictInvalid[id] = true
				delete(dict, id)
			}
			continue
		}
		if dictInvalid[id] {
			invalidDictRows++
			continue
		}
		if _, duplicate := dict[id]; duplicate {
			dictInvalid[id] = true
			delete(dict, id)
			invalidDictRows++
			continue
		}
		if !dictInvalid[id] {
			dict[id] = data
		}
	}
	if err := dictRows.Err(); err != nil {
		_ = dictRows.Close()
		return out, coverage, err
	}
	if err := dictRows.Close(); err != nil {
		return out, coverage, err
	}
	if invalidDictRows > 0 {
		dictCoverage.Skipped = fmt.Sprintf("%d data_dict row(s) rejected: invalid or duplicate typed identity", invalidDictRows)
	}
	dictCoverage.RowsEmitted = len(dict)
	rows, err := tdb.db.QueryContext(ctx, `SELECT argset, key, datatype, value FROM args ORDER BY argset, key, datatype, value`)
	if err != nil {
		return out, coverage, err
	}
	defer rows.Close()
	invalidArgRows := 0
	for rows.Next() {
		var argsetRaw, keyRaw, datatypeRaw, valueRaw any
		if err := rows.Scan(&argsetRaw, &keyRaw, &datatypeRaw, &valueRaw); err != nil {
			return out, coverage, err
		}
		argset, argsetOK := traceDBStrictSQLiteInt(argsetRaw)
		if !argsetOK || argset < 0 {
			invalidArgRows++
			continue
		}
		out.Present[argset] = true
		keyID, keyOK := traceDBStrictSQLiteInt(keyRaw)
		datatype, datatypeOK := traceDBStrictSQLiteInt(datatypeRaw)
		if !keyOK || keyID < 0 || !datatypeOK || (datatype != 0 && datatype != 1) || dictInvalid[keyID] {
			if keyOK && keyID >= 0 {
				if keyText, exists := dict[keyID]; exists {
					traceDBInvalidateArgKey(out, argset, strings.ToLower(strings.TrimSpace(keyText)))
				} else {
					out.Invalid[argset] = true
				}
			} else {
				out.Invalid[argset] = true
			}
			invalidArgRows++
			continue
		}
		keyText, keyExists := dict[keyID]
		canonicalKey := strings.ToLower(strings.TrimSpace(keyText))
		if !keyExists || canonicalKey == "" {
			out.Invalid[argset] = true
			invalidArgRows++
			continue
		}
		if keyText != canonicalKey {
			traceDBInvalidateArgKey(out, argset, canonicalKey)
			invalidArgRows++
			continue
		}
		valueText := ""
		if datatype == 0 {
			value, valueOK := traceDBStrictSQLiteInt(valueRaw)
			if !valueOK {
				traceDBInvalidateArgKey(out, argset, canonicalKey)
				invalidArgRows++
				continue
			}
			valueText = strconv.FormatInt(value, 10)
		} else {
			valueID, valueOK := traceDBStrictSQLiteInt(valueRaw)
			value, valueExists := dict[valueID]
			if !valueOK || valueID < 0 || dictInvalid[valueID] || !valueExists {
				traceDBInvalidateArgKey(out, argset, canonicalKey)
				invalidArgRows++
				continue
			}
			valueText = value
		}
		if out.Sets[argset] == nil {
			out.Sets[argset] = map[string]traceDBValue{}
		}
		if out.InvalidKeys[argset][canonicalKey] {
			// Poison is monotonic: a later syntactically valid row must never
			// resurrect a canonical key that was malformed or duplicated.
			invalidArgRows++
			continue
		}
		if _, duplicate := out.Sets[argset][canonicalKey]; duplicate {
			traceDBInvalidateArgKey(out, argset, canonicalKey)
			invalidArgRows++
			continue
		}
		out.Sets[argset][canonicalKey] = traceDBValue{Valid: true, Text: valueText, Datatype: datatype}
	}
	if invalidArgRows > 0 {
		argsCoverage.Skipped = fmt.Sprintf("%d args row(s) rejected: invalid scalar, dictionary reference, datatype, or duplicate canonical key", invalidArgRows)
	}
	argsCoverage.RowsEmitted = 0
	for argset, values := range out.Sets {
		if out.Invalid[argset] {
			continue
		}
		for key := range values {
			if !out.InvalidKeys[argset][key] {
				argsCoverage.RowsEmitted++
			}
		}
	}
	coverage[0] = argsCoverage
	coverage[1] = dictCoverage
	return out, coverage, rows.Err()
}

func traceDBInvalidateArgKey(index traceDBArgsetIndex, argset int64, key string) {
	if key == "" {
		index.Invalid[argset] = true
		return
	}
	if index.InvalidKeys[argset] == nil {
		index.InvalidKeys[argset] = map[string]bool{}
	}
	index.InvalidKeys[argset][key] = true
	if index.Sets[argset] != nil {
		delete(index.Sets[argset], key)
	}
}

func traceDBStrictArgText(value any, allowEmpty bool) (string, bool) {
	text, ok := value.(string)
	if !ok || !utf8.ValidString(text) || len(text) > 4096 || (!allowEmpty && strings.TrimSpace(text) == "") {
		return "", false
	}
	for _, r := range text {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return text, true
}

func (tdb *traceDB) loadRawWakeups(ctx context.Context) ([]traceDBRawWakeup, TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "resolver", "raw", []string{"ts", "name", "cpu", "itid"})
	var out []traceDBRawWakeup
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return out, coverage, err
	}
	hasID, err := tdb.columnExists(ctx, "raw", "id")
	if err != nil {
		coverage.Error = err.Error()
		return out, coverage, err
	}
	rowIdentity := "rowid"
	if hasID {
		rowIdentity = quoteSQLiteIdent("id")
	} else if err := traceDBRequireRowID(ctx, tdb, "raw"); err != nil {
		coverage.Skipped = "missing raw.id and usable SQLite rowid; wakeup rows have no stable source identity"
		return out, coverage, nil
	}
	duplicateIDs, err := traceDBRawDuplicateStableIDs(ctx, tdb, rowIdentity, hasID)
	if err != nil {
		coverage.Error = err.Error()
		return out, coverage, err
	}
	stableOrderExpr := traceDBStableUint32OrderExpr(rowIdentity, hasID)
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT `+rowIdentity+`, ts, name, cpu, itid
		FROM raw
		WHERE name IN ('sched_wakeup', 'sched_waking')
		ORDER BY ts, `+stableOrderExpr)
	if err != nil {
		coverage.Error = err.Error()
		return out, coverage, err
	}
	defer rows.Close()
	coverage.RowsRead = 0
	invalid := 0
	seenRowIDs := map[int64]bool{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return out, coverage, err
		}
		coverage.RowsRead++
		var rowIDRaw, tsRaw, nameRaw, cpuRaw, itidRaw any
		if err := rows.Scan(&rowIDRaw, &tsRaw, &nameRaw, &cpuRaw, &itidRaw); err != nil {
			coverage.Error = err.Error()
			return out, coverage, err
		}
		rowID, rowIDOK := traceDBStrictSQLiteInt(rowIDRaw)
		if hasID {
			rowID, rowIDOK = traceDBStrictStableUint32Projection(rowIDRaw)
		}
		ts, tsOK := traceDBStrictSQLiteInt(tsRaw)
		name, nameOK := nameRaw.(string)
		cpu, cpuOK := traceDBStrictSQLiteInt(cpuRaw)
		itid, itidOK := traceDBStrictSignedUint32Projection(itidRaw)
		if !rowIDOK || rowID < 0 || (!hasID && rowID == 0) || duplicateIDs[rowID] || seenRowIDs[rowID] ||
			!tsOK || ts < 0 || !nameOK || (name != "sched_wakeup" && name != "sched_waking") ||
			!cpuOK || !validTraceDBCPUIndex(cpu) || !itidOK || itid < 0 || itid > maxTraceDBInternalID {
			invalid++
			continue
		}
		seenRowIDs[rowID] = true
		out = append(out, traceDBRawWakeup{
			RowID:     rowID,
			TS:        ts,
			Name:      name,
			TargetCPU: cpu,
			ITID:      itid,
		})
		coverage.RowsEmitted++
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return out, coverage, err
	}
	if invalid > 0 {
		coverage.Skipped = fmt.Sprintf("%d raw wakeup row(s) skipped: invalid or duplicate typed identity/scalar metadata", invalid)
	}
	coverage.FieldSources = map[string]string{
		"stable_identity":      map[bool]string{true: "raw.id with exact full-uint32 signed-int32 projection", false: "raw.rowid"}[hasID],
		"thread_identity":      "raw.itid with exact Trace Streamer signed-int32 to canonical uint32 projection; -1 sentinel rejected",
		"same_timestamp_order": map[bool]string{true: "raw.ts then canonical uint32(raw.id)", false: "raw.ts then raw.rowid"}[hasID],
	}
	return out, coverage, nil
}

func (tdb *traceDB) loadRunningIntervals(ctx context.Context) (map[int64][]traceDBRunningInterval, traceDBRunningIntegrity, TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "resolver", "thread_state", []string{"itid", "ts", "dur", "cpu", "state"})
	out := map[int64][]traceDBRunningInterval{}
	integrity := traceDBRunningIntegrity{TaintedITIDs: map[int64]bool{}}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return out, integrity, coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT itid, ts, dur, cpu, state
		FROM thread_state
	`)
	if err != nil {
		return out, integrity, coverage, err
	}
	defer rows.Close()
	invalidRunning := 0
	taintRunningWitness := func(itidRaw any) {
		itid, itidOK := traceDBStrictSQLiteInt(itidRaw)
		if itidOK && itid >= 0 && itid <= maxTraceDBInternalID {
			integrity.TaintedITIDs[itid] = true
		} else {
			integrity.GlobalTaint = true
		}
	}
	for rows.Next() {
		var itidRaw, tsRaw, durRaw, cpuRaw, stateRaw any
		var item traceDBRunningInterval
		if err := rows.Scan(&itidRaw, &tsRaw, &durRaw, &cpuRaw, &stateRaw); err != nil {
			return out, integrity, coverage, err
		}
		state, stateOK := stateRaw.(string)
		if !stateOK || strings.TrimSpace(state) == "" {
			invalidRunning++
			taintRunningWitness(itidRaw)
			continue
		}
		if !traceDBThreadStateIsRunning(state) {
			// The upstream schema uses the exact token "Running".  A
			// case/whitespace drift that resembles it is an ambiguous potential
			// CPU witness, not a different state that can be ignored.
			if strings.EqualFold(strings.TrimSpace(state), "Running") {
				invalidRunning++
				taintRunningWitness(itidRaw)
			}
			continue
		}
		itid, itidOK := traceDBStrictSQLiteInt(itidRaw)
		item.Start, _ = traceDBStrictSQLiteInt(tsRaw)
		dur, durOK := traceDBStrictSQLiteInt(durRaw)
		item.CPU, _ = traceDBStrictSQLiteInt(cpuRaw)
		valid := itidOK && itid >= 0 && itid <= maxTraceDBInternalID
		if _, ok := traceDBStrictSQLiteInt(tsRaw); !ok || item.Start < 0 {
			valid = false
		}
		if !durOK || dur <= 0 || item.Start > math.MaxInt64-dur {
			valid = false
		}
		if _, ok := traceDBStrictSQLiteInt(cpuRaw); !ok || !validTraceDBCPUIndex(item.CPU) {
			valid = false
		}
		if !valid {
			invalidRunning++
			taintRunningWitness(itidRaw)
			continue
		}
		item.End = item.Start + dur
		item.PrefixMaxEnd = item.End
		entries := out[itid]
		if len(entries) > 0 && entries[len(entries)-1].PrefixMaxEnd > item.PrefixMaxEnd {
			item.PrefixMaxEnd = entries[len(entries)-1].PrefixMaxEnd
		}
		out[itid] = append(out[itid], item)
		coverage.RowsEmitted++
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return out, integrity, coverage, err
	}
	for itid, entries := range out {
		sort.SliceStable(entries, func(i, j int) bool {
			if entries[i].Start != entries[j].Start {
				return entries[i].Start < entries[j].Start
			}
			if entries[i].End != entries[j].End {
				return entries[i].End < entries[j].End
			}
			return entries[i].CPU < entries[j].CPU
		})
		prefixMax := int64(0)
		for i := range entries {
			if i == 0 || entries[i].End > prefixMax {
				prefixMax = entries[i].End
			}
			entries[i].PrefixMaxEnd = prefixMax
		}
		out[itid] = entries
	}
	if invalidRunning > 0 {
		coverage.Skipped = fmt.Sprintf("%d potential Running thread_state row(s) rejected: invalid state, typed identity, CPU, or time interval; affected CPU witnesses tainted", invalidRunning)
		coverage.FieldSources = map[string]string{
			"cpu_witness_integrity": "strict thread_state scalar validation; malformed potential Running rows taint the affected itid (or globally when itid is unknown)",
		}
	}
	return out, integrity, coverage, nil
}

func traceDBThreadStateIsRunning(state string) bool {
	return state == "Running"
}

func traceDBCPUAt(intervals map[int64][]traceDBRunningInterval, itid, ts, defaultCPU int64) int64 {
	if cpu, ok := traceDBKnownCPUAt(intervals, itid, ts); ok {
		return cpu
	}
	return defaultCPU
}

// traceDBKnownCPUAt returns a CPU only when every Running interval covering ts
// agrees on the same non-negative CPU. The prefix maximum keeps the common
// non-overlapping case O(log n) while still detecting malformed overlaps.
func traceDBKnownCPUAt(intervals map[int64][]traceDBRunningInterval, itid, ts int64) (int64, bool) {
	entries := intervals[itid]
	idx := sort.Search(len(entries), func(i int) bool { return entries[i].Start > ts })
	if idx == 0 {
		return 0, false
	}
	var cpu int64
	found := false
	for i := idx - 1; i >= 0; i-- {
		entry := entries[i]
		if entry.Start <= ts && ts < entry.End {
			if !validTraceDBCPUIndex(entry.CPU) {
				return 0, false
			}
			if found && cpu != entry.CPU {
				return 0, false
			}
			cpu = entry.CPU
			found = true
		}
		if i == 0 || entries[i-1].PrefixMaxEnd <= ts {
			break
		}
	}
	return cpu, found
}

func validTraceDBCPUIndex(cpu int64) bool {
	return cpu >= 0 && cpu <= maxTraceDBCPUIndex
}

func quoteSQLiteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
