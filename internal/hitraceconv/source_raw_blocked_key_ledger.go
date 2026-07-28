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
	out := newTraceDBRawBlockedKeyCoverage()
	if inventory == nil {
		out.Skipped = "raw blocked key ledger unavailable: immutable source inventory absent"
		return out
	}
	out.Found = inventory.RawDecode.Found
	if inventory.RawDecode.Metadata["decode_state"] != "strict_target_ledger_complete" {
		out.Metadata["ledger_state"] = "withheld_raw_decode_incomplete"
		out.Skipped = "raw blocked key ledger withheld: strict raw decode ledger incomplete"
		return out
	}
	rawRows := inventory.RawBlocked
	out.RowsRead = len(rawRows) + len(dbRows)
	traceDBAddCoverageMetric(&out, "raw_rows", int64(len(rawRows)))
	traceDBAddCoverageMetric(&out, "db_rows", int64(len(dbRows)))
	if int64(len(rawRows)) != inventory.RawDecode.Metrics["target_sched_blocked_reason_body_admitted"] ||
		inventory.RawDecode.Metrics["target_sched_blocked_reason_key_capture_failed"] != 0 {
		out.Metadata["ledger_state"] = "withheld_raw_record_capture_incomplete"
		out.Skipped = "raw blocked key ledger withheld: admitted-body/retained-record census mismatch"
		return out
	}

	rawCounts := map[traceDBBlockedComparableKey]int64{}
	dbCounts := map[traceDBBlockedComparableKey]int64{}
	rawByKey := map[traceDBBlockedComparableKey][]traceDBRawBlockedRecord{}
	for _, row := range rawRows {
		key, ok := traceDBRawBlockedComparableKey(row)
		if !ok {
			traceDBAddCoverageMetric(&out, "raw_rows_without_comparable_key", 1)
			continue
		}
		rawCounts[key]++
		rawByKey[key] = append(rawByKey[key], row)
	}
	for _, row := range dbRows {
		key, ok := traceDBDBBlockedComparableKey(row)
		if !ok {
			traceDBAddCoverageMetric(&out, "db_rows_without_comparable_key", 1)
			continue
		}
		dbCounts[key]++
	}
	traceDBAddCoverageMetric(&out, "raw_key_cohorts", int64(len(rawCounts)))
	traceDBAddCoverageMetric(&out, "db_key_cohorts", int64(len(dbCounts)))
	out.Metadata["raw_key_multiset_sha256"] = traceDBBlockedKeyMultisetDigest(rawCounts)
	out.Metadata["db_key_multiset_sha256"] = traceDBBlockedKeyMultisetDigest(dbCounts)
	if out.Metrics["raw_rows_without_comparable_key"] != 0 ||
		out.Metrics["db_rows_without_comparable_key"] != 0 {
		out.Metadata["ledger_state"] = "withheld_unkeyed_rows"
		out.Skipped = "raw blocked key ledger withheld: not every raw/DB row has an exact comparable content key"
		return out
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
		return out
	}

	for key, records := range rawByKey {
		if dbCounts[key] > 0 {
			traceDBAddCoverageMetric(&out, "raw_overlap_key_cohorts", 1)
			traceDBAddCoverageMetric(&out, "raw_rows_withheld_overlap_cohort", int64(len(records)))
			continue
		}
		traceDBAddCoverageMetric(&out, "raw_absent_db_key_cohorts", 1)
		traceDBAddCoverageMetric(&out, "raw_rows_absent_db_cohort", int64(len(records)))
		for _, record := range records {
			reason := traceDBRawBlockedIdentityReason(record, authority)
			if reason == "" {
				traceDBAddCoverageMetric(&out, "raw_rows_absent_db_identity_admitted", 1)
			} else {
				traceDBAddCoverageMetric(&out, "raw_rows_absent_db_identity_rejected_"+reason, 1)
			}
		}
	}
	out.Metadata["ledger_state"] = "exact_content_multiset_subset"
	out.Metadata["deduplication_limit"] =
		"DB lacks original blocked-event timestamp/CPU/delay, so any raw cohort sharing a content key with DB is wholly withheld rather than subtracting counts"
	out.Metadata["publication_authority"] =
		"withheld_requires_separate_raw_only_row_publisher_and_all_identity_gates"
	return out
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

func traceDBRawBlockedIdentityReason(
	row traceDBRawBlockedRecord,
	authority traceDBSchedulerAuthority,
) string {
	if row.TimestampNS > math.MaxInt64 || !validTraceDBCPUIndex(int64(row.CPU)) {
		return "invalid_coordinates"
	}
	timestamp := int64(row.TimestampNS)
	if _, _, reason := traceDBResolveRawPublicTID(authority, row.TargetTID, timestamp); reason != "" {
		return "target_" + reason
	}
	if _, _, reason := traceDBResolveRawPublicTID(authority, row.HeaderPID, timestamp); reason != "" {
		return "header_" + reason
	}
	return ""
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
