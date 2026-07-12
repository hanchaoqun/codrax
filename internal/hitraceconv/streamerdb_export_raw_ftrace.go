package hitraceconv

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type traceDBRawEvent struct {
	StableID  int64
	TS        int64
	Name      string
	CPU       int64
	CPUKnown  bool
	ITID      int64
	ITIDKnown bool
	TID       int64
	TIDKnown  bool
	PID       int64
	PIDKnown  bool
	ArgSetID  int64
}

type traceDBRawSubjectKind uint8

const (
	traceDBRawSubjectInvalid traceDBRawSubjectKind = iota
	traceDBRawSubjectCanonicalThread
	traceDBRawSubjectKernelThread
	traceDBRawSubjectIdle
)

type traceDBRawSubject struct {
	Kind traceDBRawSubjectKind
	Task string
	TID  int64
	TGID int64
	ITID int64
}

func exportTraceDBRawFtraceFamilies(ctx context.Context, tdb *traceDB, sink *traceDBRowSink,
	authority traceDBSchedulerAuthority, running traceDBSchedulerRunningIndex, artifactSource string,
) (result []TraceDBCoverage, resultErr error) {
	schemaCoverage, err := tdb.inspectCoverage(ctx, "raw_ftrace", "raw", []string{"ts", "name"})
	if err != nil || !schemaCoverage.Found || len(schemaCoverage.ColumnsMissing) > 0 {
		return []TraceDBCoverage{schemaCoverage}, err
	}
	argsetColumn, ok, err := traceDBFirstExistingColumn(ctx, tdb, "raw", "argset", "argsetid", "argset_id", "arg_set_id")
	if err != nil {
		schemaCoverage.Error = err.Error()
		return []TraceDBCoverage{schemaCoverage}, err
	}
	if !ok {
		schemaCoverage.Skipped = "missing argset/argsetid column; raw ftrace rows cannot be rendered safely"
		return []TraceDBCoverage{schemaCoverage}, nil
	}
	argsets, resolverCoverage, err := tdb.loadArgsets(ctx)
	outCoverage := append([]TraceDBCoverage{schemaCoverage}, resolverCoverage...)
	if err != nil {
		return outCoverage, err
	}
	if !traceDBRawArgsetsReady(resolverCoverage) {
		schemaCoverage.Skipped = "missing args/data_dict dependency; raw ftrace rows cannot be rendered safely"
		outCoverage[0] = schemaCoverage
		return outCoverage, nil
	}
	hasSourceID, err := tdb.columnExists(ctx, "raw", "id")
	if err != nil {
		return outCoverage, err
	}
	stableExpr := "rowid"
	stableSource := "raw.rowid"
	if hasSourceID {
		stableExpr = traceDBQuoteIdent("id")
		stableSource = "raw.id"
	} else if err := traceDBRequireRowID(ctx, tdb, "raw"); err != nil {
		schemaCoverage.Skipped = "missing raw.id and usable SQLite rowid; no stable source identity/order"
		outCoverage[0] = schemaCoverage
		return outCoverage, nil
	}
	if schemaCoverage.FieldSources == nil {
		schemaCoverage.FieldSources = map[string]string{}
	}
	schemaCoverage.FieldSources["stable_identity"] = stableSource
	if hasSourceID {
		schemaCoverage.FieldSources["stable_identity_projection"] = "raw.id exact full-uint32 signed-int32 projection"
		schemaCoverage.FieldSources["same_timestamp_order"] = "raw.ts,canonical_uint32(raw.id)"
	} else {
		schemaCoverage.FieldSources["same_timestamp_order"] = "raw.ts,raw.rowid"
	}
	schemaCoverage.FieldSources["header_cpu"] = "strict raw.cpu when present; otherwise the same lifecycle-filtered typed Running index used by extended consumers"
	schemaCoverage.FieldSources["header_identity"] = "typed raw.itid/raw.tid/raw.pid resolved against the shared scheduler authority; canonical subjects require point admission before either CPU branch"
	schemaCoverage.FieldSources["source_only"] = "never-observed public-TID rows are coverage-only until an independent typed inventory profile exists; standard ftrace headers/payloads are never repurposed to carry anonymous provenance"
	cpuExpr, hasCPU, err := traceDBRawOptionalExpr(ctx, tdb, "raw", "cpu")
	if err != nil {
		schemaCoverage.Error = err.Error()
		return []TraceDBCoverage{schemaCoverage}, err
	}
	// raw.itid is the only proven internal-thread identity.  "callid" is
	// table-family overloaded in Trace Streamer (thread in callstack, CPU in
	// irq), so it must never be generalized into a raw thread identity.
	itidExpr, hasITID, err := traceDBRawOptionalExpr(ctx, tdb, "raw", "itid")
	if err != nil {
		schemaCoverage.Error = err.Error()
		return []TraceDBCoverage{schemaCoverage}, err
	}
	tidExpr, hasTID, err := traceDBRawOptionalExpr(ctx, tdb, "raw", "tid")
	if err != nil {
		schemaCoverage.Error = err.Error()
		return []TraceDBCoverage{schemaCoverage}, err
	}
	pidExpr, hasPID, err := traceDBRawOptionalExpr(ctx, tdb, "raw", "pid")
	if err != nil {
		schemaCoverage.Error = err.Error()
		return []TraceDBCoverage{schemaCoverage}, err
	}
	query := fmt.Sprintf(`
		SELECT %s, ts, name, %s, %s, %s, %s, %s
		FROM raw
	`, stableExpr, cpuExpr, itidExpr, tidExpr, pidExpr, traceDBQuoteIdent(argsetColumn))
	stage, err := newTraceDBRawPairingStage(ctx, artifactSource, traceDBRawPairingStageOptions{})
	if err != nil {
		schemaCoverage.Error = err.Error()
		outCoverage[0] = schemaCoverage
		return outCoverage, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, stage.cleanup())
	}()
	rows, err := tdb.db.QueryContext(ctx, query)
	if err != nil {
		schemaCoverage.Error = err.Error()
		return []TraceDBCoverage{schemaCoverage}, err
	}
	defer rows.Close()
	classCoverage := map[string]*TraceDBCoverage{}
	classSkipped := map[string]map[string]int{}
	schemaSkipped := map[string]int{}
	unsupported := TraceDBCoverage{Family: "raw_ftrace", Table: "unsupported", Role: "unsupported_input", Found: true, Skipped: "unsupported raw ftrace event family"}
	globalStageBudget := ""
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return outCoverage, err
		}
		var raw traceDBRawEvent
		var stableIDRaw, tsRaw, nameRaw, cpuRaw, itidRaw, tidRaw, pidRaw, argsetRaw any
		if err := rows.Scan(&stableIDRaw, &tsRaw, &nameRaw, &cpuRaw, &itidRaw, &tidRaw, &pidRaw, &argsetRaw); err != nil {
			schemaCoverage.Error = err.Error()
			return []TraceDBCoverage{schemaCoverage}, err
		}
		var ok bool
		if hasSourceID {
			raw.StableID, ok = traceDBStrictStableUint32Projection(stableIDRaw)
		} else {
			raw.StableID, ok = traceDBStrictSQLiteInt(stableIDRaw)
		}
		stableKnown := ok && raw.StableID >= 0 && (hasSourceID || raw.StableID != 0)
		raw.TS, ok = traceDBStrictSQLiteInt(tsRaw)
		timestampKnown := ok && raw.TS >= 0
		raw.Name, ok = traceDBStrictArgText(nameRaw, false)
		nameKnown := ok && strings.TrimSpace(raw.Name) == raw.Name && strings.ToLower(raw.Name) == raw.Name
		if !nameKnown {
			raw.Name = ""
		}
		class := traceDBRawFtraceClass(raw.Name)

		args := map[string]traceDBValue(nil)
		invalidKeys := map[string]bool(nil)
		argsKnown := false
		argsetReason := ""
		switch {
		case argsetRaw == nil:
			argsetReason = "missing_argset"
		default:
			raw.ArgSetID, ok = traceDBStrictSQLiteInt(argsetRaw)
			if !ok || raw.ArgSetID < 0 {
				argsetReason = "invalid_argset_id"
			} else if !argsets.Present[raw.ArgSetID] {
				argsetReason = "missing_argset"
			} else if argsets.Invalid[raw.ArgSetID] {
				argsetReason = "invalid_argset"
			} else {
				args = argsets.Sets[raw.ArgSetID]
				invalidKeys = argsets.InvalidKeys[raw.ArgSetID]
				argsKnown = true
			}
		}
		requiredArgsKnown := argsKnown && traceDBRawRequiredArgs(raw.Name, args, invalidKeys)

		identityScalarsValid := true
		identityReason := ""
		if raw.ITID, raw.ITIDKnown, ok = traceDBRawOptionalInternalID(itidRaw, hasITID); !ok {
			identityScalarsValid, identityReason = false, "invalid_itid"
		}
		if raw.TID, raw.TIDKnown, ok = traceDBRawOptionalID(tidRaw, hasTID, math.MaxInt32); !ok {
			identityScalarsValid = false
			if identityReason == "" {
				identityReason = "invalid_tid"
			}
		}
		if raw.PID, raw.PIDKnown, ok = traceDBRawOptionalID(pidRaw, hasPID, math.MaxInt32); !ok {
			identityScalarsValid = false
			if identityReason == "" {
				identityReason = "invalid_pid"
			}
		}
		explicitCPUPresent := hasCPU && cpuRaw != nil
		explicitCPUValid := false
		if explicitCPUPresent {
			raw.CPU, explicitCPUValid = traceDBStrictSQLiteInt(cpuRaw)
			explicitCPUValid = explicitCPUValid && validTraceDBCPUIndex(raw.CPU)
		}

		headerTID, headerOwnerKnown, canonicalITID, canonicalITIDKnown := traceDBRawPairingOwner(raw, authority, identityScalarsValid)
		fingerprintTID := int64(-1)
		if headerOwnerKnown {
			fingerprintTID = headerTID
		}
		verdict := traceDBRawPairingVerdict(raw.Name, fingerprintTID, args, invalidKeys, argsKnown)
		laneKey := ""
		if key, laneOK := pairingEndpointLaneKey(verdict, stage.artifactSource); laneOK {
			laneKey = key
		}

		subject := traceDBRawSubject{}
		subjectReason := identityReason
		if subjectReason == "" && timestampKnown {
			subject, subjectReason = traceDBResolveRawSubject(raw, authority,
				explicitCPUPresent, traceDBRawMayFeedPairing(raw.Name))
		} else if subjectReason == "" {
			subjectReason = "invalid_timestamp"
		}
		if subjectReason == "source_only_inventory_withheld" && explicitCPUPresent && !explicitCPUValid {
			// Source-only rows have no lifecycle gate to audit. Preserve the
			// strict malformed scalar reason instead of hiding it behind the
			// independent capability-only withholding policy.
			subjectReason = "invalid_cpu"
		}
		cpuReason := ""
		if subjectReason == "" && explicitCPUPresent {
			if explicitCPUValid {
				raw.CPUKnown = true
			} else {
				cpuReason = "invalid_cpu"
			}
		} else if subjectReason == "" {
			var status traceDBSchedulerRunningLookupStatus
			raw.CPU, status = running.lookupCPUAt(subject.ITID, raw.TS)
			switch status {
			case traceDBSchedulerRunningKnown:
				raw.CPUKnown = true
			case traceDBSchedulerRunningSourceTainted:
				cpuReason = "tainted_running_cpu_witness"
			case traceDBSchedulerRunningLifecycleRejected:
				cpuReason = "lifecycle_rejected_running_cpu_witness"
			default:
				cpuReason = "unknown_running_cpu_witness"
			}
		}

		body, renderOK := "", false
		if requiredArgsKnown {
			body, renderOK = traceDBRenderRawFtrace(raw.Name, args, invalidKeys)
		}
		endpointAdmitted := !verdict.Recognized || headerOwnerKnown && verdict.KeyKnown &&
			verdict.PayloadAdmitted && verdict.EmitterKnown && verdict.EmitterAdmitted

		rowReason := ""
		switch {
		case argsetReason != "":
			rowReason = argsetReason
		case !requiredArgsKnown:
			rowReason = "missing_required_args"
		case identityReason != "":
			rowReason = identityReason
		case subjectReason != "":
			rowReason = subjectReason
		case cpuReason != "":
			rowReason = cpuReason
		case !raw.CPUKnown:
			rowReason = "unknown_running_cpu_witness"
		case !renderOK:
			rowReason = "render_rejected"
		case !endpointAdmitted:
			rowReason = "pairing_endpoint_rejected"
		}

		publishable := stableKnown && timestampKnown && nameKnown && class != "" && rowReason == ""
		line := ""
		if publishable {
			if verdict.Recognized && !traceDBRawPairingWireParity(raw.Name, body, subject.TID, verdict) {
				return outCoverage, &traceDBOutputInvariantError{Reason: "raw_pairing_typed_wire_parity_mismatch"}
			}
			rendered, renderErr := prepareTraceDBRenderedRow(raw.TS, 0, subject.Task, subject.TID, subject.TGID, raw.CPU, body)
			if renderErr != nil {
				return outCoverage, renderErr
			}
			line = rendered.line
		}

		observation := traceDBRawPairingObservation{
			StableID: raw.StableID, StableKnown: stableKnown,
			Timestamp: raw.TS, TimestampKnown: timestampKnown,
			Class: class, Line: line, Publishable: publishable,
			Verdict: verdict, LaneKey: laneKey, HeaderOwnerKnown: headerOwnerKnown,
			CanonicalITID: canonicalITID, CanonicalITIDKnown: canonicalITIDKnown,
			EndpointAdmitted: verdict.Recognized && endpointAdmitted && publishable,
		}
		if stageErr := stage.add(ctx, observation); stageErr != nil {
			if reason, budget := traceDBRawPairingStageBudgetReason(stageErr); budget {
				if reason == traceDBRawPairingBudgetLaneKeyCap {
					if class != "" {
						traceDBRawCountSkip(classSkipped, class, "pairing_family_lane_key_cap")
					}
				} else if globalStageBudget == "" {
					globalStageBudget = reason
				}
			} else {
				return outCoverage, stageErr
			}
		}

		switch {
		case !stableKnown:
			schemaSkipped["invalid_stable_id"]++
		case !timestampKnown:
			schemaSkipped["invalid_timestamp"]++
		case !nameKnown:
			schemaSkipped["invalid_event_name"]++
		case class == "":
			unsupported.RowsRead++
		default:
			item := traceDBRawClassCoverage(classCoverage, class)
			item.RowsRead++
			if rowReason != "" {
				traceDBRawCountSkip(classSkipped, class, rowReason)
			}
		}
		if globalStageBudget != "" {
			// The whole raw publication family is already fail-closed and the
			// schema preflight carries the total row inventory. Stop decoding the
			// remaining source rows so an adversarial table cannot turn a precise
			// resource cap into unbounded post-cap CPU work.
			break
		}
	}
	if err := rows.Err(); err != nil {
		schemaCoverage.Error = err.Error()
		return []TraceDBCoverage{schemaCoverage}, err
	}
	if err := rows.Close(); err != nil {
		schemaCoverage.Error = err.Error()
		return []TraceDBCoverage{schemaCoverage}, err
	}
	schemaCoverage.FieldSources["pairing_complete_set"] = "single canonical output-artifact namespace; unsorted source scan into a bounded private typed stage; private indexed raw.id/rowid physical-order audit; seal/freeze before the sole pass-2 publisher"
	schemaCoverage.FieldSources["pairing_identity"] = "tracequery.FingerprintPairingEndpoint is the sole family/phase/key authority; thread names and payload source/path never enter hard identity"
	if globalStageBudget != "" || stage.budgetReason != "" {
		if globalStageBudget == "" {
			globalStageBudget = stage.budgetReason
		}
		schemaSkipped["pairing_stage_budget_fail_closed_"+globalStageBudget]++
		schemaCoverage.FieldSources["pairing_stage_backend"] = "private SQLite 0700 workspace/0600 database; raw publication fail-closed before pass 2"
	} else {
		sealReport, sealErr := stage.seal(ctx)
		if sealErr != nil {
			if reason, budget := traceDBRawPairingStageBudgetReason(sealErr); budget {
				schemaSkipped["pairing_stage_budget_fail_closed_"+reason]++
				globalStageBudget = reason
			} else {
				return outCoverage, sealErr
			}
		} else {
			emittedByClass := map[string]int{}
			publishReport, publishErr := stage.publish(ctx, sink, emittedByClass)
			if publishErr != nil {
				if reason, budget := traceDBRawPairingStageBudgetReason(publishErr); budget {
					schemaSkipped["pairing_stage_budget_fail_closed_"+reason]++
					globalStageBudget = reason
				} else {
					return outCoverage, publishErr
				}
			} else {
				for class, count := range emittedByClass {
					traceDBRawClassCoverage(classCoverage, class).RowsEmitted += count
					schemaCoverage.RowsEmitted += count
				}
				if publishReport.SuppressedRows > 0 {
					schemaSkipped["pairing_or_duplicate_rows_fail_closed"] = int(publishReport.SuppressedRows)
				}
			}
			if sealReport.DuplicatePhysicalRows > 0 {
				schemaSkipped["duplicate_source_id"] = int(sealReport.DuplicatePhysicalRows)
			}
			schemaCoverage.FieldSources["pairing_stage_backend"] = fmt.Sprintf("private SQLite; peak_temp_bytes=%d; poisoned_lanes=%d; poisoned_families=%d",
				sealReport.PeakTempBytes, sealReport.PoisonedLanes, sealReport.PoisonedFamilies)
		}
	}
	if globalStageBudget != "" && schemaCoverage.FieldSources["pairing_stage_backend"] == "" {
		schemaCoverage.FieldSources["pairing_stage_backend"] = "private indexed SQLite stage; complete raw publication fail-closed before the first pass-2 row; reason=" + globalStageBudget
	}
	outCoverage[0] = schemaCoverage
	for _, key := range sortedRawFtraceCoverageKeys(classCoverage) {
		if summary := traceDBCountSummary(classSkipped[key]); summary != "" {
			classCoverage[key].Skipped = summary
		}
		outCoverage = append(outCoverage, *classCoverage[key])
	}
	if unsupported.RowsRead > 0 {
		outCoverage = append(outCoverage, unsupported)
	}
	if summary := traceDBCountSummary(schemaSkipped); summary != "" {
		outCoverage[0].Skipped = summary
	}
	return outCoverage, nil
}

