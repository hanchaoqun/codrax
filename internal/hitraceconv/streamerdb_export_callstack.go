package hitraceconv

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxTraceDBCallstackTokenBytes = 4096
const traceDBCallstackRejectedWitnessCap = 8
const traceDBCallstackRejectedScalarInlineBytes = 64

type traceDBCallstackRow struct {
	ID             int64
	SourceID       int64
	TS             int64
	TimestampKnown bool
	Dur            int64
	End            int64
	IntervalKnown  bool
	CallID         int64
	// LogicalOwnerITID is distinct from EmitterITID for the official
	// cookie-bearing callstack profile. In that profile callid names the
	// logical async owner while child_callid names the physical start emitter.
	LogicalOwnerITID int64
	EmitterITID      int64
	OwnerIPID        int64
	Name             string
	Flag             string
	// DistributedMetadata records official TraceStreamer chain/role metadata.
	// It is diagnostic-only and never becomes an endpoint pairing key.
	DistributedMetadata bool
	// OfficialAsyncInterval means this one high-level row already carries the
	// completed logical (ts,dur,cookie) interval. It is emitted as one typed
	// interval and never split into fabricated S/F physical endpoints.
	OfficialAsyncInterval bool
	Depth                 int64
	DepthKnown            bool
	Cookie                string
	Task                  string
	TID                   int64
	// TGID is the DB logical-owner process used as the marker payload only for
	// DB-generated rows. HeaderTGID is the host scheduler process printed in
	// the ftrace envelope. An exact raw async pair keeps its own payload PID,
	// which may differ from both under PID namespaces such as Donghu.
	TGID       int64
	HeaderTGID int64
	CPUITID    int64
	CPUAliased bool
	StartCPU   int64
	EndCPU     int64
	// CPUPlacement may be unavailable only after emitter identity, marker
	// lifecycle and timestamps are proven. The span is then retained on the
	// typed comment lane without a fabricated physical CPU.
	CPUPlacement traceDBSyncSpanCPUPlacement
}

type traceDBCallstackAsyncKey struct {
	OwnerIPID int64
	TGID      int64
	Name      string
	Cookie    string
}

