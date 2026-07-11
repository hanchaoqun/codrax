package hitraceconv

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
)

const traceDBPerfAnonymousTask = "perf-unverified"

type traceDBPerfFrame struct {
	Name        string
	DSO         string
	IP          string
	Symbolized  bool
	AddressOnly bool
	DSOKnown    bool
	Degraded    bool
}

type traceDBPerfThreadCatalogEntry struct {
	TID      int64
	PID      int64
	PIDKnown bool
	Name     string
}

type traceDBPerfThreadCatalog struct {
	ByTID   map[int64]traceDBPerfThreadCatalogEntry
	Tainted map[int64]bool
}

type traceDBPerfThreadResolution uint8

const (
	traceDBPerfThreadMissing traceDBPerfThreadResolution = iota
	traceDBPerfThreadResolved
	traceDBPerfThreadPIDConflict
	traceDBPerfThreadAmbiguous
)

type traceDBPerfFileKey struct {
	FileID   int64
	SerialID int64
}

type traceDBPerfFile struct {
	Symbol string
	Path   string
}

type traceDBPerfTextCatalog struct {
	Values  map[int64]string
	Tainted map[int64]bool
}

type traceDBPerfFileCatalog struct {
	Values  map[traceDBPerfFileKey]traceDBPerfFile
	Tainted map[traceDBPerfFileKey]bool
}

type traceDBPerfIdentityResult struct {
	Resolution traceDBPerfThreadResolution
	Thread     traceDBThread
	Process    traceDBProcess
	Task       string
	TID        int64
	PID        int64
	Note       string
}

type traceDBPerfSampleKind uint8

const (
	traceDBPerfSampleKindUnknown traceDBPerfSampleKind = iota
	traceDBPerfSampleKindOnCPU
	traceDBPerfSampleKindOffCPU
)

func (kind traceDBPerfSampleKind) wire() string {
	switch kind {
	case traceDBPerfSampleKindOnCPU:
		return "on_cpu"
	case traceDBPerfSampleKindOffCPU:
		return "off_cpu"
	case traceDBPerfSampleKindUnknown:
		return "unknown"
	default:
		return ""
	}
}

