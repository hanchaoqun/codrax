package hitraceconv

import (
	"context"
	"fmt"
	"math"
	"sort"
)

type traceDBSchedStartKey struct {
	ITID int64
	TS   int64
}

type traceDBSchedStartValue struct {
	CPU      int64
	Priority int64
}

type traceDBSchedStartCohort struct {
	Values      map[traceDBSchedStartValue]int
	InvalidRows int
}

// loadSchedStarts builds the independent next-run resolver used by wakeup
// export. A malformed nearest point is retained as an explicit barrier: simply
// deleting it would let a lookup jump to a later run and assign that later
// priority to the wrong wakeup.
func (tdb *traceDB) loadSchedStarts(ctx context.Context, index traceDBThreadIndex) (traceDBSchedStartIndex, TraceDBCoverage, error) {
	out := traceDBSchedStartIndex{
		ByITID:       map[int64][]traceDBSchedStart{},
		TaintedITIDs: map[int64]bool{},
	}
	coverage, err := tdb.inspectCoverage(ctx, "resolver", "sched_slice", []string{"itid", "ts", "cpu", "priority"})
	coverage.FieldSources = map[string]string{
		"canonical_itid": "sched_slice.itid; strict internal uint32 identity excluding UINT32_MAX sentinel; nonzero IDs must exist in the audited thread index",
		"cohort":         "exact (itid,ts); identical rows coalesce, any scalar/identity disagreement becomes a lookup barrier",
		"cpu":            "sched_slice.cpu; strict SQLite INTEGER in range 0..4095",
		"lookup":         "first sched point with ts>=query; poisoned nearest point returns unknown and is never skipped",
		"priority":       "sched_slice.priority; strict signed int32 preserving Harmony RT 140..159 and raw values above 159; exact INT32_MAX upstream sentinel rejected",
		"timestamp":      "sched_slice.ts; non-negative SQLite INTEGER; a known itid with an unplaceable timestamp taints that itid lane",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return out, coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, `SELECT itid, ts, cpu, priority FROM sched_slice`)
	if err != nil {
		coverage.Error = err.Error()
		return out, coverage, err
	}
	defer rows.Close()

	coverage.RowsRead = 0
	cohorts := map[traceDBSchedStartKey]*traceDBSchedStartCohort{}
	reasons := map[string]int{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return out, coverage, err
		}
		coverage.RowsRead++
		var itidRaw, tsRaw, cpuRaw, priorityRaw any
		if err := rows.Scan(&itidRaw, &tsRaw, &cpuRaw, &priorityRaw); err != nil {
			coverage.Error = err.Error()
			return out, coverage, err
		}
		itid, itidOK := traceDBStrictInternalID(itidRaw)
		if !itidOK {
			reasons["invalid_itid"]++
			if ts, tsOK := traceDBStrictSQLiteInt(tsRaw); tsOK && ts >= 0 {
				out.GlobalBarriers = append(out.GlobalBarriers, ts)
			} else {
				out.GlobalTaint = true
			}
			continue
		}
		ts, tsOK := traceDBStrictSQLiteInt(tsRaw)
		if !tsOK || ts < 0 {
			out.TaintedITIDs[itid] = true
			reasons["unplaceable_timestamp"]++
			continue
		}
		key := traceDBSchedStartKey{ITID: itid, TS: ts}
		cohort := cohorts[key]
		if cohort == nil {
			cohort = &traceDBSchedStartCohort{Values: map[traceDBSchedStartValue]int{}}
			cohorts[key] = cohort
		}

		cpu, cpuOK := traceDBStrictSQLiteInt(cpuRaw)
		cpuOK = cpuOK && validTraceDBCPUIndex(cpu)
		priority, priorityOK := traceDBStrictSchedPriority(priorityRaw)
		identityOK := traceDBCanonicalThreadIdentityKnown(index, itid)
		if !cpuOK || !priorityOK || !identityOK {
			cohort.InvalidRows++
			if !cpuOK {
				reasons["invalid_cpu"]++
			}
			if !priorityOK {
				reasons["invalid_priority"]++
			}
			if !identityOK {
				reasons["unresolved_itid"]++
			}
			continue
		}
		cohort.Values[traceDBSchedStartValue{CPU: cpu, Priority: priority}]++
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return out, coverage, err
	}

	keys := make([]traceDBSchedStartKey, 0, len(cohorts))
	for key := range cohorts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].ITID != keys[j].ITID {
			return keys[i].ITID < keys[j].ITID
		}
		return keys[i].TS < keys[j].TS
	})
	for _, key := range keys {
		if out.GlobalTaint || out.TaintedITIDs[key.ITID] {
			continue
		}
		cohort := cohorts[key]
		if cohort.InvalidRows > 0 || len(cohort.Values) != 1 {
			out.ByITID[key.ITID] = append(out.ByITID[key.ITID], traceDBSchedStart{TS: key.TS})
			reasons["poisoned_key_cohorts"]++
			continue
		}
		for value, count := range cohort.Values {
			out.ByITID[key.ITID] = append(out.ByITID[key.ITID], traceDBSchedStart{
				TS: key.TS, CPU: value.CPU, Priority: value.Priority, Known: true,
			})
			coverage.RowsEmitted++
			if count > 1 {
				reasons["exact_duplicate_rows_coalesced"] += count - 1
			}
		}
	}
	if len(out.TaintedITIDs) > 0 {
		reasons["tainted_itid_lanes"] = len(out.TaintedITIDs)
	}
	if len(out.GlobalBarriers) > 0 {
		sort.Slice(out.GlobalBarriers, func(i, j int) bool { return out.GlobalBarriers[i] < out.GlobalBarriers[j] })
		out.GlobalBarriers = compactSortedTraceDBInt64(out.GlobalBarriers)
		reasons["global_timestamp_barriers"] = len(out.GlobalBarriers)
	}
	if out.GlobalTaint {
		reasons["global_unplaceable_taint"] = 1
	}
	if len(reasons) > 0 {
		coverage.Skipped = "sched-start audit: " + traceDBCountSummary(reasons)
	}
	return out, coverage, nil
}

