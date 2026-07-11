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

type traceDBLifecycleCollection struct {
	Lifecycle                    traceDBLifecycleIndex
	ActiveITIDs                  map[int64]bool
	ActiveCoverage               []TraceDBCoverage
	LifecycleCoverage            []TraceDBCoverage
	FrameProfile                 traceDBActivityITIDProfile
	FrameProfileSource           string
	CreationComplete             bool
	TerminalComplete             bool
	ActivityComplete             bool
	MalformedPointBudgetExceeded bool
}

type traceDBLifecycleActivitySpec struct {
	Table             string
	TimestampColumn   string
	HasTimestamp      bool
	HasITID           bool
	HasCallID         bool
	Profile           traceDBActivityITIDProfile
	ProfileSource     string
	Complete          bool
	ActiveCoverage    TraceDBCoverage
	LifecycleCoverage TraceDBCoverage
}

type traceDBLifecycleIdentityResolution struct {
	ITID         int64
	Candidates   []int64
	Valid        bool
	UnknownClaim bool
}

// collectTraceDBLifecycle is the only SQL producer for public TID/PID
// generation evidence and active thread references. All schema and row reads
// consume the supplied queryer so R2 can move the unchanged authority into a
// single read transaction.
func collectTraceDBLifecycle(ctx context.Context, queryer traceDBQueryer, identities traceDBThreadIndex) (traceDBLifecycleCollection, error) {
	result := traceDBLifecycleCollection{ActiveITIDs: map[int64]bool{}}
	builder := newTraceDBLifecycleBuilder(identities)

	creation, creationComplete, err := collectTraceDBLifecycleCreations(ctx, queryer, builder)
	result.LifecycleCoverage = append(result.LifecycleCoverage, creation)
	if err != nil {
		return result, err
	}
	result.CreationComplete = creationComplete

	threadTerminal, threadTerminalComplete, err := collectTraceDBThreadStateTerminals(ctx, queryer, builder)
	result.LifecycleCoverage = append(result.LifecycleCoverage, threadTerminal)
	if err != nil {
		return result, err
	}
	schedTerminal, schedTerminalComplete, err := collectTraceDBSchedTerminals(ctx, queryer, builder)
	result.LifecycleCoverage = append(result.LifecycleCoverage, schedTerminal)
	if err != nil {
		return result, err
	}
	result.TerminalComplete = threadTerminalComplete && schedTerminalComplete

	specs := make([]traceDBLifecycleActivitySpec, 0, 6)
	callstack, err := inspectTraceDBCallstackActivity(ctx, queryer)
	if err != nil {
		result.ActiveCoverage = append(result.ActiveCoverage, callstack.ActiveCoverage)
		result.LifecycleCoverage = append(result.LifecycleCoverage, callstack.LifecycleCoverage)
		return result, err
	}
	specs = append(specs, callstack)
	for _, source := range []struct {
		table     string
		timestamp string
	}{
		{table: "sched_slice", timestamp: "ts"},
		{table: "thread_state", timestamp: "ts"},
		{table: "syscall", timestamp: "ts"},
		{table: "native_hook", timestamp: "start_ts"},
		{table: "frame_slice", timestamp: "ts"},
	} {
		spec, err := inspectTraceDBTableActivity(ctx, queryer, source.table, source.timestamp)
		if err != nil {
			for _, inspected := range specs {
				result.ActiveCoverage = append(result.ActiveCoverage, inspected.ActiveCoverage)
				result.LifecycleCoverage = append(result.LifecycleCoverage, inspected.LifecycleCoverage)
			}
			result.ActiveCoverage = append(result.ActiveCoverage, spec.ActiveCoverage)
			result.LifecycleCoverage = append(result.LifecycleCoverage, spec.LifecycleCoverage)
			return result, err
		}
		specs = append(specs, spec)
	}
	result.ActivityComplete = true
	for _, spec := range specs {
		if spec.Table == "frame_slice" {
			result.FrameProfile = spec.Profile
			result.FrameProfileSource = spec.ProfileSource
		}
		result.ActivityComplete = result.ActivityComplete && spec.Complete
	}

	allowInferredCuts := result.CreationComplete && result.TerminalComplete && result.ActivityComplete
	for _, spec := range specs {
		var activeItem, lifecycleItem TraceDBCoverage
		var localActive map[int64]bool
		if spec.Table == "callstack" {
			activeItem, lifecycleItem, localActive, err = scanTraceDBCallstackActivity(ctx, queryer, builder, identities, spec, allowInferredCuts)
		} else {
			activeItem, lifecycleItem, localActive, err = scanTraceDBTableActivity(ctx, queryer, builder, identities, spec, allowInferredCuts)
		}
		result.ActiveCoverage = append(result.ActiveCoverage, activeItem)
		result.LifecycleCoverage = append(result.LifecycleCoverage, lifecycleItem)
		if err != nil {
			return result, err
		}
		for itid := range localActive {
			result.ActiveITIDs[itid] = true
		}
	}

	result.Lifecycle = builder.finalize()
	result.MalformedPointBudgetExceeded = builder.malformedPointBudgetExceeded
	summary := traceDBLifecycleCollectionCoverage(result)
	result.LifecycleCoverage = append([]TraceDBCoverage{summary}, result.LifecycleCoverage...)
	return result, nil
}

