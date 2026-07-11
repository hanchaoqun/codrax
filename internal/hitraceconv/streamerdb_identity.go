package hitraceconv

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
)

const maxTraceDBIdentityDisplayBytes = 4096

// The full Trace Streamer profile exposes two names for each internal row
// identity (process.id/ipid and thread.id/itid). Upstream currently derives
// both aliases from CurrentRow. Other consumers join through different alias
// names, so equality is a profile invariant rather than permission to guess a
// translation when they diverge. The id-less compatibility profile uses the
// canonical internal ID as its own source identity.

func newTraceDBThreadIndex(traceStart int64) traceDBThreadIndex {
	return traceDBThreadIndex{
		ByITID:             map[int64]traceDBThread{},
		AmbiguousITID:      map[int64]bool{},
		ThreadIDToITID:     map[int64]int64{},
		AmbiguousThreadID:  map[int64]bool{},
		ByTIDCandidates:    map[int64][]traceDBThread{},
		Processes:          map[int64]traceDBProcess{},
		AmbiguousIPID:      map[int64]bool{},
		ProcessIDToIPID:    map[int64]int64{},
		AmbiguousProcessID: map[int64]bool{},
		ByProcess:          map[int64][]traceDBThread{},
		TraceStart:         traceStart,
	}
}

