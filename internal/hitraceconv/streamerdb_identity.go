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
		ByTID:              map[int64]traceDBThread{},
		ByTIDIncarnation:   map[int64][]traceDBThread{},
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
	sort.Strings(coverage.ColumnsPresent)
	coverage.FieldSources = map[string]string{
		"canonical_ipid": "process.ipid; strict SQLite INTEGER in 0..UINT32_MAX-1",
		"process_name":   "bounded single-physical-line display metadata; never part of identity conflict keys",
		"public_pid":     "process.pid; strict SQLite INTEGER in 0..MaxInt32",
		"source_profile": map[bool]string{true: "full current profile: process.id must equal process.ipid", false: "id-less compatibility profile: process.ipid is its own source identity"}[hasID],
	}
	idExpr := "NULL"
	if hasID {
		idExpr = quoteSQLiteIdent("id")
	}
	nameExpr := "NULL"
	if hasName {
		nameExpr = quoteSQLiteIdent("name")
	}
	rows, err := tdb.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT %s, ipid, pid, %s
		FROM process
	`, idExpr, nameExpr))
	if err != nil {
		coverage.Error = err.Error()
		return err
	}
	defer rows.Close()

	scannedRows := 0
	candidateRows := map[int64]int{}
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
		var sourceRaw, ipidRaw, pidRaw, nameRaw any
		if err := rows.Scan(&sourceRaw, &ipidRaw, &pidRaw, &nameRaw); err != nil {
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
		item := traceDBProcess{IPID: ipid, PID: pid, Name: traceDBIdentityDisplayText(nameRaw)}
		if existing, exists := index.Processes[ipid]; exists {
			if existing.PID != item.PID {
				poison(sourceID, true, ipid, true)
				continue
			}
			existing.Name = traceDBPreferDisplayName(existing.Name, item.Name)
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
	if rejected > 0 {
		coverage.Skipped = fmt.Sprintf("%d process row(s) rejected: strict scalar/identity conflict or current id/ipid alias divergence; ambiguous_ipid=%d ambiguous_process_id=%d",
			rejected, len(index.AmbiguousIPID), len(index.AmbiguousProcessID))
	}
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
	sort.Strings(coverage.ColumnsPresent)
	coverage.FieldSources = map[string]string{
		"canonical_itid": "thread.itid; strict SQLite INTEGER in 0..UINT32_MAX-1",
		"owner_ipid":     map[bool]string{true: "thread.ipid -> process.id -> canonical process.ipid after full-profile alias audit", false: "id-less compatibility profile: thread.ipid is canonical"}[index.HasProcessIDColumn],
		"public_tid":     "thread.tid; strict SQLite INTEGER in 0..MaxInt32",
		"source_profile": map[bool]string{true: "full current profile: thread.id must equal thread.itid", false: "id-less compatibility profile: thread.itid is its own source identity"}[hasID],
		"thread_name":    "display-only string metadata; never part of identity conflict keys",
	}
	idExpr := "NULL"
	if hasID {
		idExpr = quoteSQLiteIdent("id")
	}
	nameExpr := "NULL"
	if hasName {
		nameExpr = quoteSQLiteIdent("name")
	}
	rows, err := tdb.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT %s, itid, tid, ipid, %s, start_ts, is_main_thread, switch_count
		FROM thread
	`, idExpr, nameExpr))
	if err != nil {
		coverage.Error = err.Error()
		return err
	}
	defer rows.Close()

	scannedRows := 0
	candidateRows := map[int64]int{}
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
		var sourceRaw, itidRaw, tidRaw, ownerRaw, nameRaw, startRaw, mainRaw, switchRaw any
		if err := rows.Scan(&sourceRaw, &itidRaw, &tidRaw, &ownerRaw, &nameRaw, &startRaw, &mainRaw, &switchRaw); err != nil {
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
		startTS, startOK := traceDBStrictSQLiteInt(startRaw)
		mainFlag, mainOK := traceDBStrictSQLiteBool(mainRaw)
		switchCount, switchOK := traceDBStrictSQLiteInt(switchRaw)
		if !sourceOK || !itidOK || !tidOK || !ownerOK || !startOK || startTS < 0 || !mainOK || !switchOK || switchCount < 0 || switchCount > math.MaxUint32 ||
			(hasID && sourceID != itid) {
			poison(sourceID, sourceOK, itid, itidOK)
			continue
		}
		candidateRows[itid]++
		if index.AmbiguousThreadID[sourceID] || index.AmbiguousITID[itid] {
			continue
		}
		item := traceDBThread{
			ITID: itid, TID: tid, IPID: ownerIPID, Name: traceDBIdentityDisplayText(nameRaw),
			StartTS: startTS, IsMainThread: mainFlag, SwitchCount: switchCount,
		}
		if existing, exists := index.ByITID[itid]; exists {
			if existing.TID != item.TID || existing.IPID != item.IPID || existing.StartTS != item.StartTS {
				poison(sourceID, true, itid, true)
				continue
			}
			existing.Name = traceDBPreferDisplayName(existing.Name, item.Name)
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
	if rejected > 0 {
		coverage.Skipped = fmt.Sprintf("%d thread row(s) rejected: strict scalar/identity conflict, unresolved owner, or current id/itid alias divergence; ambiguous_itid=%d ambiguous_thread_id=%d",
			rejected, len(index.AmbiguousITID), len(index.AmbiguousThreadID))
	}
	return nil
}

func buildTraceDBThreadSecondaryIndexes(index *traceDBThreadIndex) {
	index.ByTID = map[int64]traceDBThread{}
	index.ByTIDIncarnation = map[int64][]traceDBThread{}
	index.ByProcess = map[int64][]traceDBThread{}
	for _, item := range index.ByITID {
		index.ByTIDIncarnation[item.TID] = append(index.ByTIDIncarnation[item.TID], item)
		index.ByProcess[item.IPID] = append(index.ByProcess[item.IPID], item)
	}
	for tid, items := range index.ByTIDIncarnation {
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].StartTS != items[j].StartTS {
				return items[i].StartTS < items[j].StartTS
			}
			return items[i].ITID < items[j].ITID
		})
		index.ByTIDIncarnation[tid] = items
		index.ByTID[tid] = items[len(items)-1]
	}
	for ipid, items := range index.ByProcess {
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].StartTS != items[j].StartTS {
				return items[i].StartTS < items[j].StartTS
			}
			return items[i].ITID < items[j].ITID
		})
		index.ByProcess[ipid] = items
	}
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
