package hitraceconv

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"math"
	"sort"
)

type traceDBBlockedComparableKey struct {
	TargetTID int64
	IOWait    int64
	Caller    string
}

type traceDBBlockedSubjectKey struct {
	TargetTID int64
	IOWait    int64
}

type traceDBRawBlockedRecoveryRow struct {
	Raw           traceDBRawBlockedRecord
	TargetThread  traceDBThread
	TargetProcess traceDBProcess
	HeaderThread  traceDBThread
	HeaderProcess traceDBProcess
}

func newTraceDBRawBlockedKeyCoverage() TraceDBCoverage {
	return TraceDBCoverage{
		Family: "source_rawtrace_blocked_key",
		Table:  "__raw_vs_db_blocked_key__",
		Role:   "diagnostic_deduplication",
		FieldSources: map[string]string{
			"raw":       "strict admitted source sched_blocked_reason payloads plus exact raw timestamp/CPU/common_pid envelope retained under the RPD target-row cap",
			"db":        "only blocked-reason rows which passed the existing DB exporter identity, lifecycle, argset and predecessor-CPU gates",
			"key":       "exact comparable content cohort=(target_tid,iowait,canonical caller symbol-or-raw-address); timestamp, CPU and delay are excluded because TraceStreamer does not retain the original blocked-event coordinates",
			"identity":  "a raw-only row is diagnostic-admitted only when both target_tid and common_pid resolve to exactly one canonical DB thread/process at the exact raw timestamp and pass the shared lifecycle point gate",
			"namespace": "public raw TIDs are never rewritten or guessed; no candidate, rejected candidate, or multiple lifecycle-valid host candidates fails closed",
			"effect":    "diagnostic cohort and identity accounting only; RowsEmitted is always zero and this ledger never adds or suppresses a systrace row",
		},
		Metadata: map[string]string{
			"ledger_state":          "unavailable",
			"publication_authority": "withheld_diagnostic_only",
		},
	}
}

func traceDBRawBlockedKeyCoverage(
	inventory *traceDBSourceNameInventory,
	dbRows []traceDBPreparedBlockedReason,
	authority traceDBSchedulerAuthority,
) TraceDBCoverage {
	coverage, _ := traceDBRawBlockedKeyLedger(inventory, dbRows, authority)
	return coverage
}