func traceDBRawArgsetsReady(coverage []TraceDBCoverage) bool {
	for _, item := range coverage {
		if item.Table != "args" && item.Table != "data_dict" {
			continue
		}
		if !item.Found || len(item.ColumnsMissing) > 0 {
			return false
		}
	}
	return true
}

func traceDBRawOptionalExpr(ctx context.Context, tdb *traceDB, table string, names ...string) (string, bool, error) {
	name, ok, err := traceDBFirstExistingColumn(ctx, tdb, table, names...)
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "NULL", false, nil
	}
	return traceDBQuoteIdent(name), true, nil
}

func traceDBRawOptionalID(value any, columnPresent bool, maxValue int64) (int64, bool, bool) {
	if !columnPresent || value == nil {
		return 0, false, true
	}
	parsed, ok := traceDBStrictSQLiteInt(value)
	if !ok || parsed < 0 || parsed > maxValue {
		return 0, false, false
	}
	return parsed, true, true
}

func traceDBRawOptionalInternalID(value any, columnPresent bool) (int64, bool, bool) {
	if !columnPresent || value == nil {
		return 0, false, true
	}
	parsed, ok := traceDBStrictSignedUint32Projection(value)
	if !ok {
		return 0, false, false
	}
	return parsed, true, true
}

func traceDBRequireRowID(ctx context.Context, tdb *traceDB, table string) error {
	rows, err := tdb.db.QueryContext(ctx, "SELECT rowid FROM "+traceDBQuoteIdent(table)+" LIMIT 0")
	if err != nil {
		return err
	}
	return rows.Close()
}

