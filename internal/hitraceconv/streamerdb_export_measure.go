package hitraceconv

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type traceDBQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type traceDBMeasureSample struct {
	StableID            int64
	StableIDValid       bool
	TS                  int64
	TSValid             bool
	Value               int64
	ValueValid          bool
	FilterID            int64
	FilterIDValid       bool
	FilterPoisonID      int64
	FilterPoisonIDKnown bool
}

type traceDBCPUMeasureFilter struct {
	ID   int64
	Name string
	CPU  int64
}

type traceDBCPUMeasureLaneKey struct {
	Name string
	CPU  int64
}

type traceDBCPUMeasureLane struct {
	Filter    traceDBCPUMeasureFilter
	Invalid   bool
	HasTS     bool
	LastTS    int64
	ValidRows int
}

type traceDBLimitAuditState struct {
	HasTS              bool
	LastTS             int64
	Min, Max           int64
	MinKnown, MaxKnown bool
	PendingTS          int64
	Pending            bool
	Invalid            bool
	Rollback           bool
	InvalidTuple       bool
	Rows               int
}

type traceDBLimitEmitState struct {
	Min, Max           int64
	MinKnown, MaxKnown bool
	PendingTS          int64
	Pending            bool
	Updates            int
	Representative     traceDBMeasureSample
}

type traceDBClockMeasureFilter struct {
	ID       int64
	Name     string
	CPU      int64
	CPUKnown bool
}

type traceDBClockMeasureLaneKey struct {
	Name     string
	CPU      int64
	CPUKnown bool
}

type traceDBClockMeasureLane struct {
	Filter    traceDBClockMeasureFilter
	Invalid   bool
	HasTS     bool
	LastTS    int64
	ValidRows int
}