func exportTraceDBPerfSamples(ctx context.Context, tdb *traceDB, sink *traceDBRowSink,
	authority traceDBSchedulerAuthority, running traceDBSchedulerRunningIndex,
) ([]TraceDBCoverage, error) {
	sampleCoverage, err := tdb.inspectCoverage(ctx, "perf", "perf_sample", []string{"callchain_id", "thread_id"})
	sampleCoverage.FieldSources = map[string]string{
		"identity":        "strict perf_thread catalog resolved against the shared canonical identity roster; rejected public-TID lanes remain distinguishable from true absence",
		"lifecycle":       "resolved samples require capture lower-bound and exact shared scheduler thread+positive-process point admission; missing samples remain explicitly anonymous",
		"clock":           "strict calibrated perf_sample.timestamp_trace only; raw timeStamp/timestamp/ts aliases never enter the shared lifecycle timeline",
		"cpu":             "explicit CPU 0..4095 is independent evidence; exact -1/NULL/missing is no-claim and may use the shared lifecycle-filtered typed Running witness only where sample kind permits",
		"sample_kind":     "producer thread_state: Running=on_cpu, Suspend=off_cpu; exact '-' and any missing/unrecognized metadata remain unknown until an exact typed Running witness may upgrade them",
		"stable_identity": "perf_sample.id signed-int32 projection when present; otherwise a provable SQLite hidden rowid",
		"weight":          "present weight column must be a strict positive SQLite INTEGER; only a missing column defaults to 1",
		"wire_identity":   "resolved rows publish canonical identity; true-missing rows use perf-unverified/0/0 and retain source coordinates only in perf_source_* fields",
	}
	if err != nil || !sampleCoverage.Found || len(sampleCoverage.ColumnsMissing) > 0 {
		return []TraceDBCoverage{sampleCoverage}, err
	}
	if !authority.initialized || !running.initialized {
		return []TraceDBCoverage{sampleCoverage}, fmt.Errorf("perf export requires shared scheduler authority and lifecycle Running view")
	}

	hasSourceID, err := tdb.columnExists(ctx, "perf_sample", "id")
	if err != nil {
		return []TraceDBCoverage{sampleCoverage}, err
	}
	tsColumn := "timestamp_trace"
	ok, err := tdb.columnExists(ctx, "perf_sample", tsColumn)
	if err != nil {
		return []TraceDBCoverage{sampleCoverage}, err
	}
	if !ok {
		sampleCoverage.ColumnsMissing = append(sampleCoverage.ColumnsMissing, "timestamp_trace")
		sampleCoverage.Skipped = "missing required columns: " + strings.Join(sampleCoverage.ColumnsMissing, ",")
		return []TraceDBCoverage{sampleCoverage}, nil
	}

	threadCatalog, threadCoverage, err := tdb.loadStrictPerfThreadCatalog(ctx)
	if err != nil {
		return []TraceDBCoverage{sampleCoverage, threadCoverage}, err
	}
	reports, reportCoverage, err := tdb.loadStrictPerfReportCatalog(ctx)
	if err != nil {
		return []TraceDBCoverage{sampleCoverage, threadCoverage, reportCoverage}, err
	}
	files, fileCoverage, err := tdb.loadStrictPerfFileCatalog(ctx)
	if err != nil {
		return []TraceDBCoverage{sampleCoverage, threadCoverage, reportCoverage, fileCoverage}, err
	}
	symbolized, symbolCoverage, err := tdb.loadStrictPerfSymbolCatalog(ctx)
	if err != nil {
		return []TraceDBCoverage{sampleCoverage, threadCoverage, reportCoverage, fileCoverage, symbolCoverage}, err
	}
	dict, dictCoverage, err := tdb.loadStrictPerfDataDict(ctx)
	if err != nil {
		return []TraceDBCoverage{sampleCoverage, threadCoverage, reportCoverage, fileCoverage, symbolCoverage, dictCoverage}, err
	}
	frames, frameCoverage, err := tdb.loadPerfFrames(ctx, dict, files, symbolized)
	coverage := []TraceDBCoverage{sampleCoverage, threadCoverage, reportCoverage, frameCoverage, fileCoverage, symbolCoverage, dictCoverage}
	if err != nil {
		return coverage, err
	}

	stableExpr := ""
	stableOrderExpr := ""
	stableSource := ""
	duplicateStableIDs := map[int64]bool{}
	if hasSourceID {
		stableExpr = quoteSQLiteIdent("id")
		stableOrderExpr = fmt.Sprintf("CASE WHEN %s < 0 THEN 1 ELSE 0 END, %s", stableExpr, stableExpr)
		stableSource = "perf_sample.id signed-int32 projection to canonical uint32 row identity"
		sampleCoverage.ColumnsPresent = appendTraceDBCoverageColumn(sampleCoverage.ColumnsPresent, "id")
		duplicateStableIDs, err = traceDBDuplicateSourceIDs(ctx, tdb, "perf_sample", "id", traceDBActivityITIDSignedInt32.decodeStableRowID)
	} else {
		stableExpr, stableSource, err = traceDBHiddenRowIDExpr(ctx, tdb.db, "perf_sample")
		stableOrderExpr = stableExpr
	}
	if err != nil {
		sampleCoverage.FieldSources["stable_identity"] = "unavailable: no usable perf_sample.id or provable SQLite hidden rowid"
		if sampleCoverage.RowsRead > 0 {
			sampleCoverage.Skipped = fmt.Sprintf("stable_row_identity_unavailable=%d", sampleCoverage.RowsRead)
		}
		coverage[0] = sampleCoverage
		return coverage, nil
	}
	sampleCoverage.FieldSources["stable_identity"] = stableSource + "; same timestamps ordered by canonical stable identity"
	sort.Strings(sampleCoverage.ColumnsPresent)

	cpuColumn, hasCPU, err := traceDBFirstExistingColumn(ctx, tdb, "perf_sample", "cpu_id", "cpu")
	if err != nil {
		return coverage, err
	}
	weightColumn, hasWeight, err := traceDBFirstExistingColumn(ctx, tdb, "perf_sample", "event_count", "period", "sample_period", "sample_weight")
	if err != nil {
		return coverage, err
	}
	hasEventTypeID, err := tdb.columnExists(ctx, "perf_sample", "event_type_id")
	if err != nil {
		return coverage, err
	}
	hasThreadState, err := tdb.columnExists(ctx, "perf_sample", "thread_state")
	if err != nil {
		return coverage, err
	}
	cpuExpr := "NULL"
	if hasCPU {
		cpuExpr = quoteSQLiteIdent(cpuColumn)
	}
	weightExpr := "1"
	if hasWeight {
		weightExpr = quoteSQLiteIdent(weightColumn)
	}
	eventTypeExpr := "NULL"
	if hasEventTypeID {
		eventTypeExpr = quoteSQLiteIdent("event_type_id")
	}
	threadStateExpr := "NULL"
	if hasThreadState {
		threadStateExpr = quoteSQLiteIdent("thread_state")
	}
	query := fmt.Sprintf(`
		SELECT %s, %s, %s, %s, %s, %s, %s, %s
		FROM perf_sample
		ORDER BY %s, %s
	`, stableExpr, quoteSQLiteIdent(tsColumn), cpuExpr, quoteSQLiteIdent("thread_id"), weightExpr,
		eventTypeExpr, quoteSQLiteIdent("callchain_id"), threadStateExpr, quoteSQLiteIdent(tsColumn), stableOrderExpr)
	rows, err := tdb.db.QueryContext(ctx, query)
	if err != nil {
		sampleCoverage.Error = err.Error()
		coverage[0] = sampleCoverage
		return coverage, err
	}
	defer rows.Close()
	skipped := map[string]int{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return coverage, err
		}
		var stableRaw, tsRaw, cpuRaw, tidRaw, weightRaw, eventTypeRaw, callchainIDRaw, threadStateRaw any
		if err := rows.Scan(&stableRaw, &tsRaw, &cpuRaw, &tidRaw, &weightRaw, &eventTypeRaw, &callchainIDRaw, &threadStateRaw); err != nil {
			sampleCoverage.Error = err.Error()
			coverage[0] = sampleCoverage
			return coverage, err
		}
		stableID, stableOK := traceDBStrictSQLiteInt(stableRaw)
		if hasSourceID {
			stableID, stableOK = traceDBActivityITIDSignedInt32.decodeStableRowID(stableRaw)
		}
		if !stableOK {
			skipped["invalid_stable_id"]++
			continue
		}
		if duplicateStableIDs[stableID] {
			skipped["duplicate_stable_id"]++
			continue
		}
		ts, ok := traceDBStrictSQLiteInt(tsRaw)
		if !ok || ts < 0 {
			skipped["invalid_timestamp"]++
			continue
		}
		tid, ok := traceDBStrictSQLiteInt(tidRaw)
		if !ok || tid <= 0 || tid > math.MaxInt32 {
			skipped["invalid_thread_id"]++
			continue
		}
		callchainID, ok := traceDBStrictSQLiteInt(callchainIDRaw)
		if !ok || callchainID < -1 || callchainID > maxTraceDBInternalID {
			skipped["invalid_callchain_id"]++
			continue
		}
		weight := int64(1)
		if hasWeight {
			weight, ok = traceDBStrictSQLiteInt(weightRaw)
			if !ok || weight <= 0 {
				skipped["invalid_weight"]++
				continue
			}
		}
		kind, stateMetadataDegraded := traceDBDecodePerfSampleKind(hasThreadState, threadStateRaw)
		if stateMetadataDegraded {
			skipped["thread_state_metadata_degraded"]++
		}

		catalogEntry, catalogKnown := threadCatalog.ByTID[tid]
		pid, pidKnown, perfName := int64(0), false, ""
		if catalogKnown {
			pid, pidKnown, perfName = catalogEntry.PID, catalogEntry.PIDKnown, catalogEntry.Name
		}
		identity := traceDBResolvePerfSampleIdentity(authority.identities, threadCatalog, tid, pid, pidKnown, perfName)
		switch identity.Resolution {
		case traceDBPerfThreadPIDConflict:
			skipped["pid_conflict"]++
			continue
		case traceDBPerfThreadAmbiguous:
			skipped["ambiguous_identity"]++
			continue
		case traceDBPerfThreadResolved:
			if traceDBBeforeCaptureStart(authority.identities, ts) {
				skipped["before_capture_start"]++
				continue
			}
			if !authority.threadPointAllows(identity.Thread.ITID, ts) {
				skipped["lifecycle_rejected"]++
				continue
			}
		case traceDBPerfThreadMissing:
			if traceDBBeforeCaptureStart(authority.identities, ts) {
				skipped["before_capture_start"]++
				continue
			}
		default:
			skipped["ambiguous_identity"]++
			continue
		}

		cpu, cpuKnown, kind, kindNote, cpuReason := traceDBResolvePerfSampleCPU(cpuRaw, hasCPU, kind, identity, running, ts)
		if cpuReason != "" {
			skipped[cpuReason]++
			continue
		}
		if !cpuKnown {
			// The systrace transport has no honest unknown-CPU header. Every
			// surviving SQL sample therefore needs either an explicit source CPU
			// or an exact typed Running witness.
			skipped["unknown_cpu"]++
			continue
		}

		eventType := "perf"
		if hasEventTypeID {
			eventTypeID, valid := traceDBStrictSQLiteInt(eventTypeRaw)
			if !valid || eventTypeID < 0 {
				skipped["event_type_metadata_degraded"]++
			} else if label := reports[eventTypeID]; label != "" {
				eventType = label
			}
		}
		sampleFrames := frames[callchainID]
		leaf := traceDBPerfFrame{Name: "perf_sample"}
		if len(sampleFrames) > 0 {
			leaf = sampleFrames[len(sampleFrames)-1]
		}
		symbol := firstNonEmpty(leaf.Name, "perf_sample")
		dso := firstNonEmpty(leaf.DSO, "unknown")
		ip := firstNonEmpty(leaf.IP, "")
		symbolizationStatus, callchainStatus := traceDBPerfSymbolizationStatus(sampleFrames)
		callchain := ""
		if callchainStatus != "missing" {
			var complete bool
			callchain, complete = traceDBPerfCallchain(sampleFrames)
			if !complete {
				callchainStatus = "partial"
				skipped["callchain_wire_degraded"]++
			}
		}

		body := ""
		if identity.Resolution == traceDBPerfThreadMissing {
			sourcePID := int64(0)
			if pidKnown {
				sourcePID = pid
			}
			body = fmt.Sprintf("perf_sample: cpu=%d cpu_known=true pid=0 tid=0 thread_comm=%s sample_weight=%d event=%s symbol=%s dso=%s ip=%s callchain=%s source=trace_streamer_db sample_kind=%s symbolization_status=%s clock=trace_streamer_db clock_confidence=calibrated callchain_status=%s thread_identity_known=false resolution=perf_source_only lifecycle_unverified=true perf_source_tid=%d perf_source_pid=%d perf_source_comm=%s%s",
				cpu, quoteTraceValue(""), weight, quoteTraceValue(traceDBPerfLabel(eventType)), quoteTraceValue(symbol), quoteTraceValue(dso), quoteTraceValue(ip), quoteTraceValue(callchain), kind.wire(), symbolizationStatus, callchainStatus,
				tid, sourcePID, quoteTraceValue(traceDBIdentityDisplayText(perfName)), kindNote)
			if err := addTraceDBInstantRow(sink, ts, traceDBPerfAnonymousTask, 0, 0, cpu, body); err != nil {
				return coverage, err
			}
		} else {
			body = fmt.Sprintf("perf_sample: cpu=%d cpu_known=true pid=%d tid=%d thread_comm=%s sample_weight=%d event=%s symbol=%s dso=%s ip=%s callchain=%s source=trace_streamer_db sample_kind=%s symbolization_status=%s clock=trace_streamer_db clock_confidence=calibrated callchain_status=%s thread_identity_known=true resolution=resolved lifecycle_unverified=false%s",
				cpu, identity.PID, identity.TID, quoteTraceValue(identity.Task), weight, quoteTraceValue(traceDBPerfLabel(eventType)),
				quoteTraceValue(symbol), quoteTraceValue(dso), quoteTraceValue(ip), quoteTraceValue(callchain), kind.wire(), symbolizationStatus, callchainStatus, kindNote)
			body += identity.Note
			if err := addTraceDBInstantRow(sink, ts, identity.Task, identity.TID, identity.PID, cpu, body); err != nil {
				return coverage, err
			}
		}
		sampleCoverage.RowsEmitted++
	}
	if err := rows.Err(); err != nil {
		sampleCoverage.Error = err.Error()
		coverage[0] = sampleCoverage
		return coverage, err
	}
	sampleCoverage.Skipped = traceDBCountSummary(skipped)
	coverage[0] = sampleCoverage
	return coverage, nil
}