func collectTraceDBLifecycleCreations(ctx context.Context, queryer traceDBQueryer, builder *traceDBLifecycleBuilder) (TraceDBCoverage, bool, error) {
	const table = "instant"
	coverage, columns, err := inspectTraceDBLifecycleSchemaCoverage(ctx, queryer, "resolver.lifecycle.creation", table,
		[]string{"ts", "name", "ref", "ref_type"})
	if err != nil || !coverage.Found {
		return coverage, false, err
	}
	complete := len(coverage.ColumnsMissing) == 0
	hasTS := traceDBColumnListHas(columns, "ts")
	hasName := traceDBColumnListHas(columns, "name")
	hasRef := traceDBColumnListHas(columns, "ref")
	hasRefType := traceDBColumnListHas(columns, "ref_type")
	coverage.FieldSources = map[string]string{
		"creation":               "raw exact instant.name=sched_wakeup_new + ref_type=itid + strict ts/ref; raw wakeup rows never create generations",
		"identity":               "instant.ref signed-int32/canonical compatibility decoder, then exact audited canonical ITID",
		"malformed_localization": "known lane+time=point; known lane+unknown time=lane taint; unknown lane+time=global point; both unknown=global taint",
	}
	if !hasName {
		traceDBAppendCoverageSkipped(&coverage, "creation_authority_complete=false")
		return coverage, complete, nil
	}
	query := fmt.Sprintf("SELECT %s, %s, %s, %s FROM %s",
		traceDBOptionalColumnExpr(hasTS, "ts"), quoteSQLiteIdent("name"),
		traceDBOptionalColumnExpr(hasRef, "ref"), traceDBOptionalColumnExpr(hasRefType, "ref_type"), quoteSQLiteIdent(table))
	rows, err := queryer.QueryContext(ctx, query)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, complete, err
	}
	defer rows.Close()
	coverage.RowsRead = 0
	reasons := map[string]int{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return coverage, complete, err
		}
		coverage.RowsRead++
		var tsRaw, nameRaw, refRaw, refTypeRaw any
		if err := rows.Scan(&tsRaw, &nameRaw, &refRaw, &refTypeRaw); err != nil {
			coverage.Error = err.Error()
			return coverage, complete, err
		}
		exact, shaped := traceDBLifecycleCreationToken(nameRaw)
		if !exact && !shaped {
			continue
		}
		ts, tsKnown := traceDBLifecycleTimestamp(tsRaw)
		ref, refKnown := traceDBStrictSignedUint32Projection(refRaw)
		refTypeExact := false
		if text, ok := refTypeRaw.(string); ok {
			refTypeExact = text == "itid"
		}
		if tsKnown && traceDBBeforeCaptureStart(builder.identities, ts) {
			reasons["pre_capture_row"]++
			continue
		}
		if refTypeExact && refKnown && ref > 0 && traceDBCanonicalThreadIdentityKnown(builder.identities, ref) {
			if _, ok := builder.thread(ref); !ok {
				reasons["public_tid_unavailable_for_generation"]++
				continue
			}
		}
		if exact && refTypeExact && refKnown && ref > 0 && tsKnown {
			if builder.addCreation(ref, ts) {
				coverage.RowsEmitted++
				continue
			}
		}
		if refTypeExact && refKnown && ref == 0 {
			reasons["idle_pseudo_not_generation"]++
			continue
		}
		candidates := []int64(nil)
		if refTypeExact && refKnown {
			candidates = append(candidates, ref)
		}
		reasons[traceDBMarkMalformedLifecycle(builder, candidates, !refKnown || !refTypeExact, ts, tsKnown)]++
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return coverage, complete, err
	}
	if !complete {
		traceDBAppendCoverageSkipped(&coverage, "creation_authority_complete=false")
	}
	traceDBAppendCoverageReasons(&coverage, "creation audit", reasons)
	return coverage, complete, nil
}

func collectTraceDBThreadStateTerminals(ctx context.Context, queryer traceDBQueryer, builder *traceDBLifecycleBuilder) (TraceDBCoverage, bool, error) {
	return collectTraceDBLifecycleTerminals(ctx, queryer, builder, "thread_state", "state", false)
}

func collectTraceDBSchedTerminals(ctx context.Context, queryer traceDBQueryer, builder *traceDBLifecycleBuilder) (TraceDBCoverage, bool, error) {
	return collectTraceDBLifecycleTerminals(ctx, queryer, builder, "sched_slice", "end_state", true)
}