type traceDBCallstackRejectedWitness struct {
	RowID  int64
	TID    int64
	ITID   int64
	TS     int64
	Reason string
	Dur    string
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
		"cpu":              "same lifecycle authority filters the strict Running witness lane; known placement uses an exact CPU, while proven spans with source/lifecycle/unknown/ambiguous placement retain a versioned cpu_status=unavailable marker and never fabricate CPU 0",
		"emitter_identity": "sync/legacy rows use strict callstack.itid and callstack.callid->audited thread.id/itid convergence; official cookie-bearing rows in a child_callid schema use callid as logical owner and child_callid as physical start emitter, with optional itid required to converge with that emitter",
		"flag":             "official TraceStreamer TEXT 'S'/'C' is distributed-call role metadata on an interval, not an async endpoint action; SQLite NULL/TEXT '' and legacy TEXT 'I' are non-distributed sync forms; every other storage class/value fails closed",
		"name":             "exact bounded TEXT; NULL/empty/edge-whitespace/control/oversize/invalid-UTF8/non-TEXT failures are counted separately; CPU-known synchronous pipe names use standard B/E only when both timestamps and the complete name round-trip losslessly, otherwise they retain the versioned exact trace-mark lane",
		"cookie":           "official non-NULL SQLite INTEGER identifies one already-associated async interval; NULL means no async cookie, zero is a valid cookie, and chainId never substitutes for cookie",
		"distributed":      "bounded chainId and flag C/S are interval metadata only; neither value selects S/F pairing or acts as an async cookie",
		"duration":         "strict signed integer nanoseconds; producer sentinel dur=-1 is counted as unfinished_duration_sentinel separately from malformed negative values and is never fabricated into a closed span",
		"async_owner":      "official child_callid schema preserves callid logical owner separately from the child_callid physical start emitter; completed high-level intervals publish source emitter/start CPU status and exact ts/end, while finish emitter and finish CPU remain explicitly unavailable unless an exact raw F endpoint proves them",
		"pid_namespace":    "marker payload PID is preserved separately from the ftrace header TGID; a differing header TGID is recovered only from one unique same-public-TID Running identity admitted at every required endpoint",
		"row_order":        "strict SQLite rowid; optional source id remains provenance only; typed endpoint phase ordering",
		"async_identity":   "official completed async intervals use cookie only and publish one typed interval; the legacy zero-duration S/C compatibility lane also requires cookie and never substitutes chainId",
		"lifecycle":        "same complete collector authority; sync positive spans require closed thread/process generation, zero spans and async endpoints require exact point admission",
		"sync_pairing":     "accepted sync rows enter the single cross-producer typed B/E authority; rejected callstack evidence with exact emitter+interval uses producer-scoped overlap fences, exact timestamp-only evidence uses suffix fences, and only time-unlocalizable evidence poisons the full callstack lane; name-only rows remain locally withheld",
		"async_generation": "legacy zero-duration endpoint compatibility rows are admitted independently; exact rejected owner/name/cookie keys fail closed locally, while official completed async intervals never enter this pairing lane",
		"raw_async":        "an official completed interval becomes standard S/F only after one unique immutable-source raw pair proves exact name/cookie/start/end and start-emitter envelope; raw payload PID is preserved independently because DB logical owner TGID may differ under PID namespaces; start and finish retain independent raw common-PID/CPU/flags, while unmatched rows remain typed",
		"diagnostics":      "accepted callstack rows expose exact zero-start, long-duration and one bounded longest-row witness; a complete independent raw target timestamp floor is compared advisory-only and never changes admission",
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
	rawAsync := newTraceDBRawAsyncMatchLedger(tdb.sourceNameInventory, authority)

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
	hasChildCallID, err := tdb.columnExists(ctx, "callstack", "child_callid")
	if err != nil {
		return fail(err)
	}
	for _, optional := range []struct {
		name    string
		present bool
	}{
		{"id", hasID}, {"itid", hasITID}, {"callid", hasCallID}, {"flag", hasFlag},
		{"cookie", hasCookie}, {"chainId", hasChainID}, {"depth", hasDepth},
		{"child_callid", hasChildCallID},
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
	childCallIDExpr := "NULL"
	if hasChildCallID {
		childCallIDExpr = quoteSQLiteIdent("child_callid")
	}
	query := fmt.Sprintf(`
		SELECT rowid, %s, ts, dur, name, %s, %s, %s, %s, %s, %s, %s
		FROM callstack
		ORDER BY rowid
	`, idExpr, callIDExpr, itidExpr, flagExpr, cookieExpr, chainIDExpr, depthExpr, childCallIDExpr)
	rows, err := tdb.db.QueryContext(ctx, query)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	skipped := map[string]int{}
	var accepted []traceDBCallstackRow
	rawTargetFirst, rawTargetFloorKnown := traceDBCallstackRawTargetFirstTimestamp(tdb)
	if rawTargetFloorKnown {
		if coverage.Metadata == nil {
			coverage.Metadata = map[string]string{}
		}
		coverage.Metadata["raw_target_first_timestamp_ns"] =
			strconv.FormatInt(rawTargetFirst, 10)
	}
	var longestAccepted *traceDBCallstackRow
	asyncGlobalPoisoned := false
	asyncTaintedKeys := map[traceDBCallstackAsyncKey]bool{}
	nameOnlyWithheld := 0
	officialAsyncEmitted := 0
	officialAsyncRejected := 0
	officialAsyncShaped := 0
	rejectedWitnessTotal := 0
	rejectedWitnesses := make([]traceDBCallstackRejectedWitness, 0,
		traceDBCallstackRejectedWitnessCap)
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		var rowIDRaw, sourceIDRaw, tsRaw, durRaw, nameRaw, callIDRaw, itidRaw, flagRaw, cookieRaw, chainIDRaw, depthRaw, childCallIDRaw any
		if err := rows.Scan(&rowIDRaw, &sourceIDRaw, &tsRaw, &durRaw, &nameRaw, &callIDRaw, &itidRaw, &flagRaw, &cookieRaw, &chainIDRaw, &depthRaw, &childCallIDRaw); err != nil {
			return fail(err)
		}
		row, reason := prepareTraceDBCallstackRow(authority, running, hasID, hasITID, hasCallID, hasFlag, hasDepth, hasChildCallID,
			rowIDRaw, sourceIDRaw, tsRaw, durRaw, nameRaw, callIDRaw, itidRaw, flagRaw, cookieRaw, chainIDRaw, depthRaw, childCallIDRaw)
		if reason != "" {
			officialAsync := traceDBCallstackOfficialAsyncRaw(flagRaw, cookieRaw, durRaw, hasFlag)
			if officialAsync {
				officialAsyncShaped++
				officialAsyncRejected++
			}
			nameOnlyRejection := traceDBCallstackNameOnlyRejection(reason)
			if nameOnlyRejection {
				nameOnlyWithheld++
			}
			if !officialAsync && traceDBCallstackPotentialAsync(flagRaw, cookieRaw, hasFlag) {
				if key, exact := traceDBCallstackExactAsyncKey(row); exact {
					asyncTaintedKeys[key] = true
				} else {
					asyncGlobalPoisoned = true
				}
			}
			if !nameOnlyRejection && !officialAsync && traceDBCallstackPotentialSync(cookieRaw) {
				for _, itid := range traceDBCallstackExactEmitterCandidates(authority, hasITID, hasCallID, itidRaw, callIDRaw) {
					thread, _, resolution := authority.resolveThreadSubject(itid)
					if resolution != traceDBSchedulerThreadResolved {
						return fail(&traceDBOutputInvariantError{Reason: "callstack_exact_lane_lost_identity"})
					}
					if row.TimestampKnown {
						fence := traceDBSyncSpanLaneFence{
							Producer:           traceDBSyncSpanProducerCallstack,
							HeaderTID:          thread.TID,
							CanonicalITID:      itid,
							CanonicalITIDKnown: true,
							Start:              row.TS,
							Kind:               traceDBSyncSpanFenceSuffix,
							Reason:             traceDBSyncSpanLanePoisonRejectedCallstackCandidate,
						}
						if row.IntervalKnown && row.End > row.TS {
							fence.End = row.End
							fence.Kind = traceDBSyncSpanFenceInterval
						}
						if err := syncSpans.fenceExactLane(ctx, fence); err != nil {
							return fail(err)
						}
						rejectedWitnessTotal++
						if len(rejectedWitnesses) < traceDBCallstackRejectedWitnessCap {
							rejectedWitnesses = append(rejectedWitnesses,
								traceDBCallstackRejectedWitness{
									RowID: row.ID, TID: thread.TID, ITID: itid,
									TS: row.TS, Reason: reason,
									Dur: traceDBCallstackRejectedScalar(durRaw),
								})
						}
					} else {
						if err := syncSpans.poisonExactLane(ctx, traceDBSyncSpanLanePoison{
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
			}
			skipped[reason]++
			continue
		}
		if row.OfficialAsyncInterval {
			officialAsyncShaped++
		}
		if row.TS == 0 {
			traceDBAddCoverageMetric(&coverage, "source_rows_accepted_start_timestamp_zero", 1)
		}
		if rawTargetFloorKnown && row.TS < rawTargetFirst {
			traceDBAddCoverageMetric(&coverage,
				"source_rows_accepted_before_raw_target_first_timestamp", 1)
		}
		if row.Dur >= 100_000_000 {
			traceDBAddCoverageMetric(&coverage, "source_rows_accepted_duration_ge_100ms", 1)
		}
		if row.Dur >= 1_000_000_000 {
			traceDBAddCoverageMetric(&coverage, "source_rows_accepted_duration_ge_1s", 1)
		}
		if longestAccepted == nil || row.Dur > longestAccepted.Dur ||
			row.Dur == longestAccepted.Dur && row.ID < longestAccepted.ID {
			copy := row
			longestAccepted = &copy
		}
		accepted = append(accepted, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	traceDBAddCoverageMetric(&coverage, "source_rows_withheld_name_only", int64(nameOnlyWithheld))
	traceDBAddCoverageMetric(&coverage, "source_rows_official_async_shaped", int64(officialAsyncShaped))
	traceDBAddCoverageMetric(&coverage, "source_rows_rejected_official_async_shape", int64(officialAsyncRejected))
	traceDBAddCoverageMetric(&coverage, "source_rows_unfinished_duration_sentinel",
		int64(skipped["unfinished_duration_sentinel"]))
	traceDBAddCoverageMetric(&coverage, "source_rows_accepted_pre_pairing", int64(len(accepted)))
	if hasChildCallID {
		childEmitterRows := 0
		for _, row := range accepted {
			if row.OfficialAsyncInterval && row.LogicalOwnerITID > 0 {
				childEmitterRows++
			}
		}
		traceDBAddCoverageMetric(&coverage,
			"source_rows_official_async_child_emitter_resolved", int64(childEmitterRows))
	}
	if longestAccepted != nil {
		if coverage.Metadata == nil {
			coverage.Metadata = map[string]string{}
		}
		coverage.Metadata["longest_accepted_span_witness"] = fmt.Sprintf(
			"row_id=%d/start_ns=%d/end_ns=%d/duration_ns=%d/header_tid=%d/name=%s",
			longestAccepted.ID, longestAccepted.TS, longestAccepted.End,
			longestAccepted.Dur, longestAccepted.TID,
			traceDBRawMarkerNameWitness(longestAccepted.Name))
	}
	aliasedRows := 0
	cpuUnavailableRows := 0
	exactNameRows := 0
	distributedMetadataRows := 0
	for _, row := range accepted {
		if row.CPUAliased {
			aliasedRows++
		}
		if row.CPUPlacement != traceDBSyncSpanCPUPlacementKnown {
			cpuUnavailableRows++
		}
		if strings.ContainsRune(row.Name, '|') {
			exactNameRows++
		}
		if row.DistributedMetadata {
			distributedMetadataRows++
		}
	}
	traceDBAddCoverageMetric(&coverage, "source_rows_recovered_same_public_tid_scheduler_alias", int64(aliasedRows))
	traceDBAddCoverageMetric(&coverage, "source_rows_preserved_cpu_unavailable", int64(cpuUnavailableRows))
	traceDBAddCoverageMetric(&coverage, "source_rows_admitted_exact_name_pre_pairing", int64(exactNameRows))
	traceDBAddCoverageMetric(&coverage, "source_rows_with_distributed_metadata", int64(distributedMetadataRows))
	suppressedPrePairing := 0
	for _, count := range skipped {
		suppressedPrePairing += count
	}
	traceDBAddCoverageMetric(&coverage, "source_rows_suppressed_pre_pairing", int64(suppressedPrePairing))
	if len(rejectedWitnesses) > 0 {
		if coverage.Metadata == nil {
			coverage.Metadata = map[string]string{}
		}
		parts := make([]string, 0, len(rejectedWitnesses))
		for _, witness := range rejectedWitnesses {
			parts = append(parts, fmt.Sprintf(
				"row_id=%d/tid=%d/itid=%d/ts_ns=%d/reason=%s/dur=%s",
				witness.RowID, witness.TID, witness.ITID, witness.TS,
				witness.Reason, witness.Dur))
		}
		coverage.Metadata["rejected_callstack_fence_witnesses"] =
			strings.Join(parts, ";")
		traceDBAddCoverageMetric(&coverage,
			"rejected_callstack_fence_witnesses_emitted",
			int64(len(rejectedWitnesses)))
		if omitted := rejectedWitnessTotal - len(rejectedWitnesses); omitted > 0 {
			traceDBAddCoverageMetric(&coverage,
				"rejected_callstack_fence_witnesses_omitted", int64(omitted))
		}
		coverage.FieldSources["rejected_callstack_fence_witnesses"] =
			"bounded exact rejected row plus resolved physical fence lane and SQLite duration scalar; diagnostic only, never alternate duration/span admission authority"
	}
	traceDBAddCoverageMetric(&coverage, "source_rows_suppressed_cpu_unavailable", int64(
		skipped["unknown_start_cpu"]+skipped["unknown_end_cpu"]+
			skipped["tainted_running_cpu_witness"]+skipped["lifecycle_rejected_running_cpu_witness"]))
	traceDBAddCoverageMetric(&coverage, "source_rows_suppressed_identity", int64(
		skipped["emitter_identity_mismatch"]+skipped["unresolved_emitter_thread"]+
			skipped["unresolved_owner_process"]+skipped["invalid_emitter_process"]+
			skipped["ambiguous_same_public_tid_scheduler_alias"]))
	var syncRows []traceDBCallstackRow
	var officialAsyncIntervals []traceDBCallstackRow
	asyncGroups := map[traceDBCallstackAsyncKey][]traceDBCallstackRow{}
	for _, row := range accepted {
		switch row.Flag {
		case "", "I":
			if row.OfficialAsyncInterval {
				officialAsyncIntervals = append(officialAsyncIntervals, row)
			} else {
				syncRows = append(syncRows, row)
			}
		case "S", "C":
			if row.OfficialAsyncInterval {
				officialAsyncIntervals = append(officialAsyncIntervals, row)
			} else {
				key := traceDBCallstackAsyncKey{OwnerIPID: row.OwnerIPID, TGID: row.TGID, Name: row.Name, Cookie: row.Cookie}
				asyncGroups[key] = append(asyncGroups[key], row)
			}
		default:
			syncRows = append(syncRows, row)
		}
	}

	rawOfficialAsyncEmitted := 0
	for _, row := range officialAsyncIntervals {
		if pair, ok := rawAsync.claim(row); ok {
			if err := pair.publish(sink); err != nil {
				return fail(err)
			}
			coverage.RowsEmitted += 2
			rawOfficialAsyncEmitted++
			continue
		}
		rendered, err := prepareTraceDBCompletedAsyncIntervalRow(row, sink.stats.RowsAccepted)
		if err != nil {
			return fail(err)
		}
		if err := sink.add(rendered); err != nil {
			return fail(err)
		}
		coverage.RowsEmitted++
		officialAsyncEmitted++
	}
	traceDBAddCoverageMetric(&coverage, "source_rows_emitted_official_async_interval", int64(officialAsyncEmitted))
	traceDBAddCoverageMetric(&coverage,
		"source_rows_emitted_official_async_raw_pair", int64(rawOfficialAsyncEmitted))
	rawAsync.applyCoverage(&coverage)

	for _, row := range syncRows {
		depthProvenance := traceDBSyncSpanDepthUnknown
		if row.DepthKnown {
			depthProvenance = traceDBSyncSpanDepthCallstack
		}
		cpuProvenance := traceDBSyncSpanCPUCallstackTypedRunning
		if row.CPUPlacement != traceDBSyncSpanCPUPlacementKnown {
			cpuProvenance = traceDBSyncSpanCPUCallstackUnavailable
		}
		if err := syncSpans.submit(ctx, traceDBSyncSpanCandidate{
			Producer:           traceDBSyncSpanProducerCallstack,
			StableKind:         traceDBSyncSpanStableCallstackRowID,
			StableID:           row.ID,
			HeaderTID:          row.TID,
			HeaderTGID:         row.HeaderTGID,
			MarkerPID:          row.TGID,
			MarkerPIDKnown:     true,
			CanonicalITID:      row.EmitterITID,
			CanonicalITIDKnown: true,
			OwnerIPID:          row.OwnerIPID,
			OwnerIPIDKnown:     true,
			Start:              row.TS,
			End:                row.End,
			StartCPU:           row.StartCPU,
			EndCPU:             row.EndCPU,
			CPUPlacement:       row.CPUPlacement,
			StartCPUProvenance: cpuProvenance,
			EndCPUProvenance:   cpuProvenance,
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
		traceDBAddCoverageMetric(&coverage, "async_source_rows_suppressed_post_pairing",
			int64(traceDBCallstackSkippedTotal(skipped)-suppressedPrePairing))
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
			if row.CPUPlacement != traceDBSyncSpanCPUPlacementKnown {
				rendered, err := prepareTraceDBCPUUnavailableTraceMarkRow(row.TS, sink.stats.RowsAccepted,
					row.Task, row.TID, row.HeaderTGID, row.TGID, action, row.Name, row.Cookie, row.CPUPlacement)
				if err != nil {
					return fail(err)
				}
				if err := sink.add(rendered); err != nil {
					return fail(err)
				}
				coverage.RowsEmitted++
				continue
			}
			if strings.ContainsRune(row.Name, '|') {
				rendered, err := prepareTraceDBExactTraceMarkRow(row.TS, sink.stats.RowsAccepted,
					row.Task, row.TID, row.HeaderTGID, row.StartCPU, row.TGID, action, row.Name, row.Cookie)
				if err != nil {
					return fail(err)
				}
				if err := sink.add(rendered); err != nil {
					return fail(err)
				}
				coverage.RowsEmitted++
				continue
			}
			body := fmt.Sprintf("tracing_mark_write: %s|%d|%s|%s", action, row.TGID, row.Name, row.Cookie)
			if err := addTraceDBInstantRow(sink, row.TS, row.Task, row.TID, row.HeaderTGID, row.StartCPU, body); err != nil {
				return fail(err)
			}
			coverage.RowsEmitted++
		}
	}
	traceDBAddCoverageMetric(&coverage, "async_source_rows_suppressed_post_pairing",
		int64(traceDBCallstackSkippedTotal(skipped)-suppressedPrePairing))
	coverage.Skipped = traceDBCallstackSkipSummary(skipped)
	return coverage, nil
}

func traceDBCallstackRejectedScalar(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case int64:
		return "integer:" + strconv.FormatInt(typed, 10)
	case float64:
		return fmt.Sprintf("real_bits:0x%016x", math.Float64bits(typed))
	case string:
		return "text_" + traceDBCallstackRejectedBytes([]byte(typed))
	case []byte:
		return "blob_" + traceDBCallstackRejectedBytes(typed)
	default:
		return fmt.Sprintf("unsupported_type:%T", value)
	}
}

func traceDBCallstackRejectedBytes(value []byte) string {
	if len(value) <= traceDBCallstackRejectedScalarInlineBytes {
		return fmt.Sprintf("bytes=%d/b64=%s", len(value),
			base64.RawURLEncoding.EncodeToString(value))
	}
	sum := sha256.Sum256(value)
	return fmt.Sprintf("bytes=%d/sha256=%x", len(value), sum)
}

func traceDBCallstackRawTargetFirstTimestamp(tdb *traceDB) (int64, bool) {
	if tdb == nil || tdb.sourceNameInventory == nil ||
		tdb.sourceNameInventory.RawDecode.Metadata["decode_state"] !=
			"strict_target_ledger_complete" {
		return 0, false
	}
	raw := tdb.sourceNameInventory.RawDecode.Metadata["target_first_timestamp_ns"]
	value, err := strconv.ParseUint(raw, 10, 63)
	if err != nil {
		return 0, false
	}
	return int64(value), true
}

func traceDBCallstackSkippedTotal(skipped map[string]int) int {
	total := 0
	for _, count := range skipped {
		total += count
	}
	return total
}

func prepareTraceDBCallstackRow(authority traceDBSchedulerAuthority, running traceDBSchedulerRunningIndex,
	hasID, hasITID, hasCallID, hasFlag, hasDepth, hasChildCallID bool,
	rowIDRaw, sourceIDRaw, tsRaw, durRaw, nameRaw, callIDRaw, itidRaw, flagRaw, cookieRaw, chainIDRaw, depthRaw, childCallIDRaw any,
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
	row.TimestampKnown = true
	if row.Dur, ok = traceDBStrictSQLiteInt(durRaw); !ok {
		return row, "invalid_duration"
	}
	if row.Dur == -1 {
		return row, "unfinished_duration_sentinel"
	}
	if row.Dur < -1 {
		return row, "invalid_duration"
	}
	if row.TS > math.MaxInt64-row.Dur {
		return row, "interval_overflow"
	}
	row.End = row.TS + row.Dur
	row.IntervalKnown = true
	officialAsyncShape := traceDBCallstackOfficialAsyncRaw(flagRaw, cookieRaw, durRaw, hasFlag)
	var identityReason string
	if officialAsyncShape && hasChildCallID {
		row.LogicalOwnerITID, row.CallID, identityReason =
			traceDBResolveCallstackEmitterIdentity(index, false, hasCallID, nil, callIDRaw)
		if identityReason != "" {
			return row, "async_owner_" + identityReason
		}
		row.EmitterITID, _, identityReason =
			traceDBResolveCallstackEmitterIdentity(index, false, true, nil, childCallIDRaw)
		if identityReason != "" {
			return row, "async_child_" + identityReason
		}
		if hasITID && itidRaw != nil {
			explicitITID, valid := traceDBStrictInternalID(itidRaw)
			if !valid {
				return row, "invalid_emitter_itid"
			}
			if explicitITID != row.EmitterITID {
				return row, "async_child_emitter_identity_mismatch"
			}
		}
	} else {
		row.EmitterITID, row.CallID, identityReason =
			traceDBResolveCallstackEmitterIdentity(index, hasITID, hasCallID, itidRaw, callIDRaw)
		if identityReason != "" {
			return row, identityReason
		}
		row.LogicalOwnerITID = row.EmitterITID
	}
	var nameReason string
	if row.Name, nameReason = traceDBCallstackName(nameRaw); nameReason != "" {
		return row, nameReason
	}
	if hasFlag {
		if row.Flag, ok = traceDBCallstackFlag(flagRaw); !ok {
			return row, "invalid_flag"
		}
	} else {
		row.Flag = ""
	}
	chainIDPresent, chainIDValid := traceDBCallstackDistributedChainID(chainIDRaw)
	if !chainIDValid {
		return row, "invalid_distributed_chain_id"
	}
	cookie, cookiePresent, cookieValid := traceDBCallstackCookie(cookieRaw)
	if !cookieValid {
		return row, "invalid_cookie"
	}
	switch row.Flag {
	case "", "I":
		if cookiePresent {
			// TraceStreamer has already associated the source S/F endpoints
			// into one (ts,dur,cookie) interval. The high-level row does not
			// prove the original finish emitter or CPU, so it is preserved as
			// one typed interval and never reinterpreted as synchronous B/E or
			// split into fabricated physical S/F endpoints.
			row.Cookie = cookie
			row.OfficialAsyncInterval = true
			row.DistributedMetadata = chainIDPresent
			break
		}
		row.DistributedMetadata = chainIDPresent
	case "S", "C":
		if !cookiePresent {
			// Official S/C values describe distributed-call role on the
			// completed interval. They are not tracing_mark_write actions.
			row.DistributedMetadata = true
			row.Flag = ""
			break
		}
		if row.Dur > 0 {
			// Some TraceStreamer versions retain the distributed S/C role on
			// a completed (ts,dur,cookie) async row. Positive duration proves
			// this is not a legacy zero-duration endpoint.
			row.Cookie = cookie
			row.OfficialAsyncInterval = true
			row.DistributedMetadata = true
			break
		}
		// Retain the pre-existing legacy endpoint compatibility lane only for
		// the explicit zero-duration S/C+cookie shape. It is separate from the
		// official completed-interval profile above.
		row.Cookie = cookie
		row.DistributedMetadata = chainIDPresent
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
	row.TID = thread.TID
	if thread.IPID < 0 || process.PID <= 0 || process.PID > math.MaxInt32 {
		return row, "unresolved_owner_process"
	}
	ownerThread, ownerProcess, ownerResolution :=
		authority.resolveThreadSubject(row.LogicalOwnerITID)
	if ownerResolution != traceDBSchedulerThreadResolved ||
		ownerThread.IPID < 0 || ownerProcess.PID <= 0 || ownerProcess.PID > math.MaxInt32 {
		return row, "unresolved_logical_owner_process"
	}
	row.OwnerIPID = ownerThread.IPID
	row.TGID = ownerProcess.PID
	if row.TGID <= 0 || row.TGID > math.MaxInt32 {
		return row, "invalid_emitter_process"
	}
	row.HeaderTGID = process.PID
	row.CPUITID = row.EmitterITID
	if _, ok := traceDBCallstackText(thread.Name, true); !ok {
		return row, "invalid_emitter_comm"
	}
	displayName := traceDBThreadDisplayNameValue(index, thread)
	if _, ok := traceDBCallstackText(displayName, true); !ok {
		return row, "invalid_emitter_comm"
	}
	row.Task = traceDBCommName(displayName, "unknown")
	if row.Flag == "S" || row.Flag == "C" {
		if row.Cookie == "" || !traceDBCallstackMarkerToken(row.Cookie) {
			return row, "missing_async_identity"
		}
	}
	switch {
	case row.OfficialAsyncInterval:
		if !authority.threadPointAllows(row.EmitterITID, row.TS) ||
			!authority.threadPointAllows(row.LogicalOwnerITID, row.TS) {
			return row, "lifecycle_rejected_official_async_interval_start"
		}
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
		row.CPUPlacement = traceDBSyncSpanCPUPlacementSourceTainted
		row.StartCPU, row.EndCPU = 0, 0
		return row, ""
	}
	if runningStatus == traceDBSchedulerRunningLifecycleRejected {
		row.CPUPlacement = traceDBSyncSpanCPUPlacementLifecycleRejected
		row.StartCPU, row.EndCPU = 0, 0
		return row, ""
	}
	if runningStatus != traceDBSchedulerRunningKnown {
		alias, aliasStatus := authority.resolveCallstackSchedulerAlias(running, thread, process, row.TS, row.End,
			!row.OfficialAsyncInterval && (row.Flag == "" || row.Flag == "I"),
			!row.OfficialAsyncInterval && row.Dur > 0)
		switch aliasStatus {
		case traceDBCallstackSchedulerAliasResolved:
			row.CPUITID = alias.ITID
			row.HeaderTGID = alias.HeaderTGID
			row.StartCPU = alias.StartCPU
			row.EndCPU = alias.EndCPU
			row.CPUAliased = true
			return row, ""
		case traceDBCallstackSchedulerAliasAmbiguous:
			row.CPUPlacement = traceDBSyncSpanCPUPlacementAliasAmbiguous
			row.StartCPU, row.EndCPU = 0, 0
			return row, ""
		default:
			row.CPUPlacement = traceDBSyncSpanCPUPlacementUnknownStart
			row.StartCPU, row.EndCPU = 0, 0
			return row, ""
		}
	}
	row.EndCPU = row.StartCPU
	if !row.OfficialAsyncInterval && (row.Flag == "" || row.Flag == "I") {
		row.EndCPU, runningStatus = running.lookupCPUAt(row.EmitterITID, row.End)
		if runningStatus == traceDBSchedulerRunningSourceTainted {
			row.CPUPlacement = traceDBSyncSpanCPUPlacementSourceTainted
			row.StartCPU, row.EndCPU = 0, 0
			return row, ""
		}
		if runningStatus == traceDBSchedulerRunningLifecycleRejected {
			row.CPUPlacement = traceDBSyncSpanCPUPlacementLifecycleRejected
			row.StartCPU, row.EndCPU = 0, 0
			return row, ""
		}
		if runningStatus != traceDBSchedulerRunningKnown {
			row.CPUPlacement = traceDBSyncSpanCPUPlacementUnknownEnd
			row.StartCPU, row.EndCPU = 0, 0
			return row, ""
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
		if unicode.IsControl(r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return "", false
		}
	}
	return text, true
}

func traceDBCallstackFlag(value any) (string, bool) {
	if value == nil {
		return "", true
	}
	return traceDBCallstackText(value, true)
}

func traceDBCallstackName(value any) (string, string) {
	if value == nil {
		return "", "invalid_name_null"
	}
	text, ok := value.(string)
	if !ok {
		return "", "invalid_name_non_text"
	}
	if !utf8.ValidString(text) {
		return "", "invalid_name_utf8"
	}
	if len(text) > maxTraceDBCallstackTokenBytes {
		return "", "invalid_name_oversize"
	}
	if strings.TrimSpace(text) == "" {
		return "", "invalid_name_empty_or_blank"
	}
	if strings.TrimSpace(text) != text {
		return "", "invalid_name_edge_whitespace"
	}
	for _, r := range text {
		if unicode.IsControl(r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return "", "invalid_name_control"
		}
	}
	return text, ""
}

func traceDBCallstackNameOnlyRejection(reason string) bool {
	return strings.HasPrefix(reason, "invalid_name_")
}

func traceDBCallstackSpanName(value string) bool {
	_, reason := traceDBCallstackName(value)
	return reason == ""
}

func appendTraceDBCoverageColumn(columns []string, column string) []string {
	for _, existing := range columns {
		if existing == column {
			return columns
		}
	}
	return append(columns, column)
}

func traceDBCallstackPotentialAsync(flagValue, cookieValue any, hasFlag bool) bool {
	if !hasFlag {
		return false
	}
	flag, ok := flagValue.(string)
	if flagValue == nil {
		flag, ok = "", true
	}
	if ok {
		return cookieValue != nil && flag != "" && flag != "I"
	}
	// Any row-level flag that is not a proven sync flag may be a malformed
	// async endpoint.  Poisoning the async lane prevents a later valid finish
	// from bridging across the rejected row and minting a false long span.
	return cookieValue != nil
}

func traceDBCallstackPotentialSync(cookieValue any) bool {
	// In the official schema cookie NULL is the exact synchronous-row signal.
	// Any non-NULL value is async-shaped even when its SQLite storage class is
	// malformed, and must not poison an unrelated synchronous B/E lane.
	return cookieValue == nil
}

func traceDBCallstackOfficialAsyncRaw(flagValue, cookieValue, durValue any, hasFlag bool) bool {
	if cookieValue == nil {
		return false
	}
	if !hasFlag || flagValue == nil {
		return true
	}
	flag, ok := flagValue.(string)
	if !ok {
		return false
	}
	if flag == "" || flag == "I" {
		return true
	}
	duration, durationOK := traceDBStrictSQLiteInt(durValue)
	return durationOK && duration > 0 && (flag == "S" || flag == "C")
}

func traceDBCallstackExactAsyncKey(row traceDBCallstackRow) (traceDBCallstackAsyncKey, bool) {
	if (row.Flag != "S" && row.Flag != "C") || row.OwnerIPID < 0 || row.TGID <= 0 ||
		!traceDBCallstackSpanName(row.Name) || !traceDBCallstackMarkerToken(row.Cookie) {
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

func traceDBCallstackCookie(value any) (string, bool, bool) {
	if value == nil {
		return "", false, true
	}
	typed, ok := value.(int64)
	if !ok {
		return "", false, false
	}
	return strconv.FormatInt(typed, 10), true, true
}

func traceDBCallstackDistributedChainID(value any) (present bool, valid bool) {
	if value == nil {
		return false, true
	}
	text, ok := value.(string)
	if !ok || !utf8.ValidString(text) {
		return false, false
	}
	if text == "" {
		return false, true
	}
	if strings.TrimSpace(text) != text ||
		len(text) > maxTraceDBCallstackTokenBytes {
		return false, false
	}
	for _, r := range text {
		if unicode.IsControl(r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return false, false
		}
	}
	return true, true
}

func traceDBCallstackMarkerToken(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || len(value) > maxTraceDBCallstackTokenBytes || strings.ContainsRune(value, '|') || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
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