func traceDBDecodePerfSampleKind(hasColumn bool, value any) (traceDBPerfSampleKind, bool) {
	if !hasColumn || value == nil {
		return traceDBPerfSampleKindUnknown, false
	}
	text, ok := value.(string)
	if !ok {
		return traceDBPerfSampleKindUnknown, true
	}
	switch text {
	case "Running":
		return traceDBPerfSampleKindOnCPU, false
	case "Suspend":
		return traceDBPerfSampleKindOffCPU, false
	case "-":
		return traceDBPerfSampleKindUnknown, false
	default:
		return traceDBPerfSampleKindUnknown, true
	}
}

func traceDBResolvePerfSampleCPU(raw any, hasColumn bool, kind traceDBPerfSampleKind,
	identity traceDBPerfIdentityResult, running traceDBSchedulerRunningIndex, ts int64,
) (int64, bool, traceDBPerfSampleKind, string, string) {
	noClaim := !hasColumn || raw == nil
	explicitCPU := int64(0)
	if !noClaim {
		value, ok := traceDBStrictSQLiteInt(raw)
		if !ok {
			return 0, false, kind, "", "invalid_cpu"
		}
		if value == -1 {
			noClaim = true
		} else if !validTraceDBCPUIndex(value) {
			return 0, false, kind, "", "invalid_cpu"
		} else {
			explicitCPU = value
		}
	}
	if !noClaim {
		// Keep the explicit value below while auditing any typed scheduler
		// witness required by the producer's sample kind.
	}
	if identity.Resolution != traceDBPerfThreadResolved {
		if !noClaim {
			return explicitCPU, true, kind, "", ""
		}
		return 0, false, kind, "", "anonymous_cpu_unclaimed"
	}
	if kind == traceDBPerfSampleKindOffCPU {
		if !noClaim {
			return explicitCPU, true, kind, "", ""
		}
		return 0, false, kind, "", "off_cpu_cpu_unclaimed"
	}
	cpu, status := running.lookupCPUAt(identity.Thread.ITID, ts)
	if kind == traceDBPerfSampleKindUnknown {
		if status == traceDBSchedulerRunningKnown {
			if noClaim {
				return cpu, true, traceDBPerfSampleKindOnCPU, " sample_kind_source=scheduler_running", ""
			}
			if explicitCPU == cpu {
				return explicitCPU, true, traceDBPerfSampleKindOnCPU, " sample_kind_source=scheduler_running", ""
			}
			return explicitCPU, true, kind, "", ""
		}
		if !noClaim {
			return explicitCPU, true, kind, "", ""
		}
	} else if kind == traceDBPerfSampleKindOnCPU {
		if status == traceDBSchedulerRunningKnown {
			if !noClaim && explicitCPU != cpu {
				return 0, false, kind, "", "cpu_conflict_with_running"
			}
			if noClaim {
				return cpu, true, kind, "", ""
			}
			return explicitCPU, true, kind, "", ""
		}
		if !noClaim {
			return explicitCPU, true, kind, "", ""
		}
	}
	switch status {
	case traceDBSchedulerRunningSourceTainted:
		return 0, false, kind, "", "tainted_running_cpu_witness"
	case traceDBSchedulerRunningLifecycleRejected:
		return 0, false, kind, "", "lifecycle_rejected_running_cpu_witness"
	default:
		return 0, false, kind, "", "unknown_running_cpu_witness"
	}
}