func collectTraceDBLifecycleTerminals(ctx context.Context, queryer traceDBQueryer, builder *traceDBLifecycleBuilder,
	table, stateColumn string, checkedEnd bool,
) (TraceDBCoverage, bool, error) {
	required := []string{"ts", "itid", stateColumn}
	if checkedEnd {
		required = append(required, "dur")
	}
	coverage, columns, err := inspectTraceDBLifecycleSchemaCoverage(ctx, queryer, "resolver.lifecycle.terminal", table, required)
	if err != nil || !coverage.Found {
		return coverage, false, err
	}
	complete := len(coverage.ColumnsMissing) == 0
	hasTS := traceDBColumnListHas(columns, "ts")
	hasITID := traceDBColumnListHas(columns, "itid")
	hasState := traceDBColumnListHas(columns, stateColumn)
	hasDur := !checkedEnd || traceDBColumnListHas(columns, "dur")
	coverage.FieldSources = map[string]string{
		"terminal":  "raw exact X/Z token only; near tokens poison but never gain terminal authority",
		"timestamp": map[bool]string{true: "checked sched_slice.ts+dur; ts_end is never consumed", false: "thread_state.ts"}[checkedEnd],
		"identity":  table + ".itid strict canonical internal uint32 resolved through audited thread identity",
	}
	if !hasState {
		traceDBAppendCoverageSkipped(&coverage, "terminal_authority_complete=false")
		return coverage, complete, nil
	}
	durExpr := "NULL"
	if checkedEnd && hasDur {
		durExpr = quoteSQLiteIdent("dur")
	}
	query := fmt.Sprintf("SELECT %s, %s, %s, %s FROM %s",
		traceDBOptionalColumnExpr(hasTS, "ts"), durExpr, traceDBOptionalColumnExpr(hasITID, "itid"),
		quoteSQLiteIdent(stateColumn), quoteSQLiteIdent(table))
	rows, err := queryer.QueryContext(ctx, query)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, complete, err
	}
	defer rows.Close()
	coverage.RowsRead = 0
	reasons := map[string]int{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return coverage, complete, err
		}
		coverage.RowsRead++
		var tsRaw, durRaw, itidRaw, stateRaw any
		if err := rows.Scan(&tsRaw, &durRaw, &itidRaw, &stateRaw); err != nil {
			coverage.Error = err.Error()
			return coverage, complete, err
		}
		kind, exact, shaped := traceDBLifecycleTerminalToken(stateRaw)
		if !exact && !shaped {
			continue
		}
		timestamp, timestampKnown := traceDBLifecycleTimestamp(tsRaw)
		if checkedEnd {
			dur, durKnown := traceDBStrictSQLiteInt(durRaw)
			if !timestampKnown || !durKnown || dur < 0 || timestamp > math.MaxInt64-dur {
				timestampKnown = false
			} else {
				timestamp += dur
			}
		}
		if timestampKnown && traceDBBeforeCaptureStart(builder.identities, timestamp) {
			reasons["pre_capture_row"]++
			continue
		}
		itid, itidKnown := traceDBStrictInternalID(itidRaw)
		if itidKnown && itid > 0 && traceDBCanonicalThreadIdentityKnown(builder.identities, itid) {
			if _, ok := builder.thread(itid); !ok {
				reasons["public_tid_unavailable_for_generation"]++
				continue
			}
		}
		if exact && itidKnown && itid > 0 && timestampKnown {
			if builder.addTerminal(itid, timestamp, kind) {
				coverage.RowsEmitted++
				continue
			}
		}
		if itidKnown && itid == 0 {
			reasons["idle_pseudo_not_generation"]++
			continue
		}
		candidates := []int64(nil)
		if itidKnown {
			candidates = append(candidates, itid)
		}
		reasons[traceDBMarkMalformedLifecycle(builder, candidates, !itidKnown, timestamp, timestampKnown)]++
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return coverage, complete, err
	}
	if !complete {
		traceDBAppendCoverageSkipped(&coverage, "terminal_authority_complete=false")
	}
	traceDBAppendCoverageReasons(&coverage, "terminal audit", reasons)
	return coverage, complete, nil
}