func traceDBRawDuplicateStableIDs(ctx context.Context, tdb *traceDB, stableExpr string, sourceID bool) (map[int64]bool, error) {
	rows, err := tdb.db.QueryContext(ctx, `SELECT `+stableExpr+` FROM raw`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	seen := map[int64]bool{}
	for rows.Next() {
		var identityRaw any
		if err := rows.Scan(&identityRaw); err != nil {
			return nil, err
		}
		identity, identityOK := traceDBStrictSQLiteInt(identityRaw)
		if sourceID {
			identity, identityOK = traceDBStrictStableUint32Projection(identityRaw)
		}
		if !identityOK || identity < 0 {
			continue
		}
		if seen[identity] {
			out[identity] = true
		}
		seen[identity] = true
	}
	return out, rows.Err()
}

func traceDBRawCountSkip(items map[string]map[string]int, class, reason string) {
	if items[class] == nil {
		items[class] = map[string]int{}
	}
	items[class][reason]++
}

func traceDBQuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func traceDBRawClassCoverage(items map[string]*TraceDBCoverage, class string) *TraceDBCoverage {
	item := items[class]
	if item != nil {
		return item
	}
	item = &TraceDBCoverage{Family: "raw_ftrace", Table: class, Role: "query_ready_export", Found: true}
	items[class] = item
	return item
}

func sortedRawFtraceCoverageKeys(items map[string]*TraceDBCoverage) []string {
	out := make([]string, 0, len(items))
	for key := range items {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func traceDBRawFtraceClass(name string) string {
	if traceDBFilemapNameGoverned(name) {
		return "page_cache"
	}
	lower := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(lower, "binder_"):
		return "binder"
	case lower == "block_rq_issue" || lower == "block_rq_insert" || lower == "block_rq_complete" || lower == "block_rq_remap" ||
		lower == "block_bio_queue" || lower == "block_bio_complete" || lower == "block_bio_remap" ||
		strings.HasPrefix(lower, "ufshcd_") || strings.HasPrefix(lower, "mmc_request_") || strings.HasPrefix(lower, "scsi_dispatch_cmd"):
		return "block_storage"
	case strings.HasPrefix(lower, "android_fs_dataread") || strings.HasPrefix(lower, "android_fs_datawrite") ||
		strings.HasPrefix(lower, "f2fs_direct_io") || strings.HasPrefix(lower, "f2fs_sync_file"):
		return "file_io"
	case lower == "workqueue_execute_start" || lower == "workqueue_execute_end":
		return "workqueue"
	case strings.HasPrefix(lower, "dma_fence"):
		return "dma_fence"
	default:
		return ""
	}
}

func traceDBRenderRawFtrace(name string, args map[string]traceDBValue, invalidKeys map[string]bool) (string, bool) {
	if directPairNameGoverned(name) {
		payload, ok := traceDBRawPairPayload(name, args, invalidKeys)
		if !ok {
			return "", false
		}
		body, ok := renderCanonicalPairPayload(payload)
		if !ok {
			return "", false
		}
		return name + ": " + body, true
	}
	if traceDBFilemapNameGoverned(name) {
		payload, ok := decodeTraceDBFilemapPayload(name, args, invalidKeys)
		if !ok {
			return "", false
		}
		body, ok := renderCanonicalFilemapPayload(payload)
		if !ok {
			return "", false
		}
		return name + ": " + body, true
	}
	lower := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(lower, "binder_"):
		return traceDBRenderRawBinder(name, args)
	case lower == "block_rq_issue" || lower == "block_rq_insert" || lower == "block_rq_complete" || lower == "block_rq_remap" ||
		lower == "block_bio_queue" || lower == "block_bio_complete" || lower == "block_bio_remap":
		return renderTraceDBBlockEvent(name, args, invalidKeys)
	case strings.HasPrefix(lower, "android_fs_dataread") || strings.HasPrefix(lower, "android_fs_datawrite"):
		return traceDBRenderRawFileIO(name, args, "bytes"), true
	case strings.HasPrefix(lower, "f2fs_direct_io") || strings.HasPrefix(lower, "f2fs_sync_file"):
		return traceDBRenderRawFileIO(name, args, "len"), true
	case strings.HasPrefix(lower, "scsi_dispatch_cmd"):
		return traceDBRenderRawSCSI(name, args), true
	case strings.HasPrefix(lower, "mmc_request_start"):
		return traceDBRenderRawMMCRequestStart(args), true
	case strings.HasPrefix(lower, "mmc_request_done"):
		return traceDBRenderRawMMCRequestDone(args), true
	case strings.HasPrefix(lower, "ufshcd_"):
		return traceDBRenderRawStorageKV(name, args), true
	case lower == "workqueue_execute_start" || lower == "workqueue_execute_end":
		return traceDBRenderRawWorkqueue(name, args), true
	case strings.HasPrefix(lower, "dma_fence"):
		return traceDBRenderRawDMAFence(name, args), true
	default:
		return "", false
	}
}

func traceDBRawRequiredArgs(name string, args map[string]traceDBValue, invalidKeys map[string]bool) bool {
	if traceDBFilemapNameGoverned(name) {
		_, ok := decodeTraceDBFilemapPayload(name, args, invalidKeys)
		return ok
	}
	lower := strings.ToLower(strings.TrimSpace(name))
	require := func(groups ...[]string) bool {
		for _, names := range groups {
			if _, ok := traceDBRawValidatedAlias(args, invalidKeys, true, names...); !ok {
				return false
			}
		}
		return true
	}
	optional := func(groups ...[]string) bool {
		for _, names := range groups {
			if _, ok := traceDBRawValidatedAlias(args, invalidKeys, false, names...); !ok {
				return false
			}
		}
		return true
	}
	requireAny := func(names ...string) bool {
		found := false
		for _, name := range names {
			key := strings.ToLower(strings.TrimSpace(name))
			if invalidKeys[key] {
				return false
			}
			if value, exists := args[key]; exists {
				if !value.Valid || strings.TrimSpace(value.Text) == "" {
					return false
				}
				found = true
			}
		}
		return found
	}
	switch lower {
	case "binder_transaction":
		return require([]string{"transaction", "debug_id", "transaction_id"}, []string{"dest_node", "target_node"},
			[]string{"dest_proc", "target_proc"}, []string{"dest_thread", "target_thread"}, []string{"reply"}, []string{"flags"}, []string{"code"}) &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 1, math.MaxInt64, "transaction", "debug_id", "transaction_id") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "dest_node", "target_node") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt32, "dest_proc", "target_proc") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt32, "dest_thread", "target_thread") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, 1, "reply") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "flags") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "code")
	case "binder_transaction_received":
		return require([]string{"transaction", "debug_id", "transaction_id"}) &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 1, math.MaxInt64, "transaction", "debug_id", "transaction_id")
	case "binder_transaction_alloc_buf", "binder_alloc_buf":
		return require([]string{"transaction", "debug_id", "transaction_id"}, []string{"data_size"}, []string{"offsets_size"}) &&
			optional([]string{"extra_buffers_size", "extra_size"}) &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 1, math.MaxInt64, "transaction", "debug_id", "transaction_id") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "data_size") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "offsets_size") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "extra_buffers_size", "extra_size")
	case "binder_transaction_reply", "binder_reply":
		return require([]string{"transaction", "debug_id", "transaction_id"}) && optional([]string{"tag"}) &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 1, math.MaxInt64, "transaction", "debug_id", "transaction_id")
	case "binder_transaction_lock", "binder_lock", "binder_transaction_locked", "binder_locked", "binder_transaction_unlock", "binder_unlock":
		return require([]string{"tag"}) && traceDBRawWireTextAlias(args, invalidKeys, true, "tag")
	case "block_rq_issue", "block_rq_insert", "block_rq_complete", "block_rq_remap",
		"block_bio_queue", "block_bio_complete", "block_bio_remap":
		_, ok := decodeTraceDBBlockPayload(lower, args, invalidKeys)
		return ok
	case "mmc_request_start":
		return require([]string{"name", "dev_name"}, []string{"tag"}, []string{"cmd_opcode", "opcode"},
			[]string{"blocks"}, []string{"block_size"}, []string{"blk_addr", "lba"}) &&
			traceDBRawWireTextAlias(args, invalidKeys, true, "name", "dev_name") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, math.MinInt64, math.MaxInt64, "tag") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "cmd_opcode", "opcode") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "blocks") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "block_size") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "blk_addr", "lba")
	case "mmc_request_done":
		return require([]string{"name", "dev_name"}, []string{"tag"}, []string{"cmd_opcode", "opcode"},
			[]string{"bytes_xfered", "bytes", "len"}) && requireAny("ret", "cmd_err", "data_err") &&
			optional([]string{"ret"}, []string{"cmd_err"}, []string{"data_err"}) &&
			traceDBRawWireTextAlias(args, invalidKeys, true, "name", "dev_name") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, math.MinInt64, math.MaxInt64, "tag") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "cmd_opcode", "opcode") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "bytes_xfered", "bytes", "len") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, math.MinInt64, math.MaxInt64, "ret") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, math.MinInt64, math.MaxInt64, "cmd_err") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, math.MinInt64, math.MaxInt64, "data_err")
	case "workqueue_execute_start":
		return traceDBRawNonZeroPointer(args, invalidKeys, "work", "addr", "address") &&
			traceDBRawOptionalNonZeroPointer(args, invalidKeys, "function", "func")
	case "workqueue_execute_end":
		return traceDBRawNonZeroPointer(args, invalidKeys, "work", "addr", "address") &&
			traceDBRawOptionalNonZeroPointer(args, invalidKeys, "function", "func")
	}
	switch {
	case strings.HasPrefix(lower, "android_fs_dataread"), strings.HasPrefix(lower, "android_fs_datawrite"):
		return require([]string{"dev", "s_dev", "fs_dev", "dev_t"}, []string{"ino", "inode", "i_ino"}, []string{"bytes", "len", "length", "size"}) &&
			optional([]string{"entry_name", "name", "file", "filename"}, []string{"offset", "ofs", "pos", "off"},
				[]string{"rw", "rwbs", "op", "operation"}, []string{"ret", "res", "error", "err"},
				[]string{"latency_us", "duration_us", "time_us", "usecs"}) &&
			traceDBRawDeviceAlias(args, invalidKeys, "dev", "s_dev", "fs_dev", "dev_t") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "ino", "inode", "i_ino") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "bytes", "len", "length", "size") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "offset", "ofs", "pos", "off") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, math.MinInt64, math.MaxInt64, "ret", "res", "error", "err") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "latency_us", "duration_us", "time_us", "usecs")
	case strings.HasPrefix(lower, "f2fs_direct_io"), strings.HasPrefix(lower, "f2fs_sync_file"):
		return require([]string{"dev", "s_dev", "fs_dev", "dev_t"}, []string{"ino", "inode", "i_ino"}) &&
			optional([]string{"entry_name", "name", "file", "filename"}, []string{"offset", "ofs", "pos", "off"},
				[]string{"bytes", "len", "length", "size"}, []string{"rw", "rwbs", "op", "operation"},
				[]string{"ret", "res", "error", "err"}, []string{"latency_us", "duration_us", "time_us", "usecs"}) &&
			traceDBRawDeviceAlias(args, invalidKeys, "dev", "s_dev", "fs_dev", "dev_t") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "ino", "inode", "i_ino") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "offset", "ofs", "pos", "off") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "bytes", "len", "length", "size") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, math.MinInt64, math.MaxInt64, "ret", "res", "error", "err") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "latency_us", "duration_us", "time_us", "usecs")
	case strings.HasPrefix(lower, "scsi_dispatch_cmd"):
		return require([]string{"tag"}, []string{"dev", "sdev", "dev_t"}, []string{"lba", "sector"},
			[]string{"len", "length", "bytes", "transfer_len"}) && requireAny("opcode", "op", "rw", "rwbs") &&
			optional([]string{"ret", "res", "error", "err"}, []string{"latency_us", "duration_us", "time_us", "usecs"}) &&
			traceDBRawIntegerAlias(args, invalidKeys, true, math.MinInt64, math.MaxInt64, "tag") &&
			traceDBRawDeviceAlias(args, invalidKeys, "dev", "sdev", "dev_t") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "lba", "sector") &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "len", "length", "bytes", "transfer_len") &&
			traceDBRawAnyWireText(args, invalidKeys, "opcode", "op", "rw", "rwbs") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, math.MinInt64, math.MaxInt64, "ret", "res", "error", "err") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "latency_us", "duration_us", "time_us", "usecs")
	case strings.HasPrefix(lower, "ufshcd_"):
		return require([]string{"tag"}) && requireAny("opcode", "op", "rw", "rwbs") &&
			optional([]string{"dev", "dev_name", "devname"}, []string{"lba", "sector"},
				[]string{"len", "length", "bytes", "transfer_len", "size"}, []string{"ret", "res", "error", "err"},
				[]string{"latency_us", "duration_us", "time_us", "usecs"}) &&
			traceDBRawIntegerAlias(args, invalidKeys, true, 0, math.MaxInt64, "tag") &&
			traceDBRawAnyWireText(args, invalidKeys, "opcode", "op", "rw", "rwbs") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "lba", "sector") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "len", "length", "bytes", "transfer_len", "size") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, math.MinInt64, math.MaxInt64, "ret", "res", "error", "err") &&
			traceDBRawIntegerAlias(args, invalidKeys, false, 0, math.MaxInt64, "latency_us", "duration_us", "time_us", "usecs")
	case strings.HasPrefix(lower, "dma_fence"):
		return require([]string{"driver"}, []string{"timeline"}, []string{"context"}, []string{"seqno"}) &&
			traceDBRawWireTextAlias(args, invalidKeys, true, "driver") &&
			traceDBRawWireTextAlias(args, invalidKeys, true, "timeline") &&
			traceDBRawUnsignedIntegerAlias(args, invalidKeys, true, "context") &&
			traceDBRawUnsignedIntegerAlias(args, invalidKeys, true, "seqno")
	}
	return false
}

