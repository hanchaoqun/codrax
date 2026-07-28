package hitraceconv

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type traceDBSchedSlice struct {
	TS  int64
	Dur int64
	CPU int64
	// Preserve the exact internal source identities used by the authority;
	// never reconstruct them later from display TID/TGID values.
	ITID     int64
	IPID     int64
	EndState string
	Priority int64
	TID      int64
	TGID     int64
	Name     string
}

type traceDBSchedSourceRow struct {
	Slice            traceDBSchedSlice
	Subject          traceDBSchedulerSubject
	CPUAssigned      bool
	CPUInRange       bool
	CPUAssignmentGap string
	TSValid          bool
	DurationNull     bool
	DurationValid    bool
	ValidationGaps   []string
}

type traceDBSchedCPUAudit struct {
	Rows    int
	Reasons map[string]int
}

type traceDBSchedAudit struct {
	RowsRead                  int
	CPUs                      map[int64]*traceDBSchedCPUAudit
	InvalidCPURows            int
	InvalidCPUValues          []int64
	InvalidCPUValuesTruncated bool
	UnassignedCPURows         int
	UnassignedCPUReasons      map[string]int
	OpenTailRows              int
	ExpectedBoundaryRows      int
	PeakBufferedSourceRows    int
}

type traceDBWakeupInstant struct {
	TS         int64
	Name       string
	Ref        int64
	WakeupFrom int64
}

type traceDBWakeupKey struct {
	TS   int64
	Name string
}

func exportTraceDBSchedulerFamilies(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, syncSpans *traceDBSyncSpanAuthority) (
	coverage []TraceDBCoverage,
	authority traceDBSchedulerAuthority,
	err error,
) {
	if syncSpans == nil {
		return nil, authority, &traceDBOutputInvariantError{Reason: "missing_sync_span_authority"}
	}
	var lifecycleCoverage []TraceDBCoverage
	var invalidHandoffAuthority traceDBSchedulerAuthority
	defer func() {
		coverage = append(coverage, lifecycleCoverage...)
		if err != nil {
			// A scheduler stage failure invalidates the handoff as a whole.
			// Keep partial coverage for diagnosis, but never expose a reusable
			// authority to a caller that accidentally ignores the error.
			authority = invalidHandoffAuthority
		}
	}()
	index, threadCoverage, err := tdb.loadThreadIndex(ctx)
	coverage = append(coverage, threadCoverage...)
	if err != nil {
		return coverage, authority, err
	}
	lifecycle, err := collectTraceDBLifecycle(ctx, tdb.db, index)
	coverage = append(coverage, lifecycle.ActiveCoverage...)
	lifecycleCoverage = lifecycle.LifecycleCoverage
	if err != nil {
		return coverage, authority, err
	}
	authority = newTraceDBSchedulerAuthority(index, lifecycle)
	rawWakeupNameCoverage := traceDBApplyRawWakeupNewDisplayNames(
		tdb.sourceNameInventory, &authority)
	traceDBApplyRawWakeupNameRecoveryToThreadCoverage(
		coverage, authority, rawWakeupNameCoverage.RowsEmitted)
	coverage = append(coverage, rawWakeupNameCoverage)
	active := lifecycle.ActiveITIDs
	stageStart := time.Now()
	metadataCoverage, err := exportTraceDBThreadRegistrations(ctx, sink, syncSpans, index, active)
	traceDBSetCoverageElapsed(&metadataCoverage, stageStart)
	coverage = append(coverage, metadataCoverage)
	if err != nil {
		return coverage, authority, err
	}
	stageStart = time.Now()
	schedCoverage, err := exportTraceDBSchedSwitch(ctx, tdb, sink, authority)
	traceDBSetCoverageElapsed(&schedCoverage, stageStart)
	coverage = append(coverage, schedCoverage)
	if err != nil {
		return coverage, authority, err
	}
	rawWakeups, rawCoverage, err := tdb.loadRawWakeups(ctx)
	coverage = append(coverage, rawCoverage)
	if err != nil {
		return coverage, authority, err
	}
	starts, startsCoverage, err := tdb.loadSchedStarts(ctx, authority)
	coverage = append(coverage, startsCoverage)
	if err != nil {
		return coverage, authority, err
	}
	running, runningCoverage, err := tdb.loadSchedulerRunningIndex(ctx, authority)
	coverage = append(coverage, runningCoverage)
	if err != nil {
		return coverage, authority, err
	}
	stageStart = time.Now()
	wakeupCoverage, err := exportTraceDBWakeups(ctx, tdb, sink, authority, rawWakeups, starts, running)
	traceDBSetCoverageElapsed(&wakeupCoverage, stageStart)
	coverage = append(coverage, wakeupCoverage)
	if err != nil {
		return coverage, authority, err
	}
	stageStart = time.Now()
	blockedCoverage, err := exportTraceDBBlockedReasons(ctx, tdb, sink, authority)
	traceDBSetCoverageElapsed(&blockedCoverage, stageStart)
	coverage = append(coverage, blockedCoverage)
	if err != nil {
		return coverage, authority, err
	}
	stageStart = time.Now()
	irqCoverage, err := exportTraceDBIRQ(ctx, tdb, sink)
	traceDBSetCoverageElapsed(&irqCoverage, stageStart)
	coverage = append(coverage, irqCoverage)
	return coverage, authority, err
}