func inspectTraceDBCallstackActivity(ctx context.Context, queryer traceDBQueryer) (traceDBLifecycleActivitySpec, error) {
	const table = "callstack"
	spec := traceDBLifecycleActivitySpec{Table: table, TimestampColumn: "ts", Profile: traceDBActivityITIDCanonical}
	base, columns, err := inspectTraceDBLifecycleSchemaCoverage(ctx, queryer, "resolver.active_thread", table, nil)
	spec.ActiveCoverage = base
	spec.ActiveCoverage.FieldSources = map[string]string{
		"canonical_identity": "row-level callstack.itid and callstack.callid->audited source map; both must converge when non-NULL",
		"dedup":              "after strict Go decoding; SQL DISTINCT is forbidden because numeric-equivalent SQLite storage classes are order-sensitive",
	}
	spec.LifecycleCoverage = traceDBLifecycleCoverageForColumns(base, "resolver.lifecycle.activity", nil, nil)
	if err != nil || !base.Found {
		return spec, err
	}
	spec.HasTimestamp = traceDBColumnListHas(columns, "ts")
	spec.HasITID = traceDBColumnListHas(columns, "itid")
	spec.HasCallID = traceDBColumnListHas(columns, "callid")
	active := traceDBLifecycleCoverageForColumns(base, "resolver.active_thread", nil, columns)
	lifecycle := traceDBLifecycleCoverageForColumns(base, "resolver.lifecycle.activity", []string{"ts"}, columns)
	for column, present := range map[string]bool{"itid": spec.HasITID, "callid": spec.HasCallID} {
		if present {
			active.ColumnsPresent = appendTraceDBCoverageColumn(active.ColumnsPresent, column)
			lifecycle.ColumnsPresent = appendTraceDBCoverageColumn(lifecycle.ColumnsPresent, column)
		}
	}
	if !spec.HasITID && !spec.HasCallID {
		active.ColumnsMissing = append(active.ColumnsMissing, "itid|callid")
		active.Skipped = "missing required identity columns: itid|callid"
		lifecycle.ColumnsMissing = append(lifecycle.ColumnsMissing, "itid|callid")
	}
	sort.Strings(active.ColumnsPresent)
	sort.Strings(active.ColumnsMissing)
	sort.Strings(lifecycle.ColumnsPresent)
	sort.Strings(lifecycle.ColumnsMissing)
	if len(lifecycle.ColumnsMissing) > 0 {
		lifecycle.Skipped = "missing required columns: " + strings.Join(lifecycle.ColumnsMissing, ",")
	}
	spec.Complete = spec.HasTimestamp && (spec.HasITID || spec.HasCallID)
	spec.ProfileSource = "callstack.itid canonical and/or callid canonical source alias; dual claims must converge"
	active.FieldSources = spec.ActiveCoverage.FieldSources
	lifecycle.FieldSources = map[string]string{
		"canonical_identity": spec.ProfileSource,
		"timestamp":          "callstack.ts strict non-negative SQLite INTEGER",
		"physical_rows":      "every row audited before Go-side identity convergence; no SQL repair or deduplication",
	}
	if !spec.Complete {
		traceDBAppendCoverageSkipped(&lifecycle, "activity_authority_complete=false")
	}
	spec.ActiveCoverage = active
	spec.LifecycleCoverage = lifecycle
	return spec, nil
}

func inspectTraceDBTableActivity(ctx context.Context, queryer traceDBQueryer, table, timestampColumn string) (traceDBLifecycleActivitySpec, error) {
	spec := traceDBLifecycleActivitySpec{Table: table, TimestampColumn: timestampColumn}
	base, columns, err := inspectTraceDBLifecycleSchemaCoverage(ctx, queryer, "resolver.active_thread", table, nil)
	spec.ActiveCoverage = base
	spec.LifecycleCoverage = traceDBLifecycleCoverageForColumns(base, "resolver.lifecycle.activity", nil, nil)
	if err != nil || !base.Found {
		return spec, err
	}
	spec.HasTimestamp = traceDBColumnListHas(columns, timestampColumn)
	spec.HasITID = traceDBColumnListHas(columns, "itid")
	active := traceDBLifecycleCoverageForColumns(base, "resolver.active_thread", []string{"itid"}, columns)
	lifecycle := traceDBLifecycleCoverageForColumns(base, "resolver.lifecycle.activity", []string{timestampColumn, "itid"}, columns)
	spec.Profile, spec.ProfileSource, err = traceDBActivityProfile(ctx, queryer, table)
	if err != nil {
		active.Error = err.Error()
		lifecycle.Error = err.Error()
		spec.ActiveCoverage = active
		spec.LifecycleCoverage = lifecycle
		return spec, err
	}
	spec.Complete = spec.HasTimestamp && spec.HasITID && spec.Profile != traceDBActivityITIDUnsupported
	if spec.HasITID {
		active.FieldSources = map[string]string{
			"canonical_identity": table + ".itid; " + spec.Profile.provenance() + "; nonzero identity must exist in the audited thread index",
			"dedup":              "after strict Go decoding; every physical row is audited without DISTINCT/COALESCE/NULL filtering",
			"schema_profile":     spec.ProfileSource,
		}
		if spec.Profile == traceDBActivityITIDUnsupported {
			active.Skipped = spec.ProfileSource
		}
	}
	lifecycle.FieldSources = map[string]string{
		"canonical_identity": table + ".itid; " + spec.Profile.provenance() + "; nonzero identity must resolve through the audited thread index",
		"schema_profile":     spec.ProfileSource,
		"timestamp":          table + "." + timestampColumn + " strict non-negative SQLite INTEGER",
		"physical_rows":      "every row audited without SQL repair or deduplication",
	}
	if !spec.Complete {
		if spec.Profile == traceDBActivityITIDUnsupported {
			traceDBAppendCoverageSkipped(&lifecycle, spec.ProfileSource)
		}
		traceDBAppendCoverageSkipped(&lifecycle, "activity_authority_complete=false")
	}
	spec.ActiveCoverage = active
	spec.LifecycleCoverage = lifecycle
	return spec, nil
}