func traceDBRawValidatedAlias(args map[string]traceDBValue, invalidKeys map[string]bool, required bool, names ...string) (string, bool) {
	valueText := ""
	var datatype int64
	found := false
	for _, name := range names {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" || invalidKeys[key] {
			return "", false
		}
		value, exists := args[key]
		if !exists {
			continue
		}
		text := strings.TrimSpace(value.Text)
		if !value.Valid || text == "" || value.Text != text {
			return "", false
		}
		if found && (text != valueText || value.Datatype != datatype) {
			return "", false
		}
		valueText = text
		datatype = value.Datatype
		found = true
	}
	if required && !found {
		return "", false
	}
	return valueText, true
}

func traceDBRawIntegerAlias(args map[string]traceDBValue, invalidKeys map[string]bool, required bool, minValue, maxValue int64, names ...string) bool {
	text, ok := traceDBRawValidatedAlias(args, invalidKeys, required, names...)
	if !ok || text == "" {
		return ok && !required
	}
	for _, name := range names {
		if value, exists := args[strings.ToLower(strings.TrimSpace(name))]; exists && value.Datatype != 0 {
			return false
		}
	}
	value, err := strconv.ParseInt(text, 10, 64)
	return err == nil && value >= minValue && value <= maxValue
}

func traceDBRawUnsignedIntegerAlias(args map[string]traceDBValue, invalidKeys map[string]bool, required bool, names ...string) bool {
	text, ok := traceDBRawValidatedAlias(args, invalidKeys, required, names...)
	if !ok || text == "" {
		return ok && !required
	}
	for _, name := range names {
		if value, exists := args[strings.ToLower(strings.TrimSpace(name))]; exists && value.Datatype != 0 {
			return false
		}
	}
	_, err := strconv.ParseUint(text, 10, 64)
	return err == nil
}