func traceDBRawBlockedKeyLedger(
	inventory *traceDBSourceNameInventory,
	dbRows []traceDBPreparedBlockedReason,
	authority traceDBSchedulerAuthority,
) (TraceDBCoverage, []traceDBRawBlockedRecoveryRow) {
	out := newTraceDBRawBlockedKeyCoverage()
	if inventory == nil {
		out.Skipped = "raw blocked key ledger unavailable: immutable source inventory absent"
		return out, nil
	}
	out.Found = inventory.RawDecode.Found
	if geometry := inventory.RawDecode.Metadata["blocked_reason_format_geometry_witnesses"]; geometry != "" {
		out.Metadata["blocked_reason_format_geometry_witnesses"] = geometry
	}
	if !traceDBRawDecodeFamilyComplete(
		inventory.RawDecode, traceDBRawRetentionBlocked) {
		out.Metadata["ledger_state"] = "withheld_raw_decode_incomplete"
		out.Skipped = "raw blocked key ledger withheld: strict raw decode ledger incomplete"
		return out, nil
	}
	rawRows := inventory.RawBlocked
	out.RowsRead = len(rawRows) + len(dbRows)
	traceDBAddCoverageMetric(&out, "raw_rows", int64(len(rawRows)))
	traceDBAddCoverageMetric(&out, "db_rows", int64(len(dbRows)))
	if int64(len(rawRows)) != inventory.RawDecode.Metrics["target_sched_blocked_reason_body_admitted"] ||
		inventory.RawDecode.Metrics["target_sched_blocked_reason_key_capture_failed"] != 0 {
		out.Metadata["ledger_state"] = "withheld_raw_record_capture_incomplete"
		out.Skipped = "raw blocked key ledger withheld: admitted-body/retained-record census mismatch"
		return out, nil
	}

	rawCounts := map[traceDBBlockedComparableKey]int64{}
	dbCounts := map[traceDBBlockedComparableKey]int64{}
	rawSubjectCounts := map[traceDBBlockedSubjectKey]int64{}
	dbSubjectCounts := map[traceDBBlockedSubjectKey]int64{}
	for _, row := range rawRows {
		key, ok := traceDBRawBlockedComparableKey(row)
		if !ok {
			traceDBAddCoverageMetric(&out, "raw_rows_without_comparable_key", 1)
			continue
		}
		rawCounts[key]++
		rawSubjectCounts[traceDBBlockedSubjectKey{
			TargetTID: key.TargetTID,
			IOWait:    key.IOWait,
		}]++
		if row.CallerSymbolized {
			traceDBAddCoverageMetric(&out, "raw_caller_symbolized_rows", 1)
		} else {
			traceDBAddCoverageMetric(&out, "raw_caller_opaque_rows", 1)
		}
	}
	for _, row := range dbRows {
		key, ok := traceDBDBBlockedComparableKey(row)
		if !ok {
			traceDBAddCoverageMetric(&out, "db_rows_without_comparable_key", 1)
			continue
		}
		dbCounts[key]++
		dbSubjectCounts[traceDBBlockedSubjectKey{
			TargetTID: key.TargetTID,
			IOWait:    key.IOWait,
		}]++
		switch row.CallerQuality {
		case "symbolized":
			traceDBAddCoverageMetric(&out, "db_caller_symbolized_rows", 1)
		case "opaque":
			traceDBAddCoverageMetric(&out, "db_caller_opaque_rows", 1)
		}
	}
	traceDBAddCoverageMetric(&out, "raw_key_cohorts", int64(len(rawCounts)))
	traceDBAddCoverageMetric(&out, "db_key_cohorts", int64(len(dbCounts)))
	traceDBAddCoverageMetric(&out, "raw_subject_key_cohorts", int64(len(rawSubjectCounts)))
	traceDBAddCoverageMetric(&out, "db_subject_key_cohorts", int64(len(dbSubjectCounts)))
	for key, dbCount := range dbSubjectCounts {
		rawCount := rawSubjectCounts[key]
		if rawCount == 0 {
			traceDBAddCoverageMetric(&out, "db_subject_key_cohorts_absent_raw", 1)
			traceDBAddCoverageMetric(&out, "db_subject_rows_absent_raw", dbCount)
			continue
		}
		if dbCount > rawCount {
			traceDBAddCoverageMetric(&out, "db_subject_key_cohorts_exceed_raw", 1)
			traceDBAddCoverageMetric(&out, "db_subject_rows_exceeding_raw", dbCount-rawCount)
		}
	}
	if out.Metrics["db_subject_key_cohorts_absent_raw"] == 0 &&
		out.Metrics["db_subject_key_cohorts_exceed_raw"] == 0 {
		out.Metadata["subject_key_relation"] =
			"db_target_tid_iowait_multiset_is_raw_subset"
	} else {
		out.Metadata["subject_key_relation"] =
			"db_target_tid_iowait_multiset_is_not_raw_subset"
	}
	out.Metadata["raw_key_multiset_sha256"] = traceDBBlockedKeyMultisetDigest(rawCounts)
	out.Metadata["db_key_multiset_sha256"] = traceDBBlockedKeyMultisetDigest(dbCounts)
	if out.Metrics["raw_rows_without_comparable_key"] != 0 ||
		out.Metrics["db_rows_without_comparable_key"] != 0 {
		out.Metadata["ledger_state"] = "withheld_unkeyed_rows"
		out.Skipped = "raw blocked key ledger withheld: not every raw/DB row has an exact comparable content key"
		return out, nil
	}

	for key, dbCount := range dbCounts {
		rawCount := rawCounts[key]
		if rawCount == 0 {
			traceDBAddCoverageMetric(&out, "db_key_cohorts_absent_raw", 1)
			traceDBAddCoverageMetric(&out, "db_rows_absent_raw", dbCount)
			continue
		}
		if dbCount > rawCount {
			traceDBAddCoverageMetric(&out, "db_key_cohorts_exceed_raw", 1)
			traceDBAddCoverageMetric(&out, "db_rows_exceeding_raw", dbCount-rawCount)
		}
	}
	if out.Metrics["db_key_cohorts_absent_raw"] != 0 ||
		out.Metrics["db_key_cohorts_exceed_raw"] != 0 {
		out.Metadata["ledger_state"] = "exact_content_multiset_subset_mismatch"
		out.Skipped = "raw blocked key ledger mismatch: DB content cohorts are not a multiset subset of raw"
		return out, nil
	}

	eligibleKeys := map[traceDBBlockedComparableKey]bool{}
	for key, rawCount := range rawCounts {
		if dbCounts[key] > 0 {
			traceDBAddCoverageMetric(&out, "raw_overlap_key_cohorts", 1)
			traceDBAddCoverageMetric(&out, "raw_rows_withheld_overlap_cohort", rawCount)
			continue
		}
		traceDBAddCoverageMetric(&out, "raw_absent_db_key_cohorts", 1)
		traceDBAddCoverageMetric(&out, "raw_rows_absent_db_cohort", rawCount)
		eligibleKeys[key] = true
	}
	if traceDBRawBlockedFamilyAuthorityEligible(inventory, rawRows) {
		// A complete immutable raw family has the original physical timestamp,
		// CPU, envelope and delay which the DB projection cannot retain. Select
		// it as the sole event authority instead of trying to subtract DB rows
		// from coarse content cohorts.
		var recovery []traceDBRawBlockedRecoveryRow
		for _, record := range rawRows {
			prepared, reason := traceDBPrepareRawBlockedRecovery(record, authority)
			if reason == "" {
				traceDBAddCoverageMetric(
					&out, "raw_family_identity_admitted", 1)
				recovery = append(recovery, prepared)
			} else {
				traceDBAddCoverageMetric(
					&out, "raw_family_identity_rejected_"+
						traceDBRawDecodeReasonMetric(reason), 1)
			}
		}
		traceDBAddCoverageMetric(
			&out, "db_rows_suppressed_by_raw_family", int64(len(dbRows)))
		out.Metadata["ledger_state"] = "exact_raw_family_authority"
		out.Metadata["deduplication_limit"] =
			"not_applicable: complete raw physical-event family selected as sole publication authority"
		out.Metadata["publication_authority"] =
			"source_rawtrace_blocked_family_authority"
		return out, recovery
	}
	// Preserve immutable raw capture order. Iterating the cohort map here would
	// make equal-timestamp output ordering depend on randomized Go map order.
	var recovery []traceDBRawBlockedRecoveryRow
	for _, record := range rawRows {
		key, ok := traceDBRawBlockedComparableKey(record)
		if !ok || !eligibleKeys[key] {
			continue
		}
		prepared, reason := traceDBPrepareRawBlockedRecovery(record, authority)
		if reason == "" {
			traceDBAddCoverageMetric(&out, "raw_rows_absent_db_identity_admitted", 1)
			recovery = append(recovery, prepared)
		} else {
			traceDBAddCoverageMetric(&out, "raw_rows_absent_db_identity_rejected_"+reason, 1)
		}
	}
	out.Metadata["ledger_state"] = "exact_content_multiset_subset"
	out.Metadata["deduplication_limit"] =
		"DB lacks original blocked-event timestamp/CPU/delay, so any raw cohort sharing a content key with DB is wholly withheld rather than subtracting counts"
	out.Metadata["publication_authority"] =
		"delegated_to_source_rawtrace_blocked_recovery_after_all_identity_gates"
	return out, recovery
}