// inspectTraceDBLifecycleSchemaCoverage deliberately performs no COUNT query.
// Every collector data pass counts the physical rows it actually reads; a
// preflight COUNT would add a full redundant scan to the largest tables.
func inspectTraceDBLifecycleSchemaCoverage(ctx context.Context, queryer traceDBQueryer, family, table string,
	requiredColumns []string,
) (item TraceDBCoverage, columns []string, err error) {
	start := time.Now()
	defer func() { traceDBSetCoverageElapsed(&item, start) }()
	item = TraceDBCoverage{Family: family, Table: table, Role: traceDBCoverageRole(family, table)}
	var one int
	err = queryer.QueryRowContext(ctx,
		`SELECT 1 FROM sqlite_master WHERE type='table' AND name=?1 COLLATE NOCASE LIMIT 1`, table).Scan(&one)
	if err == sql.ErrNoRows {
		item.Skipped = "missing table"
		return item, nil, nil
	}
	if err != nil {
		item.Error = err.Error()
		return item, nil, err
	}
	item.Found = true
	columns, err = traceDBColumnNames(ctx, queryer, table)
	if err != nil {
		item.Error = err.Error()
		return item, nil, err
	}
	columnSet := make(map[string]bool, len(columns))
	for _, column := range columns {
		columnSet[sqliteASCIIIdentifierFold(column)] = true
	}
	for _, column := range requiredColumns {
		if columnSet[sqliteASCIIIdentifierFold(column)] {
			item.ColumnsPresent = append(item.ColumnsPresent, column)
		} else {
			item.ColumnsMissing = append(item.ColumnsMissing, column)
		}
	}
	sort.Strings(item.ColumnsPresent)
	sort.Strings(item.ColumnsMissing)
	if len(item.ColumnsMissing) > 0 {
		item.Skipped = "missing required columns: " + strings.Join(item.ColumnsMissing, ",")
	}
	return item, columns, nil
}

func traceDBLifecycleCoverageForColumns(base TraceDBCoverage, family string, required, columns []string) TraceDBCoverage {
	out := base
	out.Family = family
	out.Role = traceDBCoverageRole(family, base.Table)
	if !base.Found || base.Error != "" {
		return out
	}
	out.ColumnsPresent = nil
	out.ColumnsMissing = nil
	out.Skipped = ""
	for _, column := range required {
		if traceDBColumnListHas(columns, column) {
			out.ColumnsPresent = append(out.ColumnsPresent, column)
		} else {
			out.ColumnsMissing = append(out.ColumnsMissing, column)
		}
	}
	sort.Strings(out.ColumnsPresent)
	sort.Strings(out.ColumnsMissing)
	if len(out.ColumnsMissing) > 0 {
		out.Skipped = "missing required columns: " + strings.Join(out.ColumnsMissing, ",")
	}
	return out
}