func traceDBResolvePerfSampleIdentity(index traceDBThreadIndex, catalog traceDBPerfThreadCatalog,
	tid, pid int64, pidKnown bool, perfName string,
) traceDBPerfIdentityResult {
	result := traceDBPerfIdentityResult{Resolution: traceDBPerfThreadMissing}
	if catalog.Tainted[tid] || index.RejectedPublicTID[tid] {
		result.Resolution = traceDBPerfThreadAmbiguous
		return result
	}
	items := index.ByTIDCandidates[tid]
	if len(items) == 0 {
		if index.ObservedPublicTID[tid] {
			result.Resolution = traceDBPerfThreadAmbiguous
		}
		return result
	}
	var matches []traceDBThread
	for _, item := range items {
		if index.AmbiguousITID[item.ITID] || index.AmbiguousIPID[item.IPID] {
			result.Resolution = traceDBPerfThreadAmbiguous
			return result
		}
		process, ok := index.Processes[item.IPID]
		if !ok || process.IPID != item.IPID || process.PID <= 0 || process.PID > math.MaxInt32 {
			continue
		}
		if !pidKnown || process.PID == pid {
			matches = append(matches, item)
		}
	}
	if len(matches) > 1 {
		result.Resolution = traceDBPerfThreadAmbiguous
		return result
	}
	if len(matches) == 0 {
		if pidKnown {
			result.Resolution = traceDBPerfThreadPIDConflict
		} else {
			result.Resolution = traceDBPerfThreadAmbiguous
		}
		return result
	}
	thread := matches[0]
	process := index.Processes[thread.IPID]
	result.Resolution = traceDBPerfThreadResolved
	result.Thread = thread
	result.Process = process
	result.TID = thread.TID
	result.PID = process.PID
	result.Task = traceDBCommName(thread.Name, "perf")
	perfTask := traceDBCommName(perfName, "perf")
	if strings.TrimSpace(perfName) != "" && !strings.EqualFold(result.Task, perfTask) {
		result.Note = fmt.Sprintf(" perf_thread_comm=%s comm_source=trace_thread", quoteTraceValue(perfTask))
	}
	if !pidKnown {
		result.Note += " process_id_source=trace_thread"
	}
	return result
}

