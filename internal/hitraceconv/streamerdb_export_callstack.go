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
func exportTraceDBCallstack(ctx context.Context, tdb *traceDB, sink *traceDBRowSink,
	authority traceDBSchedulerAuthority, running traceDBSchedulerRunningIndex, syncSpans *traceDBSyncSpanAuthority,
) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "slice", "callstack", []string{"ts", "dur", "name"})
	coverage.FieldSources = map[string]string{
		"cpu":              "same lifecycle authority filters the strict Running witness lane; every endpoint uses typed source/lifecycle/unknown lookup status",
		"emitter_identity": "row-level strict callstack.itid and callstack.callid->audited thread.id/itid profile; both must converge when present",
		"async_owner":      "exact non-ambiguous process.ipid generation and process.pid",
		"row_order":        "strict SQLite rowid; optional source id remains provenance only; typed endpoint phase ordering",
		"async_identity":   "nonzero cookie or chainId; both must agree when both are present",
		"lifecycle":        "same complete collector authority; sync positive spans require closed thread/process generation, zero spans and async endpoints require exact point admission",
		"sync_pairing":     "accepted sync rows and exact rejected-lane poison are handed to the single cross-producer typed B/E authority; this exporter never publishes sync endpoints directly",
		"async_generation": "each endpoint is admitted independently; exact rejected owner/name/cookie keys fail closed locally, unplaceable async rows fail the family; a paired task may migrate threads but cannot cross its positive owner-process generation",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	fail := func(err error) (TraceDBCoverage, error) {
		coverage.Error = err.Error()
		return coverage, err
	}
	if syncSpans == nil {
		return fail(&traceDBOutputInvariantError{Reason: "missing_sync_span_authority"})
	}

	hasITID, err := tdb.columnExists(ctx, "callstack", "itid")
	if err != nil {
		return fail(err)
	}
	hasID, err := tdb.columnExists(ctx, "callstack", "id")
	if err != nil {
		return fail(err)
	}
	hasDepth, err := tdb.columnExists(ctx, "callstack", "depth")
	if err != nil {
		return fail(err)
	}
	hasCallID, err := tdb.columnExists(ctx, "callstack", "callid")
	if err != nil {
		return fail(err)
	}
	if !hasITID && !hasCallID {
		coverage.ColumnsMissing = append(coverage.ColumnsMissing, "itid|callid")
		coverage.Skipped = "missing required emitter identity column: itid|callid"
		return coverage, nil
	}
	hasFlag, err := tdb.columnExists(ctx, "callstack", "flag")
	if err != nil {
		return fail(err)
	}
	hasCookie, err := tdb.columnExists(ctx, "callstack", "cookie")
	if err != nil {
		return fail(err)
	}
	hasChainID, err := tdb.columnExists(ctx, "callstack", "chainId")
	if err != nil {
		return fail(err)
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
		return fail(err)
	}
	defer rows.Close()

	skipped := map[string]int{}
	var accepted []traceDBCallstackRow
	asyncGlobalPoisoned := false
	asyncTaintedKeys := map[traceDBCallstackAsyncKey]bool{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		var rowIDRaw, sourceIDRaw, tsRaw, durRaw, nameRaw, callIDRaw, itidRaw, flagRaw, cookieRaw, chainIDRaw, depthRaw any
		if err := rows.Scan(&rowIDRaw, &sourceIDRaw, &tsRaw, &durRaw, &nameRaw, &callIDRaw, &itidRaw, &flagRaw, &cookieRaw, &chainIDRaw, &depthRaw); err != nil {
			return fail(err)
		}
		row, reason := prepareTraceDBCallstackRow(authority, running, hasID, hasITID, hasCallID, hasFlag, hasDepth,
			rowIDRaw, sourceIDRaw, tsRaw, durRaw, nameRaw, callIDRaw, itidRaw, flagRaw, cookieRaw, chainIDRaw, depthRaw)
		if reason != "" {
			if traceDBCallstackPotentialAsync(flagRaw, cookieRaw, chainIDRaw, hasFlag) {
				if key, exact := traceDBCallstackExactAsyncKey(row); exact {
					asyncTaintedKeys[key] = true
				} else {
					asyncGlobalPoisoned = true
				}
			}
			if traceDBCallstackPotentialSync(flagRaw, hasFlag) {
				for _, itid := range traceDBCallstackExactEmitterCandidates(authority, hasITID, hasCallID, itidRaw, callIDRaw) {
					thread, _, resolution := authority.resolveThreadSubject(itid)
					if resolution != traceDBSchedulerThreadResolved {
						return fail(&traceDBOutputInvariantError{Reason: "callstack_exact_lane_lost_identity"})
					}
					if err := syncSpans.poisonExactLane(traceDBSyncSpanLanePoison{
						Producer:           traceDBSyncSpanProducerCallstack,
						HeaderTID:          thread.TID,
						CanonicalITID:      itid,
						CanonicalITIDKnown: true,
						Reason:             traceDBSyncSpanLanePoisonRejectedCallstackCandidate,
					}); err != nil {
						return fail(err)
					}
				}
			}
			skipped[reason]++
			continue
		}
		accepted = append(accepted, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	var syncRows []traceDBCallstackRow
	asyncGroups := map[traceDBCallstackAsyncKey][]traceDBCallstackRow{}
	for _, row := range accepted {
		switch row.Flag {
		case "S", "C":
			key := traceDBCallstackAsyncKey{OwnerIPID: row.OwnerIPID, TGID: row.TGID, Name: row.Name, Cookie: row.Cookie}
			asyncGroups[key] = append(asyncGroups[key], row)
		default:
			syncRows = append(syncRows, row)
		}
	}

	for _, row := range syncRows {
		depthProvenance := traceDBSyncSpanDepthUnknown
		if row.DepthKnown {
			depthProvenance = traceDBSyncSpanDepthCallstack
		}
		if err := syncSpans.submit(traceDBSyncSpanCandidate{
			Producer:           traceDBSyncSpanProducerCallstack,
			StableKind:         traceDBSyncSpanStableCallstackRowID,
			StableID:           row.ID,
			HeaderTID:          row.TID,
			HeaderTGID:         row.TGID,
			CanonicalITID:      row.EmitterITID,
			CanonicalITIDKnown: true,
			OwnerIPID:          row.OwnerIPID,
			OwnerIPIDKnown:     true,
			Start:              row.TS,
			End:                row.End,
			StartCPU:           row.StartCPU,
			EndCPU:             row.EndCPU,
			StartCPUProvenance: traceDBSyncSpanCPUCallstackTypedRunning,
			EndCPUProvenance:   traceDBSyncSpanCPUCallstackTypedRunning,
			Task:               row.Task,
			Name:               row.Name,
			NameProvenance:     traceDBSyncSpanNameCallstack,
			Depth:              row.Depth,
			DepthKnown:         row.DepthKnown,
			DepthProvenance:    depthProvenance,
		}); err != nil {
			return fail(err)
		}
	}
	if asyncGlobalPoisoned {
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
		if asyncTaintedKeys[key] {
			skipped["async_key_fail_closed"] += len(group)
			continue
		}
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].TS != group[j].TS {
				return group[i].TS < group[j].TS
			}
			return group[i].ID < group[j].ID
		})
		if reason := auditTraceDBCallstackAsyncGroup(authority, group); reason != "" {
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
				return fail(err)
			}
			coverage.RowsEmitted++
		}
	}
	coverage.Skipped = traceDBCallstackSkipSummary(skipped)
	return coverage, nil
}