func traceDBRawWireTextAlias(args map[string]traceDBValue, invalidKeys map[string]bool, required bool, names ...string) bool {
	text, ok := traceDBRawValidatedAlias(args, invalidKeys, required, names...)
	if !ok || text == "" {
		return ok && !required
	}
	for _, name := range names {
		if value, exists := args[strings.ToLower(strings.TrimSpace(name))]; exists && value.Datatype != 1 {
			return false
		}
	}
	return !strings.ContainsAny(text, " \t\r\n=")
}

func traceDBRawAnyWireText(args map[string]traceDBValue, invalidKeys map[string]bool, names ...string) bool {
	found := false
	for _, name := range names {
		key := strings.ToLower(strings.TrimSpace(name))
		if invalidKeys[key] {
			return false
		}
		value, exists := args[key]
		if !exists {
			continue
		}
		text := strings.TrimSpace(value.Text)
		if !value.Valid || value.Datatype != 1 || text == "" || value.Text != text || strings.ContainsAny(text, " \t\r\n=") {
			return false
		}
		found = true
	}
	return found
}

func traceDBRawDeviceAlias(args map[string]traceDBValue, invalidKeys map[string]bool, names ...string) bool {
	text, ok := traceDBRawValidatedAlias(args, invalidKeys, true, names...)
	if !ok {
		return false
	}
	var datatype int64 = -1
	for _, name := range names {
		if value, exists := args[strings.ToLower(strings.TrimSpace(name))]; exists {
			datatype = value.Datatype
			break
		}
	}
	if datatype == 0 {
		value, err := strconv.ParseInt(text, 10, 64)
		return err == nil && value >= 0
	}
	if datatype != 1 || strings.ContainsAny(text, " \t\r\n=") {
		return false
	}
	separator := ""
	if strings.Count(text, ":") == 1 && !strings.Contains(text, ",") {
		separator = ":"
	} else if strings.Count(text, ",") == 1 && !strings.Contains(text, ":") {
		separator = ","
	}
	if separator == "" {
		_, err := strconv.ParseUint(text, 10, 64)
		return err == nil
	}
	major, minor, _ := strings.Cut(text, separator)
	_, majorErr := strconv.ParseUint(major, 10, 32)
	_, minorErr := strconv.ParseUint(minor, 10, 32)
	return majorErr == nil && minorErr == nil
}