func scanTraceDBCallstackActivity(ctx context.Context, queryer traceDBQueryer, builder *traceDBLifecycleBuilder,
	identities traceDBThreadIndex, spec traceDBLifecycleActivitySpec, allowEvidence bool,
) (TraceDBCoverage, TraceDBCoverage, map[int64]bool, error) {
	activeCoverage := spec.ActiveCoverage
	lifecycleCoverage := spec.LifecycleCoverage
	localActive := map[int64]bool{}
	if !activeCoverage.Found {
		return activeCoverage, lifecycleCoverage, localActive, nil
	}
	query := fmt.Sprintf("SELECT %s, %s, %s FROM %s",
		traceDBOptionalColumnExpr(spec.HasTimestamp, "ts"), traceDBOptionalColumnExpr(spec.HasITID, "itid"),
		traceDBOptionalColumnExpr(spec.HasCallID, "callid"), quoteSQLiteIdent(spec.Table))
	rows, err := queryer.QueryContext(ctx, query)
	if err != nil {
		activeCoverage.Error = err.Error()
		lifecycleCoverage.Error = err.Error()
		return activeCoverage, lifecycleCoverage, localActive, err
	}
	defer rows.Close()
	activeCoverage.RowsRead = 0
	lifecycleCoverage.RowsRead = 0
	activeReasons := map[string]int{}
	lifecycleReasons := map[string]int{}
	var cursor *traceDBLifecycleActivityCursor
	if allowEvidence {
		cursor = builder.newActivityCursor()
	}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return activeCoverage, lifecycleCoverage, localActive, err
		}
		activeCoverage.RowsRead++
		lifecycleCoverage.RowsRead++
		var tsRaw, itidRaw, callIDRaw any
		if err := rows.Scan(&tsRaw, &itidRaw, &callIDRaw); err != nil {
			activeCoverage.Error = err.Error()
			lifecycleCoverage.Error = err.Error()
			return activeCoverage, lifecycleCoverage, localActive, err
		}

		if spec.HasITID || spec.HasCallID {
			activeITID, _, reason := traceDBResolveCallstackEmitterIdentity(identities, spec.HasITID, spec.HasCallID, itidRaw, callIDRaw)
			switch {
			case reason != "":
				activeReasons[reason]++
			case activeITID == 0:
				activeReasons["idle_pseudo_not_callstack_emitter"]++
			case !traceDBCanonicalThreadIdentityKnown(identities, activeITID):
				activeReasons["unresolved_canonical_itid"]++
			default:
				localActive[activeITID] = true
			}
		}

		ts, tsKnown := traceDBLifecycleTimestamp(tsRaw)
		if tsKnown && traceDBBeforeCaptureStart(identities, ts) {
			lifecycleReasons["pre_capture_row"]++
			continue
		}
		resolved := traceDBResolveLifecycleCallstackIdentity(identities, spec.HasITID, spec.HasCallID, itidRaw, callIDRaw)
		if resolved.Valid && resolved.ITID == 0 {
			lifecycleReasons["idle_pseudo_not_callstack_emitter"]++
			continue
		}
		if resolved.Valid {
			if traceDBCanonicalThreadIdentityKnown(identities, resolved.ITID) {
				if _, ok := builder.thread(resolved.ITID); !ok {
					lifecycleReasons["public_tid_unavailable_for_generation"]++
					continue
				}
				if !tsKnown {
					builder.taintITID(resolved.ITID)
					lifecycleReasons["lane_unplaceable_timestamp"]++
					continue
				}
				if cursor != nil {
					if cursor.observe(resolved.ITID, ts) {
						lifecycleCoverage.RowsEmitted++
					}
				} else {
					lifecycleReasons["evidence_withheld_incomplete_authority"]++
				}
				continue
			}
		}
		lifecycleReasons[traceDBMarkMalformedLifecycle(builder, resolved.Candidates, resolved.UnknownClaim, ts, tsKnown)]++
	}
	if err := rows.Err(); err != nil {
		activeCoverage.Error = err.Error()
		lifecycleCoverage.Error = err.Error()
		return activeCoverage, lifecycleCoverage, localActive, err
	}
	activeCoverage.RowsEmitted = len(localActive)
	if spec.HasITID || spec.HasCallID {
		activeCoverage.Skipped = traceDBActiveThreadSkipSummary(activeReasons)
	}
	traceDBAppendCoverageReasons(&lifecycleCoverage, "activity audit", lifecycleReasons)
	return activeCoverage, lifecycleCoverage, localActive, nil
}