func traceDBNextSchedMeta(starts traceDBSchedStartIndex, itid, ts int64) (int64, int64, bool) {
	if starts.GlobalTaint || starts.TaintedITIDs[itid] {
		return 0, 0, false
	}
	entries := starts.ByITID[itid]
	idx := sort.Search(len(entries), func(i int) bool { return entries[i].TS >= ts })
	barrier := sort.Search(len(starts.GlobalBarriers), func(i int) bool { return starts.GlobalBarriers[i] >= ts })
	if barrier < len(starts.GlobalBarriers) && (idx >= len(entries) || starts.GlobalBarriers[barrier] <= entries[idx].TS) {
		return 0, 0, false
	}
	if idx >= len(entries) || !entries[idx].Known {
		return 0, 0, false
	}
	return entries[idx].CPU, entries[idx].Priority, true
}

func traceDBStrictSchedPriority(value any) (int64, bool) {
	priority, ok := traceDBStrictSQLiteInt(value)
	return priority, ok && priority >= math.MinInt32 && priority < math.MaxInt32
}

// Trace Streamer's internal-identity columns in instant/raw virtual tables
// project uint32 identities through int32 before handing them to SQLite.
// Values -2..MinInt32 therefore have one exact uint32 interpretation; -1 is
// the UINT32_MAX missing-internal-identity sentinel. Positive vendor/newer
// schemas may expose the canonical uint32 directly.
func traceDBStrictSignedUint32Projection(value any) (int64, bool) {
	raw, ok := traceDBStrictSQLiteInt(value)
	if !ok || raw < math.MinInt32 || raw > maxTraceDBInternalID || raw == -1 {
		return 0, false
	}
	if raw < 0 {
		return raw + (int64(1) << 32), true
	}
	return raw, true
}

// Stable row/event IDs are not internal thread identities. Trace Streamer
// exposes several CacheBase uint64 counters through signed int32 projections,
// so the complete uint32 domain is valid here: notably -1 is canonical stable
// ID 4294967295, not the internal-ID missing sentinel.
func traceDBStrictStableUint32Projection(value any) (int64, bool) {
	raw, ok := traceDBStrictSQLiteInt(value)
	if !ok || raw < math.MinInt32 || raw > math.MaxUint32 {
		return 0, false
	}
	if raw < 0 {
		return raw + (int64(1) << 32), true
	}
	return raw, true
}

func traceDBStableUint32OrderExpr(stableExpr string, projectedUint32 bool) string {
	if !projectedUint32 {
		return stableExpr
	}
	// SQLite sorts the projected negative int32 half before non-negative IDs.
	// Normalize only INTEGER storage-class values so valid stable IDs follow
	// their canonical uint32 order while malformed values stay non-authoritative.
	return fmt.Sprintf("CASE WHEN typeof(%s)='integer' AND %s < 0 THEN %s + 4294967296 ELSE %s END", stableExpr, stableExpr, stableExpr, stableExpr)
}

func compactSortedTraceDBInt64(values []int64) []int64 {
	if len(values) < 2 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

func traceDBNextSchedPriority(starts traceDBSchedStartIndex, itid, ts int64) (int64, bool) {
	_, priority, known := traceDBNextSchedMeta(starts, itid, ts)
	return priority, known
}

func (tdb *traceDB) loadActiveThreadIDs(ctx context.Context, index traceDBThreadIndex) (map[int64]bool, []TraceDBCoverage, error) {
	collection, err := collectTraceDBLifecycle(ctx, tdb.db, index)
	return collection.ActiveITIDs, collection.ActiveCoverage, err
}

func traceDBCanonicalThreadIdentityKnown(index traceDBThreadIndex, itid int64) bool {
	if index.AmbiguousITID[itid] {
		return false
	}
	if itid == 0 {
		// ProcessFilter seeds the canonical idle identity even when the virtual
		// thread table does not materialize a row for it.
		return true
	}
	_, ok := index.ByITID[itid]
	return ok
}