func (tdb *traceDB) loadStrictPerfThreadCatalog(ctx context.Context) (traceDBPerfThreadCatalog, TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "resolver.perf", "perf_thread", []string{"thread_id"})
	out := traceDBPerfThreadCatalog{ByTID: map[int64]traceDBPerfThreadCatalogEntry{}, Tainted: map[int64]bool{}}
	coverage.FieldSources = map[string]string{
		"identity": "single physical scan; strict positive public thread_id plus optional strict positive process_id",
		"cohort":   "exact (TID,PID-presence,PID) duplicates fold; rename is display-only; mixed/malformed PID claims taint the complete TID lane",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return out, coverage, err
	}
	hasPID, err := tdb.columnExists(ctx, "perf_thread", "process_id")
	if err != nil {
		return out, coverage, err
	}
	hasName, err := tdb.columnExists(ctx, "perf_thread", "thread_name")
	if err != nil {
		return out, coverage, err
	}
	pidExpr, nameExpr := "NULL", "NULL"
	if hasPID {
		pidExpr = quoteSQLiteIdent("process_id")
	}
	if hasName {
		nameExpr = quoteSQLiteIdent("thread_name")
	}
	rows, err := tdb.db.QueryContext(ctx, fmt.Sprintf("SELECT thread_id, %s, %s FROM perf_thread", pidExpr, nameExpr))
	if err != nil {
		coverage.Error = err.Error()
		return out, coverage, err
	}
	defer rows.Close()
	skipped := map[string]int{}
	for rows.Next() {
		var tidRaw, pidRaw, nameRaw any
		if err := rows.Scan(&tidRaw, &pidRaw, &nameRaw); err != nil {
			coverage.Error = err.Error()
			return out, coverage, err
		}
		tid, tidOK := traceDBStrictSQLiteInt(tidRaw)
		if !tidOK || tid <= 0 || tid > math.MaxInt32 {
			skipped["invalid_thread_id"]++
			continue
		}
		pid, pidKnown := int64(0), false
		if pidRaw != nil {
			pid, pidKnown = traceDBStrictSQLiteInt(pidRaw)
			if !pidKnown || pid <= 0 || pid > math.MaxInt32 {
				delete(out.ByTID, tid)
				out.Tainted[tid] = true
				skipped["tainted_process_id_lane"]++
				continue
			}
		}
		if out.Tainted[tid] {
			skipped["tainted_thread_lane"]++
			continue
		}
		name := traceDBIdentityDisplayText(nameRaw)
		item := traceDBPerfThreadCatalogEntry{TID: tid, PID: pid, PIDKnown: pidKnown, Name: name}
		if prior, exists := out.ByTID[tid]; exists {
			if prior.PIDKnown != item.PIDKnown || prior.PIDKnown && prior.PID != item.PID {
				delete(out.ByTID, tid)
				out.Tainted[tid] = true
				skipped["ambiguous_thread_lane"]++
				continue
			}
			prior.Name = traceDBPreferDisplayName(prior.Name, item.Name)
			out.ByTID[tid] = prior
			continue
		}
		out.ByTID[tid] = item
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return out, coverage, err
	}
	coverage.RowsEmitted = len(out.ByTID)
	coverage.Skipped = traceDBCountSummary(skipped)
	return out, coverage, nil
}

func (tdb *traceDB) loadStrictPerfReportCatalog(ctx context.Context) (map[int64]string, TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "resolver.perf", "perf_report", []string{"id", "report_type", "report_value"})
	out := map[int64]string{}
	coverage.FieldSources = map[string]string{
		"materialization": "independent strict id/report_type/value scan; only a globally unique id row with exact report_type=config_name labels events; any duplicate/malformed cohort degrades to event=perf",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return out, coverage, err
	}
	duplicateIDs, err := traceDBDuplicateSourceIDs(ctx, tdb, "perf_report", "id", traceDBStrictNonNegativeRowID)
	if err != nil {
		return out, coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, "SELECT id, report_type, report_value FROM perf_report")
	if err != nil {
		coverage.Error = err.Error()
		return out, coverage, err
	}
	defer rows.Close()
	poisoned := map[int64]bool{}
	skipped := map[string]int{}
	for rows.Next() {
		var idRaw, typeRaw, valueRaw any
		if err := rows.Scan(&idRaw, &typeRaw, &valueRaw); err != nil {
			coverage.Error = err.Error()
			return out, coverage, err
		}
		id, idOK := traceDBStrictSQLiteInt(idRaw)
		reportType, typeOK := typeRaw.(string)
		if !idOK || id < 0 {
			skipped["invalid_id"]++
			continue
		}
		if duplicateIDs[id] {
			delete(out, id)
			poisoned[id] = true
			skipped["duplicate_id"]++
			continue
		}
		if !typeOK || reportType != "config_name" {
			skipped["non_config_name"]++
			continue
		}
		value, valueOK := valueRaw.(string)
		if !valueOK || len(value) > maxTraceDBIdentityDisplayBytes || !traceDBSinglePhysicalLine(value, true) {
			delete(out, id)
			poisoned[id] = true
			skipped["invalid_value"]++
			continue
		}
		value = traceDBPerfOptionalLabel(value)
		if value == "" {
			delete(out, id)
			poisoned[id] = true
			skipped["invalid_value"]++
			continue
		}
		if poisoned[id] {
			skipped["poisoned_id"]++
			continue
		}
		if prior, exists := out[id]; exists && prior != value {
			delete(out, id)
			poisoned[id] = true
			skipped["conflicting_id"]++
			continue
		}
		out[id] = value
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return out, coverage, err
	}
	coverage.RowsEmitted = len(out)
	coverage.Skipped = traceDBCountSummary(skipped)
	return out, coverage, nil
}