func scanTraceDBTableActivity(ctx context.Context, queryer traceDBQueryer, builder *traceDBLifecycleBuilder,
	identities traceDBThreadIndex, spec traceDBLifecycleActivitySpec, allowEvidence bool,
) (TraceDBCoverage, TraceDBCoverage, map[int64]bool, error) {
	activeCoverage := spec.ActiveCoverage
	lifecycleCoverage := spec.LifecycleCoverage
	localActive := map[int64]bool{}
	if !activeCoverage.Found {
		return activeCoverage, lifecycleCoverage, localActive, nil
	}
	query := fmt.Sprintf("SELECT %s, %s FROM %s",
		traceDBOptionalColumnExpr(spec.HasTimestamp, spec.TimestampColumn), traceDBOptionalColumnExpr(spec.HasITID, "itid"), quoteSQLiteIdent(spec.Table))
	rows, err := queryer.QueryContext(ctx, query)
	if err != nil {
		activeCoverage.Error = err.Error()
		lifecycleCoverage.Error = err.Error()
		return activeCoverage, lifecycleCoverage, localActive, err
	}
	defer rows.Close()
	activeCoverage.RowsRead = 0
	lifecycleCoverage.RowsRead = 0
	activeReasons := map[string]int{}
	lifecycleReasons := map[string]int{}
	activeAuthority := spec.HasITID && spec.Profile != traceDBActivityITIDUnsupported
	var cursor *traceDBLifecycleActivityCursor
	if allowEvidence {
		cursor = builder.newActivityCursor()
	}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return activeCoverage, lifecycleCoverage, localActive, err
		}
		activeCoverage.RowsRead++
		lifecycleCoverage.RowsRead++
		var tsRaw, itidRaw any
		if err := rows.Scan(&tsRaw, &itidRaw); err != nil {
			activeCoverage.Error = err.Error()
			lifecycleCoverage.Error = err.Error()
			return activeCoverage, lifecycleCoverage, localActive, err
		}
		itid, itidKnown := spec.Profile.decode(itidRaw)
		if activeAuthority {
			switch {
			case !itidKnown:
				activeReasons["invalid_itid"]++
			case itid == 0 && spec.Table != "sched_slice" && spec.Table != "thread_state":
				activeReasons["idle_pseudo_not_thread_emitter"]++
			case !traceDBCanonicalThreadIdentityKnown(identities, itid):
				activeReasons["unresolved_itid"]++
			default:
				localActive[itid] = true
			}
		}

		ts, tsKnown := traceDBLifecycleTimestamp(tsRaw)
		if tsKnown && traceDBBeforeCaptureStart(identities, ts) {
			lifecycleReasons["pre_capture_row"]++
			continue
		}
		if itidKnown && itid == 0 {
			if spec.Table != "sched_slice" && spec.Table != "thread_state" {
				lifecycleReasons["idle_pseudo_not_thread_emitter"]++
			}
			continue
		}
		if itidKnown {
			if traceDBCanonicalThreadIdentityKnown(identities, itid) {
				if _, ok := builder.thread(itid); !ok {
					lifecycleReasons["public_tid_unavailable_for_generation"]++
					continue
				}
				if !tsKnown {
					builder.taintITID(itid)
					lifecycleReasons["lane_unplaceable_timestamp"]++
					continue
				}
				if cursor != nil {
					if cursor.observe(itid, ts) {
						lifecycleCoverage.RowsEmitted++
					}
				} else {
					lifecycleReasons["evidence_withheld_incomplete_authority"]++
				}
				continue
			}
		}
		candidates := []int64(nil)
		if itidKnown {
			candidates = append(candidates, itid)
		}
		lifecycleReasons[traceDBMarkMalformedLifecycle(builder, candidates, !itidKnown, ts, tsKnown)]++
	}
	if err := rows.Err(); err != nil {
		activeCoverage.Error = err.Error()
		lifecycleCoverage.Error = err.Error()
		return activeCoverage, lifecycleCoverage, localActive, err
	}
	activeCoverage.RowsEmitted = len(localActive)
	if activeAuthority {
		activeCoverage.Skipped = traceDBActiveThreadSkipSummary(activeReasons)
	}
	traceDBAppendCoverageReasons(&lifecycleCoverage, "activity audit", lifecycleReasons)
	return activeCoverage, lifecycleCoverage, localActive, nil
}

func traceDBResolveLifecycleCallstackIdentity(index traceDBThreadIndex, hasITID, hasCallID bool, itidRaw, callIDRaw any) traceDBLifecycleIdentityResolution {
	var out traceDBLifecycleIdentityResolution
	explicitPresent := hasITID && itidRaw != nil
	callPresent := hasCallID && callIDRaw != nil
	valid := true
	var explicit, mapped int64
	explicitKnown, callKnown := false, false
	if explicitPresent {
		explicit, explicitKnown = traceDBStrictInternalID(itidRaw)
		if explicitKnown {
			out.Candidates = append(out.Candidates, explicit)
		} else {
			valid = false
			out.UnknownClaim = true
		}
	}
	if callPresent {
		callID, ok := traceDBStrictInternalID(callIDRaw)
		if !ok || index.AmbiguousThreadID[callID] {
			valid = false
			out.UnknownClaim = true
		} else if callID == 0 {
			mapped, callKnown = 0, true
			out.Candidates = append(out.Candidates, 0)
		} else {
			candidate, mappedOK := index.ThreadIDToITID[callID]
			if !mappedOK || index.AmbiguousITID[candidate] {
				valid = false
				out.UnknownClaim = true
			} else {
				mapped, callKnown = candidate, true
				out.Candidates = append(out.Candidates, mapped)
			}
		}
	}
	if !explicitPresent && !callPresent {
		valid = false
		out.UnknownClaim = true
	}
	switch {
	case !valid:
		return out
	case explicitKnown && callKnown && explicit != mapped:
		return out
	case explicitKnown:
		out.ITID, out.Valid = explicit, true
	case callKnown:
		out.ITID, out.Valid = mapped, true
	default:
		out.UnknownClaim = true
	}
	return out
}

func traceDBMarkMalformedLifecycle(builder *traceDBLifecycleBuilder, candidates []int64, unknownClaim bool,
	timestamp int64, timestampKnown bool,
) string {
	lanes := map[int64]bool{}
	for _, itid := range candidates {
		if itid == 0 {
			continue
		}
		thread, ok := builder.thread(itid)
		if !ok {
			unknownClaim = true
			continue
		}
		lanes[thread.TID] = true
	}
	if len(lanes) == 0 && !unknownClaim {
		return "idle_pseudo_not_generation"
	}
	for tid := range lanes {
		if timestampKnown {
			builder.addPoison(tid, timestamp)
		} else {
			builder.lane(tid).tainted = true
		}
	}
	if unknownClaim || len(lanes) == 0 {
		if timestampKnown {
			builder.addGlobalPoison(timestamp)
			if len(lanes) > 0 {
				return "lane_and_global_point_poison"
			}
			return "global_point_poison"
		}
		builder.taintGlobal()
		if len(lanes) > 0 {
			return "lane_and_global_taint"
		}
		return "global_unplaceable_taint"
	}
	if timestampKnown {
		return "lane_point_poison"
	}
	return "lane_unplaceable_taint"
}

