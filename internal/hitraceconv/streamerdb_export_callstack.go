package hitraceconv

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxTraceDBCallstackTokenBytes = 4096

type traceDBCallstackRow struct {
	ID          int64
	SourceID    int64
	TS          int64
	Dur         int64
	End         int64
	CallID      int64
	EmitterITID int64
	OwnerIPID   int64
	Name        string
	Flag        string
	Depth       int64
	DepthKnown  bool
	Cookie      string
	Task        string
	TID         int64
	TGID        int64
	StartCPU    int64
	EndCPU      int64
}

type traceDBCallstackEndpoint struct {
	Row   traceDBCallstackRow
	TS    int64
	CPU   int64
	Begin bool
}

type traceDBCallstackAsyncKey struct {
	OwnerIPID int64
	TGID      int64
	Name      string
	Cookie    string
}

// exportTraceDBCallstack publishes trace-marker endpoints only after the SQL
// rows have passed strict scalar, identity, CPU and pairing/laminar audits.
// A malformed row is never repaired with pid/cpu/cookie zero: those values are
// valid protocol tokens and would turn missing evidence into fabricated facts.
func exportTraceDBCallstack(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, index traceDBThreadIndex, running map[int64][]traceDBRunningInterval, _ map[int64]string) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "slice", "callstack", []string{"ts", "dur", "name"})
	coverage.FieldSources = map[string]string{
		"cpu":              "all Running thread_state intervals covering an endpoint agree on one valid CPU",
		"emitter_identity": "callstack.itid when present, otherwise callstack.callid; exact thread.itid join",
		"async_owner":      "exact non-ambiguous process.ipid generation and process.pid",
		"row_order":        "strict SQLite rowid; optional source id remains provenance only; typed endpoint phase ordering",
		"async_identity":   "nonzero cookie or chainId; both must agree when both are present",
		"sync_pairing":     "complete per-emitter laminar interval audit before atomic publication",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}

	hasITID, err := tdb.columnExists(ctx, "callstack", "itid")
	if err != nil {
		return coverage, err
	}
	hasID, err := tdb.columnExists(ctx, "callstack", "id")
	if err != nil {
		return coverage, err
	}
	hasDepth, err := tdb.columnExists(ctx, "callstack", "depth")
	if err != nil {
		return coverage, err
	}
	hasCallID, err := tdb.columnExists(ctx, "callstack", "callid")
	if err != nil {
		return coverage, err
	}
	if !hasITID && !hasCallID {
		coverage.ColumnsMissing = append(coverage.ColumnsMissing, "itid|callid")
		coverage.Skipped = "missing required emitter identity column: itid|callid"
		return coverage, nil
	}
	hasFlag, err := tdb.columnExists(ctx, "callstack", "flag")
	if err != nil {
		return coverage, err
	}
	hasCookie, err := tdb.columnExists(ctx, "callstack", "cookie")
	if err != nil {
		return coverage, err
	}
	hasChainID, err := tdb.columnExists(ctx, "callstack", "chainId")
	if err != nil {
		return coverage, err
	}
	for _, optional := range []struct {
		name    string
		present bool
	}{
		{"id", hasID}, {"itid", hasITID}, {"callid", hasCallID}, {"flag", hasFlag},
		{"cookie", hasCookie}, {"chainId", hasChainID}, {"depth", hasDepth},
	} {
		if optional.present {
			coverage.ColumnsPresent = appendTraceDBCoverageColumn(coverage.ColumnsPresent, optional.name)
		}
	}
	sort.Strings(coverage.ColumnsPresent)

	itidExpr := "NULL"
	if hasITID {
		itidExpr = quoteSQLiteIdent("itid")
	}
	callIDExpr := "NULL"
	if hasCallID {
		callIDExpr = quoteSQLiteIdent("callid")
	}
	flagExpr := "NULL"
	if hasFlag {
		flagExpr = quoteSQLiteIdent("flag")
	}
	cookieExpr := "NULL"
	if hasCookie {
		cookieExpr = quoteSQLiteIdent("cookie")
	}
	chainIDExpr := "NULL"
	if hasChainID {
		chainIDExpr = quoteSQLiteIdent("chainId")
	}
	idExpr := "NULL"
	if hasID {
		idExpr = quoteSQLiteIdent("id")
	}
	depthExpr := "NULL"
	if hasDepth {
		depthExpr = quoteSQLiteIdent("depth")
	}
	query := fmt.Sprintf(`
		SELECT rowid, %s, ts, dur, name, %s, %s, %s, %s, %s, %s
		FROM callstack
		ORDER BY rowid
	`, idExpr, callIDExpr, itidExpr, flagExpr, cookieExpr, chainIDExpr, depthExpr)
	rows, err := tdb.db.QueryContext(ctx, query)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()

	skipped := map[string]int{}
	var accepted []traceDBCallstackRow
	asyncPoisoned := false
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return coverage, err
		}
		var rowIDRaw, sourceIDRaw, tsRaw, durRaw, nameRaw, callIDRaw, itidRaw, flagRaw, cookieRaw, chainIDRaw, depthRaw any
		if err := rows.Scan(&rowIDRaw, &sourceIDRaw, &tsRaw, &durRaw, &nameRaw, &callIDRaw, &itidRaw, &flagRaw, &cookieRaw, &chainIDRaw, &depthRaw); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		row, reason := prepareTraceDBCallstackRow(index, running, hasID, hasITID, hasFlag, hasDepth,
			rowIDRaw, sourceIDRaw, tsRaw, durRaw, nameRaw, callIDRaw, itidRaw, flagRaw, cookieRaw, chainIDRaw, depthRaw)
		if reason != "" {
			if traceDBCallstackPotentialAsync(flagRaw, cookieRaw, chainIDRaw, hasFlag) {
				asyncPoisoned = true
			}
			skipped[reason]++
			continue
		}
		accepted = append(accepted, row)
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	syncLanes := map[int64][]traceDBCallstackRow{}
	asyncGroups := map[traceDBCallstackAsyncKey][]traceDBCallstackRow{}
	for _, row := range accepted {
		switch row.Flag {
		case "S", "C":
			key := traceDBCallstackAsyncKey{OwnerIPID: row.OwnerIPID, TGID: row.TGID, Name: row.Name, Cookie: row.Cookie}
			asyncGroups[key] = append(asyncGroups[key], row)
		default:
			syncLanes[row.EmitterITID] = append(syncLanes[row.EmitterITID], row)
		}
	}

	for _, itid := range sortedTraceDBCallstackLaneIDs(syncLanes) {
		lane := syncLanes[itid]
		if reason := auditTraceDBCallstackSyncLane(lane); reason != "" {
			skipped[reason] += len(lane)
			continue
		}
		endpoints := traceDBCallstackSyncEndpoints(lane)
		for _, endpoint := range endpoints {
			body := fmt.Sprintf("tracing_mark_write: E|%d|", endpoint.Row.TGID)
			if endpoint.Begin {
				body = fmt.Sprintf("tracing_mark_write: B|%d|%s", endpoint.Row.TGID, endpoint.Row.Name)
			}
			if err := addTraceDBInstantRow(sink, endpoint.TS, endpoint.Row.Task, endpoint.Row.TID,
				endpoint.Row.TGID, endpoint.CPU, body); err != nil {
				return coverage, err
			}
			coverage.RowsEmitted++
		}
	}
	if asyncPoisoned {
		for _, group := range asyncGroups {
			skipped["async_family_fail_closed"] += len(group)
		}
		coverage.Skipped = traceDBCallstackSkipSummary(skipped)
		return coverage, nil
	}

	asyncKeys := make([]traceDBCallstackAsyncKey, 0, len(asyncGroups))
	for key := range asyncGroups {
		asyncKeys = append(asyncKeys, key)
	}
	sort.Slice(asyncKeys, func(i, j int) bool {
		if asyncKeys[i].OwnerIPID != asyncKeys[j].OwnerIPID {
			return asyncKeys[i].OwnerIPID < asyncKeys[j].OwnerIPID
		}
		if asyncKeys[i].TGID != asyncKeys[j].TGID {
			return asyncKeys[i].TGID < asyncKeys[j].TGID
		}
		if asyncKeys[i].Name != asyncKeys[j].Name {
			return asyncKeys[i].Name < asyncKeys[j].Name
		}
		return asyncKeys[i].Cookie < asyncKeys[j].Cookie
	})
	for _, key := range asyncKeys {
		group := asyncGroups[key]
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].TS != group[j].TS {
				return group[i].TS < group[j].TS
			}
			return group[i].ID < group[j].ID
		})
		if reason := auditTraceDBCallstackAsyncGroup(group); reason != "" {
			skipped[reason] += len(group)
			continue
		}
		for _, row := range group {
			action := "S"
			if row.Flag == "C" {
				action = "F"
			}
			body := fmt.Sprintf("tracing_mark_write: %s|%d|%s|%s", action, row.TGID, row.Name, row.Cookie)
			if err := addTraceDBInstantRow(sink, row.TS, row.Task, row.TID, row.TGID, row.StartCPU, body); err != nil {
				return coverage, err
			}
			coverage.RowsEmitted++
		}
	}
	coverage.Skipped = traceDBCallstackSkipSummary(skipped)
	return coverage, nil
}

