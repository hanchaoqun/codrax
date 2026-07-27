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

func newTraceDBThreadIndex(traceStart int64, traceStartKnown bool) traceDBThreadIndex {
	return traceDBThreadIndex{
		ByITID:             map[int64]traceDBThread{},
		AmbiguousITID:      map[int64]bool{},
		ThreadIDToITID:     map[int64]int64{},
		AmbiguousThreadID:  map[int64]bool{},
		ByTIDCandidates:    map[int64][]traceDBThread{},
		ObservedPublicTID:  map[int64]bool{},
		RejectedPublicTID:  map[int64]bool{},
		Processes:          map[int64]traceDBProcess{},
		AmbiguousIPID:      map[int64]bool{},
		ProcessIDToIPID:    map[int64]int64{},
		AmbiguousProcessID: map[int64]bool{},
		ByProcess:          map[int64][]traceDBThread{},
		TraceStart:         traceStart,
		TraceStartKnown:    traceStartKnown,
	}
}

// traceDBCanonicalIdleIdentityExact is the single identity authority for the
// scheduler's synthetic ITID/IPID zero subject. Missing materialized zero rows
// are compatible with the producer profile, but an ambiguous or conflicting
// materialization must never acquire swapper semantics.
func traceDBCanonicalIdleIdentityExact(index traceDBThreadIndex) bool {
	if index.AmbiguousITID[0] || index.AmbiguousIPID[0] {
		return false
	}
	if materialized, ok := index.ByITID[0]; ok &&
		(materialized.ITID != 0 || materialized.TID != 0 || materialized.IPID != 0) {
		return false
	}
	if materialized, ok := index.Processes[0]; ok &&
		(materialized.IPID != 0 || materialized.PID != 0) {
		return false
	}
	return true
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
	if index.ObservedPublicTID == nil {
		index.ObservedPublicTID = map[int64]bool{}
	}
	if index.RejectedPublicTID == nil {
		index.RejectedPublicTID = map[int64]bool{}
	}
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
	ownerWireSource := "id-less thread compatibility profile: thread.ipid is canonical internal uint32 in 0..UINT32_MAX-1"
	if hasID {
		ownerWireSource = "current thread.ipid strict signed-int32 -> uint32 projection (-1 sentinel rejected)"
	}
	ownerJoinSource := "; exact canonical process owner required"
	if index.HasProcessIDColumn {
		ownerJoinSource = "; process.id -> canonical process.ipid after full-profile alias audit"
	}
	coverage.FieldSources = map[string]string{
		"canonical_itid":    "thread.itid; strict SQLite INTEGER in 0..UINT32_MAX-1",
		"owner_ipid":        ownerWireSource + ownerJoinSource,
		"public_tid":        "thread.tid; strict SQLite INTEGER in 0..MaxInt32",
		"public_tid_roster": "all strict physical thread.tid values retained separately from the rejected-row subset; audit evidence only, never an alternate resolver",
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
	candidateTIDsByITID := map[int64]map[int64]bool{}
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
		if tidOK {
			index.ObservedPublicTID[tid] = true
		}
		ownerIPID, ownerOK := traceDBResolveThreadOwner(index, ownerRaw)
		mainFlag, mainOK := traceDBStrictSQLiteBool(mainRaw)
		switchCount, switchOK := traceDBStrictSQLiteInt(switchRaw)
		if !sourceOK || !itidOK || !tidOK || !ownerOK || !mainOK || !switchOK || switchCount < 0 || switchCount > math.MaxUint32 ||
			(hasID && sourceID != itid) {
			if tidOK {
				index.RejectedPublicTID[tid] = true
			}
			poison(sourceID, sourceOK, itid, itidOK)
			continue
		}
		if candidateTIDsByITID[itid] == nil {
			candidateTIDsByITID[itid] = map[int64]bool{}
		}
		candidateTIDsByITID[itid][tid] = true
		candidateRows[itid]++
		if index.AmbiguousThreadID[sourceID] || index.AmbiguousITID[itid] {
			index.RejectedPublicTID[tid] = true
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
				index.RejectedPublicTID[existing.TID] = true
				index.RejectedPublicTID[item.TID] = true
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
			index.RejectedPublicTID[tid] = true
			if prior, ok := index.ByITID[existing]; ok {
				index.RejectedPublicTID[prior.TID] = true
			}
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
		for tid := range candidateTIDsByITID[itid] {
			index.RejectedPublicTID[tid] = true
		}
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
	coverage.Metrics = map[string]int64{
		"thread_rows_scanned":  int64(scannedRows),
		"thread_rows_accepted": int64(acceptedRows),
		"thread_rows_rejected": int64(rejected),
	}
	unnamedThreads := 0
	unresolvedThreadNames := 0
	recoveredMainProcessNames := 0
	recoveredUniquePublicTIDNames := 0
	multiITIDPublicTIDs := 0
	multiOwnerPublicTIDs := 0
	for _, item := range index.ByITID {
		if strings.TrimSpace(item.Name) == "" {
			unnamedThreads++
			_, source := traceDBThreadDisplayName(*index, item)
			switch source {
			case traceDBDisplayNameMainProcess:
				recoveredMainProcessNames++
			case traceDBDisplayNameUniquePublicTID:
				recoveredUniquePublicTIDNames++
			default:
				unresolvedThreadNames++
			}
		}
	}
	for _, candidates := range index.ByTIDCandidates {
		if len(candidates) <= 1 {
			continue
		}
		multiITIDPublicTIDs++
		owners := make(map[int64]struct{}, len(candidates))
		for _, candidate := range candidates {
			owners[candidate.IPID] = struct{}{}
		}
		if len(owners) > 1 {
			multiOwnerPublicTIDs++
		}
	}
	if unnamedThreads > 0 {
		coverage.Metrics["unnamed_threads"] = int64(unnamedThreads)
	}
	if unresolvedThreadNames > 0 {
		coverage.Metrics["unresolved_thread_names"] = int64(unresolvedThreadNames)
	}
	if recoveredMainProcessNames > 0 {
		coverage.Metrics["thread_names_recovered_main_process"] = int64(recoveredMainProcessNames)
	}
	if recoveredUniquePublicTIDNames > 0 {
		coverage.Metrics["thread_names_recovered_unique_public_tid"] = int64(recoveredUniquePublicTIDNames)
	}
	if multiITIDPublicTIDs > 0 {
		coverage.Metrics["public_tids_with_multiple_itids"] = int64(multiITIDPublicTIDs)
	}
	if multiOwnerPublicTIDs > 0 {
		coverage.Metrics["public_tids_with_multiple_owner_ipids"] = int64(multiOwnerPublicTIDs)
	}
	var notes []string
	if rejected > 0 {
		notes = append(notes, fmt.Sprintf("%d thread row(s) rejected: strict scalar/identity conflict, unresolved owner, or current id/itid alias divergence; ambiguous_itid=%d ambiguous_thread_id=%d",
			rejected, len(index.AmbiguousITID), len(index.AmbiguousThreadID)))
	}
	if len(index.RejectedPublicTID) > 0 {
		notes = append(notes, fmt.Sprintf("rejected_public_tid_lanes=%d retained_for_absence_vs_rejection_audit", len(index.RejectedPublicTID)))
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
	index.DisplayNameByITID = map[int64]string{}
	index.DisplayNameSourceByITID = map[int64]string{}
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
	traceDBBuildDisplayNameIndex(index)
}

const (
	traceDBDisplayNameDirect          = "thread_name"
	traceDBDisplayNameMainProcess     = "main_process_name"
	traceDBDisplayNameUniquePublicTID = "unique_same_public_tid_name"
)

func traceDBBuildDisplayNameIndex(index *traceDBThreadIndex) {
	if index == nil {
		return
	}
	uniqueByTID := make(map[int64]string, len(index.ByTIDCandidates))
	for tid, candidates := range index.ByTIDCandidates {
		var unique string
		ambiguous := false
		for _, candidate := range candidates {
			name := strings.TrimSpace(candidate.Name)
			if name == "" || !traceDBSinglePhysicalLine(name, true) {
				continue
			}
			if unique == "" {
				unique = name
				continue
			}
			if unique != name {
				ambiguous = true
				break
			}
		}
		if !ambiguous && unique != "" {
			uniqueByTID[tid] = unique
		}
	}
	for itid, thread := range index.ByITID {
		if direct := strings.TrimSpace(thread.Name); direct != "" && traceDBSinglePhysicalLine(direct, true) {
			index.DisplayNameByITID[itid] = direct
			index.DisplayNameSourceByITID[itid] = traceDBDisplayNameDirect
			continue
		}
		process, processOK := index.Processes[thread.IPID]
		isMain := thread.IsMainThread || processOK && process.PID > 0 && thread.TID == process.PID
		if processName := strings.TrimSpace(process.Name); isMain && processName != "" &&
			traceDBSinglePhysicalLine(processName, true) {
			index.DisplayNameByITID[itid] = processName
			index.DisplayNameSourceByITID[itid] = traceDBDisplayNameMainProcess
			continue
		}
		if unique := uniqueByTID[thread.TID]; unique != "" {
			index.DisplayNameByITID[itid] = unique
			index.DisplayNameSourceByITID[itid] = traceDBDisplayNameUniquePublicTID
		}
	}
}

func traceDBThreadDisplayName(index traceDBThreadIndex, thread traceDBThread) (string, string) {
	if name := strings.TrimSpace(index.DisplayNameByITID[thread.ITID]); name != "" {
		return name, index.DisplayNameSourceByITID[thread.ITID]
	}
	if name := strings.TrimSpace(thread.Name); name != "" {
		if traceDBSinglePhysicalLine(name, true) {
			return name, traceDBDisplayNameDirect
		}
		return thread.Name, traceDBDisplayNameDirect
	}
	return "", ""
}

func traceDBThreadDisplayNameValue(index traceDBThreadIndex, thread traceDBThread) string {
	name, _ := traceDBThreadDisplayName(index, thread)
	return name
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

func traceDBRegistrationTimestamp(metadata traceDBTimestampMetadata, traceStart int64, traceStartKnown bool) (int64, bool) {
	if metadata.Known && !metadata.Tainted {
		return metadata.Value, true
	}
	if traceStartKnown {
		return traceStart, true
	}
	return 0, false
}

func traceDBBeforeCaptureStart(index traceDBThreadIndex, timestamp int64) bool {
	return index.TraceStartKnown && timestamp < index.TraceStart
}

func traceDBStrictInternalID(value any) (int64, bool) {
	id, ok := traceDBStrictSQLiteInt(value)
	return id, ok && id >= 0 && id <= maxTraceDBInternalID
}

// traceDBStrictSignedInt32InternalID decodes internal uint32 identities that
// an audited producer projects through signed int32. -1 is INVALID_UINT32;
// positive high-half uint32 values require a different explicit profile.
func traceDBStrictSignedInt32InternalID(value any) (int64, bool) {
	raw, ok := traceDBStrictSQLiteInt(value)
	if !ok || raw < math.MinInt32 || raw > math.MaxInt32 || raw == -1 {
		return 0, false
	}
	if raw < 0 {
		return raw + (int64(1) << 32), true
	}
	return raw, true
}

func traceDBResolveThreadOwner(index *traceDBThreadIndex, value any) (int64, bool) {
	sourceID, ok := traceDBStrictInternalID(value)
	if index.HasThreadIDColumn {
		sourceID, ok = traceDBStrictSignedInt32InternalID(value)
	}
	if !ok {
		return 0, false
	}
	ownerIPID := sourceID
	if index.HasProcessIDColumn {
		if index.AmbiguousProcessID[sourceID] {
			return 0, false
		}
		var mapped bool
		ownerIPID, mapped = index.ProcessIDToIPID[sourceID]
		if !mapped {
			return 0, false
		}
	}
	if index.AmbiguousIPID[ownerIPID] {
		return 0, false
	}
	process, exists := index.Processes[ownerIPID]
	return ownerIPID, exists && process.IPID == ownerIPID
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