func (tdb *traceDB) loadStrictPerfDataDict(ctx context.Context) (traceDBPerfTextCatalog, TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "resolver.perf", "data_dict", []string{"id", "data"})
	out := traceDBPerfTextCatalog{Values: map[int64]string{}, Tainted: map[int64]bool{}}
	coverage.FieldSources = map[string]string{
		"materialization": "perf-local strict id/TEXT catalog; exact duplicates fold and conflicting metadata removes only that dictionary resolver key",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return out, coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, "SELECT id, data FROM data_dict")
	if err != nil {
		coverage.Error = err.Error()
		return out, coverage, err
	}
	defer rows.Close()
	skipped := map[string]int{}
	for rows.Next() {
		var idRaw, dataRaw any
		if err := rows.Scan(&idRaw, &dataRaw); err != nil {
			coverage.Error = err.Error()
			return out, coverage, err
		}
		id, idOK := traceDBStrictSQLiteInt(idRaw)
		data, dataOK := traceDBOptionalStrictText(dataRaw)
		dataOK = dataOK && dataRaw != nil
		if dataOK {
			data = traceDBPerfOptionalLabel(data)
			dataOK = data != ""
		}
		if !idOK || id < 0 {
			skipped["invalid_id"]++
			continue
		}
		if !dataOK {
			delete(out.Values, id)
			out.Tainted[id] = true
			skipped["invalid_value"]++
			continue
		}
		if out.Tainted[id] {
			skipped["poisoned_id"]++
			continue
		}
		if prior, exists := out.Values[id]; exists && prior != data {
			delete(out.Values, id)
			out.Tainted[id] = true
			skipped["conflicting_id"]++
			continue
		}
		out.Values[id] = data
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return out, coverage, err
	}
	coverage.RowsEmitted = len(out.Values)
	coverage.Skipped = traceDBCountSummary(skipped)
	return out, coverage, nil
}

func (tdb *traceDB) loadStrictPerfFileCatalog(ctx context.Context) (traceDBPerfFileCatalog, TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "resolver.perf", "perf_files", []string{"file_id", "serial_id"})
	out := traceDBPerfFileCatalog{Values: map[traceDBPerfFileKey]traceDBPerfFile{}, Tainted: map[traceDBPerfFileKey]bool{}}
	coverage.FieldSources = map[string]string{
		"materialization": "independent strict composite (file_id,serial_id) scan; metadata conflicts remove only that resolver key",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return out, coverage, err
	}
	hasSymbol, err := tdb.columnExists(ctx, "perf_files", "symbol")
	if err != nil {
		return out, coverage, err
	}
	hasPath, err := tdb.columnExists(ctx, "perf_files", "path")
	if err != nil {
		return out, coverage, err
	}
	symbolExpr, pathExpr := "NULL", "NULL"
	if hasSymbol {
		symbolExpr = quoteSQLiteIdent("symbol")
	}
	if hasPath {
		pathExpr = quoteSQLiteIdent("path")
	}
	rows, err := tdb.db.QueryContext(ctx, fmt.Sprintf("SELECT file_id, serial_id, %s, %s FROM perf_files", symbolExpr, pathExpr))
	if err != nil {
		coverage.Error = err.Error()
		return out, coverage, err
	}
	defer rows.Close()
	skipped := map[string]int{}
	for rows.Next() {
		var fileRaw, serialRaw, symbolRaw, pathRaw any
		if err := rows.Scan(&fileRaw, &serialRaw, &symbolRaw, &pathRaw); err != nil {
			coverage.Error = err.Error()
			return out, coverage, err
		}
		fileID, fileOK := traceDBStrictSQLiteInt(fileRaw)
		serialID, serialOK := traceDBStrictSQLiteInt(serialRaw)
		if !fileOK || !serialOK || fileID < 0 || serialID < -1 {
			skipped["invalid_key"]++
			continue
		}
		key := traceDBPerfFileKey{FileID: fileID, SerialID: serialID}
		symbol, symbolOK := traceDBOptionalStrictText(symbolRaw)
		path, pathOK := traceDBOptionalStrictText(pathRaw)
		if !symbolOK || !pathOK {
			delete(out.Values, key)
			out.Tainted[key] = true
			skipped["invalid_metadata"]++
			continue
		}
		item := traceDBPerfFile{Symbol: traceDBPerfOptionalLabel(symbol), Path: traceDBPerfOptionalLabel(path)}
		if out.Tainted[key] {
			skipped["poisoned_key"]++
			continue
		}
		if prior, exists := out.Values[key]; exists && prior != item {
			delete(out.Values, key)
			out.Tainted[key] = true
			skipped["conflicting_key"]++
			continue
		}
		out.Values[key] = item
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return out, coverage, err
	}
	coverage.RowsEmitted = len(out.Values)
	coverage.Skipped = traceDBCountSummary(skipped)
	return out, coverage, nil
}

func (tdb *traceDB) loadStrictPerfSymbolCatalog(ctx context.Context) (traceDBPerfTextCatalog, TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "resolver.perf", "hmtrace_perf_symbolized_frame", []string{"perf_callchain_row_id", "display_name"})
	out := traceDBPerfTextCatalog{Values: map[int64]string{}, Tainted: map[int64]bool{}}
	coverage.FieldSources = map[string]string{
		"materialization": "independent strict non-negative canonical callchain row counter; conflicting symbol metadata removes only that resolver key",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return out, coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, "SELECT perf_callchain_row_id, display_name FROM hmtrace_perf_symbolized_frame")
	if err != nil {
		coverage.Error = err.Error()
		return out, coverage, err
	}
	defer rows.Close()
	skipped := map[string]int{}
	for rows.Next() {
		var idRaw, nameRaw any
		if err := rows.Scan(&idRaw, &nameRaw); err != nil {
			coverage.Error = err.Error()
			return out, coverage, err
		}
		id, idOK := traceDBStrictNonNegativeRowID(idRaw)
		name, nameOK := traceDBOptionalStrictText(nameRaw)
		if nameOK {
			name = traceDBPerfOptionalLabel(name)
		}
		if !idOK {
			skipped["invalid_id"]++
			continue
		}
		if !nameOK || name == "" {
			delete(out.Values, id)
			out.Tainted[id] = true
			skipped["invalid_name"]++
			continue
		}
		if out.Tainted[id] {
			skipped["poisoned_id"]++
			continue
		}
		if prior, exists := out.Values[id]; exists && prior != name {
			delete(out.Values, id)
			out.Tainted[id] = true
			skipped["conflicting_id"]++
			continue
		}
		out.Values[id] = name
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return out, coverage, err
	}
	coverage.RowsEmitted = len(out.Values)
	coverage.Skipped = traceDBCountSummary(skipped)
	return out, coverage, nil
}