func (tdb *traceDB) loadStrictProcessIndex(ctx context.Context, index *traceDBThreadIndex, coverage *TraceDBCoverage) error {
	hasID := index.HasProcessIDColumn
	hasName, err := tdb.columnExists(ctx, "process", "name")
	if err != nil {
		coverage.Error = err.Error()
		return err
	}
	if hasID {
		coverage.ColumnsPresent = appendTraceDBCoverageColumn(coverage.ColumnsPresent, "id")
	}
	if hasName {
		coverage.ColumnsPresent = appendTraceDBCoverageColumn(coverage.ColumnsPresent, "name")
	}
	hasStart, err := tdb.columnExists(ctx, "process", "start_ts")
	if err != nil {
		coverage.Error = err.Error()
		return err
	}
	if hasStart {
		coverage.ColumnsPresent = appendTraceDBCoverageColumn(coverage.ColumnsPresent, "start_ts")
	}
	sort.Strings(coverage.ColumnsPresent)
	coverage.FieldSources = map[string]string{
		"canonical_ipid":    "process.ipid; strict SQLite INTEGER in 0..UINT32_MAX-1",
		"process_name":      "bounded single-physical-line display metadata; never part of identity conflict keys",
		"public_pid":        "process.pid; strict SQLite INTEGER in 0..MaxInt32",
		"registration_hint": "optional process.start_ts tri-state metadata only; never process birth or generation authority",
		"source_profile":    map[bool]string{true: "full current profile: process.id must equal process.ipid", false: "id-less compatibility profile: process.ipid is its own source identity"}[hasID],
	}
	idExpr := "NULL"
	if hasID {
		idExpr = quoteSQLiteIdent("id")
	}
	nameExpr := "NULL"
	if hasName {
		nameExpr = quoteSQLiteIdent("name")
	}
	startExpr := "NULL"
	if hasStart {
		startExpr = quoteSQLiteIdent("start_ts")
	}
	rows, err := tdb.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT %s, ipid, pid, %s, %s
		FROM process
	`, idExpr, nameExpr, startExpr))
	if err != nil {
		coverage.Error = err.Error()
		return err
	}
	defer rows.Close()

	scannedRows := 0
	candidateRows := map[int64]int{}
	metadataTaintedRows := 0
	poison := func(sourceID int64, sourceOK bool, ipid int64, ipidOK bool) {
		// Trusted full-profile and compatibility mappings are identity aliases.
		// A malformed row that connects two IDs poisons both namespaces on both
		// ends; otherwise a previously accepted canonical/source sibling could
		// survive depending on row order.
		poisonID := func(id int64) {
			index.AmbiguousProcessID[id] = true
			index.AmbiguousIPID[id] = true
			delete(index.ProcessIDToIPID, id)
			delete(index.Processes, id)
		}
		if sourceOK {
			poisonID(sourceID)
		}
		if ipidOK && (!sourceOK || ipid != sourceID) {
			poisonID(ipid)
		}
	}
	for rows.Next() {
		scannedRows++
		var sourceRaw, ipidRaw, pidRaw, nameRaw, startRaw any
		if err := rows.Scan(&sourceRaw, &ipidRaw, &pidRaw, &nameRaw, &startRaw); err != nil {
			coverage.Error = err.Error()
			return err
		}
		ipid, ipidOK := traceDBStrictInternalID(ipidRaw)
		sourceID, sourceOK := ipid, ipidOK
		if hasID {
			sourceID, sourceOK = traceDBStrictInternalID(sourceRaw)
		}
		pid, pidOK := traceDBStrictPublicID(pidRaw)
		if !sourceOK || !ipidOK || !pidOK || (hasID && sourceID != ipid) {
			poison(sourceID, sourceOK, ipid, ipidOK)
			continue
		}
		candidateRows[ipid]++
		if index.AmbiguousProcessID[sourceID] || index.AmbiguousIPID[ipid] {
			continue
		}
		registration := traceDBTimestampMetadataFrom(startRaw)
		if registration.Tainted {
			metadataTaintedRows++
		}
		item := traceDBProcess{IPID: ipid, PID: pid, Name: traceDBIdentityDisplayText(nameRaw), RegistrationHint: registration}
		if existing, exists := index.Processes[ipid]; exists {
			if existing.PID != item.PID {
				poison(sourceID, true, ipid, true)
				continue
			}
			existing.Name = traceDBPreferDisplayName(existing.Name, item.Name)
			existing.RegistrationHint = traceDBMergeTimestampMetadata(existing.RegistrationHint, item.RegistrationHint)
			index.Processes[ipid] = existing
		} else {
			index.Processes[ipid] = item
		}
		if existing, exists := index.ProcessIDToIPID[sourceID]; exists && existing != ipid {
			poison(sourceID, true, ipid, true)
			continue
		}
		index.ProcessIDToIPID[sourceID] = ipid
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return err
	}
	for sourceID, ipid := range index.ProcessIDToIPID {
		if index.AmbiguousProcessID[sourceID] || index.AmbiguousIPID[ipid] {
			delete(index.ProcessIDToIPID, sourceID)
		}
	}
	for ipid := range index.AmbiguousIPID {
		delete(index.Processes, ipid)
	}
	acceptedRows := 0
	for ipid, count := range candidateRows {
		if !index.AmbiguousIPID[ipid] {
			if _, exists := index.Processes[ipid]; exists {
				acceptedRows += count
			}
		}
	}
	rejected := scannedRows - acceptedRows
	coverage.RowsEmitted = len(index.Processes)
	var notes []string
	if rejected > 0 {
		notes = append(notes, fmt.Sprintf("%d process row(s) rejected: strict scalar/identity conflict or current id/ipid alias divergence; ambiguous_ipid=%d ambiguous_process_id=%d",
			rejected, len(index.AmbiguousIPID), len(index.AmbiguousProcessID)))
	}
	metadataTaintedCohorts := 0
	for _, item := range index.Processes {
		if item.RegistrationHint.Tainted {
			metadataTaintedCohorts++
		}
	}
	if metadataTaintedRows > 0 || metadataTaintedCohorts > 0 {
		notes = append(notes, fmt.Sprintf("registration metadata ignored for hard identity: tainted_rows=%d tainted_cohorts=%d", metadataTaintedRows, metadataTaintedCohorts))
	}
	coverage.Skipped = strings.Join(notes, "; ")
	return nil
}

func (tdb *traceDB) loadStrictThreadIndex(ctx context.Context, index *traceDBThreadIndex, coverage *TraceDBCoverage) error {
	hasID := index.HasThreadIDColumn
	hasName, err := tdb.columnExists(ctx, "thread", "name")
	if err != nil {
		coverage.Error = err.Error()
		return err
	}
	if hasID {
		coverage.ColumnsPresent = appendTraceDBCoverageColumn(coverage.ColumnsPresent, "id")
	}
	if hasName {
		coverage.ColumnsPresent = appendTraceDBCoverageColumn(coverage.ColumnsPresent, "name")
	}
	hasStart, err := tdb.columnExists(ctx, "thread", "start_ts")
	if err != nil {
		coverage.Error = err.Error()
		return err
	}
	hasEnd, err := tdb.columnExists(ctx, "thread", "end_ts")
	if err != nil {
		coverage.Error = err.Error()
		return err
	}
	if hasStart {
		coverage.ColumnsPresent = appendTraceDBCoverageColumn(coverage.ColumnsPresent, "start_ts")
	}
	if hasEnd {
		coverage.ColumnsPresent = appendTraceDBCoverageColumn(coverage.ColumnsPresent, "end_ts")
	}
	sort.Strings(coverage.ColumnsPresent)
	coverage.FieldSources = map[string]string{
		"canonical_itid":    "thread.itid; strict SQLite INTEGER in 0..UINT32_MAX-1",
		"owner_ipid":        map[bool]string{true: "thread.ipid -> process.id -> canonical process.ipid after full-profile alias audit", false: "id-less compatibility profile: thread.ipid is canonical"}[index.HasProcessIDColumn],
		"public_tid":        "thread.tid; strict SQLite INTEGER in 0..MaxInt32",
		"source_profile":    map[bool]string{true: "full current profile: thread.id must equal thread.itid", false: "id-less compatibility profile: thread.itid is its own source identity"}[hasID],
		"thread_name":       "display-only string metadata; never part of identity conflict keys",
		"registration_hint": "optional thread.start_ts tri-state metadata only; current producer normally exposes NULL and it never defines generation",
		"observed_end_hint": "optional thread.end_ts tri-state metadata only; exit/free/reuse may overwrite it and it never defines generation alone",
	}
	idExpr := "NULL"
	if hasID {
		idExpr = quoteSQLiteIdent("id")
	}
	nameExpr := "NULL"
	if hasName {
		nameExpr = quoteSQLiteIdent("name")
	}
	startExpr := "NULL"
	if hasStart {
		startExpr = quoteSQLiteIdent("start_ts")
	}
	endExpr := "NULL"
	if hasEnd {
		endExpr = quoteSQLiteIdent("end_ts")
	}
	rows, err := tdb.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT %s, itid, tid, ipid, %s, %s, %s, is_main_thread, switch_count
		FROM thread
	`, idExpr, nameExpr, startExpr, endExpr))
	if err != nil {
		coverage.Error = err.Error()
		return err
	}
	defer rows.Close()

	scannedRows := 0
	candidateRows := map[int64]int{}
	metadataTaintedRows := 0
	poison := func(sourceID int64, sourceOK bool, itid int64, itidOK bool) {
		poisonID := func(id int64) {
			index.AmbiguousThreadID[id] = true
			index.AmbiguousITID[id] = true
			delete(index.ThreadIDToITID, id)
			delete(index.ByITID, id)
		}
		if sourceOK {
			poisonID(sourceID)
		}
		if itidOK && (!sourceOK || itid != sourceID) {
			poisonID(itid)
		}
	}
	for rows.Next() {
		scannedRows++
		var sourceRaw, itidRaw, tidRaw, ownerRaw, nameRaw, startRaw, endRaw, mainRaw, switchRaw any
		if err := rows.Scan(&sourceRaw, &itidRaw, &tidRaw, &ownerRaw, &nameRaw, &startRaw, &endRaw, &mainRaw, &switchRaw); err != nil {
			coverage.Error = err.Error()
			return err
		}
		itid, itidOK := traceDBStrictInternalID(itidRaw)
		sourceID, sourceOK := itid, itidOK
		if hasID {
			sourceID, sourceOK = traceDBStrictInternalID(sourceRaw)
		}
		tid, tidOK := traceDBStrictPublicID(tidRaw)
		ownerID, ownerOK := traceDBStrictInternalID(ownerRaw)
		ownerIPID := ownerID
		if ownerOK && index.HasProcessIDColumn {
			ownerIPID, ownerOK = index.ProcessIDToIPID[ownerID]
			if index.AmbiguousProcessID[ownerID] {
				ownerOK = false
			}
		}
		if ownerOK && index.AmbiguousIPID[ownerIPID] {
			ownerOK = false
		}
		mainFlag, mainOK := traceDBStrictSQLiteBool(mainRaw)
		switchCount, switchOK := traceDBStrictSQLiteInt(switchRaw)
		if !sourceOK || !itidOK || !tidOK || !ownerOK || !mainOK || !switchOK || switchCount < 0 || switchCount > math.MaxUint32 ||
			(hasID && sourceID != itid) {
			poison(sourceID, sourceOK, itid, itidOK)
			continue
		}
		candidateRows[itid]++
		if index.AmbiguousThreadID[sourceID] || index.AmbiguousITID[itid] {
			continue
		}
		registration := traceDBTimestampMetadataFrom(startRaw)
		observedEnd := traceDBTimestampMetadataFrom(endRaw)
		if registration.Tainted || observedEnd.Tainted {
			metadataTaintedRows++
		}
		item := traceDBThread{
			ITID: itid, TID: tid, IPID: ownerIPID, Name: traceDBIdentityDisplayText(nameRaw),
			RegistrationHint: registration, ObservedEndHint: observedEnd,
			IsMainThread: mainFlag, SwitchCount: switchCount,
		}
		if existing, exists := index.ByITID[itid]; exists {
			if existing.TID != item.TID || existing.IPID != item.IPID {
				poison(sourceID, true, itid, true)
				continue
			}
			existing.Name = traceDBPreferDisplayName(existing.Name, item.Name)
			existing.RegistrationHint = traceDBMergeTimestampMetadata(existing.RegistrationHint, item.RegistrationHint)
			existing.ObservedEndHint = traceDBMergeTimestampMetadata(existing.ObservedEndHint, item.ObservedEndHint)
			existing.IsMainThread = existing.IsMainThread || item.IsMainThread
			if item.SwitchCount > existing.SwitchCount {
				existing.SwitchCount = item.SwitchCount
			}
			index.ByITID[itid] = existing
		} else {
			index.ByITID[itid] = item
		}
		if existing, exists := index.ThreadIDToITID[sourceID]; exists && existing != itid {
			poison(sourceID, true, itid, true)
			continue
		}
		index.ThreadIDToITID[sourceID] = itid
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return err
	}
	for sourceID, itid := range index.ThreadIDToITID {
		if index.AmbiguousThreadID[sourceID] || index.AmbiguousITID[itid] {
			delete(index.ThreadIDToITID, sourceID)
		}
	}
	for itid := range index.AmbiguousITID {
		delete(index.ByITID, itid)
	}
	acceptedRows := 0
	for itid, count := range candidateRows {
		if !index.AmbiguousITID[itid] {
			if _, exists := index.ByITID[itid]; exists {
				acceptedRows += count
			}
		}
	}
	rejected := scannedRows - acceptedRows
	buildTraceDBThreadSecondaryIndexes(index)
	coverage.RowsEmitted = len(index.ByITID)
	var notes []string
	if rejected > 0 {
		notes = append(notes, fmt.Sprintf("%d thread row(s) rejected: strict scalar/identity conflict, unresolved owner, or current id/itid alias divergence; ambiguous_itid=%d ambiguous_thread_id=%d",
			rejected, len(index.AmbiguousITID), len(index.AmbiguousThreadID)))
	}
	metadataTaintedCohorts := 0
	for _, item := range index.ByITID {
		if item.RegistrationHint.Tainted || item.ObservedEndHint.Tainted {
			metadataTaintedCohorts++
		}
	}
	if metadataTaintedRows > 0 || metadataTaintedCohorts > 0 {
		notes = append(notes, fmt.Sprintf("lifetime metadata ignored for hard identity: tainted_rows=%d tainted_cohorts=%d", metadataTaintedRows, metadataTaintedCohorts))
	}
	coverage.Skipped = strings.Join(notes, "; ")
	return nil
}