func traceDBRawNonZeroPointer(args map[string]traceDBValue, invalidKeys map[string]bool, names ...string) bool {
	text, ok := traceDBRawValidatedAlias(args, invalidKeys, true, names...)
	if !ok {
		return false
	}
	for _, name := range names {
		if value, exists := args[strings.ToLower(strings.TrimSpace(name))]; exists && value.Datatype != 0 {
			return false
		}
	}
	base := 10
	valueText := text
	if strings.HasPrefix(valueText, "0x") || strings.HasPrefix(valueText, "0X") {
		base = 16
		valueText = valueText[2:]
	}
	value, err := strconv.ParseUint(valueText, base, 64)
	return err == nil && value > 0
}

func traceDBRawOptionalNonZeroPointer(args map[string]traceDBValue, invalidKeys map[string]bool, names ...string) bool {
	if !traceDBRawAliasPresence(args, names...) {
		for _, name := range names {
			if invalidKeys[strings.ToLower(strings.TrimSpace(name))] {
				return false
			}
		}
		return true
	}
	return traceDBRawNonZeroPointer(args, invalidKeys, names...)
}

func traceDBRenderRawBinder(name string, args map[string]traceDBValue) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "binder_transaction":
		return fmt.Sprintf("%s: transaction=%s dest_node=%s dest_proc=%s dest_thread=%s reply=%s flags=%s code=%s",
			name,
			traceDBRawArg(args, "0", "transaction", "debug_id", "transaction_id"),
			traceDBRawArg(args, "0", "dest_node", "target_node"),
			traceDBRawArg(args, "0", "dest_proc", "target_proc"),
			traceDBRawArg(args, "0", "dest_thread", "target_thread"),
			traceDBRawArg(args, "0", "reply"),
			traceDBRawHexArg(args, "0x0", "flags"),
			traceDBRawHexArg(args, "0x0", "code")), true
	case "binder_transaction_received":
		return fmt.Sprintf("%s: transaction=%s", name, traceDBRawArg(args, "0", "transaction", "debug_id", "transaction_id")), true
	case "binder_transaction_alloc_buf", "binder_alloc_buf":
		parts := []string{
			name + ":",
			"transaction=" + traceDBRawArg(args, "", "transaction", "debug_id", "transaction_id"),
			"debug_id=" + traceDBRawArg(args, "", "debug_id", "transaction"),
			"data_size=" + traceDBRawArg(args, "", "data_size"),
			"offsets_size=" + traceDBRawArg(args, "", "offsets_size"),
		}
		if extra := traceDBRawArg(args, "", "extra_buffers_size", "extra_size"); extra != "" {
			parts = append(parts, "extra_buffers_size="+extra)
		}
		return strings.Join(parts, " "), true
	case "binder_transaction_reply", "binder_reply":
		parts := []string{
			name + ":",
			"transaction=" + traceDBRawArg(args, "", "transaction", "debug_id", "transaction_id"),
			"debug_id=" + traceDBRawArg(args, "", "debug_id", "transaction"),
		}
		if tag := traceDBRawArg(args, "", "tag"); tag != "" {
			parts = append(parts, "tag="+tag)
		}
		return strings.Join(parts, " "), true
	case "binder_transaction_lock", "binder_lock", "binder_transaction_locked", "binder_locked", "binder_transaction_unlock", "binder_unlock":
		return fmt.Sprintf("%s: tag=%s", name, traceDBRawArg(args, "", "tag")), true
	default:
		return "", false
	}
}