func exportTraceDBThreadRegistrations(ctx context.Context, sink *traceDBRowSink, syncSpans *traceDBSyncSpanAuthority, index traceDBThreadIndex, active map[int64]bool) (TraceDBCoverage, error) {
	coverage := TraceDBCoverage{
		Family:   "metadata",
		Table:    "thread",
		Role:     "query_ready_export",
		Found:    len(index.ByITID) > 0,
		RowsRead: len(index.ByITID),
		FieldSources: map[string]string{
			"registration_wire": "task_rename publishes immediately; zero-duration B/E is staged in the shared typed sync-span authority",
			"stable_identity":   "canonical thread ITID; exact canonical idle ITID/TID/TGID zero is admitted only when the shared identity index proves it",
		},
	}
	unknownTimestamps := 0
	unprovenIdle := 0
	for _, thread := range sortedTraceDBThreads(index.ByITID) {
		if err := ctx.Err(); err != nil {
			return coverage, err
		}
		process := index.Processes[thread.IPID]
		isMain := thread.IsMainThread || (process.PID != 0 && thread.TID == process.PID)
		if isMain && thread.SwitchCount <= 0 && !active[thread.ITID] {
			continue
		}
		// This is display/registration metadata only. It must never be consumed
		// as a thread birth or generation boundary.
		ts, timestampKnown := traceDBRegistrationTimestamp(thread.RegistrationHint, index.TraceStart, index.TraceStartKnown)
		if !timestampKnown {
			unknownTimestamps++
			continue
		}
		displayName, _ := traceDBThreadDisplayName(index, thread)
		task := traceDBCommName(displayName, "unknown")
		processName := traceDBProcessName(index, thread)
		threadComm := traceDBCommName(displayName, "unknown")
		if isMain {
			threadComm = traceDBCommName(processName, threadComm)
		}
		tgid := firstNonZero(process.PID, thread.TID)
		if thread.TID == 0 && (tgid != 0 || thread.ITID != 0 || !traceDBCanonicalIdleIdentityExact(index)) {
			unprovenIdle++
			continue
		}
		if err := addTraceDBInstantRow(sink, ts, task, thread.TID, tgid, 0,
			fmt.Sprintf("task_rename: pid=%d oldcomm=%s newcomm=%s oom_score_adj=0", thread.TID, threadComm, threadComm)); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted++
		markerName := processName
		if !traceDBCallstackMarkerToken(markerName) {
			markerName = threadComm
		}
		if !traceDBCallstackMarkerToken(markerName) {
			markerName = "unknown"
		}
		if err := syncSpans.submit(ctx, traceDBSyncSpanCandidate{
			Producer:           traceDBSyncSpanProducerRegistration,
			StableKind:         traceDBSyncSpanStableRegistrationITID,
			StableID:           thread.ITID,
			HeaderTID:          thread.TID,
			HeaderTGID:         tgid,
			CanonicalITID:      thread.ITID,
			CanonicalITIDKnown: true,
			OwnerIPID:          thread.IPID,
			OwnerIPIDKnown:     true,
			Start:              ts,
			End:                ts,
			StartCPU:           0,
			EndCPU:             0,
			StartCPUProvenance: traceDBSyncSpanCPURegistrationMetadata,
			EndCPUProvenance:   traceDBSyncSpanCPURegistrationMetadata,
			Task:               task,
			Name:               markerName,
			NameProvenance:     traceDBSyncSpanNameRegistration,
			DepthProvenance:    traceDBSyncSpanDepthUnknown,
		}); err != nil {
			return coverage, err
		}
	}
	if unknownTimestamps > 0 {
		traceDBAppendCoverageSkipped(&coverage,
			fmt.Sprintf("%d thread registration(s) skipped: no known registration hint or capture start", unknownTimestamps))
	}
	if unprovenIdle > 0 {
		traceDBAppendCoverageSkipped(&coverage,
			fmt.Sprintf("unproven_idle_registration=%d", unprovenIdle))
	}
	return coverage, nil
}