func buildTraceDBThreadSecondaryIndexes(index *traceDBThreadIndex) {
	index.ByTIDCandidates = map[int64][]traceDBThread{}
	index.ByProcess = map[int64][]traceDBThread{}
	for _, item := range index.ByITID {
		index.ByTIDCandidates[item.TID] = append(index.ByTIDCandidates[item.TID], item)
		index.ByProcess[item.IPID] = append(index.ByProcess[item.IPID], item)
	}
	for tid, items := range index.ByTIDCandidates {
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].ITID < items[j].ITID
		})
		index.ByTIDCandidates[tid] = items
	}
	for ipid, items := range index.ByProcess {
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].ITID < items[j].ITID
		})
		index.ByProcess[ipid] = items
	}
}

func traceDBTimestampMetadataFrom(value any) traceDBTimestampMetadata {
	if value == nil {
		return traceDBTimestampMetadata{}
	}
	timestamp, ok := traceDBStrictSQLiteInt(value)
	if !ok || timestamp < 0 {
		return traceDBTimestampMetadata{Tainted: true}
	}
	return traceDBTimestampMetadata{Value: timestamp, Known: true}
}

func traceDBMergeTimestampMetadata(left, right traceDBTimestampMetadata) traceDBTimestampMetadata {
	if left.Tainted || right.Tainted || left.Known != right.Known || left.Known && left.Value != right.Value {
		return traceDBTimestampMetadata{Tainted: true}
	}
	if left.Known {
		return left
	}
	return traceDBTimestampMetadata{}
}