func traceDBLifecycleCreationToken(value any) (exact, shaped bool) {
	text, ok := value.(string)
	if !ok {
		if blob, blobOK := value.([]byte); blobOK {
			return false, strings.EqualFold(strings.TrimSpace(string(blob)), "sched_wakeup_new")
		}
		return false, false
	}
	if text == "sched_wakeup_new" {
		return true, true
	}
	return false, strings.EqualFold(strings.TrimSpace(text), "sched_wakeup_new")
}

func traceDBLifecycleTerminalToken(value any) (kind string, exact, shaped bool) {
	if text, ok := value.(string); ok {
		if text == "X" || text == "Z" {
			return text, true, true
		}
		normalized := strings.ToUpper(strings.TrimSpace(text))
		if normalized == "X" || normalized == "Z" {
			return normalized, false, true
		}
		return "", false, false
	}
	if blob, ok := value.([]byte); ok {
		normalized := strings.ToUpper(strings.TrimSpace(string(blob)))
		if normalized == "X" || normalized == "Z" {
			return normalized, false, true
		}
	}
	return "", false, false
}

func traceDBLifecycleTimestamp(value any) (int64, bool) {
	timestamp, ok := traceDBStrictSQLiteInt(value)
	return timestamp, ok && timestamp >= 0
}

func traceDBOptionalColumnExpr(present bool, column string) string {
	if !present {
		return "NULL"
	}
	return quoteSQLiteIdent(column)
}

func traceDBColumnListHas(columns []string, want string) bool {
	for _, column := range columns {
		if sqliteASCIIIdentifierEqual(column, want) {
			return true
		}
	}
	return false
}

func traceDBAppendCoverageSkipped(coverage *TraceDBCoverage, detail string) {
	detail = strings.TrimSpace(detail)
	if detail == "" || strings.Contains(coverage.Skipped, detail) {
		return
	}
	if coverage.Skipped != "" {
		coverage.Skipped += "; "
	}
	coverage.Skipped += detail
}

func traceDBAppendCoverageReasons(coverage *TraceDBCoverage, prefix string, reasons map[string]int) {
	if len(reasons) == 0 {
		return
	}
	traceDBAppendCoverageSkipped(coverage, prefix+": "+traceDBCountSummary(reasons))
}

func traceDBActiveThreadSkipSummary(reasons map[string]int) string {
	total := 0
	for _, count := range reasons {
		total += count
	}
	if total == 0 {
		return ""
	}
	return fmt.Sprintf("%d active-thread row(s) skipped: %s", total, traceDBCountSummary(reasons))
}

func traceDBLifecycleCollectionCoverage(collection traceDBLifecycleCollection) TraceDBCoverage {
	coverage := TraceDBCoverage{
		Family: "resolver.lifecycle", Table: "__authority__", Role: "resolver_index", Found: true,
		FieldSources: map[string]string{
			"creation_complete":               fmt.Sprintf("%t", collection.CreationComplete),
			"terminal_complete":               fmt.Sprintf("%t", collection.TerminalComplete),
			"activity_complete":               fmt.Sprintf("%t", collection.ActivityComplete),
			"inferred_cut_gate":               "creation_complete && terminal_complete && activity_complete",
			"global_taint":                    fmt.Sprintf("%t", collection.Lifecycle.GlobalTaint),
			"global_poison_points":            fmt.Sprintf("%d", len(collection.Lifecycle.GlobalPoison)),
			"malformed_point_budget_exceeded": fmt.Sprintf("%t", collection.MalformedPointBudgetExceeded),
		},
	}
	coverage.RowsEmitted += len(collection.Lifecycle.GlobalPoison)
	for _, lane := range collection.Lifecycle.ByTID {
		coverage.RowsEmitted += len(lane.Cuts) + len(lane.Terminals) + len(lane.PoisonPoints) + len(lane.UnknownStarts)
	}
	var incomplete []string
	if !collection.CreationComplete {
		incomplete = append(incomplete, "creation")
	}
	if !collection.TerminalComplete {
		incomplete = append(incomplete, "terminal")
	}
	if !collection.ActivityComplete {
		incomplete = append(incomplete, "activity")
	}
	if len(incomplete) > 0 {
		coverage.Skipped = "lifecycle_authority_incomplete=" + strings.Join(incomplete, ",")
	}
	if collection.MalformedPointBudgetExceeded {
		traceDBAppendCoverageSkipped(&coverage, "malformed_point_budget_exceeded=true; escalated_to_global_taint")
	} else if collection.Lifecycle.GlobalTaint {
		traceDBAppendCoverageSkipped(&coverage, "global_taint=true")
	}
	return coverage
}