func prepareTraceDBCallstackRow(index traceDBThreadIndex, running map[int64][]traceDBRunningInterval,
	hasID, hasITID, hasFlag, hasDepth bool,
	rowIDRaw, sourceIDRaw, tsRaw, durRaw, nameRaw, callIDRaw, itidRaw, flagRaw, cookieRaw, chainIDRaw, depthRaw any,
) (traceDBCallstackRow, string) {
	var row traceDBCallstackRow
	var ok bool
	if row.ID, ok = traceDBStrictSQLiteInt(rowIDRaw); !ok || row.ID <= 0 {
		return row, "invalid_row_id"
	}
	if hasID {
		if sourceID, valid := traceDBStrictSQLiteInt(sourceIDRaw); valid && sourceID >= 0 {
			row.SourceID = sourceID
		}
	}
	if row.TS, ok = traceDBStrictSQLiteInt(tsRaw); !ok || row.TS < 0 {
		return row, "invalid_timestamp"
	}
	if row.Dur, ok = traceDBStrictSQLiteInt(durRaw); !ok || row.Dur < 0 {
		return row, "invalid_duration"
	}
	if row.TS > math.MaxInt64-row.Dur {
		return row, "interval_overflow"
	}
	row.End = row.TS + row.Dur
	if hasITID {
		if row.EmitterITID, ok = traceDBStrictSQLiteInt(itidRaw); !ok || row.EmitterITID <= 0 {
			return row, "invalid_emitter_itid"
		}
	} else {
		if row.CallID, ok = traceDBStrictSQLiteInt(callIDRaw); !ok || row.CallID <= 0 {
			return row, "invalid_callid"
		}
		row.EmitterITID = row.CallID
	}
	if row.Name, ok = traceDBCallstackText(nameRaw, false); !ok || !traceDBCallstackMarkerToken(row.Name) {
		return row, "invalid_name"
	}
	if hasFlag {
		if row.Flag, ok = traceDBCallstackText(flagRaw, true); !ok {
			return row, "invalid_flag"
		}
	} else {
		row.Flag = ""
	}
	switch row.Flag {
	case "", "I":
		if traceDBCallstackRawIdentityPresent(cookieRaw) || traceDBCallstackRawIdentityPresent(chainIDRaw) {
			return row, "sync_with_async_identity"
		}
	case "S", "C":
		if row.Dur != 0 {
			return row, "async_nonzero_duration"
		}
	default:
		return row, "unknown_flag"
	}
	if hasDepth {
		if row.Depth, ok = traceDBStrictSQLiteInt(depthRaw); !ok || row.Depth < 0 || row.Depth > math.MaxInt32 {
			return row, "invalid_depth"
		}
		row.DepthKnown = true
	}
	thread, exists := index.ByITID[row.EmitterITID]
	if !exists || index.AmbiguousITID[row.EmitterITID] || thread.TID <= 0 || thread.TID > math.MaxInt32 {
		return row, "unresolved_emitter_thread"
	}
	if thread.StartTS < 0 {
		return row, "invalid_emitter_lifetime"
	}
	if row.TS < thread.StartTS {
		return row, "outside_emitter_lifetime"
	}
	if index.AmbiguousIPID[thread.IPID] {
		return row, "ambiguous_emitter_process"
	}
	process, processExists := index.Processes[thread.IPID]
	row.OwnerIPID = thread.IPID
	row.TID = thread.TID
	if !processExists || thread.IPID <= 0 || process.PID <= 0 || process.PID > math.MaxInt32 {
		return row, "unresolved_owner_process"
	}
	row.TGID = process.PID
	if row.TGID <= 0 || row.TGID > math.MaxInt32 {
		return row, "invalid_emitter_process"
	}
	if _, ok := traceDBCallstackText(thread.Name, true); !ok {
		return row, "invalid_emitter_comm"
	}
	row.Task = traceDBCommName(thread.Name, "unknown")
	if row.StartCPU, ok = traceDBKnownCPUAt(running, row.EmitterITID, row.TS); !ok {
		return row, "unknown_start_cpu"
	}
	row.EndCPU = row.StartCPU
	if row.Flag == "" || row.Flag == "I" {
		if row.EndCPU, ok = traceDBKnownCPUAt(running, row.EmitterITID, row.End); !ok {
			return row, "unknown_end_cpu"
		}
		return row, ""
	}
	cookie, cookiePresent, cookieValid := traceDBCallstackCookie(cookieRaw)
	if !cookieValid {
		return row, "invalid_cookie"
	}
	chainID, chainPresent, chainValid := traceDBCallstackCookie(chainIDRaw)
	if !chainValid {
		return row, "invalid_chain_id"
	}
	if cookiePresent && chainPresent && cookie != chainID {
		return row, "cookie_chain_id_conflict"
	}
	row.Cookie = cookie
	if !cookiePresent {
		row.Cookie = chainID
	}
	if row.Cookie == "" || !traceDBCallstackMarkerToken(row.Cookie) {
		return row, "missing_async_identity"
	}
	return row, ""
}

