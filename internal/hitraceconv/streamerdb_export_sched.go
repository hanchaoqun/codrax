package hitraceconv

import (
	"context"
	"database/sql"
	"fmt"
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
	schedCoverage, err := exportTraceDBSchedSwitch(ctx, tdb, sink)
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
	running, runningCoverage, err := tdb.loadRunningIntervals(ctx)
	coverage = append(coverage, runningCoverage)
	if err != nil {
		return coverage, err
	}
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
	argsets, argCoverage, err := tdb.loadArgsets(ctx)
	coverage = append(coverage, argCoverage...)
	if err != nil {
		return coverage, err
	}
	stageStart = time.Now()
	irqCoverage, err := exportTraceDBIRQ(ctx, tdb, sink, argsets)
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
		if ts == 0 {
			ts = index.TraceStart
		}
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
			if err := sink.add(renderedRow{
				tsNS: uint64(ts),
				seq:  sink.stats.RowsAccepted,
				line: traceDBFormatLine(task, thread.TID, tgid, 0, ts, body),
			}); err != nil {
				return coverage, err
			}
			coverage.RowsEmitted++
		}
	}
	return coverage, nil
}

func exportTraceDBSchedSwitch(ctx context.Context, tdb *traceDB, sink *traceDBRowSink) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "scheduler", "sched_slice", []string{"ts", "dur", "cpu", "end_state", "priority", "itid"})
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT s.ts, COALESCE(s.dur, 0), s.cpu,
		       COALESCE(s.end_state, 'R'), COALESCE(s.priority, 120),
		       t.tid, COALESCE(t.name, 'unknown'), COALESCE(p.pid, t.tid)
		FROM sched_slice s
		JOIN thread t ON t.itid = s.itid
		LEFT JOIN process p ON p.ipid = t.ipid
		WHERE s.ts IS NOT NULL AND s.cpu IS NOT NULL
		ORDER BY s.cpu, s.ts
	`)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	var prev *traceDBSchedSlice
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return coverage, err
		}
		cur, err := scanTraceDBSchedSlice(rows)
		if err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		if prev != nil && prev.CPU != cur.CPU {
			if err := emitTraceDBSchedSwitch(sink, *prev, nil); err != nil {
				return coverage, err
			}
			coverage.RowsEmitted++
			prev = nil
		}
		if prev != nil {
			if err := emitTraceDBSchedSwitch(sink, *prev, &cur); err != nil {
				return coverage, err
			}
			coverage.RowsEmitted++
		}
		prev = &cur
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	if prev != nil {
		if err := emitTraceDBSchedSwitch(sink, *prev, nil); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted++
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
			if err := sink.add(renderedRow{
				tsNS: uint64(instant.TS),
				seq:  sink.stats.RowsAccepted,
				line: traceDBFormatLine(traceDBCommName(waker.Name, "unknown"), waker.TID, firstNonZero(wakerProcess.PID, waker.TID), eventCPU, instant.TS, body),
			}); err != nil {
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

func exportTraceDBIRQ(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, argsets map[int64]map[string]traceDBValue) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "irq", "irq", []string{"ts", "dur", "callid", "cat", "name", "argsetid"})
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT ts, COALESCE(dur, 0), COALESCE(callid, 0),
		       COALESCE(cat, ''), COALESCE(name, ''), COALESCE(argsetid, 0)
		FROM irq
		WHERE dur >= 0
		ORDER BY ts
	`)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return coverage, err
		}
		var ts, dur, cpu, argsetID int64
		var cat, name string
		if err := rows.Scan(&ts, &dur, &cpu, &cat, &name, &argsetID); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		args := argsets[argsetID]
		if cat == "softirq" {
			vec := traceDBArgText(args, "vec", "0")
			action := firstNonEmpty(name, "unknown")
			if err := addTraceDBInstantRow(sink, ts, "<softirq>", 0, 0, cpu, fmt.Sprintf("softirq_entry: vec=%s [action=%s]", vec, action)); err != nil {
				return coverage, err
			}
			if err := addTraceDBInstantRow(sink, ts+dur, "<softirq>", 0, 0, cpu, fmt.Sprintf("softirq_exit: vec=%s [action=%s]", vec, action)); err != nil {
				return coverage, err
			}
			coverage.RowsEmitted += 2
			continue
		}
		irqNum := traceDBArgText(args, "irq", "0")
		ret := traceDBArgText(args, "irq_ret", "handled")
		displayName := firstNonEmpty(name, "None")
		if err := addTraceDBInstantRow(sink, ts, "<irq>", 0, 0, cpu, fmt.Sprintf("irq_handler_entry: irq=%s name=%s", irqNum, displayName)); err != nil {
			return coverage, err
		}
		if err := addTraceDBInstantRow(sink, ts+dur, "<irq>", 0, 0, cpu, fmt.Sprintf("irq_handler_exit: irq=%s ret=%s", irqNum, ret)); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted += 2
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	return coverage, nil
}

func scanTraceDBSchedSlice(rows *sql.Rows) (traceDBSchedSlice, error) {
	var item traceDBSchedSlice
	err := rows.Scan(&item.TS, &item.Dur, &item.CPU, &item.EndState, &item.Priority, &item.TID, &item.Name, &item.TGID)
	return item, err
}

func emitTraceDBSchedSwitch(sink *traceDBRowSink, prev traceDBSchedSlice, next *traceDBSchedSlice) error {
	nextName := "swapper"
	var nextTID int64
	nextPrio := int64(120)
	if next != nil {
		nextName = traceDBCommName(next.Name, "unknown")
		nextTID = next.TID
		nextPrio = next.Priority
	}
	ts := prev.TS + prev.Dur
	body := fmt.Sprintf("sched_switch: prev_comm=%s prev_pid=%d prev_prio=%d prev_state=%s ==> next_comm=%s next_pid=%d next_prio=%d",
		traceDBCommName(prev.Name, "unknown"), prev.TID, prev.Priority, firstNonEmpty(prev.EndState, "R"), nextName, nextTID, nextPrio)
	return sink.add(renderedRow{
		tsNS: uint64(ts),
		seq:  sink.stats.RowsAccepted,
		line: traceDBFormatLine(traceDBCommName(prev.Name, "unknown"), prev.TID, firstNonZero(prev.TGID, prev.TID), prev.CPU, ts, body),
	})
}

func addTraceDBInstantRow(sink *traceDBRowSink, ts int64, task string, tid, tgid, cpu int64, body string) error {
	return sink.add(renderedRow{
		tsNS: uint64(ts),
		seq:  sink.stats.RowsAccepted,
		line: traceDBFormatLine(task, tid, tgid, cpu, ts, body),
	})
}

func traceDBFormatLine(task string, tid, tgid, cpu, tsNS int64, body string) string {
	task = traceDBCommName(task, "unknown")
	if tgid == 0 {
		tgid = tid
	}
	return fmt.Sprintf("%16s-%-6d (%5d) [%03d] .... %s: %s",
		task, tid, tgid, cpu, formatTimestamp(uint64(tsNS)), body)
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
			return out[i].TID < out[j].TID
		}
		return out[i].StartTS < out[j].StartTS
	})
	return out
}

func traceDBArgText(args map[string]traceDBValue, key, fallback string) string {
	if value, ok := args[key]; ok && value.Valid && strings.TrimSpace(value.Text) != "" {
		return value.Text
	}
	return fallback
}

func nullInt64Value(value sql.NullInt64) int64 {
	if value.Valid {
		return value.Int64
	}
	return 0
}
