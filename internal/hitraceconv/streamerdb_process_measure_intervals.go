package hitraceconv

import (
	"context"
	"encoding/base64"
	"math"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

type processMeasureFilter struct {
	name      string
	nameKnown bool
	ipid      tracewire.ProcessMeasureScalar
}

func exportTraceDBProcessMeasureIntervals(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, index traceDBThreadIndex) (TraceDBCoverage, error) {
	c, err := tdb.inspectCoverage(ctx, "counter", "process_measure", []string{"ts", "value", "filter_id"})
	c.SourceTables = []string{"process_measure", "process_measure_filter"}
	if err != nil || !c.Found || len(c.ColumnsMissing) > 0 {
		return c, err
	}
	filters, ambiguous, err := loadProcessMeasureFilters(ctx, tdb)
	if err != nil {
		c.Error = err.Error()
		return c, err
	}
	stable, source, err := traceDBHiddenRowIDExpr(ctx, tdb.db, "process_measure")
	if err != nil {
		c.Skipped = "physical row identity unavailable"
		return c, nil
	}
	c.FieldSources = map[string]string{"row_id": source, "process_interval": "process_measure.ts/dur; exact SQLite storage classes; no next-sample extension", "owner": "process_measure_filter.ipid -> canonical process.ipid; not a thread or lifetime proof"}
	columns, err := tdb.columnNames(ctx, "process_measure")
	if err != nil {
		return c, err
	}
	for i := range columns {
		columns[i] = sqliteASCIIIdentifierFold(columns[i])
	}
	optional := resourceOptionalSelects(columns, []string{"dur", "type"})
	rows, err := tdb.db.QueryContext(ctx, `SELECT `+stable+`,ts,value,filter_id,`+strings.Join(optional, ",")+` FROM process_measure ORDER BY `+stable)
	if err != nil {
		c.Error = err.Error()
		return c, err
	}
	defer rows.Close()
	issues := map[string]int{}
	for rows.Next() {
		if err = ctx.Err(); err != nil {
			return c, err
		}
		var id, ts, value, filter, dur, kind any
		if err = rows.Scan(&id, &ts, &value, &filter, &dur, &kind); err != nil {
			return c, err
		}
		rowID, _ := traceDBStrictSQLiteInt(id)
		r := tracewire.ProcessMeasureInterval{RowID: rowID, StartNS: processMeasureSQLScalar(ts, true), DurationNS: processMeasureSQLScalar(dur, traceDBStringSliceContains(columns, "dur")), Value: processMeasureSQLScalar(value, true), FilterID: processMeasureSQLScalar(filter, true), IPID: processMeasureSQLScalar(nil, false), OwnerStatus: "unknown"}
		r.MeasureType, r.TypeKnown = kind.(string)
		if fid, ok := r.FilterID.Integer(); ok && fid >= 0 {
			if ambiguous[fid] {
				r.OwnerStatus = "ambiguous"
			} else if f, found := filters[fid]; found {
				r.Name, r.NameKnown, r.IPID = f.name, f.nameKnown, f.ipid
				if ipid, exact := f.ipid.Integer(); exact && ipid >= 0 && ipid <= maxTraceDBInternalID {
					if index.AmbiguousIPID[ipid] {
						r.OwnerStatus = "ambiguous"
					} else if p, exists := index.Processes[ipid]; exists && p.IPID == ipid && p.PID > 0 && p.PID <= math.MaxInt32 {
						pid := int(p.PID)
						r.PID, r.ProcessName, r.OwnerStatus = &pid, p.Name, "known"
					}
				}
			}
		}
		if r.OwnerStatus != "known" {
			issues["retained_owner_"+r.OwnerStatus]++
		}
		if _, ok := r.Value.Integer(); !ok {
			issues["retained_value_"+r.Value.Status]++
		}
		if _, ok := r.EndNS(); !ok {
			issues["retained_interval_unavailable"]++
		}
		line, formatErr := tracewire.FormatProcessMeasureInterval(r)
		if formatErr != nil {
			return c, formatErr
		}
		if err = addTraceDBTypedCommentRow(sink, r.TimestampNS(), line); err != nil {
			return c, err
		}
		c.RowsEmitted++
	}
	c.Skipped = traceDBCountSummary(issues)
	return c, rows.Err()
}

func loadProcessMeasureFilters(ctx context.Context, tdb *traceDB) (map[int64]processMeasureFilter, map[int64]bool, error) {
	out, bad := map[int64]processMeasureFilter{}, map[int64]bool{}
	c, err := tdb.inspectCoverage(ctx, "counter", "process_measure_filter", []string{"id", "name", "ipid"})
	if err != nil || !c.Found || len(c.ColumnsMissing) > 0 {
		return out, bad, err
	}
	rows, err := tdb.db.QueryContext(ctx, `SELECT id,name,ipid FROM process_measure_filter`)
	if err != nil {
		return out, bad, err
	}
	defer rows.Close()
	seen := map[int64]bool{}
	for rows.Next() {
		if err = ctx.Err(); err != nil {
			return out, bad, err
		}
		var id, name, ipid any
		if err = rows.Scan(&id, &name, &ipid); err != nil {
			return out, bad, err
		}
		fid, ok := traceDBStrictSQLiteInt(id)
		if !ok {
			if poison, possible := traceDBPoisonEquivalentNonNegativeInt(id); possible {
				bad[poison] = true
				delete(out, poison)
			}
			continue
		}
		if seen[fid] {
			bad[fid] = true
			delete(out, fid)
			continue
		}
		seen[fid] = true
		if bad[fid] {
			continue
		}
		label, known := name.(string)
		out[fid] = processMeasureFilter{label, known, processMeasureSQLScalar(ipid, true)}
	}
	return out, bad, rows.Err()
}

func processMeasureSQLScalar(raw any, present bool) tracewire.ProcessMeasureScalar {
	if !present {
		return tracewire.ProcessMeasureScalar{Status: "unavailable", StorageClass: "absent"}
	}
	if raw == nil {
		return tracewire.ProcessMeasureScalar{Status: "null", StorageClass: "null"}
	}
	s := tracewire.ProcessMeasureScalar{Status: "invalid_storage"}
	switch v := raw.(type) {
	case int64:
		s.Status, s.StorageClass, s.Value = "known", "integer", strconv.FormatInt(v, 10)
	case float64:
		s.StorageClass, s.Value = "real", strconv.FormatFloat(v, 'g', -1, 64)
	case string:
		s.StorageClass, s.Value = "text", v
	case []byte:
		s.StorageClass, s.Value = "blob", base64.RawStdEncoding.EncodeToString(v)
	}
	return s
}