func traceDBRegistrationTimestamp(metadata traceDBTimestampMetadata, traceStart int64) int64 {
	if metadata.Known && !metadata.Tainted {
		return metadata.Value
	}
	return traceStart
}

func traceDBStrictInternalID(value any) (int64, bool) {
	id, ok := traceDBStrictSQLiteInt(value)
	return id, ok && id >= 0 && id <= maxTraceDBInternalID
}

func traceDBStrictPublicID(value any) (int64, bool) {
	id, ok := traceDBStrictSQLiteInt(value)
	return id, ok && id >= 0 && id <= math.MaxInt32
}

func traceDBStrictSQLiteBool(value any) (bool, bool) {
	n, ok := traceDBStrictSQLiteInt(value)
	if !ok || (n != 0 && n != 1) {
		return false, false
	}
	return n == 1, true
}

func traceDBIdentityDisplayText(value any) string {
	text, ok := value.(string)
	if !ok || len(text) > maxTraceDBIdentityDisplayBytes || !traceDBSinglePhysicalLine(text, true) {
		return ""
	}
	return text
}

func traceDBPreferDisplayName(left, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" {
		return right
	}
	if right == "" || left >= right {
		return left
	}
	return right
}

func traceDBResolveCallstackEmitterIdentity(index traceDBThreadIndex, hasITID, hasCallID bool, itidRaw, callIDRaw any) (int64, int64, string) {
	explicitPresent := hasITID && itidRaw != nil
	callIDPresent := hasCallID && callIDRaw != nil
	var explicitITID int64
	if explicitPresent {
		var ok bool
		explicitITID, ok = traceDBStrictInternalID(itidRaw)
		if !ok {
			return 0, 0, "invalid_emitter_itid"
		}
	}
	var callID, mappedITID int64
	if callIDPresent {
		var ok bool
		callID, ok = traceDBStrictInternalID(callIDRaw)
		if !ok {
			return 0, 0, "invalid_callid"
		}
		if index.AmbiguousThreadID[callID] {
			return 0, callID, "ambiguous_callid_thread"
		}
		mappedITID, ok = index.ThreadIDToITID[callID]
		if !ok || index.AmbiguousITID[mappedITID] {
			return 0, callID, "unresolved_callid_thread"
		}
	}
	switch {
	case explicitPresent && callIDPresent:
		if explicitITID != mappedITID {
			return 0, callID, "emitter_identity_mismatch"
		}
		return explicitITID, callID, ""
	case explicitPresent:
		return explicitITID, 0, ""
	case callIDPresent:
		return mappedITID, callID, ""
	default:
		return 0, 0, "missing_emitter_identity"
	}
}