func traceDBRenderRawFileIO(name string, args map[string]traceDBValue, sizeKey string) string {
	parts := []string{name + ":"}
	parts = appendRawKV(parts, "dev", traceDBRawDevArg(args, ":", "dev", "s_dev", "fs_dev", "dev_t"))
	parts = appendRawKV(parts, "ino", traceDBRawArg(args, "", "ino", "inode", "i_ino"))
	parts = appendRawKV(parts, "entry_name", traceDBRawArg(args, "", "entry_name", "name", "file", "filename"))
	parts = appendRawKV(parts, "offset", traceDBRawArg(args, "", "offset", "ofs", "pos", "off"))
	parts = appendRawKV(parts, sizeKey, traceDBRawArg(args, "", "bytes", "len", "length", "size"))
	parts = appendRawKV(parts, "rw", firstNonEmpty(traceDBRawArg(args, "", "rw", "rwbs", "op", "operation"), traceIOOperationFromName(name)))
	parts = appendRawKV(parts, "ret", traceDBRawArg(args, "", "ret", "res", "error", "err"))
	parts = appendRawKV(parts, "latency_us", traceDBRawArg(args, "", "latency_us", "duration_us", "time_us", "usecs"))
	return strings.Join(parts, " ")
}

func traceDBRenderRawSCSI(name string, args map[string]traceDBValue) string {
	parts := []string{name + ":"}
	parts = appendRawKV(parts, "tag", traceDBRawArg(args, "", "tag"))
	parts = appendRawKV(parts, "dev", traceDBRawDevArg(args, ":", "dev", "sdev", "dev_t"))
	parts = appendRawKV(parts, "lba", traceDBRawArg(args, "", "lba", "sector"))
	parts = appendRawKV(parts, "len", traceDBRawArg(args, "", "len", "length", "bytes", "transfer_len"))
	parts = appendRawKV(parts, "opcode", traceDBRawArg(args, "", "opcode", "op", "rw", "rwbs"))
	parts = appendRawKV(parts, "ret", traceDBRawArg(args, "", "ret", "res", "error", "err"))
	parts = appendRawKV(parts, "latency_us", traceDBRawArg(args, "", "latency_us", "duration_us", "time_us", "usecs"))
	return strings.Join(parts, " ")
}

func traceDBRenderRawMMCRequestStart(args map[string]traceDBValue) string {
	name := traceDBRawArg(args, "mmc0", "name", "dev_name")
	return fmt.Sprintf("mmc_request_start: %s tag=%s opcode=%s blocks=%s block_size=%s blk_addr=%s",
		name,
		traceDBRawArg(args, "0", "tag"),
		traceDBRawArg(args, "0", "cmd_opcode", "opcode"),
		traceDBRawArg(args, "0", "blocks"),
		traceDBRawArg(args, "0", "block_size"),
		traceDBRawArg(args, "0", "blk_addr", "lba"))
}

func traceDBRenderRawMMCRequestDone(args map[string]traceDBValue) string {
	name := traceDBRawArg(args, "mmc0", "name", "dev_name")
	parts := []string{fmt.Sprintf("mmc_request_done: %s", name),
		"tag=" + traceDBRawArg(args, "0", "tag"),
		"opcode=" + traceDBRawArg(args, "0", "cmd_opcode", "opcode"),
		"bytes_xfered=" + traceDBRawArg(args, "0", "bytes_xfered", "bytes", "len")}
	parts = appendRawKV(parts, "ret", traceDBRawArg(args, "", "ret"))
	parts = appendRawKV(parts, "cmd_err", traceDBRawArg(args, "", "cmd_err"))
	parts = appendRawKV(parts, "data_err", traceDBRawArg(args, "", "data_err"))
	return strings.Join(parts, " ")
}

func traceDBRenderRawStorageKV(name string, args map[string]traceDBValue) string {
	parts := []string{name + ":"}
	for _, item := range []struct {
		key   string
		names []string
	}{
		{"dev", []string{"dev", "dev_name", "devname"}},
		{"tag", []string{"tag"}},
		{"lba", []string{"lba", "sector"}},
		{"len", []string{"len", "length", "bytes", "transfer_len", "size"}},
		{"opcode", []string{"opcode", "op", "rw", "rwbs"}},
		{"ret", []string{"ret", "res", "error", "err"}},
		{"latency_us", []string{"latency_us", "duration_us", "time_us", "usecs"}},
	} {
		parts = appendRawKV(parts, item.key, traceDBRawArg(args, "", item.names...))
	}
	return strings.Join(parts, " ")
}

func traceDBRenderRawWorkqueue(name string, args map[string]traceDBValue) string {
	parts := []string{name + ":", "work=" + traceDBRawHexArg(args, "", "work", "addr", "address")}
	if function := traceDBRawHexArg(args, "", "function", "func"); function != "" {
		parts = append(parts, "function="+function)
	}
	return strings.Join(parts, " ")
}

func traceDBRenderRawDMAFence(name string, args map[string]traceDBValue) string {
	parts := []string{}
	parts = appendRawKV(parts, "driver", traceDBRawArg(args, "", "driver"))
	parts = appendRawKV(parts, "timeline", traceDBRawArg(args, "", "timeline"))
	parts = appendRawKV(parts, "context", traceDBRawArg(args, "", "context"))
	parts = appendRawKV(parts, "seqno", traceDBRawArg(args, "", "seqno"))
	return name + ": " + strings.Join(parts, " ")
}