func traceDBOptionalStrictText(value any) (string, bool) {
	if value == nil {
		return "", true
	}
	text, ok := value.(string)
	if !ok || len(text) > maxTraceDBIdentityDisplayBytes || !traceDBSinglePhysicalLine(text, true) {
		return "", false
	}
	return text, true
}

func (tdb *traceDB) loadPerfFrames(ctx context.Context, dict traceDBPerfTextCatalog,
	files traceDBPerfFileCatalog, symbolized traceDBPerfTextCatalog,
) (map[int64][]traceDBPerfFrame, TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "perf", "perf_callchain", []string{"callchain_id", "depth"})
	out := map[int64][]traceDBPerfFrame{}
	coverage.FieldSources = map[string]string{
		"materialization": "single physical callchain scan after independent dictionary/file/symbol catalogs; no resolver JOIN or row fanout",
		"stable_identity": "perf_callchain.id strict non-negative canonical row counter when present; otherwise a provable SQLite hidden rowid",
		"depth":           "strict non-negative INTEGER; depth 0 is root, maximum depth is leaf, and wire callchain order is root-to-leaf",
		"conflict_scope":  "malformed or conflicting callchain metadata removes only that callchain resolver cohort; the owning perf sample remains inventory",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return out, coverage, err
	}
	hasID, err := tdb.columnExists(ctx, "perf_callchain", "id")
	if err != nil {
		return out, coverage, err
	}
	stableExpr, stableOrderExpr, stableSource := "", "", ""
	duplicateStableIDs := map[int64]bool{}
	if hasID {
		stableExpr = quoteSQLiteIdent("id")
		stableOrderExpr = stableExpr
		stableSource = "perf_callchain.id strict non-negative canonical row counter"
		duplicateStableIDs, err = traceDBDuplicateSourceIDs(ctx, tdb, "perf_callchain", "id", traceDBStrictNonNegativeRowID)
	} else {
		stableExpr, stableSource, err = traceDBHiddenRowIDExpr(ctx, tdb.db, "perf_callchain")
		stableOrderExpr = stableExpr
	}
	if err != nil {
		coverage.FieldSources["stable_identity"] = "unavailable: no usable perf_callchain.id or provable SQLite hidden rowid"
		if coverage.RowsRead > 0 {
			coverage.Skipped = fmt.Sprintf("stable_row_identity_unavailable=%d", coverage.RowsRead)
		}
		return out, coverage, nil
	}
	coverage.FieldSources["stable_identity"] = stableSource
	hasName, err := tdb.columnExists(ctx, "perf_callchain", "name")
	if err != nil {
		return out, coverage, err
	}
	hasIP, err := tdb.columnExists(ctx, "perf_callchain", "ip")
	if err != nil {
		return out, coverage, err
	}
	hasFileID, err := tdb.columnExists(ctx, "perf_callchain", "file_id")
	if err != nil {
		return out, coverage, err
	}
	hasSymbolID, err := tdb.columnExists(ctx, "perf_callchain", "symbol_id")
	if err != nil {
		return out, coverage, err
	}
	nameExpr, ipExpr, fileExpr, symbolExpr := "NULL", "NULL", "NULL", "NULL"
	if hasName {
		nameExpr = quoteSQLiteIdent("name")
	}
	if hasIP {
		ipExpr = quoteSQLiteIdent("ip")
	}
	if hasFileID {
		fileExpr = quoteSQLiteIdent("file_id")
	}
	if hasSymbolID {
		symbolExpr = quoteSQLiteIdent("symbol_id")
	}
	query := fmt.Sprintf(`
		SELECT %s, callchain_id, depth, %s, %s, %s, %s
		FROM perf_callchain
		ORDER BY callchain_id, depth, %s
	`, stableExpr, nameExpr, ipExpr, fileExpr, symbolExpr, stableOrderExpr)
	rows, err := tdb.db.QueryContext(ctx, query)
	if err != nil {
		coverage.Error = err.Error()
		return out, coverage, err
	}
	defer rows.Close()
	type depthFrame struct {
		frame traceDBPerfFrame
	}
	byDepth := map[int64]map[int64]depthFrame{}
	poisoned := map[int64]bool{}
	skipped := map[string]int{}
	poison := func(callchainID int64, reason string) {
		delete(byDepth, callchainID)
		poisoned[callchainID] = true
		skipped[reason]++
	}
	for rows.Next() {
		var stableRaw, callchainRaw, depthRaw, nameRaw, ipRaw, fileRaw, symbolRaw any
		if err := rows.Scan(&stableRaw, &callchainRaw, &depthRaw, &nameRaw, &ipRaw, &fileRaw, &symbolRaw); err != nil {
			coverage.Error = err.Error()
			return out, coverage, err
		}
		callchainID, callchainOK := traceDBStrictSQLiteInt(callchainRaw)
		if !callchainOK || callchainID < 0 || callchainID > maxTraceDBInternalID {
			skipped["invalid_callchain_id"]++
			continue
		}
		if poisoned[callchainID] {
			skipped["poisoned_callchain"]++
			continue
		}
		stableID, stableOK := traceDBStrictSQLiteInt(stableRaw)
		if hasID {
			stableID, stableOK = traceDBStrictNonNegativeRowID(stableRaw)
		}
		if !stableOK {
			poison(callchainID, "invalid_stable_id")
			continue
		}
		if duplicateStableIDs[stableID] {
			poison(callchainID, "duplicate_stable_id")
			continue
		}
		depth, depthOK := traceDBStrictSQLiteInt(depthRaw)
		if !depthOK || depth < 0 || depth > math.MaxInt32 {
			poison(callchainID, "invalid_depth")
			continue
		}
		resolvedName := symbolized.Values[stableID]
		resolvedNameSymbolized := resolvedName != ""
		symbolBlocked := symbolized.Tainted[stableID]
		degraded := symbolBlocked
		name := ""
		if resolvedName == "" && !symbolBlocked && nameRaw != nil {
			switch value := nameRaw.(type) {
			case string:
				if len(value) > maxTraceDBIdentityDisplayBytes || !traceDBSinglePhysicalLine(value, true) {
					skipped["invalid_name_metadata"]++
					degraded = true
					symbolBlocked = true
				} else {
					name = value
				}
			default:
				nameID, nameIDOK := traceDBStrictSQLiteInt(nameRaw)
				if nameIDOK && nameID == -1 {
					name = ""
				} else if !nameIDOK || nameID < 0 {
					skipped["invalid_name_metadata"]++
					degraded = true
					symbolBlocked = true
				} else if dict.Tainted[nameID] {
					skipped["tainted_name_dictionary"]++
					degraded = true
					symbolBlocked = true
				} else {
					name = dict.Values[nameID]
				}
			}
		}
		ip := ""
		if ipRaw != nil {
			ipValue, ipOK := traceDBStrictSQLiteInt(ipRaw)
			if !ipOK {
				skipped["invalid_ip_metadata"]++
				degraded = true
			} else {
				ip = fmt.Sprintf("0x%x", uint64(ipValue))
			}
		}
		file := traceDBPerfFile{}
		if fileRaw != nil || symbolRaw != nil {
			fileID, fileOK := traceDBStrictSQLiteInt(fileRaw)
			serialID, serialOK := traceDBStrictSQLiteInt(symbolRaw)
			if fileOK && fileID == -1 {
				// INVALID_UINT64: this frame has no file metadata.
			} else if !fileOK || fileID < 0 || !serialOK || serialID < -1 {
				skipped["invalid_file_metadata"]++
				degraded = true
			} else {
				key := traceDBPerfFileKey{FileID: fileID, SerialID: serialID}
				if files.Tainted[key] {
					skipped["tainted_file_metadata"]++
					degraded = true
				} else {
					file = files.Values[key]
				}
			}
		}
		if resolvedName == "" && !symbolBlocked {
			resolvedName = firstNonEmpty(name, file.Symbol)
			resolvedName = traceDBPerfOptionalLabel(resolvedName)
			resolvedNameSymbolized = resolvedName != "" && !traceDBPerfAddressOnlyLabel(resolvedName)
		}
		frame := traceDBPerfFrame{
			Name: firstNonEmpty(traceDBPerfOptionalLabel(resolvedName), ip),
			DSO:  traceDBPerfOptionalLabel(file.Path), IP: ip, Symbolized: resolvedNameSymbolized,
			AddressOnly: resolvedName != "" && !resolvedNameSymbolized,
			DSOKnown:    file.Path != "", Degraded: degraded,
		}
		if byDepth[callchainID] == nil {
			byDepth[callchainID] = map[int64]depthFrame{}
		}
		if _, exists := byDepth[callchainID][depth]; exists {
			poison(callchainID, "conflicting_depth")
			continue
		}
		byDepth[callchainID][depth] = depthFrame{frame: frame}
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return out, coverage, err
	}
	for callchainID, depths := range byDepth {
		if poisoned[callchainID] {
			continue
		}
		depthValues := make([]int64, 0, len(depths))
		for depth := range depths {
			depthValues = append(depthValues, depth)
		}
		sort.Slice(depthValues, func(i, j int) bool { return depthValues[i] < depthValues[j] })
		for _, depth := range depthValues {
			out[callchainID] = append(out[callchainID], depths[depth].frame)
			coverage.RowsEmitted++
		}
	}
	coverage.Skipped = traceDBCountSummary(skipped)
	return out, coverage, nil
}

