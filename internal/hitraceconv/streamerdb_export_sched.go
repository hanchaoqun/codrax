package hitraceconv

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type traceDBSchedSlice struct {
	TS       int64
	Dur      int64
	CPU      int64
	EndState string
	Priority int64
	TID      int64
	TGID     int64
	Name     string
}

type traceDBSchedSourceRow struct {
	Slice            traceDBSchedSlice
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

func exportTraceDBSchedulerFamilies(ctx context.Context, tdb *traceDB, sink *traceDBRowSink) ([]TraceDBCoverage, error) {
	var coverage []TraceDBCoverage
	index, threadCoverage, err := tdb.loadThreadIndex(ctx)
	coverage = append(coverage, threadCoverage...)
	if err != nil {
		return coverage, err
	}
	active, activeCoverage, err := tdb.loadActiveThreadIDs(ctx)
	coverage = append(coverage, activeCoverage...)
	if err != nil {
		return coverage, err
	}
	stageStart := time.Now()
	metadataCoverage, err := exportTraceDBThreadRegistrations(ctx, sink, index, active)
	traceDBSetCoverageElapsed(&metadataCoverage, stageStart)
	coverage = append(coverage, metadataCoverage)
	if err != nil {
		return coverage, err
	}
	stageStart = time.Now()
	schedCoverage, err := exportTraceDBSchedSwitch(ctx, tdb, sink, index)
	traceDBSetCoverageElapsed(&schedCoverage, stageStart)
	coverage = append(coverage, schedCoverage)
	if err != nil {
		return coverage, err
	}
	rawWakeups, rawCoverage, err := tdb.loadRawWakeups(ctx)
	coverage = append(coverage, rawCoverage)
	if err != nil {
		return coverage, err
	}
	starts, startsCoverage, err := tdb.loadSchedStarts(ctx)
	coverage = append(coverage, startsCoverage)
	if err != nil {
		return coverage, err
	}
	running, runningIntegrity, runningCoverage, err := tdb.loadRunningIntervals(ctx)
	coverage = append(coverage, runningCoverage)
	if err != nil {
		return coverage, err
	}
	index.RunningTaintedITID = runningIntegrity.TaintedITIDs
	index.RunningGlobalTaint = runningIntegrity.GlobalTaint
	stageStart = time.Now()
	wakeupCoverage, err := exportTraceDBWakeups(ctx, tdb, sink, index, rawWakeups, starts, running)
	traceDBSetCoverageElapsed(&wakeupCoverage, stageStart)
	coverage = append(coverage, wakeupCoverage)
	if err != nil {
		return coverage, err
	}
	stageStart = time.Now()
	blockedCoverage, err := exportTraceDBBlockedReasons(ctx, tdb, sink, index)
	traceDBSetCoverageElapsed(&blockedCoverage, stageStart)
	coverage = append(coverage, blockedCoverage)
	if err != nil {
		return coverage, err
	}
	stageStart = time.Now()
	irqCoverage, err := exportTraceDBIRQ(ctx, tdb, sink)
	traceDBSetCoverageElapsed(&irqCoverage, stageStart)
	coverage = append(coverage, irqCoverage)
	return coverage, err
}

func exportTraceDBThreadRegistrations(ctx context.Context, sink *traceDBRowSink, index traceDBThreadIndex, active map[int64]bool) (TraceDBCoverage, error) {
	coverage := TraceDBCoverage{
		Family:   "metadata",
		Table:    "thread",
		Role:     "query_ready_export",
		Found:    len(index.ByITID) > 0,
		RowsRead: len(index.ByITID),
	}
	for _, thread := range sortedTraceDBThreads(index.ByITID) {
		if err := ctx.Err(); err != nil {
			return coverage, err
		}
		process := index.Processes[thread.IPID]
		isMain := thread.IsMainThread || (process.PID != 0 && thread.TID == process.PID)
		if isMain && thread.SwitchCount <= 0 && !active[thread.ITID] {
			continue
		}
		ts := thread.StartTS
		task := traceDBCommName(thread.Name, "unknown")
		processName := traceDBProcessName(index, thread)
		threadComm := traceDBCommName(thread.Name, "unknown")
		if isMain {
			threadComm = traceDBCommName(processName, threadComm)
		}
		tgid := firstNonZero(process.PID, thread.TID)
		for _, body := range []string{
			fmt.Sprintf("task_rename: pid=%d oldcomm=%s newcomm=%s oom_score_adj=0", thread.TID, threadComm, threadComm),
			fmt.Sprintf("tracing_mark_write: B|%d|%s", tgid, processName),
			fmt.Sprintf("tracing_mark_write: E|%d|", tgid),
		} {
			if err := addTraceDBInstantRow(sink, ts, task, thread.TID, tgid, 0, body); err != nil {
				return coverage, err
			}
			coverage.RowsEmitted++
		}
	}
	return coverage, nil
}

func exportTraceDBSchedSwitch(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, index traceDBThreadIndex) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "scheduler", "sched_slice", []string{"ts", "dur", "cpu", "end_state", "priority", "itid"})
	coverage.FieldSources = map[string]string{
		"boundary_timestamp": "prev_sched_slice.ts+dur; requires exact equality with next_sched_slice.ts",
		"continuity":         "complete_per_cpu_audit_before_publish; gap_overlap_mid_null_overflow_fail_cpu_lane",
		"header_cpu":         "sched_slice.cpu; strict SQLite INTEGER in range 0..4095",
		"next_identity":      "next_sched_slice.itid->thread.tid/name; canonical itid=0 is swapper; tgid=positive_process.pid_else_thread.tid",
		"open_tail":          "final sched_slice is retained as an unclosed tail; no synthetic idle close",
		"prev_identity":      "prev_sched_slice.itid->thread.tid/name; canonical itid=0 is swapper; tgid=positive_process.pid_else_thread.tid",
		"priority":           "sched_slice.priority; strict signed int32",
		"state":              "sched_slice.end_state; nonempty TEXT",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	audit, err := auditTraceDBSchedSwitchRows(ctx, tdb, index)
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
	rows, err := queryTraceDBSchedSliceRows(ctx, tdb)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	previous := map[int64]traceDBSchedSourceRow{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return coverage, err
		}
		current, err := scanTraceDBSchedSourceRow(rows, index)
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
			if err := emitTraceDBSchedSwitch(sink, prev.Slice, current.Slice); err != nil {
				return coverage, err
			}
			coverage.RowsEmitted++
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
	return coverage, nil
}