func prepareTraceDBCallstackRow(authority traceDBSchedulerAuthority, running traceDBSchedulerRunningIndex,
	hasID, hasITID, hasCallID, hasFlag, hasDepth bool,
	rowIDRaw, sourceIDRaw, tsRaw, durRaw, nameRaw, callIDRaw, itidRaw, flagRaw, cookieRaw, chainIDRaw, depthRaw any,
) (traceDBCallstackRow, string) {
	index := authority.identities
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
	var identityReason string
	row.EmitterITID, row.CallID, identityReason = traceDBResolveCallstackEmitterIdentity(index, hasITID, hasCallID, itidRaw, callIDRaw)
	if identityReason != "" {
		return row, identityReason
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
	thread, process, resolution := authority.resolveThreadSubject(row.EmitterITID)
	if resolution != traceDBSchedulerThreadResolved || thread.TID <= 0 || thread.TID > math.MaxInt32 {
		return row, "unresolved_emitter_thread"
	}
	if traceDBBeforeCaptureStart(index, row.TS) {
		return row, "before_capture_start"
	}
	row.OwnerIPID = thread.IPID
	row.TID = thread.TID
	if thread.IPID < 0 || process.PID <= 0 || process.PID > math.MaxInt32 {
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
	if row.Flag == "S" || row.Flag == "C" {
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
	}
	switch {
	case row.Flag == "S" || row.Flag == "C":
		if !authority.threadPointAllows(row.EmitterITID, row.TS) {
			return row, "lifecycle_rejected_async_endpoint"
		}
	case row.Dur == 0:
		if !authority.threadPointAllows(row.EmitterITID, row.TS) {
			return row, "lifecycle_rejected_sync_point"
		}
	case !authority.threadClosedEndpointAllows(row.EmitterITID, row.TS, row.End):
		return row, "lifecycle_rejected_sync_closed_interval"
	}
	var runningStatus traceDBSchedulerRunningLookupStatus
	row.StartCPU, runningStatus = running.lookupCPUAt(row.EmitterITID, row.TS)
	if runningStatus == traceDBSchedulerRunningSourceTainted {
		return row, "tainted_running_cpu_witness"
	}
	if runningStatus == traceDBSchedulerRunningLifecycleRejected {
		return row, "lifecycle_rejected_running_cpu_witness"
	}
	if runningStatus != traceDBSchedulerRunningKnown {
		return row, "unknown_start_cpu"
	}
	row.EndCPU = row.StartCPU
	if row.Flag == "" || row.Flag == "I" {
		row.EndCPU, runningStatus = running.lookupCPUAt(row.EmitterITID, row.End)
		if runningStatus == traceDBSchedulerRunningSourceTainted {
			return row, "tainted_running_cpu_witness"
		}
		if runningStatus == traceDBSchedulerRunningLifecycleRejected {
			return row, "lifecycle_rejected_running_cpu_witness"
		}
		if runningStatus != traceDBSchedulerRunningKnown {
			return row, "unknown_end_cpu"
		}
		return row, ""
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

func traceDBCallstackPotentialSync(flagValue any, hasFlag bool) bool {
	if !hasFlag {
		return true
	}
	flag, ok := flagValue.(string)
	if !ok {
		return true
	}
	return flag != "S" && flag != "C"
}

func traceDBCallstackExactAsyncKey(row traceDBCallstackRow) (traceDBCallstackAsyncKey, bool) {
	if (row.Flag != "S" && row.Flag != "C") || row.OwnerIPID < 0 || row.TGID <= 0 ||
		!traceDBCallstackMarkerToken(row.Name) || !traceDBCallstackMarkerToken(row.Cookie) {
		return traceDBCallstackAsyncKey{}, false
	}
	return traceDBCallstackAsyncKey{
		OwnerIPID: row.OwnerIPID,
		TGID:      row.TGID,
		Name:      row.Name,
		Cookie:    row.Cookie,
	}, true
}

func traceDBCallstackExactEmitterCandidates(authority traceDBSchedulerAuthority,
	hasITID, hasCallID bool, itidRaw, callIDRaw any,
) []int64 {
	resolution := traceDBResolveLifecycleCallstackIdentity(authority.identities, hasITID, hasCallID, itidRaw, callIDRaw)
	seen := map[int64]bool{}
	var out []int64
	for _, candidate := range resolution.Candidates {
		if candidate <= 0 || seen[candidate] {
			continue
		}
		_, process, status := authority.resolveThreadSubject(candidate)
		if status != traceDBSchedulerThreadResolved || process.PID <= 0 || process.PID > math.MaxInt32 {
			continue
		}
		seen[candidate] = true
		out = append(out, candidate)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
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

func auditTraceDBCallstackAsyncGroup(authority traceDBSchedulerAuthority, rows []traceDBCallstackRow) string {
	var open *traceDBCallstackRow
	for _, row := range rows {
		switch row.Flag {
		case "S":
			if open != nil {
				return "ambiguous_async_cohort"
			}
			candidate := row
			open = &candidate
		case "C":
			if open == nil {
				return "unpaired_async_finish"
			}
			if row.TS > open.TS && !authority.processClosedEndpointAllows(open.OwnerIPID, open.TS, row.TS) {
				return "lifecycle_rejected_async_process_interval"
			}
			open = nil
		}
	}
	if open != nil {
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