func exportTraceDBSchedSwitch(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, authority traceDBSchedulerAuthority) (coverage TraceDBCoverage, err error) {
	rawLiteJoin := newTraceDBRawSchedSwitchLiteJoin(tdb.sourceNameInventory)
	defer func() {
		joinCoverage, joinErr := rawLiteJoin.finalize()
		tdb.rawSchedSwitchJoinCoverage = joinCoverage
		if err == nil && joinErr != nil {
			err = joinErr
		}
	}()
	coverage, err = tdb.inspectCoverage(ctx, "scheduler", "sched_slice", []string{"ts", "dur", "cpu", "end_state", "priority", "itid"})
	coverage.FieldSources = map[string]string{
		"boundary_timestamp":     "prev_sched_slice.ts+dur; requires exact equality with next_sched_slice.ts",
		"continuity":             "complete_per_cpu_audit_before_publish; gap_overlap_mid_null_overflow_fail_cpu_lane",
		"header_cpu":             "sched_slice.cpu; strict SQLite INTEGER in range 0..4095",
		"lifecycle":              "same collector authority; positive-duration slices, including a final row, require half-open thread and positive-process generation admission; zero-duration and NULL-duration open-tail rows require point admission",
		"next_identity":          "next_sched_slice.itid->thread.tid/name; canonical itid=0 is swapper; tgid=positive_process.pid_else_thread.tid",
		"open_tail":              "final sched_slice is retained as an unclosed tail; no synthetic idle close",
		"prev_identity":          "prev_sched_slice.itid->thread.tid/name; canonical itid=0 is swapper; tgid=positive_process.pid_else_thread.tid",
		"priority":               "sched_slice.priority; strict signed int32 excluding the upstream INT32_MAX sentinel; Harmony RT 140..159 and raw values above 159 are preserved",
		"state":                  "sched_slice.end_state; nonempty TEXT",
		"source_identity":        "strict sched_slice.itid canonical scalar projected into an exact scheduler subject; missing/default zero cannot acquire idle authority",
		"unknown_comm_witnesses": "bounded unique canonical itid/tid/tgid subjects from emitted boundaries whose display comm is unavailable; diagnosis only and never identity authority",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	audit, err := auditTraceDBSchedSwitchRows(ctx, tdb, authority)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	coverage.RowsRead = audit.RowsRead
	coverage.PeakBuffered = audit.PeakBufferedSourceRows
	coverage.Skipped = traceDBSchedAuditSummary(audit)
	if audit.UnassignedCPURows > 0 {
		return coverage, nil
	}
	if err := rawLiteJoin.auditDBBoundaries(ctx, tdb, authority, audit); err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	rows, err := queryTraceDBSchedSliceRows(ctx, tdb)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	previous := map[int64]traceDBSchedSourceRow{}
	unknownSubjects := map[string]bool{}
	var unknownFirstTS, unknownLastTS int64
	unknownTimestampKnown := false
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return coverage, err
		}
		current, err := scanTraceDBSchedSourceRow(rows, authority)
		if err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		if !current.CPUAssigned || !current.CPUInRange {
			continue
		}
		cpuAudit := audit.CPUs[current.Slice.CPU]
		if cpuAudit == nil || len(cpuAudit.Reasons) > 0 {
			continue
		}
		if len(current.ValidationGaps) > 0 {
			return coverage, fmt.Errorf("sched_slice changed after continuity audit on cpu %d", current.Slice.CPU)
		}
		if prev, ok := previous[current.Slice.CPU]; ok {
			if prev.DurationNull || !prev.DurationValid || !prev.TSValid || !current.TSValid ||
				prev.Slice.TS > math.MaxInt64-prev.Slice.Dur || prev.Slice.TS+prev.Slice.Dur != current.Slice.TS {
				return coverage, fmt.Errorf("sched_slice continuity changed after audit on cpu %d", current.Slice.CPU)
			}
			rawLite := rawLiteJoin.match(prev.Slice, current.Slice)
			if err := emitTraceDBSchedSwitchWithRawLite(sink, prev.Slice, current.Slice, rawLite); err != nil {
				return coverage, err
			}
			coverage.RowsEmitted++
			if strings.TrimSpace(prev.Slice.Name) == "" || strings.TrimSpace(current.Slice.Name) == "" {
				traceDBAddCoverageMetric(&coverage, "boundaries_with_unknown_comm", 1)
				if !unknownTimestampKnown {
					unknownFirstTS = current.Slice.TS
					unknownTimestampKnown = true
				}
				unknownLastTS = current.Slice.TS
				for _, subject := range []traceDBSchedSlice{prev.Slice, current.Slice} {
					if strings.TrimSpace(subject.Name) != "" {
						continue
					}
					unknownSubjects[fmt.Sprintf("itid=%d/tid=%d/tgid=%d",
						subject.ITID, subject.TID, subject.TGID)] = true
				}
			}
		}
		previous[current.Slice.CPU] = current
		if len(previous) > coverage.PeakBuffered {
			coverage.PeakBuffered = len(previous)
		}
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	if coverage.RowsEmitted != audit.ExpectedBoundaryRows {
		return coverage, fmt.Errorf("sched_slice boundary publication mismatch: emitted=%d audited=%d", coverage.RowsEmitted, audit.ExpectedBoundaryRows)
	}
	if len(unknownSubjects) > 0 {
		const witnessLimit = 16
		witnesses := make([]string, 0, len(unknownSubjects))
		for witness := range unknownSubjects {
			witnesses = append(witnesses, witness)
		}
		sort.Strings(witnesses)
		traceDBAddCoverageMetric(&coverage, "unknown_comm_unique_subjects", int64(len(witnesses)))
		if unknownTimestampKnown {
			coverage.Metrics["unknown_comm_first_boundary_ts_ns"] = unknownFirstTS
			coverage.Metrics["unknown_comm_last_boundary_ts_ns"] = unknownLastTS
		}
		omitted := 0
		if len(witnesses) > witnessLimit {
			omitted = len(witnesses) - witnessLimit
			witnesses = witnesses[:witnessLimit]
		}
		if coverage.Metadata == nil {
			coverage.Metadata = map[string]string{}
		}
		coverage.Metadata["unknown_comm_witnesses"] = strings.Join(witnesses, ",")
		traceDBAddCoverageMetric(&coverage, "unknown_comm_witnesses_omitted", int64(omitted))
	}
	return coverage, nil
}