func traceDBCallstackText(value any, allowEmpty bool) (string, bool) {
	text, ok := value.(string)
	if !ok || !utf8.ValidString(text) {
		return "", false
	}
	if !allowEmpty && strings.TrimSpace(text) == "" {
		return "", false
	}
	for _, r := range text {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return text, true
}

func appendTraceDBCoverageColumn(columns []string, column string) []string {
	for _, existing := range columns {
		if existing == column {
			return columns
		}
	}
	return append(columns, column)
}

func traceDBCallstackPotentialAsync(flagValue, cookieValue, chainIDValue any, hasFlag bool) bool {
	if !hasFlag {
		return false
	}
	flag, ok := flagValue.(string)
	if ok && (flag == "S" || flag == "C") {
		return true
	}
	if ok && (flag == "" || flag == "I") {
		return traceDBCallstackRawIdentityPresent(cookieValue) || traceDBCallstackRawIdentityPresent(chainIDValue)
	}
	// Any row-level flag that is not a proven sync flag may be a malformed
	// async endpoint.  Poisoning the async lane prevents a later valid finish
	// from bridging across the rejected row and minting a false long span.
	return true
}

func traceDBCallstackRawIdentityPresent(value any) bool {
	if value == nil {
		return false
	}
	switch typed := value.(type) {
	case int64:
		return typed != 0
	case string:
		trimmed := strings.TrimSpace(typed)
		return trimmed != "" && trimmed != "0"
	default:
		return true
	}
}

func traceDBCallstackCookie(value any) (string, bool, bool) {
	if value == nil {
		return "", false, true
	}
	switch typed := value.(type) {
	case int64:
		if typed == 0 {
			return "", false, true
		}
		return strconv.FormatInt(typed, 10), true, true
	case string:
		if typed == "" || typed == "0" {
			return "", false, true
		}
		if !traceDBCallstackMarkerToken(typed) {
			return "", false, false
		}
		return typed, true, true
	default:
		return "", false, false
	}
}

func traceDBCallstackMarkerToken(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || len(value) > maxTraceDBCallstackTokenBytes || strings.ContainsRune(value, '|') || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func sortedTraceDBCallstackLaneIDs(lanes map[int64][]traceDBCallstackRow) []int64 {
	ids := make([]int64, 0, len(lanes))
	for id := range lanes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func auditTraceDBCallstackSyncLane(rows []traceDBCallstackRow) string {
	if len(rows) < 2 {
		return ""
	}
	ordered := append([]traceDBCallstackRow(nil), rows...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].TS != ordered[j].TS {
			return ordered[i].TS < ordered[j].TS
		}
		if ordered[i].End != ordered[j].End {
			return ordered[i].End > ordered[j].End
		}
		if ordered[i].DepthKnown && ordered[j].DepthKnown && ordered[i].Depth != ordered[j].Depth {
			return ordered[i].Depth < ordered[j].Depth
		}
		return ordered[i].ID < ordered[j].ID
	})
	seenIntervals := make(map[[2]int64]traceDBCallstackRow, len(ordered))
	for _, row := range ordered {
		key := [2]int64{row.TS, row.End}
		if previous, exists := seenIntervals[key]; exists &&
			(!row.DepthKnown || !previous.DepthKnown || row.Depth == previous.Depth) {
			return "ambiguous_identical_interval"
		}
		seenIntervals[key] = row
	}
	stack := make([]traceDBCallstackRow, 0, len(ordered))
	for _, row := range ordered {
		for len(stack) > 0 && row.TS >= stack[len(stack)-1].End {
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 {
			parent := stack[len(stack)-1]
			if row.TS == parent.TS && row.End == parent.End &&
				(!row.DepthKnown || !parent.DepthKnown || row.Depth == parent.Depth) {
				return "ambiguous_identical_interval"
			}
			if row.End > parent.End {
				return "crossing_sync_intervals"
			}
			if row.DepthKnown && parent.DepthKnown && row.Depth <= parent.Depth {
				return "non_increasing_sync_depth"
			}
		}
		stack = append(stack, row)
	}
	return ""
}

func traceDBCallstackSyncEndpoints(rows []traceDBCallstackRow) []traceDBCallstackEndpoint {
	endpoints := make([]traceDBCallstackEndpoint, 0, len(rows)*2)
	for _, row := range rows {
		endpoints = append(endpoints,
			traceDBCallstackEndpoint{Row: row, TS: row.TS, CPU: row.StartCPU, Begin: true},
			traceDBCallstackEndpoint{Row: row, TS: row.End, CPU: row.EndCPU, Begin: false},
		)
	}
	sort.SliceStable(endpoints, func(i, j int) bool {
		left, right := endpoints[i], endpoints[j]
		if left.TS != right.TS {
			return left.TS < right.TS
		}
		leftPhase := traceDBCallstackEndpointPhase(left)
		rightPhase := traceDBCallstackEndpointPhase(right)
		if leftPhase != rightPhase {
			return leftPhase < rightPhase
		}
		if left.Begin && right.Begin {
			if left.Row.DepthKnown && right.Row.DepthKnown && left.Row.Depth != right.Row.Depth {
				return left.Row.Depth < right.Row.Depth
			}
			if left.Row.End != right.Row.End {
				return left.Row.End > right.Row.End
			}
			return left.Row.ID < right.Row.ID
		}
		if !left.Begin && !right.Begin {
			if left.Row.DepthKnown && right.Row.DepthKnown && left.Row.Depth != right.Row.Depth {
				return left.Row.Depth > right.Row.Depth
			}
			if left.Row.TS != right.Row.TS {
				return left.Row.TS > right.Row.TS
			}
			return left.Row.ID > right.Row.ID
		}
		return left.Row.ID < right.Row.ID
	})
	return endpoints
}

func traceDBCallstackEndpointPhase(endpoint traceDBCallstackEndpoint) int {
	if !endpoint.Begin && endpoint.Row.Dur > 0 {
		return 0
	}
	if endpoint.Begin {
		return 1
	}
	return 2
}

func auditTraceDBCallstackAsyncGroup(rows []traceDBCallstackRow) string {
	open := false
	for _, row := range rows {
		switch row.Flag {
		case "S":
			if open {
				return "ambiguous_async_cohort"
			}
			open = true
		case "C":
			if !open {
				return "unpaired_async_finish"
			}
			open = false
		}
	}
	if open {
		return "unpaired_async_start"
	}
	return ""
}

func traceDBCallstackSkipSummary(skipped map[string]int) string {
	if len(skipped) == 0 {
		return ""
	}
	total := skipped["family_fail_closed"]
	if total == 0 {
		for reason, count := range skipped {
			if reason != "family_fail_closed" {
				total += count
			}
		}
	}
	return fmt.Sprintf("%d callstack row(s) suppressed: %s", total, traceDBCountSummary(skipped))
}