func traceDBRawBlockedFamilyAuthorityEligible(
	inventory *traceDBSourceNameInventory,
	rawRows []traceDBRawBlockedRecord,
) bool {
	if inventory == nil || inventory.RawDecode.Role == "" ||
		!traceDBRawDecodeFamilyComplete(
			inventory.RawDecode, traceDBRawRetentionBlocked) {
		return false
	}
	records := inventory.RawDecode.Metrics["target_sched_blocked_reason_records"]
	admitted := inventory.RawDecode.Metrics["target_sched_blocked_reason_body_admitted"]
	return records == admitted &&
		admitted == int64(len(rawRows)) &&
		inventory.RawDecode.Metrics["target_sched_blocked_reason_body_rejected"] == 0 &&
		inventory.RawDecode.Metrics["target_sched_blocked_reason_key_capture_failed"] == 0
}

func traceDBRawBlockedComparableKey(row traceDBRawBlockedRecord) (traceDBBlockedComparableKey, bool) {
	if row.TargetTID <= 0 || row.TargetTID > math.MaxInt32 || row.IOWait < 0 || row.IOWait > 1 {
		return traceDBBlockedComparableKey{}, false
	}
	caller := ""
	if row.CallerSymbolized {
		if !traceDBSingleToken(row.Caller) {
			return traceDBBlockedComparableKey{}, false
		}
		caller = "symbol:" + row.Caller
	} else {
		caller = fmt.Sprintf("raw:0x%x", row.CallerRaw)
	}
	return traceDBBlockedComparableKey{TargetTID: row.TargetTID, IOWait: row.IOWait, Caller: caller}, true
}