func traceDBStrictNonNegativeRowID(value any) (int64, bool) {
	raw, ok := traceDBStrictSQLiteInt(value)
	if !ok || raw < 0 {
		return 0, false
	}
	return raw, true
}

func traceDBPerfOptionalLabel(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	return traceDBPerfLabel(raw)
}

func traceDBPerfSymbolizationStatus(frames []traceDBPerfFrame) (symbolization, callchain string) {
	if len(frames) == 0 {
		return "unsymbolized", "missing"
	}
	symbolized := 0
	ipOnly := 0
	usable := 0
	degraded := false
	for _, frame := range frames {
		if frame.Symbolized {
			symbolized++
		}
		if !frame.Symbolized && (frame.IP != "" || frame.AddressOnly) && !frame.DSOKnown {
			ipOnly++
		}
		if frame.Symbolized || frame.IP != "" || frame.AddressOnly || frame.DSOKnown {
			usable++
		}
		degraded = degraded || frame.Degraded
	}
	switch {
	case usable == 0:
		return "unsymbolized", "missing"
	case symbolized == len(frames) && !degraded:
		return "symbolized", "symbolized"
	case ipOnly == len(frames) && !degraded:
		return "unsymbolized", "ip_only"
	case symbolized > 0 || usable > 0:
		return "partial", "partial"
	}
	return "unsymbolized", "missing"
}

func traceDBPerfAddressOnlyLabel(value string) bool {
	text := strings.ToLower(strings.TrimSpace(value))
	for _, prefix := range []string{"@0x", "+0x", "0x"} {
		if !strings.HasPrefix(text, prefix) || len(text) == len(prefix) {
			continue
		}
		digits := text[len(prefix):]
		allHex := true
		for _, digit := range digits {
			if !(digit >= '0' && digit <= '9' || digit >= 'a' && digit <= 'f') {
				allHex = false
				break
			}
		}
		if allHex {
			return true
		}
	}
	return false
}