func exportTraceDBWakeups(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, index traceDBThreadIndex, rawWakeups []traceDBRawWakeup, starts map[int64][]traceDBSchedStart, running map[int64][]traceDBRunningInterval) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "scheduler", "instant", []string{"ts", "name", "ref", "wakeup_from", "ref_type"})
	coverage.FieldSources = map[string]string{
		"header_cpu":                "thread_state.Running.cpu",
		"priority":                  "inferred_next_sched_slice",
		"raw_identity.sched_waking": "raw.itid==instant.wakeup_from",
		"raw_identity.sched_wakeup": "producer_shape(raw.itid==instant.ref|instant.wakeup_from)+unique_bipartite_matching",
		"target_cpu":                "raw.cpu",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT i.ts, i.name, i.ref, i.wakeup_from
		FROM instant i
		WHERE i.name IN ('sched_wakeup', 'sched_waking')
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
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return coverage, err
		}
		coverage.RowsRead++
		var ts, ref, wakeupFrom sql.NullInt64
		var name sql.NullString
		if err := rows.Scan(&ts, &name, &ref, &wakeupFrom); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		trimmedName := strings.TrimSpace(name.String)
		if !ts.Valid || ts.Int64 < 0 || !name.Valid ||
			(trimmedName != "sched_wakeup" && trimmedName != "sched_waking") ||
			!ref.Valid || ref.Int64 < 0 || !wakeupFrom.Valid || wakeupFrom.Int64 < 0 {
			skipped["invalid_instant_metadata"]++
			continue
		}
		instant := traceDBWakeupInstant{TS: ts.Int64, Name: trimmedName, Ref: ref.Int64, WakeupFrom: wakeupFrom.Int64}
		key := traceDBWakeupKey{TS: instant.TS, Name: instant.Name}
		groups[key] = append(groups[key], instant)
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	rawGroups := map[traceDBWakeupKey][]traceDBRawWakeup{}
	for _, raw := range rawWakeups {
		key := traceDBWakeupKey{TS: raw.TS, Name: raw.Name}
		rawGroups[key] = append(rawGroups[key], raw)
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
			woken, wokenOK := index.ByITID[instant.Ref]
			if !wokenOK {
				skipped["missing_wakee_thread"]++
				continue
			}
			waker, wakerOK := index.ByITID[instant.WakeupFrom]
			if !wakerOK {
				skipped["missing_waker_thread"]++
				continue
			}
			if index.RunningGlobalTaint || index.RunningTaintedITID[instant.WakeupFrom] {
				skipped["tainted_emitter_running_cpu"]++
				continue
			}
			eventCPU, cpuKnown := traceDBKnownCPUAt(running, instant.WakeupFrom, instant.TS)
			if !cpuKnown {
				skipped["missing_or_ambiguous_emitter_running_cpu"]++
				continue
			}
			targetPrio, priorityKnown := traceDBNextSchedPriority(starts, instant.Ref, instant.TS)
			if !priorityKnown {
				skipped["missing_inferred_next_sched_slice_priority"]++
				continue
			}
			wakerProcess := index.Processes[waker.IPID]
			body := fmt.Sprintf("%s: comm=%s pid=%d prio=%d target_cpu=%03d",
				instant.Name, traceDBCommName(woken.Name, "unknown"), woken.TID, targetPrio, raw.TargetCPU)
			if err := addTraceDBInstantRow(sink, instant.TS, traceDBCommName(waker.Name, "unknown"), waker.TID,
				firstNonZero(wakerProcess.PID, waker.TID), eventCPU, body); err != nil {
				return coverage, err
			}
			coverage.RowsEmitted++
		}
	}
	coverage.Skipped = traceDBWakeupSkipSummary(skipped)
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
	if raw.TS != instant.TS || raw.Name != instant.Name {
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

func auditTraceDBSchedSwitchRows(ctx context.Context, tdb *traceDB, index traceDBThreadIndex) (traceDBSchedAudit, error) {
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
		current, err := scanTraceDBSchedSourceRow(rows, index)
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

func scanTraceDBSchedSourceRow(rows *sql.Rows, index traceDBThreadIndex) (traceDBSchedSourceRow, error) {
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
	if priority, ok := traceDBStrictSQLiteInt(rawPriority); ok && priority >= math.MinInt32 && priority <= math.MaxInt32 {
		row.Slice.Priority = priority
	} else {
		row.ValidationGaps = append(row.ValidationGaps, "invalid_priority")
	}

	itid, itidOK := traceDBStrictSQLiteInt(rawITID)
	if !itidOK || itid < 0 {
		row.ValidationGaps = append(row.ValidationGaps, "invalid_itid")
		return row, nil
	}
	if itid == 0 {
		// ProcessFilter seeds tidMappingSet_(0, 0): internal thread id 0 is the
		// one canonical idle/swapper identity even though the thread table omits
		// it. No other missing itid is eligible for this projection.
		row.Slice.Name = "swapper"
		return row, nil
	}
	thread, threadOK := index.ByITID[itid]
	if !threadOK {
		row.ValidationGaps = append(row.ValidationGaps, "missing_thread_identity")
		return row, nil
	}
	if thread.TID <= 0 || thread.TID > math.MaxInt32 {
		row.ValidationGaps = append(row.ValidationGaps, "invalid_thread_tid")
		return row, nil
	}
	row.Slice.TID = thread.TID
	row.Slice.Name = thread.Name
	row.Slice.TGID = thread.TID
	if process, ok := index.Processes[thread.IPID]; ok {
		if process.PID < 0 || process.PID > math.MaxInt32 {
			row.ValidationGaps = append(row.ValidationGaps, "invalid_tgid")
			return row, nil
		}
		if process.PID > 0 {
			row.Slice.TGID = process.PID
		}
	}
	return row, nil
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
	if prev.CPU != next.CPU || prev.TS < 0 || prev.Dur < 0 || next.TS < 0 ||
		prev.TS > math.MaxInt64-prev.Dur || prev.TS+prev.Dur != next.TS {
		return fmt.Errorf("refusing non-contiguous sched_slice boundary on cpu %d", prev.CPU)
	}
	ts := prev.TS + prev.Dur
	body := fmt.Sprintf("sched_switch: prev_comm=%s prev_pid=%d prev_prio=%d prev_state=%s ==> next_comm=%s next_pid=%d next_prio=%d",
		traceDBCommName(prev.Name, "unknown"), prev.TID, prev.Priority, prev.EndState,
		traceDBCommName(next.Name, "unknown"), next.TID, next.Priority)
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
		if out[i].StartTS == out[j].StartTS {
			if out[i].TID != out[j].TID {
				return out[i].TID < out[j].TID
			}
			return out[i].ITID < out[j].ITID
		}
		return out[i].StartTS < out[j].StartTS
	})
	return out
}

func nullInt64Value(value sql.NullInt64) int64 {
	if value.Valid {
		return value.Int64
	}
	return 0
}