func traceDBDBBlockedComparableKey(row traceDBPreparedBlockedReason) (traceDBBlockedComparableKey, bool) {
	if row.Row.TID <= 0 || row.Row.TID > math.MaxInt32 || row.IOWait < 0 || row.IOWait > 1 {
		return traceDBBlockedComparableKey{}, false
	}
	caller := ""
	switch row.CallerQuality {
	case "symbolized":
		if !traceDBSingleToken(row.Caller) {
			return traceDBBlockedComparableKey{}, false
		}
		caller = "symbol:" + row.Caller
	case "opaque":
		normalized, ok := normalizeTraceDBBlockedRawString(row.CallerRaw)
		if !ok {
			return traceDBBlockedComparableKey{}, false
		}
		caller = "raw:" + normalized
	default:
		return traceDBBlockedComparableKey{}, false
	}
	return traceDBBlockedComparableKey{TargetTID: row.Row.TID, IOWait: row.IOWait, Caller: caller}, true
}

func traceDBPrepareRawBlockedRecovery(
	row traceDBRawBlockedRecord,
	authority traceDBSchedulerAuthority,
) (traceDBRawBlockedRecoveryRow, string) {
	if row.TimestampNS > math.MaxInt64 || !validTraceDBCPUIndex(int64(row.CPU)) {
		return traceDBRawBlockedRecoveryRow{}, "invalid_coordinates"
	}
	timestamp := int64(row.TimestampNS)
	targetThread, targetProcess, reason :=
		traceDBResolveRawPublicTID(authority, row.TargetTID, timestamp)
	if reason != "" {
		return traceDBRawBlockedRecoveryRow{}, "target_" + reason
	}
	headerThread, headerProcess, reason :=
		traceDBResolveRawPublicTID(authority, row.HeaderPID, timestamp)
	if reason != "" {
		return traceDBRawBlockedRecoveryRow{}, "header_" + reason
	}
	return traceDBRawBlockedRecoveryRow{
		Raw: row, TargetThread: targetThread, TargetProcess: targetProcess,
		HeaderThread: headerThread, HeaderProcess: headerProcess,
	}, ""
}

func traceDBResolveRawPublicTID(
	authority traceDBSchedulerAuthority,
	tid, timestamp int64,
) (traceDBThread, traceDBProcess, string) {
	if !authority.initialized || !authority.complete || tid <= 0 || tid > math.MaxInt32 || timestamp < 0 {
		return traceDBThread{}, traceDBProcess{}, "invalid_or_incomplete_authority"
	}
	candidates := authority.identities.ByTIDCandidates[tid]
	var selectedThread traceDBThread
	var selectedProcess traceDBProcess
	admitted := 0
	for _, candidate := range candidates {
		thread, process, resolution := authority.resolveThreadSubject(candidate.ITID)
		if resolution != traceDBSchedulerThreadResolved || thread.TID != tid ||
			!authority.threadPointAllows(thread.ITID, timestamp) {
			continue
		}
		admitted++
		selectedThread, selectedProcess = thread, process
	}
	switch admitted {
	case 0:
		if authority.identities.RejectedPublicTID[tid] {
			return traceDBThread{}, traceDBProcess{}, "rejected_or_lifecycle_absent"
		}
		return traceDBThread{}, traceDBProcess{}, "absent_or_lifecycle_absent"
	case 1:
		return selectedThread, selectedProcess, ""
	default:
		return traceDBThread{}, traceDBProcess{}, "ambiguous_incarnation"
	}
}

func traceDBBlockedKeyMultisetDigest(counts map[traceDBBlockedComparableKey]int64) string {
	keys := make([]traceDBBlockedComparableKey, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].TargetTID != keys[j].TargetTID {
			return keys[i].TargetTID < keys[j].TargetTID
		}
		if keys[i].IOWait != keys[j].IOWait {
			return keys[i].IOWait < keys[j].IOWait
		}
		return keys[i].Caller < keys[j].Caller
	})
	sum := sha256.New()
	for _, key := range keys {
		traceDBBlockedDigestInt64(sum, key.TargetTID)
		traceDBBlockedDigestInt64(sum, key.IOWait)
		traceDBBlockedDigestInt64(sum, int64(len(key.Caller)))
		_, _ = sum.Write([]byte(key.Caller))
		traceDBBlockedDigestInt64(sum, counts[key])
	}
	return hex.EncodeToString(sum.Sum(nil))
}

func traceDBBlockedDigestInt64(target hash.Hash, value int64) {
	var buffer [8]byte
	binary.LittleEndian.PutUint64(buffer[:], uint64(value))
	_, _ = target.Write(buffer[:])
}