func exportTraceDBMeasureFamilies(ctx context.Context, tdb *traceDB, sink *traceDBRowSink) ([]TraceDBCoverage, error) {
	tx, err := tdb.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	measureCoverage, err := inspectTraceDBMeasureCoverage(ctx, tx, "counter", "measure", []string{"ts", "value", "filter_id"})
	if err != nil {
		return []TraceDBCoverage{measureCoverage}, err
	}
	cpuCoverage := measureCoverage
	cpuCoverage.SourceTables = []string{"measure", "cpu_measure_filter"}
	cpuFilterCoverage, err := inspectTraceDBMeasureCoverage(ctx, tx, "counter", "cpu_measure_filter", []string{"id", "name", "cpu"})
	if err != nil {
		return []TraceDBCoverage{cpuCoverage, cpuFilterCoverage}, err
	}
	genericClockCoverage, err := inspectTraceDBMeasureCoverage(ctx, tx, "counter", "measure_filter", []string{"id", "name", "type"})
	if err != nil {
		return []TraceDBCoverage{cpuCoverage, genericClockCoverage}, err
	}
	specializedClockCoverage, err := inspectTraceDBMeasureCoverage(ctx, tx, "counter", "clock_event_filter", []string{"id", "type", "name", "cpu"})
	if err != nil {
		return []TraceDBCoverage{cpuCoverage, specializedClockCoverage}, err
	}
	useSpecializedClock := specializedClockCoverage.Found
	clockCoverage := genericClockCoverage
	clockCoverage.SourceTables = []string{"measure", "measure_filter"}
	if useSpecializedClock {
		clockCoverage = specializedClockCoverage
		clockCoverage.SourceTables = []string{"measure", "clock_event_filter"}
		if genericClockCoverage.Found {
			clockCoverage.SourceTables = append(clockCoverage.SourceTables, "measure_filter")
		}
	}
	cpuReady := measureCoverage.Found && len(measureCoverage.ColumnsMissing) == 0 &&
		cpuFilterCoverage.Found && len(cpuFilterCoverage.ColumnsMissing) == 0
	genericClockReady := genericClockCoverage.Found && len(genericClockCoverage.ColumnsMissing) == 0
	specializedClockReady := specializedClockCoverage.Found && len(specializedClockCoverage.ColumnsMissing) == 0
	clockRegistryReady := genericClockReady
	if useSpecializedClock {
		// A present specialized registry is authoritative.  A present generic
		// registry is also part of the proof because equal IDs must agree on
		// their clock names.  Never hide a damaged authority table by falling
		// back to a weaker registry.
		clockRegistryReady = specializedClockReady && (!genericClockCoverage.Found || genericClockReady)
	}
	clockReady := measureCoverage.Found && len(measureCoverage.ColumnsMissing) == 0 && clockRegistryReady
	if !measureCoverage.Found || len(measureCoverage.ColumnsMissing) > 0 {
		if clockRegistryReady {
			clockCoverage.Skipped = "missing measure dependency"
		}
		return []TraceDBCoverage{cpuCoverage, clockCoverage}, nil
	}
	if !cpuFilterCoverage.Found || len(cpuFilterCoverage.ColumnsMissing) > 0 {
		cpuCoverage.Skipped = "missing cpu_measure_filter dependency"
	}
	if useSpecializedClock && !clockRegistryReady {
		switch {
		case !specializedClockReady:
			clockCoverage.Skipped = "clock_event_filter is present but incomplete; generic fallback forbidden"
		case genericClockCoverage.Found && !genericClockReady:
			clockCoverage.Skipped = "measure_filter cross-check registry is present but incomplete; specialized export withheld"
		}
	}
	if !cpuReady && !clockReady {
		return []TraceDBCoverage{cpuCoverage, clockCoverage}, nil
	}
	cpuSkipped := map[string]int{}
	clockSkipped := map[string]int{}
	cpuFilters := map[int64]traceDBCPUMeasureFilter{}
	cpuClaimed := map[int64]bool{}
	clockFilters := map[int64]traceDBClockMeasureFilter{}
	clockClaimed := map[int64]bool{}
	if cpuReady {
		cpuFilters, cpuClaimed, err = loadTraceDBCPUMeasureFilters(ctx, tx, cpuSkipped)
		if err != nil {
			cpuCoverage.Error = err.Error()
			return []TraceDBCoverage{cpuCoverage, clockCoverage}, err
		}
	} else if cpuFilterCoverage.Found {
		cpuClaimed, err = loadTraceDBCPUFilterClaims(ctx, tx, cpuFilterCoverage.ColumnsPresent)
		if err != nil {
			clockCoverage.Error = err.Error()
			return []TraceDBCoverage{cpuCoverage, clockCoverage}, err
		}
	}
	if clockReady {
		if useSpecializedClock {
			var specializedFilters map[int64]traceDBClockMeasureFilter
			var specializedClaimed map[int64]bool
			specializedFilters, specializedClaimed, err = loadTraceDBClockEventFilters(ctx, tx, clockSkipped)
			if err == nil && genericClockCoverage.Found {
				var genericFilters map[int64]traceDBClockMeasureFilter
				var genericClaimed map[int64]bool
				genericFilters, genericClaimed, err = loadTraceDBClockMeasureFilters(ctx, tx, clockSkipped)
				if err == nil {
					clockFilters, clockClaimed = reconcileTraceDBSpecializedClockFilters(
						specializedFilters, specializedClaimed, genericFilters, genericClaimed, clockSkipped,
					)
				}
			} else if err == nil {
				clockFilters, clockClaimed = specializedFilters, specializedClaimed
			}
		} else {
			clockFilters, clockClaimed, err = loadTraceDBClockMeasureFilters(ctx, tx, clockSkipped)
		}
		if err != nil {
			clockCoverage.Error = err.Error()
			return []TraceDBCoverage{cpuCoverage, clockCoverage}, err
		}
	} else if clockCoverage.Found {
		var typeColumnMissing bool
		if useSpecializedClock {
			clockClaimed, typeColumnMissing, err = loadTraceDBClockEventFilterClaims(ctx, tx, specializedClockCoverage.ColumnsPresent)
		} else {
			clockClaimed, typeColumnMissing, err = loadTraceDBClockFilterClaims(ctx, tx, clockCoverage.ColumnsPresent)
		}
		if err != nil {
			cpuCoverage.Error = err.Error()
			return []TraceDBCoverage{cpuCoverage, clockCoverage}, err
		}
		if typeColumnMissing && cpuReady {
			cpuSkipped["peer_filter_type_unavailable"]++
		}
	}
	if cpuReady || clockReady {
		for id := range cpuClaimed {
			if !clockClaimed[id] {
				continue
			}
			delete(cpuFilters, id)
			delete(clockFilters, id)
			cpuSkipped["cross_filter_owner_conflict"]++
			clockSkipped["cross_filter_owner_conflict"]++
		}
	}
	relevantFilterIDs := traceDBExportableMeasureFilterIDs(cpuFilters, clockFilters)
	if len(relevantFilterIDs) == 0 {
		if cpuReady {
			cpuCoverage.RowsRead = 0
			cpuCoverage.Skipped = traceDBMeasureNoOutputSummary(cpuSkipped)
		}
		if clockReady {
			clockCoverage.RowsEmitted = 0
			clockCoverage.Skipped = traceDBMeasureNoOutputSummary(clockSkipped)
		}
		if err := tx.Commit(); err != nil {
			return []TraceDBCoverage{cpuCoverage, clockCoverage}, err
		}
		return []TraceDBCoverage{cpuCoverage, clockCoverage}, nil
	}
	hasRelevantSample, err := traceDBHasStrictMeasureSample(ctx, tx, relevantFilterIDs)
	if err != nil {
		return []TraceDBCoverage{cpuCoverage, clockCoverage}, err
	}
	if !hasRelevantSample {
		if cpuReady {
			cpuCoverage.RowsRead = 0
			cpuCoverage.Skipped = traceDBMeasureNoOutputSummary(cpuSkipped)
		}
		if clockReady {
			clockCoverage.RowsEmitted = 0
			clockCoverage.Skipped = traceDBMeasureNoOutputSummary(clockSkipped)
		}
		if err := tx.Commit(); err != nil {
			return []TraceDBCoverage{cpuCoverage, clockCoverage}, err
		}
		return []TraceDBCoverage{cpuCoverage, clockCoverage}, nil
	}
	stableExpr, stableSource, err := traceDBHiddenRowIDExpr(ctx, tx, "measure")
	if err != nil {
		cpuCoverage.Error = err.Error()
		clockCoverage.Error = err.Error()
		return []TraceDBCoverage{cpuCoverage, clockCoverage}, err
	}
	if cpuReady {
		cpuCoverage, err = exportTraceDBCPUMeasuresStrict(ctx, tx, stableExpr, stableSource, sink, cpuCoverage, cpuFilters, cpuClaimed, cpuSkipped)
		if err != nil {
			return []TraceDBCoverage{cpuCoverage, clockCoverage}, err
		}
	}
	if clockReady {
		clockCoverage, err = exportTraceDBClockRatesStrict(ctx, tx, stableExpr, stableSource, sink, clockCoverage, clockFilters, clockClaimed, clockSkipped, useSpecializedClock)
		if err != nil {
			return []TraceDBCoverage{cpuCoverage, clockCoverage}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return []TraceDBCoverage{cpuCoverage, clockCoverage}, err
	}
	return []TraceDBCoverage{cpuCoverage, clockCoverage}, nil
}

func inspectTraceDBMeasureCoverage(ctx context.Context, queryer traceDBQueryer, family, table string, requiredColumns []string) (item TraceDBCoverage, err error) {
	start := time.Now()
	defer func() { traceDBSetCoverageElapsed(&item, start) }()
	item = TraceDBCoverage{Family: family, Table: table, Role: traceDBCoverageRole(family, table)}
	var one int
	err = queryer.QueryRowContext(ctx, `SELECT 1 FROM sqlite_master WHERE type='table' AND name=?1 LIMIT 1`, table).Scan(&one)
	if err == sql.ErrNoRows {
		item.Skipped = "missing table"
		return item, nil
	}
	if err != nil {
		item.Error = err.Error()
		return item, err
	}
	item.Found = true
	columns, err := traceDBColumnNames(ctx, queryer, table)
	if err != nil {
		item.Error = err.Error()
		return item, err
	}
	columnSet := make(map[string]bool, len(columns))
	for _, column := range columns {
		columnSet[column] = true
	}
	for _, column := range requiredColumns {
		if columnSet[column] {
			item.ColumnsPresent = append(item.ColumnsPresent, column)
		} else {
			item.ColumnsMissing = append(item.ColumnsMissing, column)
		}
	}
	sort.Strings(item.ColumnsPresent)
	sort.Strings(item.ColumnsMissing)
	var rowCount int64
	if err := queryer.QueryRowContext(ctx, `SELECT COUNT(1) FROM `+quoteSQLiteIdent(table)).Scan(&rowCount); err != nil {
		item.Error = err.Error()
		return item, err
	}
	if rowCount < 0 || uint64(rowCount) > uint64(^uint(0)>>1) {
		err := fmt.Errorf("%s row count is outside host int range: %d", table, rowCount)
		item.Error = err.Error()
		return item, err
	}
	item.RowsRead = int(rowCount)
	if len(item.ColumnsMissing) > 0 {
		item.Skipped = "missing required columns: " + strings.Join(item.ColumnsMissing, ",")
	}
	return item, nil
}

// loadTraceDBCPUFilterClaims preserves CPU ownership of the shared
// measure.filter_id namespace even when a vendor schema omits display or CPU
// metadata.  The registry's typed owner key is its id column.
func loadTraceDBCPUFilterClaims(ctx context.Context, queryer traceDBQueryer, columnsPresent []string) (map[int64]bool, error) {
	claimed := map[int64]bool{}
	if !traceDBStringSliceContains(columnsPresent, "id") {
		return claimed, nil
	}
	rows, err := queryer.QueryContext(ctx, `SELECT `+quoteSQLiteIdent("id")+` FROM `+quoteSQLiteIdent("cpu_measure_filter"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var idRaw any
		if err := rows.Scan(&idRaw); err != nil {
			return nil, err
		}
		if id, ok := traceDBPoisonEquivalentNonNegativeInt(idRaw); ok {
			claimed[id] = true
		}
	}
	return claimed, rows.Err()
}

// loadTraceDBClockFilterClaims needs id+type to identify a clock owner when
// only display name is missing.  If an entire vendor schema omits type, exact
// IDs are reserved conservatively and that downgrade is surfaced to coverage.
func loadTraceDBClockFilterClaims(ctx context.Context, queryer traceDBQueryer, columnsPresent []string) (map[int64]bool, bool, error) {
	claimed := map[int64]bool{}
	if !traceDBStringSliceContains(columnsPresent, "id") {
		return claimed, false, nil
	}
	typePresent := traceDBStringSliceContains(columnsPresent, "type")
	selectList := quoteSQLiteIdent("id")
	if typePresent {
		selectList += `, ` + quoteSQLiteIdent("type")
	}
	rows, err := queryer.QueryContext(ctx, `SELECT `+selectList+` FROM `+quoteSQLiteIdent("measure_filter"))
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var idRaw any
		if typePresent {
			var typeRaw any
			if err := rows.Scan(&idRaw, &typeRaw); err != nil {
				return nil, false, err
			}
			typeText, ok := typeRaw.(string)
			if !ok || typeText != "clock_rate_filter" {
				continue
			}
		} else if err := rows.Scan(&idRaw); err != nil {
			return nil, false, err
		}
		if id, ok := traceDBPoisonEquivalentNonNegativeInt(idRaw); ok {
			claimed[id] = true
		}
	}
	return claimed, !typePresent, rows.Err()
}

// loadTraceDBClockEventFilterClaims preserves the authoritative specialized
// owner namespace when a vendor table is present but cannot be exported.  A
// missing type column forces conservative reservation of every comparable ID;
// it never enables generic fallback.
func loadTraceDBClockEventFilterClaims(ctx context.Context, queryer traceDBQueryer, columnsPresent []string) (map[int64]bool, bool, error) {
	claimed := map[int64]bool{}
	if !traceDBStringSliceContains(columnsPresent, "id") {
		return claimed, false, nil
	}
	typePresent := traceDBStringSliceContains(columnsPresent, "type")
	selectList := quoteSQLiteIdent("id")
	if typePresent {
		selectList += `, ` + quoteSQLiteIdent("type")
	}
	rows, err := queryer.QueryContext(ctx, `SELECT `+selectList+` FROM `+quoteSQLiteIdent("clock_event_filter"))
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var idRaw any
		if typePresent {
			var typeRaw any
			if err := rows.Scan(&idRaw, &typeRaw); err != nil {
				return nil, false, err
			}
			typeText, ok := typeRaw.(string)
			if !ok || typeText != "clock_set_rate" {
				continue
			}
		} else if err := rows.Scan(&idRaw); err != nil {
			return nil, false, err
		}
		if id, ok := traceDBPoisonEquivalentNonNegativeInt(idRaw); ok {
			claimed[id] = true
		}
	}
	return claimed, !typePresent, rows.Err()
}

func traceDBStringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func traceDBExportableMeasureFilterIDs(cpuFilters map[int64]traceDBCPUMeasureFilter, clockFilters map[int64]traceDBClockMeasureFilter) map[int64]bool {
	out := make(map[int64]bool, len(cpuFilters)+len(clockFilters))
	limitNames := map[int64]map[string]int64{}
	for id, filter := range cpuFilters {
		switch filter.Name {
		case "cpu_idle", "cpu_frequency":
			out[id] = true
		case "cpu_frequency_limits_min", "cpu_frequency_limits_max":
			if limitNames[filter.CPU] == nil {
				limitNames[filter.CPU] = map[string]int64{}
			}
			limitNames[filter.CPU][filter.Name] = id
		}
	}
	for _, names := range limitNames {
		minID, minOK := names["cpu_frequency_limits_min"]
		maxID, maxOK := names["cpu_frequency_limits_max"]
		if minOK && maxOK {
			out[minID] = true
			out[maxID] = true
		}
	}
	for id := range clockFilters {
		out[id] = true
	}
	return out
}

func traceDBHasStrictMeasureSample(ctx context.Context, queryer traceDBQueryer, relevantFilterIDs map[int64]bool) (bool, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT `+quoteSQLiteIdent("filter_id")+` FROM `+quoteSQLiteIdent("measure"))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var filterRaw any
		if err := rows.Scan(&filterRaw); err != nil {
			return false, err
		}
		filterID, ok := traceDBStrictSQLiteInt(filterRaw)
		if ok && filterID >= 0 && relevantFilterIDs[filterID] {
			return true, nil
		}
	}
	return false, rows.Err()
}

func traceDBMeasureNoOutputSummary(skipped map[string]int) string {
	if summary := traceDBCountSummary(skipped); summary != "" {
		return summary + ",no_strict_exportable_measure_rows=1"
	}
	return "no_strict_exportable_measure_rows=1"
}

func exportTraceDBCPUMeasuresStrict(ctx context.Context, queryer traceDBQueryer, stableExpr, stableSource string, sink *traceDBRowSink, coverage TraceDBCoverage, filters map[int64]traceDBCPUMeasureFilter, claimedFilterIDs map[int64]bool, skipped map[string]int) (TraceDBCoverage, error) {
	coverage.RowsRead = 0
	lanes := map[traceDBCPUMeasureLaneKey]*traceDBCPUMeasureLane{}
	limitAudit := map[int64]*traceDBLimitAuditState{}
	_, err := scanTraceDBMeasureSamples(ctx, queryer, stableExpr, func(sample traceDBMeasureSample) error {
		filterID := sample.FilterID
		if !sample.FilterIDValid {
			if !sample.FilterPoisonIDKnown || !claimedFilterIDs[sample.FilterPoisonID] {
				return nil
			}
			filterID = sample.FilterPoisonID
			coverage.RowsRead++
			if filter, ok := filters[filterID]; ok {
				key := traceDBCPUMeasureLaneKey{Name: filter.Name, CPU: filter.CPU}
				lane := lanes[key]
				if lane == nil {
					lane = &traceDBCPUMeasureLane{Filter: filter}
					lanes[key] = lane
				}
				lane.Invalid = true
			}
			skipped["invalid_filter_id_scalar"]++
			return nil
		}
		if !claimedFilterIDs[filterID] {
			return nil
		}
		coverage.RowsRead++
		filter, ok := filters[filterID]
		if !ok {
			skipped["invalid_or_ambiguous_filter"]++
			return nil
		}
		key := traceDBCPUMeasureLaneKey{Name: filter.Name, CPU: filter.CPU}
		lane := lanes[key]
		if lane == nil {
			lane = &traceDBCPUMeasureLane{Filter: filter}
			lanes[key] = lane
		}
		if !traceDBMeasureSampleStructuralValid(sample) {
			lane.Invalid = true
			skipped["invalid_sample_scalar_or_identity"]++
			return nil
		}
		if !traceDBCPUMeasureSemanticValueValid(filter.Name, sample.Value) {
			lane.Invalid = true
			skipped["invalid_semantic_value"]++
			return nil
		}
		lane.ValidRows++
		if lane.HasTS {
			switch {
			case sample.TS < lane.LastTS:
				lane.Invalid = true
				skipped["timestamp_rollback"]++
			case sample.TS == lane.LastTS:
				lane.Invalid = true
				skipped["duplicate_lane_timestamp"]++
			}
		}
		lane.HasTS = true
		lane.LastTS = sample.TS
		if filter.Name == "cpu_frequency_limits_min" || filter.Name == "cpu_frequency_limits_max" {
			state := limitAudit[filter.CPU]
			if state == nil {
				state = &traceDBLimitAuditState{}
				limitAudit[filter.CPU] = state
			}
			traceDBAuditLimitUpdate(state, sample, filter.Name == "cpu_frequency_limits_min")
		}
		return nil
	})
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	for _, state := range limitAudit {
		traceDBFinishLimitAuditGroup(state)
	}

	limitCPUs := map[int64]bool{}
	for key := range lanes {
		if key.Name == "cpu_frequency_limits_min" || key.Name == "cpu_frequency_limits_max" {
			limitCPUs[key.CPU] = true
		}
	}
	validLimitCPU := map[int64]bool{}
	for cpu := range limitCPUs {
		minLane := lanes[traceDBCPUMeasureLaneKey{Name: "cpu_frequency_limits_min", CPU: cpu}]
		maxLane := lanes[traceDBCPUMeasureLaneKey{Name: "cpu_frequency_limits_max", CPU: cpu}]
		if minLane == nil || maxLane == nil {
			skipped["incomplete_limit_tuple"]++
			if minLane != nil {
				skipped["incomplete_limit_updates"] += minLane.ValidRows
			}
			if maxLane != nil {
				skipped["incomplete_limit_updates"] += maxLane.ValidRows
			}
			continue
		}
		audit := limitAudit[cpu]
		if audit != nil && audit.Rollback {
			skipped["limit_timestamp_rollback"] += audit.Rows
		}
		if audit != nil && audit.InvalidTuple {
			skipped["invalid_limit_tuple"] += audit.Rows
		}
		if minLane.Invalid || maxLane.Invalid || audit == nil || audit.Invalid {
			skipped["limit_lane_fail_closed"] += minLane.ValidRows + maxLane.ValidRows
			continue
		}
		validLimitCPU[cpu] = true
	}

	for key, lane := range lanes {
		if key.Name == "cpu_frequency_limits_min" || key.Name == "cpu_frequency_limits_max" {
			continue
		}
		if lane.Invalid {
			skipped["lane_fail_closed"] += lane.ValidRows
		}
	}
	limitEmit := map[int64]*traceDBLimitEmitState{}
	_, err = scanTraceDBMeasureSamples(ctx, queryer, stableExpr, func(sample traceDBMeasureSample) error {
		if !sample.FilterIDValid {
			return nil
		}
		filter, ok := filters[sample.FilterID]
		if !ok || !traceDBMeasureSampleStructuralValid(sample) || !traceDBCPUMeasureSemanticValueValid(filter.Name, sample.Value) {
			return nil
		}
		lane := lanes[traceDBCPUMeasureLaneKey{Name: filter.Name, CPU: filter.CPU}]
		if lane == nil || lane.Invalid {
			return nil
		}
		switch filter.Name {
		case "cpu_idle":
			body := fmt.Sprintf("cpu_idle: state=%d cpu_id=%d", sample.Value, filter.CPU)
			if err := addTraceDBInstantRow(sink, sample.TS, "<idle>", 0, 0, filter.CPU, body); err != nil {
				return err
			}
			coverage.RowsEmitted++
		case "cpu_frequency":
			body := fmt.Sprintf("cpu_frequency: state=%d cpu_id=%d", sample.Value, filter.CPU)
			if err := addTraceDBInstantRow(sink, sample.TS, "<idle>", 0, 0, filter.CPU, body); err != nil {
				return err
			}
			coverage.RowsEmitted++
		case "cpu_frequency_limits_min", "cpu_frequency_limits_max":
			if !validLimitCPU[filter.CPU] {
				return nil
			}
			state := limitEmit[filter.CPU]
			if state == nil {
				state = &traceDBLimitEmitState{}
				limitEmit[filter.CPU] = state
			}
			if state.Pending && state.PendingTS != sample.TS {
				if err := flushTraceDBLimitTuple(sink, filter.CPU, state, &coverage, skipped); err != nil {
					return err
				}
			}
			if !state.Pending {
				state.Pending = true
				state.PendingTS = sample.TS
				state.Representative = sample
			}
			state.Updates++
			if filter.Name == "cpu_frequency_limits_min" {
				state.Min, state.MinKnown = sample.Value, true
			} else {
				state.Max, state.MaxKnown = sample.Value, true
			}
		}
		return nil
	})
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	limitCPUList := make([]int64, 0, len(limitEmit))
	for cpu := range limitEmit {
		limitCPUList = append(limitCPUList, cpu)
	}
	sort.Slice(limitCPUList, func(i, j int) bool { return limitCPUList[i] < limitCPUList[j] })
	for _, cpu := range limitCPUList {
		if err := flushTraceDBLimitTuple(sink, cpu, limitEmit[cpu], &coverage, skipped); err != nil {
			return coverage, err
		}
	}
	coverage.Skipped = traceDBCountSummary(skipped)
	coverage.FieldSources = map[string]string{
		"stable_identity": stableSource,
		"timestamp":       "measure.ts exact SQLite INTEGER; hidden-rowid source order must be monotonic per semantic lane",
		"value":           "measure.value exact finite integral SQLite INTEGER or lossless integral REAL",
		"cpu":             "cpu_measure_filter.cpu exact SQLite INTEGER in 0..4095; CPU0 preserved",
		"limit_tuple":     "per-CPU atomic state; publish only after exact min and max are both known and min<=max",
	}
	return coverage, nil
}

func exportTraceDBClockRatesStrict(ctx context.Context, queryer traceDBQueryer, stableExpr, stableSource string, sink *traceDBRowSink, coverage TraceDBCoverage, filters map[int64]traceDBClockMeasureFilter, claimedFilterIDs map[int64]bool, skipped map[string]int, specializedAuthority bool) (TraceDBCoverage, error) {
	lanes := map[traceDBClockMeasureLaneKey]*traceDBClockMeasureLane{}
	relatedSamples := 0
	_, err := scanTraceDBMeasureSamples(ctx, queryer, stableExpr, func(sample traceDBMeasureSample) error {
		filterID := sample.FilterID
		if !sample.FilterIDValid {
			if !sample.FilterPoisonIDKnown || !claimedFilterIDs[sample.FilterPoisonID] {
				return nil
			}
			filterID = sample.FilterPoisonID
			relatedSamples++
			if filter, ok := filters[filterID]; ok {
				key := traceDBClockMeasureLaneKey{Name: filter.Name, CPU: filter.CPU, CPUKnown: filter.CPUKnown}
				lane := lanes[key]
				if lane == nil {
					lane = &traceDBClockMeasureLane{Filter: filter}
					lanes[key] = lane
				}
				lane.Invalid = true
			}
			skipped["invalid_filter_id_scalar"]++
			return nil
		}
		if !claimedFilterIDs[filterID] {
			return nil
		}
		relatedSamples++
		filter, ok := filters[filterID]
		if !ok {
			skipped["invalid_or_ambiguous_filter"]++
			return nil
		}
		key := traceDBClockMeasureLaneKey{Name: filter.Name, CPU: filter.CPU, CPUKnown: filter.CPUKnown}
		lane := lanes[key]
		if lane == nil {
			lane = &traceDBClockMeasureLane{Filter: filter}
			lanes[key] = lane
		}
		if !traceDBMeasureSampleStructuralValid(sample) || sample.Value < 0 {
			lane.Invalid = true
			skipped["invalid_sample_scalar_or_identity"]++
			return nil
		}
		lane.ValidRows++
		if lane.HasTS {
			switch {
			case sample.TS < lane.LastTS:
				lane.Invalid = true
				skipped["timestamp_rollback"]++
			case sample.TS == lane.LastTS:
				lane.Invalid = true
				skipped["duplicate_lane_timestamp"]++
			}
		}
		lane.HasTS = true
		lane.LastTS = sample.TS
		return nil
	})
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	for _, lane := range lanes {
		if lane.Invalid {
			skipped["lane_fail_closed"] += lane.ValidRows
		}
	}
	_, err = scanTraceDBMeasureSamples(ctx, queryer, stableExpr, func(sample traceDBMeasureSample) error {
		if !sample.FilterIDValid {
			return nil
		}
		filter, ok := filters[sample.FilterID]
		if !ok || !traceDBMeasureSampleStructuralValid(sample) || sample.Value < 0 {
			return nil
		}
		key := traceDBClockMeasureLaneKey{Name: filter.Name, CPU: filter.CPU, CPUKnown: filter.CPUKnown}
		lane := lanes[key]
		if lane == nil || lane.Invalid {
			return nil
		}
		body := fmt.Sprintf("clock_set_rate: %s %d", filter.Name, sample.Value)
		rowCPU := int64(0)
		if filter.CPUKnown {
			body = fmt.Sprintf("clock_set_rate: %s state=%d cpu_id=%d", filter.Name, sample.Value, filter.CPU)
			rowCPU = filter.CPU
		}
		if err := addTraceDBInstantRow(sink, sample.TS, "<kworker>", 0, 0, rowCPU, body); err != nil {
			return err
		}
		coverage.RowsEmitted++
		return nil
	})
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	coverage.Skipped = traceDBCountSummary(skipped)
	coverage.FieldSources = map[string]string{
		"sample_table":    fmt.Sprintf("measure; related_rows_read=%d", relatedSamples),
		"stable_identity": stableSource,
		"timestamp":       "measure.ts exact SQLite INTEGER; hidden-rowid source order must be monotonic per clock lane",
		"value":           "measure.value exact finite integral SQLite INTEGER or lossless integral REAL",
		"clock_identity":  "measure_filter.name exact single wire token",
		"cpu_owner":       "not present in measure_filter schema; cpu_id intentionally omitted",
	}
	if specializedAuthority {
		coverage.FieldSources["clock_identity"] = "clock_event_filter.name exact single wire token; equal measure_filter ID/name cross-checked when generic registry is present"
		coverage.FieldSources["cpu_owner"] = "clock_event_filter.cpu exact SQLite INTEGER in 0..4095; emitted as header CPU and cpu_id"
		coverage.FieldSources["filter_authority"] = "clock_event_filter type=clock_set_rate is authoritative; generic-only IDs withheld"
	}
	return coverage, nil
}

func traceDBMeasureSampleStructuralValid(sample traceDBMeasureSample) bool {
	return sample.StableIDValid && sample.TSValid && sample.ValueValid
}

func traceDBCPUMeasureSemanticValueValid(name string, value int64) bool {
	switch name {
	case "cpu_idle":
		return value >= 0 && value <= math.MaxUint32
	case "cpu_frequency":
		return value > 0
	case "cpu_frequency_limits_min", "cpu_frequency_limits_max":
		return value >= 0
	default:
		return false
	}
}

func traceDBAuditLimitUpdate(state *traceDBLimitAuditState, sample traceDBMeasureSample, isMin bool) {
	state.Rows++
	if state.HasTS && sample.TS < state.LastTS {
		state.Invalid = true
		state.Rollback = true
	}
	if state.Pending && state.PendingTS != sample.TS {
		traceDBFinishLimitAuditGroup(state)
	}
	if !state.Pending {
		state.Pending = true
		state.PendingTS = sample.TS
	}
	if isMin {
		state.Min, state.MinKnown = sample.Value, true
	} else {
		state.Max, state.MaxKnown = sample.Value, true
	}
	state.HasTS = true
	state.LastTS = sample.TS
}

func traceDBFinishLimitAuditGroup(state *traceDBLimitAuditState) {
	if state == nil || !state.Pending {
		return
	}
	if state.MinKnown && state.MaxKnown && state.Min > state.Max {
		state.Invalid = true
		state.InvalidTuple = true
	}
	state.Pending = false
}

func flushTraceDBLimitTuple(sink *traceDBRowSink, cpu int64, state *traceDBLimitEmitState, coverage *TraceDBCoverage, skipped map[string]int) error {
	if state == nil || !state.Pending {
		return nil
	}
	if state.Updates > 1 {
		skipped["limit_updates_coalesced"] += state.Updates - 1
	}
	if !state.MinKnown || !state.MaxKnown {
		skipped["limit_updates_waiting_for_peer"] += state.Updates
		state.Pending = false
		state.Updates = 0
		return nil
	}
	body := fmt.Sprintf("cpu_frequency_limits: min=%d max=%d cpu_id=%d", state.Min, state.Max, cpu)
	if err := addTraceDBInstantRow(sink, state.PendingTS, "<idle>", 0, 0, cpu, body); err != nil {
		return err
	}
	coverage.RowsEmitted++
	state.Pending = false
	state.Updates = 0
	return nil
}

func loadTraceDBCPUMeasureFilters(ctx context.Context, queryer traceDBQueryer, skipped map[string]int) (map[int64]traceDBCPUMeasureFilter, map[int64]bool, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT id, name, cpu FROM cpu_measure_filter ORDER BY id, name, cpu`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	closedName := func(name string) bool {
		switch name {
		case "cpu_idle", "cpu_frequency", "cpu_frequency_limits_min", "cpu_frequency_limits_max":
			return true
		default:
			return false
		}
	}
	candidates := map[int64][]traceDBCPUMeasureFilter{}
	claimed := map[int64]bool{}
	invalidID := map[int64]bool{}
	rowCounts := map[int64]int{}
	for rows.Next() {
		var idRaw, nameRaw, cpuRaw any
		if err := rows.Scan(&idRaw, &nameRaw, &cpuRaw); err != nil {
			return nil, nil, err
		}
		id, idOK := traceDBStrictSQLiteInt(idRaw)
		comparableID, comparableOK := traceDBPoisonEquivalentNonNegativeInt(idRaw)
		if comparableOK {
			rowCounts[comparableID]++
			claimed[comparableID] = true
		}
		name, nameOK := nameRaw.(string)
		if !nameOK || !closedName(name) {
			continue
		}
		if !idOK || id < 0 {
			if comparableOK {
				invalidID[comparableID] = true
			}
		}
		cpu, cpuOK := traceDBStrictSQLiteInt(cpuRaw)
		if !idOK || id < 0 || !cpuOK || !validTraceDBCPUIndex(cpu) {
			if comparableOK {
				invalidID[comparableID] = true
			}
			skipped["invalid_cpu_filter"]++
			continue
		}
		candidates[id] = append(candidates[id], traceDBCPUMeasureFilter{ID: id, Name: name, CPU: cpu})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	filters := map[int64]traceDBCPUMeasureFilter{}
	laneIDs := map[traceDBCPUMeasureLaneKey][]int64{}
	for id, items := range candidates {
		if invalidID[id] || len(items) != 1 || rowCounts[id] != 1 {
			skipped["duplicate_or_invalid_filter_id"] += maxInt(1, len(items))
			continue
		}
		filters[id] = items[0]
		key := traceDBCPUMeasureLaneKey{Name: items[0].Name, CPU: items[0].CPU}
		laneIDs[key] = append(laneIDs[key], id)
	}
	for _, ids := range laneIDs {
		if len(ids) == 1 {
			continue
		}
		for _, id := range ids {
			delete(filters, id)
		}
		skipped["duplicate_semantic_filter"] += len(ids)
	}
	return filters, claimed, nil
}

func loadTraceDBClockMeasureFilters(ctx context.Context, queryer traceDBQueryer, skipped map[string]int) (map[int64]traceDBClockMeasureFilter, map[int64]bool, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT id, name, type FROM measure_filter ORDER BY id, name, type`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	candidates := map[int64][]traceDBClockMeasureFilter{}
	claimed := map[int64]bool{}
	invalidID := map[int64]bool{}
	rowCounts := map[int64]int{}
	for rows.Next() {
		var idRaw, nameRaw, typeRaw any
		if err := rows.Scan(&idRaw, &nameRaw, &typeRaw); err != nil {
			return nil, nil, err
		}
		id, idOK := traceDBStrictSQLiteInt(idRaw)
		comparableID, comparableOK := traceDBPoisonEquivalentNonNegativeInt(idRaw)
		if comparableOK {
			rowCounts[comparableID]++
		}
		typeText, typeOK := typeRaw.(string)
		if !typeOK || typeText != "clock_rate_filter" {
			continue
		}
		if idOK && id >= 0 {
			claimed[id] = true
		} else if comparableOK {
			claimed[comparableID] = true
			invalidID[comparableID] = true
		}
		name, nameOK := nameRaw.(string)
		if !idOK || id < 0 || !nameOK || !traceDBMeasureWireToken(name) {
			if comparableOK {
				invalidID[comparableID] = true
			}
			skipped["invalid_clock_filter"]++
			continue
		}
		candidates[id] = append(candidates[id], traceDBClockMeasureFilter{ID: id, Name: name})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	filters := map[int64]traceDBClockMeasureFilter{}
	nameIDs := map[string][]int64{}
	for id, items := range candidates {
		if invalidID[id] || len(items) != 1 || rowCounts[id] != 1 {
			skipped["duplicate_or_invalid_filter_id"] += maxInt(1, len(items))
			continue
		}
		filters[id] = items[0]
		nameIDs[items[0].Name] = append(nameIDs[items[0].Name], id)
	}
	for _, ids := range nameIDs {
		if len(ids) == 1 {
			continue
		}
		for _, id := range ids {
			delete(filters, id)
		}
		skipped["duplicate_semantic_filter"] += len(ids)
	}
	return filters, claimed, nil
}

func loadTraceDBClockEventFilters(ctx context.Context, queryer traceDBQueryer, skipped map[string]int) (map[int64]traceDBClockMeasureFilter, map[int64]bool, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT id, type, name, cpu FROM clock_event_filter ORDER BY id, type, name, cpu`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	candidates := map[int64][]traceDBClockMeasureFilter{}
	claimed := map[int64]bool{}
	invalidID := map[int64]bool{}
	rowCounts := map[int64]int{}
	for rows.Next() {
		var idRaw, typeRaw, nameRaw, cpuRaw any
		if err := rows.Scan(&idRaw, &typeRaw, &nameRaw, &cpuRaw); err != nil {
			return nil, nil, err
		}
		id, idOK := traceDBStrictSQLiteInt(idRaw)
		comparableID, comparableOK := traceDBPoisonEquivalentNonNegativeInt(idRaw)
		if comparableOK {
			rowCounts[comparableID]++
		}
		typeText, typeOK := typeRaw.(string)
		if !typeOK || typeText != "clock_set_rate" {
			continue
		}
		if idOK && id >= 0 {
			claimed[id] = true
		} else if comparableOK {
			claimed[comparableID] = true
			invalidID[comparableID] = true
		}
		name, nameOK := nameRaw.(string)
		cpu, cpuOK := traceDBStrictSQLiteInt(cpuRaw)
		if !idOK || id < 0 || !nameOK || !traceDBMeasureWireToken(name) || !cpuOK || !validTraceDBCPUIndex(cpu) {
			if comparableOK {
				invalidID[comparableID] = true
			}
			skipped["invalid_specialized_clock_filter"]++
			continue
		}
		candidates[id] = append(candidates[id], traceDBClockMeasureFilter{
			ID: id, Name: name, CPU: cpu, CPUKnown: true,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	filters := map[int64]traceDBClockMeasureFilter{}
	laneIDs := map[traceDBClockMeasureLaneKey][]int64{}
	for id, items := range candidates {
		if invalidID[id] || len(items) != 1 || rowCounts[id] != 1 {
			skipped["duplicate_or_invalid_specialized_filter_id"] += maxInt(1, len(items))
			continue
		}
		filters[id] = items[0]
		key := traceDBClockMeasureLaneKey{Name: items[0].Name, CPU: items[0].CPU, CPUKnown: true}
		laneIDs[key] = append(laneIDs[key], id)
	}
	for _, ids := range laneIDs {
		if len(ids) == 1 {
			continue
		}
		for _, id := range ids {
			delete(filters, id)
		}
		skipped["duplicate_specialized_semantic_filter"] += len(ids)
	}
	return filters, claimed, nil
}

// reconcileTraceDBSpecializedClockFilters publishes only the specialized
// registry.  Generic rows are corroboration for equal IDs, never a second
// producer and never a fallback while clock_event_filter exists.
func reconcileTraceDBSpecializedClockFilters(
	specializedFilters map[int64]traceDBClockMeasureFilter,
	specializedClaimed map[int64]bool,
	genericFilters map[int64]traceDBClockMeasureFilter,
	genericClaimed map[int64]bool,
	skipped map[string]int,
) (map[int64]traceDBClockMeasureFilter, map[int64]bool) {
	out := make(map[int64]traceDBClockMeasureFilter, len(specializedFilters))
	for id, filter := range specializedFilters {
		if genericClaimed[id] {
			generic, ok := genericFilters[id]
			if !ok || generic.Name != filter.Name {
				skipped["specialized_generic_filter_conflict"]++
				continue
			}
		}
		out[id] = filter
	}
	for id := range genericClaimed {
		if !specializedClaimed[id] {
			skipped["generic_only_clock_filter_withheld"]++
		}
	}
	return out, specializedClaimed
}

func scanTraceDBMeasureSamples(ctx context.Context, queryer traceDBQueryer, stableExpr string, visit func(traceDBMeasureSample) error) (string, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT `+stableExpr+`, ts, value, filter_id FROM measure ORDER BY `+stableExpr)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		var stableRaw, tsRaw, valueRaw, filterRaw any
		if err := rows.Scan(&stableRaw, &tsRaw, &valueRaw, &filterRaw); err != nil {
			return "", err
		}
		stableID, stableOK := traceDBStrictSQLiteInt(stableRaw)
		ts, tsOK := traceDBStrictSQLiteInt(tsRaw)
		if tsOK && ts < 0 {
			tsOK = false
		}
		value, valueOK := traceDBExactIntegralMeasureValue(valueRaw)
		filterID, filterOK := traceDBStrictSQLiteInt(filterRaw)
		if filterOK && filterID < 0 {
			filterOK = false
		}
		filterPoisonID, filterPoisonKnown := traceDBPoisonEquivalentNonNegativeInt(filterRaw)
		if err := visit(traceDBMeasureSample{
			StableID: stableID, StableIDValid: stableOK, TS: ts, TSValid: tsOK,
			Value: value, ValueValid: valueOK, FilterID: filterID, FilterIDValid: filterOK,
			FilterPoisonID: filterPoisonID, FilterPoisonIDKnown: filterPoisonKnown,
		}); err != nil {
			return "", err
		}
	}
	return stableExpr, rows.Err()
}

func traceDBHiddenRowIDExpr(ctx context.Context, queryer traceDBQueryer, table string) (string, string, error) {
	columns, err := traceDBColumnNames(ctx, queryer, table)
	if err != nil {
		return "", "", err
	}
	declared := map[string]bool{}
	for _, column := range columns {
		declared[strings.ToLower(column)] = true
	}
	var lastErr error
	for _, candidate := range []string{"rowid", "_rowid_", "oid"} {
		if declared[candidate] {
			continue
		}
		// These three tokens are a closed SQLite grammar set.  Keep the alias
		// unquoted: with DQS compatibility enabled, "rowid" on a WITHOUT ROWID
		// table can degrade into a string literal instead of failing.
		expr := candidate
		rows, err := queryer.QueryContext(ctx, `SELECT `+expr+`, typeof(`+expr+`) FROM `+quoteSQLiteIdent(table)+` LIMIT 1`)
		if err != nil {
			lastErr = err
			continue
		}
		if rows.Next() {
			var idRaw, typeRaw any
			if err := rows.Scan(&idRaw, &typeRaw); err != nil {
				_ = rows.Close()
				return "", "", err
			}
			_, idOK := traceDBStrictSQLiteInt(idRaw)
			typeText, typeOK := typeRaw.(string)
			if !idOK || !typeOK || typeText != "integer" {
				_ = rows.Close()
				lastErr = fmt.Errorf("%s is not an integer hidden rowid", candidate)
				continue
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return "", "", err
		}
		if err := rows.Close(); err != nil {
			return "", "", err
		}
		return expr, table + ".hidden_" + candidate, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("all SQLite rowid aliases are shadowed")
	}
	return "", "", fmt.Errorf("%s has no provable hidden rowid source order: %w", table, lastErr)
}

func traceDBColumnNames(ctx context.Context, queryer traceDBQueryer, table string) ([]string, error) {
	rows, err := queryer.QueryContext(ctx, `PRAGMA table_info(`+quoteSQLiteIdent(table)+`)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func traceDBExactIntegralMeasureValue(value any) (int64, bool) {
	if integer, ok := traceDBStrictSQLiteInt(value); ok {
		return integer, true
	}
	floatValue, ok := value.(float64)
	if !ok || math.IsNaN(floatValue) || math.IsInf(floatValue, 0) || math.Trunc(floatValue) != floatValue ||
		floatValue < -9223372036854775808.0 || floatValue >= 9223372036854775808.0 {
		return 0, false
	}
	return int64(floatValue), true
}

// traceDBPoisonEquivalentNonNegativeInt never promotes a malformed value to
// identity. It only locates the exact typed lane that must be tainted when a
// REAL or canonical decimal string is numerically equivalent to that lane.
func traceDBPoisonEquivalentNonNegativeInt(value any) (int64, bool) {
	if integer, ok := traceDBStrictSQLiteInt(value); ok {
		return integer, integer >= 0
	}
	if floatValue, ok := value.(float64); ok && !math.IsNaN(floatValue) && !math.IsInf(floatValue, 0) &&
		math.Trunc(floatValue) == floatValue && floatValue >= 0 && floatValue < 9223372036854775808.0 {
		return int64(floatValue), true
	}
	text, ok := value.(string)
	if !ok || text == "" || text != strings.TrimSpace(text) {
		return 0, false
	}
	for _, r := range text {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	parsed, err := strconv.ParseInt(text, 10, 64)
	return parsed, err == nil
}

func traceDBMeasureWireToken(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 256 || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) || r == '=' {
			return false
		}
	}
	return true
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