func exportTraceDBWakeups(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, authority traceDBSchedulerAuthority, rawWakeups []traceDBRawWakeup, starts traceDBSchedStartIndex, running traceDBSchedulerRunningIndex) (coverage TraceDBCoverage, err error) {
	rawLiteJoin := newTraceDBRawSchedWakeupLiteJoin(tdb.sourceNameInventory)
	defer func() {
		joinCoverage, joinErr := rawLiteJoin.finalize()
		tdb.rawSchedWakeupJoinCoverage = joinCoverage
		if err == nil && joinErr != nil {
			err = joinErr
		}
	}()
	coverage, err = tdb.inspectCoverage(ctx, "scheduler", "instant", []string{"ts", "name", "ref", "wakeup_from", "ref_type"})
	coverage.FieldSources = map[string]string{
		"header_cpu":                "thread_state.Running.cpu",
		"header_cpu_exact_lite":     "exact source raw page CPU after a one-to-one sched_wakeup_lite/DB edge join; bypasses Running inference only for that matched physical event",
		"header_cpu_unavailable":    "when the emitter Running lookup is missing/ambiguous or source-tainted, preserve the proven dependency as codrax_sched_wakeup_cpu_unavailable/v1 with no physical CPU envelope; lifecycle-rejected emitter intervals remain suppressed",
		"priority":                  "field-level optional inference from the first audited sched_slice at/after the wakeup; wire provenance marks it non-exact and hard priority-inversion gates must not consume it",
		"priority_exact_lite":       "exact positive signed-16 sched_wakeup_lite priority after a one-to-one source raw/DB edge join",
		"running_lifecycle":         "scheduler_lifecycle_gated Running index from the same collector authority; emitter CPU requires a half-open generation-valid interval",
		"wakeup_endpoints":          "after unique raw/instant matching, wakee and waker each require non-idle thread plus positive-process lifecycle point admission from the same authority",
		"raw_identity.sched_waking": "raw.itid==instant.wakeup_from",
		"raw_identity.sched_wakeup": "producer_shape(raw.itid==instant.ref|instant.wakeup_from)+unique_bipartite_matching",
		"sched_wakeup_new":          "instant name preserved; pairs against the upstream raw sched_wakeup shape through one closed canonical matching kind",
		"target_cpu":                "raw.cpu",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT i.ts, i.name, i.ref, i.wakeup_from
		FROM instant i
		WHERE i.name IN ('sched_wakeup', 'sched_wakeup_new', 'sched_waking')
		  AND i.ref_type = 'itid'
		ORDER BY i.ts, i.name, i.ref, i.wakeup_from
	`)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	coverage.RowsRead = 0
	groups := map[traceDBWakeupKey][]traceDBWakeupInstant{}
	skipped := map[string]int{}
	priorityUnknown := 0
	cpuUnavailablePreserved := 0
	cpuUnavailableUnknown := 0
	cpuUnavailableTainted := 0
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return coverage, err
		}
		coverage.RowsRead++
		var tsRaw, nameRaw, refRaw, wakeupFromRaw any
		if err := rows.Scan(&tsRaw, &nameRaw, &refRaw, &wakeupFromRaw); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		ts, tsOK := traceDBStrictSQLiteInt(tsRaw)
		name, nameOK := nameRaw.(string)
		ref, refOK := traceDBStrictSignedUint32Projection(refRaw)
		wakeupFrom, wakeupFromOK := traceDBStrictSignedUint32Projection(wakeupFromRaw)
		if !tsOK || ts < 0 || !nameOK ||
			(name != "sched_wakeup" && name != "sched_wakeup_new" && name != "sched_waking") || !refOK || !wakeupFromOK {
			skipped["invalid_instant_metadata"]++
			continue
		}
		instant := traceDBWakeupInstant{TS: ts, Name: name, Ref: ref, WakeupFrom: wakeupFrom}
		key := traceDBWakeupKey{TS: instant.TS, Name: traceDBWakeupPairingName(instant.Name)}
		groups[key] = append(groups[key], instant)
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	rawGroups := map[traceDBWakeupKey][]traceDBRawWakeup{}
	for _, raw := range rawWakeups {
		key := traceDBWakeupKey{TS: raw.TS, Name: traceDBWakeupPairingName(raw.Name)}
		rawGroups[key] = append(rawGroups[key], raw)
	}
	if err := rawLiteJoin.auditDBEdges(ctx, groups, rawGroups, authority); err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	keys := make([]traceDBWakeupKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].TS == keys[j].TS {
			return keys[i].Name < keys[j].Name
		}
		return keys[i].TS < keys[j].TS
	})
	for _, key := range keys {
		instants := groups[key]
		raws := rawGroups[key]
		matching, reason := traceDBUniqueWakeupMatching(instants, raws)
		if reason != "" {
			skipped[reason] += len(instants)
			continue
		}
		for instantIndex, rawIndex := range matching {
			instant := instants[instantIndex]
			raw := raws[rawIndex]
			woken, _, wakeeResolution := authority.resolveThreadSubject(instant.Ref)
			if wakeeResolution != traceDBSchedulerThreadResolved {
				if instant.Ref == 0 {
					skipped["idle_wakee_endpoint_forbidden"]++
				} else if wakeeResolution == traceDBSchedulerThreadMissing {
					skipped["missing_wakee_thread"]++
				} else {
					skipped["invalid_or_ambiguous_wakee_identity"]++
				}
				continue
			}
			waker, wakerProcess, wakerResolution := authority.resolveThreadSubject(instant.WakeupFrom)
			if wakerResolution != traceDBSchedulerThreadResolved {
				if instant.WakeupFrom == 0 {
					skipped["idle_waker_endpoint_forbidden"]++
				} else if wakerResolution == traceDBSchedulerThreadMissing {
					skipped["missing_waker_thread"]++
				} else {
					skipped["invalid_or_ambiguous_waker_identity"]++
				}
				continue
			}
			if !authority.threadPointAllows(instant.Ref, instant.TS) {
				skipped["lifecycle_rejected_wakee_endpoint"]++
				continue
			}
			if !authority.threadPointAllows(instant.WakeupFrom, instant.TS) {
				skipped["lifecycle_rejected_waker_endpoint"]++
				continue
			}
			wakerTask := traceDBCommName(authority.threadDisplayName(waker), "unknown")
			wakeeTask := traceDBCommName(authority.threadDisplayName(woken), "unknown")
			wakerTGID := firstNonZero(wakerProcess.PID, waker.TID)
			if rawLite := rawLiteJoin.match(instant, raw, waker, woken); rawLite != nil {
				rawKey, rawOK := traceDBRawSchedWakeupLiteKey(*rawLite)
				dbKey, dbOK := traceDBSchedWakeupLiteDBKey(instant, raw, waker, woken)
				if !rawOK || !dbOK || rawKey != dbKey {
					return coverage, &traceDBOutputInvariantError{Reason: "scheduler_lite_wakeup_join_identity_mismatch"}
				}
				body := fmt.Sprintf(
					"%s: comm=%s pid=%d prio=%d target_cpu=%03d codrax_wakeup_source=official_raw_sched_wakeup_lite",
					instant.Name, wakeeTask, woken.TID, rawLite.Priority, rawLite.TargetCPU)
				rendered, err := prepareTraceDBRenderedRowWithTraceFlagsContext(
					ctx, instant.TS, sink.stats.RowsAccepted,
					wakerTask, waker.TID, wakerTGID, int64(rawLite.CPU),
					rawLite.Flags, rawLite.PreemptCount, body)
				if err != nil {
					return coverage, err
				}
				if err := sink.add(rendered); err != nil {
					return coverage, err
				}
				coverage.RowsEmitted++
				continue
			}
			eventCPU, runningStatus := running.lookupCPUAt(instant.WakeupFrom, instant.TS)
			cpuUnavailableReason := ""
			switch runningStatus {
			case traceDBSchedulerRunningSourceTainted:
				cpuUnavailableReason = tracequery.SchedulerEmitterCPUReasonSourceTainted
			case traceDBSchedulerRunningLifecycleRejected:
				skipped["lifecycle_rejected_emitter_running_cpu"]++
				continue
			case traceDBSchedulerRunningUnknown:
				cpuUnavailableReason = tracequery.SchedulerEmitterCPUReasonUnknown
			case traceDBSchedulerRunningKnown:
			default:
				cpuUnavailableReason = tracequery.SchedulerEmitterCPUReasonUnknown
			}
			targetPrio, priorityKnown := traceDBNextSchedPriority(starts, instant.Ref, instant.TS)
			if cpuUnavailableReason != "" {
				rendered, err := prepareTraceDBCPUUnavailableWakeupRow(
					instant.TS, sink.stats.RowsAccepted, instant.Name,
					wakerTask, waker.TID, wakerTGID, wakeeTask, woken.TID,
					raw.TargetCPU, targetPrio, priorityKnown, cpuUnavailableReason)
				if err != nil {
					return coverage, err
				}
				if err := sink.add(rendered); err != nil {
					return coverage, err
				}
				coverage.RowsEmitted++
				cpuUnavailablePreserved++
				if cpuUnavailableReason == tracequery.SchedulerEmitterCPUReasonSourceTainted {
					cpuUnavailableTainted++
				} else {
					cpuUnavailableUnknown++
				}
				if !priorityKnown {
					priorityUnknown++
				}
				continue
			}
			body := fmt.Sprintf("%s: comm=%s pid=%d target_cpu=%03d codrax_prio_source=unknown",
				instant.Name, wakeeTask, woken.TID, raw.TargetCPU)
			if priorityKnown {
				body = fmt.Sprintf("%s: comm=%s pid=%d prio=%d target_cpu=%03d codrax_prio_source=inferred_next_sched_slice",
					instant.Name, wakeeTask, woken.TID, targetPrio, raw.TargetCPU)
			} else {
				priorityUnknown++
			}
			if err := addTraceDBInstantRow(sink, instant.TS, wakerTask, waker.TID,
				wakerTGID, eventCPU, body); err != nil {
				return coverage, err
			}
			coverage.RowsEmitted++
		}
	}
	traceDBAddCoverageMetric(&coverage, "wakeup_edges_preserved_cpu_unavailable", int64(cpuUnavailablePreserved))
	traceDBAddCoverageMetric(&coverage, "wakeup_edges_preserved_missing_or_ambiguous_emitter_running_cpu", int64(cpuUnavailableUnknown))
	traceDBAddCoverageMetric(&coverage, "wakeup_edges_preserved_tainted_emitter_running_cpu", int64(cpuUnavailableTainted))
	coverage.Skipped = traceDBWakeupSkipSummary(skipped)
	if cpuUnavailablePreserved > 0 {
		if coverage.Skipped != "" {
			coverage.Skipped += "; "
		}
		coverage.Skipped += fmt.Sprintf(
			"field-level unavailable: emitter_cpu_unavailable_edges_preserved=%d missing_or_ambiguous=%d tainted=%d; per_cpu_attribution_withheld=%d",
			cpuUnavailablePreserved, cpuUnavailableUnknown, cpuUnavailableTainted, cpuUnavailablePreserved)
	}
	if priorityUnknown > 0 {
		if coverage.Skipped != "" {
			coverage.Skipped += "; "
		}
		coverage.Skipped += fmt.Sprintf("field-level unknown: priority_unknown_edges_preserved=%d; priority_unknown_edges_suppressed=0", priorityUnknown)
	}
	return coverage, nil
}

func traceDBUniqueWakeupMatching(instants []traceDBWakeupInstant, raws []traceDBRawWakeup) ([]int, string) {
	if len(instants) == 0 {
		return nil, ""
	}
	if len(instants) != len(raws) {
		return nil, "raw_instant_count_mismatch"
	}
	adjacency := make([][]int, len(instants))
	for instantIndex, instant := range instants {
		for rawIndex, raw := range raws {
			if traceDBRawWakeupMatchesInstant(raw, instant) {
				adjacency[instantIndex] = append(adjacency[instantIndex], rawIndex)
			}
		}
		if len(adjacency[instantIndex]) == 0 {
			return nil, "raw_identity_mismatch"
		}
	}
	matchedRaw := make([]int, len(raws))
	for i := range matchedRaw {
		matchedRaw[i] = -1
	}
	var augment func(int, []bool) bool
	augment = func(instantIndex int, seen []bool) bool {
		for _, rawIndex := range adjacency[instantIndex] {
			if seen[rawIndex] {
				continue
			}
			seen[rawIndex] = true
			if matchedRaw[rawIndex] == -1 || augment(matchedRaw[rawIndex], seen) {
				matchedRaw[rawIndex] = instantIndex
				return true
			}
		}
		return false
	}
	for instantIndex := range instants {
		if !augment(instantIndex, make([]bool, len(raws))) {
			return nil, "raw_identity_mismatch"
		}
	}
	matchedInstant := make([]int, len(instants))
	for rawIndex, instantIndex := range matchedRaw {
		matchedInstant[instantIndex] = rawIndex
	}
	// A perfect bipartite matching is unique iff its alternating graph has no
	// cycle. Each non-matching instant->raw edge points to the instant that owns
	// that raw row in the selected matching.
	alternating := make([][]int, len(instants))
	for instantIndex, candidates := range adjacency {
		for _, rawIndex := range candidates {
			if rawIndex != matchedInstant[instantIndex] {
				alternating[instantIndex] = append(alternating[instantIndex], matchedRaw[rawIndex])
			}
		}
	}
	colors := make([]uint8, len(instants))
	var hasCycle func(int) bool
	hasCycle = func(node int) bool {
		colors[node] = 1
		for _, next := range alternating[node] {
			if colors[next] == 1 || colors[next] == 0 && hasCycle(next) {
				return true
			}
		}
		colors[node] = 2
		return false
	}
	for node := range instants {
		if colors[node] == 0 && hasCycle(node) {
			return nil, "ambiguous_raw_identity_mapping"
		}
	}
	return matchedInstant, ""
}

func traceDBRawWakeupMatchesInstant(raw traceDBRawWakeup, instant traceDBWakeupInstant) bool {
	if raw.TS != instant.TS || traceDBWakeupPairingName(raw.Name) != traceDBWakeupPairingName(instant.Name) {
		return false
	}
	if instant.Name == "sched_waking" {
		// Both upstream PB and bytrace importers store the waker itid here.
		return raw.ITID == instant.WakeupFrom
	}
	// The upstream PB importer stores wakee(ref), while the bytrace importer
	// stores waker(wakeup_from). Accept both documented producer shapes, then
	// require a unique one-to-one group matching before minting any row.
	return raw.ITID == instant.Ref || raw.ITID == instant.WakeupFrom
}

func traceDBWakeupPairingName(name string) string {
	if name == "sched_wakeup_new" {
		return "sched_wakeup"
	}
	return name
}

func traceDBWakeupSkipSummary(skipped map[string]int) string {
	if len(skipped) == 0 {
		return ""
	}
	keys := make([]string, 0, len(skipped))
	total := 0
	for key, count := range skipped {
		if count <= 0 {
			continue
		}
		keys = append(keys, key)
		total += count
	}
	if total == 0 {
		return ""
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, skipped[key]))
	}
	return fmt.Sprintf("%d wakeup row(s) skipped: %s", total, strings.Join(parts, ","))
}

func queryTraceDBSchedSliceRows(ctx context.Context, tdb *traceDB) (*sql.Rows, error) {
	return tdb.db.QueryContext(ctx, `
		SELECT ts, dur, cpu, end_state, priority, itid
		FROM sched_slice
		ORDER BY cpu, ts, rowid
	`)
}

func auditTraceDBSchedSwitchRows(ctx context.Context, tdb *traceDB, authority traceDBSchedulerAuthority) (traceDBSchedAudit, error) {
	audit := traceDBSchedAudit{
		CPUs:                 map[int64]*traceDBSchedCPUAudit{},
		UnassignedCPUReasons: map[string]int{},
	}
	rows, err := queryTraceDBSchedSliceRows(ctx, tdb)
	if err != nil {
		return audit, err
	}
	defer rows.Close()
	lastByCPU := map[int64]traceDBSchedSourceRow{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return audit, err
		}
		current, err := scanTraceDBSchedSourceRow(rows, authority)
		if err != nil {
			return audit, err
		}
		audit.RowsRead++
		if !current.CPUAssigned {
			audit.UnassignedCPURows++
			audit.UnassignedCPUReasons[firstNonEmpty(current.CPUAssignmentGap, "non_integer_cpu")]++
			continue
		}
		if !current.CPUInRange {
			audit.InvalidCPURows++
			var truncated bool
			audit.InvalidCPUValues, truncated = appendBoundedUniqueInt64(audit.InvalidCPUValues, current.Slice.CPU, 8)
			audit.InvalidCPUValuesTruncated = audit.InvalidCPUValuesTruncated || truncated
			continue
		}
		lane := audit.CPUs[current.Slice.CPU]
		if lane == nil {
			lane = &traceDBSchedCPUAudit{Reasons: map[string]int{}}
			audit.CPUs[current.Slice.CPU] = lane
		}
		lane.Rows++
		for _, reason := range current.ValidationGaps {
			lane.Reasons[reason]++
		}
		if previous, ok := lastByCPU[current.Slice.CPU]; ok {
			switch {
			case previous.DurationNull:
				lane.Reasons["midstream_null_duration"]++
			case previous.TSValid && previous.DurationValid && current.TSValid &&
				previous.Slice.TS <= math.MaxInt64-previous.Slice.Dur:
				end := previous.Slice.TS + previous.Slice.Dur
				switch {
				case end < current.Slice.TS:
					lane.Reasons["sched_slice_gap"]++
				case end > current.Slice.TS:
					lane.Reasons["sched_slice_overlap"]++
				}
			}
		}
		lastByCPU[current.Slice.CPU] = current
		if len(lastByCPU) > audit.PeakBufferedSourceRows {
			audit.PeakBufferedSourceRows = len(lastByCPU)
		}
	}
	if err := rows.Err(); err != nil {
		return audit, err
	}
	for _, lane := range audit.CPUs {
		if len(lane.Reasons) != 0 || lane.Rows == 0 {
			continue
		}
		audit.OpenTailRows++
		audit.ExpectedBoundaryRows += lane.Rows - 1
	}
	return audit, nil
}

func scanTraceDBSchedSourceRow(rows *sql.Rows, authority traceDBSchedulerAuthority) (traceDBSchedSourceRow, error) {
	var rawTS, rawDur, rawCPU, rawState, rawPriority, rawITID any
	if err := rows.Scan(&rawTS, &rawDur, &rawCPU, &rawState, &rawPriority, &rawITID); err != nil {
		return traceDBSchedSourceRow{}, err
	}
	var row traceDBSchedSourceRow
	cpu, cpuOK := traceDBStrictSQLiteInt(rawCPU)
	if !cpuOK {
		row.CPUAssignmentGap = "non_integer_cpu"
		if rawCPU == nil {
			row.CPUAssignmentGap = "null_cpu"
		}
	} else {
		row.CPUAssigned = true
		row.CPUInRange = validTraceDBCPUIndex(cpu)
		row.Slice.CPU = cpu
	}

	if ts, ok := traceDBStrictSQLiteInt(rawTS); ok && ts >= 0 {
		row.Slice.TS = ts
		row.TSValid = true
	} else {
		row.ValidationGaps = append(row.ValidationGaps, "invalid_timestamp")
	}
	if rawDur == nil {
		row.DurationNull = true
	} else if dur, ok := traceDBStrictSQLiteInt(rawDur); ok && dur >= 0 {
		row.Slice.Dur = dur
		row.DurationValid = true
	} else {
		row.ValidationGaps = append(row.ValidationGaps, "invalid_duration")
	}
	if row.TSValid && row.DurationValid && row.Slice.TS > math.MaxInt64-row.Slice.Dur {
		row.ValidationGaps = append(row.ValidationGaps, "sched_slice_end_overflow")
	}

	state, stateOK := rawState.(string)
	state = strings.TrimSpace(state)
	if !stateOK || state == "" {
		row.ValidationGaps = append(row.ValidationGaps, "invalid_or_empty_state")
	} else {
		row.Slice.EndState = state
	}
	if priority, ok := traceDBStrictSchedPriority(rawPriority); ok {
		row.Slice.Priority = priority
	} else {
		row.ValidationGaps = append(row.ValidationGaps, "invalid_priority")
	}

	itid, itidOK := traceDBStrictInternalID(rawITID)
	if !itidOK {
		row.ValidationGaps = append(row.ValidationGaps, "invalid_itid")
		return row, nil
	}
	subject, subjectOK := authority.schedulerSubjectFromExactITID(itid, itidOK)
	if !subjectOK {
		row.ValidationGaps = append(row.ValidationGaps, "invalid_or_ambiguous_scheduler_identity")
		return row, nil
	}
	row.Subject = subject
	row.Slice.ITID = itid
	if itid == 0 {
		// ProcessFilter seeds tidMappingSet_(0, 0): internal thread id 0 is the
		// one canonical idle/swapper identity even though the thread table omits
		// it. No other missing itid is eligible for this projection.
		row.Slice.Name = "swapper"
		validateTraceDBSchedLifecycle(&row, authority)
		return row, nil
	}
	thread, process, resolution := authority.resolveThreadSubject(itid)
	if resolution != traceDBSchedulerThreadResolved {
		switch resolution {
		case traceDBSchedulerThreadMissing:
			row.ValidationGaps = append(row.ValidationGaps, "missing_thread_identity")
		case traceDBSchedulerProcessMissing:
			row.ValidationGaps = append(row.ValidationGaps, "missing_process_identity")
		default:
			row.ValidationGaps = append(row.ValidationGaps, "ambiguous_or_invalid_thread_process_identity")
		}
		return row, nil
	}
	if thread.TID <= 0 || thread.TID > math.MaxInt32 {
		row.ValidationGaps = append(row.ValidationGaps, "invalid_thread_tid")
		return row, nil
	}
	row.Slice.TID = thread.TID
	row.Slice.IPID = thread.IPID
	row.Slice.Name = authority.threadDisplayName(thread)
	row.Slice.TGID = thread.TID
	if process.PID > math.MaxInt32 {
		row.ValidationGaps = append(row.ValidationGaps, "invalid_tgid")
		return row, nil
	}
	if process.PID > 0 {
		row.Slice.TGID = process.PID
	}
	validateTraceDBSchedLifecycle(&row, authority)
	return row, nil
}

func validateTraceDBSchedLifecycle(row *traceDBSchedSourceRow, authority traceDBSchedulerAuthority) {
	if row == nil || !row.TSValid || !row.Subject.exact {
		return
	}
	allowed := false
	reason := "lifecycle_point_rejected"
	switch {
	case row.DurationNull:
		allowed = authority.schedulerPointAllows(row.Subject, row.Slice.TS)
	case !row.DurationValid:
		return
	case row.Slice.Dur == 0:
		allowed = authority.schedulerPointAllows(row.Subject, row.Slice.TS)
	case row.Slice.TS > math.MaxInt64-row.Slice.Dur:
		return
	default:
		allowed = authority.schedulerSourceIntervalAllows(row.Subject, row.Slice.TS, row.Slice.TS+row.Slice.Dur)
		reason = "lifecycle_half_open_rejected"
	}
	if !allowed {
		row.ValidationGaps = append(row.ValidationGaps, reason)
	}
}

func traceDBStrictSQLiteInt(value any) (int64, bool) {
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case int16:
		return int64(v), true
	case int8:
		return int64(v), true
	default:
		return 0, false
	}
}

func appendBoundedUniqueInt64(values []int64, value int64, limit int) ([]int64, bool) {
	for _, current := range values {
		if current == value {
			return values, false
		}
	}
	if limit > 0 && len(values) >= limit {
		return values, true
	}
	return append(values, value), false
}

func traceDBSchedAuditSummary(audit traceDBSchedAudit) string {
	var parts []string
	if audit.UnassignedCPURows > 0 {
		parts = append(parts, fmt.Sprintf("family_fail_closed=true rows_suppressed=%d unassigned_cpu_rows=%d reasons=%s",
			audit.RowsRead, audit.UnassignedCPURows, traceDBCountSummary(audit.UnassignedCPUReasons)))
	} else {
		suppressedRows := audit.InvalidCPURows
		for _, lane := range audit.CPUs {
			if len(lane.Reasons) > 0 {
				suppressedRows += lane.Rows
			}
		}
		if suppressedRows > 0 {
			parts = append(parts, fmt.Sprintf("rows_suppressed=%d", suppressedRows))
		}
	}
	if audit.InvalidCPURows > 0 {
		values := append([]int64(nil), audit.InvalidCPUValues...)
		sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
		detail := fmt.Sprintf("invalid_cpu_rows=%d values=%v", audit.InvalidCPURows, values)
		if audit.InvalidCPUValuesTruncated {
			detail += " values_truncated=true"
		}
		parts = append(parts, detail)
	}
	cpus := make([]int64, 0, len(audit.CPUs))
	for cpu, lane := range audit.CPUs {
		if len(lane.Reasons) > 0 {
			cpus = append(cpus, cpu)
		}
	}
	sort.Slice(cpus, func(i, j int) bool { return cpus[i] < cpus[j] })
	for _, cpu := range cpus {
		lane := audit.CPUs[cpu]
		parts = append(parts, fmt.Sprintf("cpu=%03d suppressed_rows=%d reasons=%s", cpu, lane.Rows, traceDBCountSummary(lane.Reasons)))
	}
	if audit.OpenTailRows > 0 {
		parts = append(parts, fmt.Sprintf("accounting: open_tail_rows=%d synthetic_idle_closes=0", audit.OpenTailRows))
	}
	return strings.Join(parts, "; ")
}

func traceDBCountSummary(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for key, count := range counts {
		if count > 0 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, counts[key]))
	}
	return strings.Join(parts, ",")
}

func emitTraceDBSchedSwitch(sink *traceDBRowSink, prev, next traceDBSchedSlice) error {
	return emitTraceDBSchedSwitchWithRawLite(sink, prev, next, nil)
}

func emitTraceDBSchedSwitchWithRawLite(
	sink *traceDBRowSink,
	prev, next traceDBSchedSlice,
	raw *traceDBRawSchedSwitchLiteRecord,
) error {
	if prev.CPU != next.CPU || prev.TS < 0 || prev.Dur < 0 || next.TS < 0 ||
		prev.TS > math.MaxInt64-prev.Dur || prev.TS+prev.Dur != next.TS {
		return fmt.Errorf("refusing non-contiguous sched_slice boundary on cpu %d", prev.CPU)
	}
	ts := prev.TS + prev.Dur
	prevPriority := prev.Priority
	if raw != nil {
		rawKey, rawOK := traceDBRawSchedSwitchLiteKey(*raw)
		dbKey, dbOK := traceDBSchedSwitchLiteBoundaryKey(prev, next)
		if !rawOK || !dbOK || rawKey != dbKey {
			return &traceDBOutputInvariantError{Reason: "scheduler_lite_switch_join_identity_mismatch"}
		}
		prevPriority = raw.PrevPriority
	}
	body := fmt.Sprintf("sched_switch: prev_comm=%s prev_pid=%d prev_prio=%d prev_state=%s ==> next_comm=%s next_pid=%d next_prio=%d",
		traceDBCommName(prev.Name, "unknown"), prev.TID, prevPriority, prev.EndState,
		traceDBCommName(next.Name, "unknown"), next.TID, next.Priority)
	if raw != nil {
		body += fmt.Sprintf(" next_info=%s codrax_next_info_raw=0x%016x codrax_next_info_source=official_raw_sched_switch_lite",
			formatHarmonySchedInfo(raw.NextInfo, true), raw.NextInfo)
		row, err := prepareTraceDBRenderedRowWithTraceFlags(
			ts, sink.stats.RowsAccepted,
			traceDBCommName(prev.Name, "unknown"), prev.TID,
			firstNonZero(prev.TGID, prev.TID), prev.CPU,
			raw.Flags, raw.PreemptCount, body)
		if err != nil {
			return err
		}
		return sink.add(row)
	}
	return addTraceDBInstantRow(sink, ts, traceDBCommName(prev.Name, "unknown"), prev.TID,
		firstNonZero(prev.TGID, prev.TID), prev.CPU, body)
}

func traceDBCommName(name, fallback string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		trimmed = fallback
	}
	if trimmed == "" {
		trimmed = "unknown"
	}
	runes := []rune(trimmed)
	if len(runes) > 15 {
		return string(runes[:15])
	}
	return trimmed
}

func traceDBProcessName(index traceDBThreadIndex, thread traceDBThread) string {
	process := index.Processes[thread.IPID]
	for _, candidate := range index.ByProcess[thread.IPID] {
		if candidate.IsMainThread && strings.TrimSpace(candidate.Name) != "" {
			return candidate.Name
		}
	}
	if process.PID != 0 {
		for _, candidate := range index.ByProcess[thread.IPID] {
			if candidate.TID == process.PID && strings.TrimSpace(candidate.Name) != "" {
				return candidate.Name
			}
		}
	}
	return firstNonEmpty(process.Name, thread.Name, "unknown")
}

func sortedTraceDBThreads(items map[int64]traceDBThread) []traceDBThread {
	out := make([]traceDBThread, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		leftKnown := out[i].RegistrationHint.Known && !out[i].RegistrationHint.Tainted
		rightKnown := out[j].RegistrationHint.Known && !out[j].RegistrationHint.Tainted
		if leftKnown != rightKnown {
			return leftKnown
		}
		if !leftKnown || out[i].RegistrationHint.Value == out[j].RegistrationHint.Value {
			if out[i].TID != out[j].TID {
				return out[i].TID < out[j].TID
			}
			return out[i].ITID < out[j].ITID
		}
		return out[i].RegistrationHint.Value < out[j].RegistrationHint.Value
	})
	return out
}

func nullInt64Value(value sql.NullInt64) int64 {
	if value.Valid {
		return value.Int64
	}
	return 0
}