func appendRawKV(parts []string, key, value string) []string {
	if strings.TrimSpace(value) == "" {
		return parts
	}
	return append(parts, key+"="+value)
}

func traceDBRawArg(args map[string]traceDBValue, fallback string, names ...string) string {
	for _, name := range names {
		if value := traceDBRawArgExact(args, name); value != "" {
			return value
		}
	}
	return fallback
}

func traceDBRawArgExact(args map[string]traceDBValue, name string) string {
	value := args[strings.ToLower(strings.TrimSpace(name))]
	if value.Valid {
		return strings.TrimSpace(value.Text)
	}
	return ""
}

func traceDBRawHexArg(args map[string]traceDBValue, fallback string, names ...string) string {
	text := traceDBRawArg(args, "", names...)
	if text == "" {
		return fallback
	}
	if strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") {
		return text
	}
	if value, err := strconv.ParseInt(text, 10, 64); err == nil {
		return fmt.Sprintf("0x%x", value)
	}
	return text
}

func traceDBRawDevArg(args map[string]traceDBValue, sep string, names ...string) string {
	text := traceDBRawArg(args, "", names...)
	if text == "" {
		return ""
	}
	if strings.ContainsAny(text, ":,") {
		return text
	}
	if value, err := strconv.ParseInt(text, 10, 64); err == nil {
		return devMajorMinor(value, sep)
	}
	return text
}

func traceDBRawMayFeedPairing(name string) bool {
	class := traceDBRawFtraceClass(name)
	return class != "" && class != "page_cache"
}

func traceDBResolveRawSubject(raw traceDBRawEvent, authority traceDBSchedulerAuthority,
	hasExplicitCPU, pairingCapable bool,
) (traceDBRawSubject, string) {
	if !authority.initialized {
		return traceDBRawSubject{}, "missing_scheduler_authority"
	}
	if traceDBBeforeCaptureStart(authority.identities, raw.TS) {
		return traceDBRawSubject{}, "pre_capture_timestamp"
	}
	if raw.ITIDKnown && raw.ITID == 0 {
		if raw.TIDKnown && raw.TID != 0 || raw.PIDKnown && raw.PID != 0 {
			return traceDBRawSubject{}, "idle_public_identity_conflict"
		}
		schedulerSubject, ok := authority.schedulerSubjectFromExactITID(0, true)
		if !ok || !authority.schedulerPointAllows(schedulerSubject, raw.TS) {
			return traceDBRawSubject{}, "idle_lifecycle_rejected"
		}
		return traceDBRawSubject{Kind: traceDBRawSubjectIdle, Task: "swapper", ITID: 0}, ""
	}

	if raw.ITIDKnown {
		thread, process, resolution := authority.resolveThreadSubject(raw.ITID)
		if resolution != traceDBSchedulerThreadResolved {
			return traceDBRawSubject{}, "canonical_subject_unresolved"
		}
		if !traceDBRawPublicClaimsMatch(raw, thread, process) {
			return traceDBRawSubject{}, "canonical_public_identity_conflict"
		}
		return traceDBAdmitRawCanonicalSubject(authority, thread, process, raw.TS)
	}

	if !raw.TIDKnown || raw.TID == 0 {
		return traceDBRawSubject{}, "missing_thread_identity"
	}
	if authority.identities.RejectedPublicTID[raw.TID] {
		return traceDBRawSubject{}, "rejected_public_tid_candidate"
	}
	items := authority.identities.ByTIDCandidates[raw.TID]
	if len(items) == 0 {
		if authority.identities.ObservedPublicTID[raw.TID] {
			return traceDBRawSubject{}, "rejected_public_tid_candidate"
		}
		if pairingCapable {
			return traceDBRawSubject{}, "source_only_pairing_forbidden"
		}
		if !hasExplicitCPU {
			return traceDBRawSubject{}, "source_only_requires_explicit_cpu"
		}
		return traceDBRawSubject{}, "source_only_inventory_withheld"
	}

	var selectedThread traceDBThread
	var selectedProcess traceDBProcess
	matches := 0
	for _, candidate := range items {
		thread, process, resolution := authority.resolveThreadSubject(candidate.ITID)
		if resolution != traceDBSchedulerThreadResolved {
			continue
		}
		if raw.PIDKnown && process.PID != raw.PID {
			continue
		}
		selectedThread = thread
		selectedProcess = process
		matches++
	}
	if matches == 0 {
		if raw.PIDKnown {
			return traceDBRawSubject{}, "canonical_pid_conflict"
		}
		return traceDBRawSubject{}, "canonical_subject_unresolved"
	}
	if matches != 1 {
		return traceDBRawSubject{}, "canonical_subject_ambiguous"
	}
	return traceDBAdmitRawCanonicalSubject(authority, selectedThread, selectedProcess, raw.TS)
}

func traceDBAdmitRawCanonicalSubject(authority traceDBSchedulerAuthority, thread traceDBThread,
	process traceDBProcess, timestamp int64,
) (traceDBRawSubject, string) {
	if !traceDBRawThreadIdentityValid(thread) || process.IPID != thread.IPID ||
		process.PID < 0 || process.PID > math.MaxInt32 {
		return traceDBRawSubject{}, "canonical_subject_unresolved"
	}
	if !authority.threadPointAllows(thread.ITID, timestamp) {
		return traceDBRawSubject{}, "lifecycle_rejected_subject"
	}
	name := thread.Name
	if _, ok := traceDBStrictArgText(name, true); !ok {
		name = "unknown"
	}
	subject := traceDBRawSubject{
		Kind: traceDBRawSubjectCanonicalThread, Task: traceDBCommName(name, "unknown"),
		TID: thread.TID, TGID: process.PID, ITID: thread.ITID,
	}
	if process.PID == 0 {
		subject.Kind = traceDBRawSubjectKernelThread
		subject.TGID = thread.TID
	}
	return subject, ""
}

func traceDBRawPublicClaimsMatch(raw traceDBRawEvent, thread traceDBThread, process traceDBProcess) bool {
	if process.PID == 0 {
		return (!raw.TIDKnown || raw.TID == thread.TID) &&
			(!raw.PIDKnown || raw.PID == 0)
	}
	return (!raw.TIDKnown || raw.TID == thread.TID) &&
		(!raw.PIDKnown || raw.PID == process.PID)
}

func traceDBRawThreadIdentityValid(thread traceDBThread) bool {
	return thread.ITID > 0 && thread.ITID <= maxTraceDBInternalID &&
		thread.TID > 0 && thread.TID <= math.MaxInt32 && thread.IPID >= 0 &&
		thread.IPID <= maxTraceDBInternalID
}
