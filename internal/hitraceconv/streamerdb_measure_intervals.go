package hitraceconv

import (
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func measureSQLScalar(raw any, present bool) tracewire.MeasureScalar {
	if !present {
		return tracewire.MeasureScalar{StorageClass: "absent"}
	}
	s := tracewire.MeasureScalar{StorageClass: "null"}
	switch v := raw.(type) {
	case int64:
		s.StorageClass, s.Value = "integer", strconv.FormatInt(v, 10)
	case float64:
		s.StorageClass, s.Value = "real", strconv.FormatFloat(v, 'g', -1, 64)
	case string:
		s.StorageClass, s.Value = "text", v
		if !utf8.ValidString(v) {
			s.Encoding, s.Value = "base64", base64.RawStdEncoding.EncodeToString([]byte(v))
		}
	case []byte:
		s.StorageClass, s.Value = "blob", base64.RawStdEncoding.EncodeToString(v)
	}
	return s
}

// A Go-side strict join avoids SQLite affinity coercion and duplicate fanout.
// Registry type and source_arg_set_id are raw metadata, never owner authority.
func loadMeasureFilters(ctx context.Context, tdb *traceDB, referenced map[int64]bool) (map[int64]tracewire.MeasureFilter, map[int64]bool, error) {
	out, bad := map[int64]tracewire.MeasureFilter{}, map[int64]bool{}
	c, err := tdb.inspectCoverage(ctx, "measurement", "measure_filter", []string{"id"})
	if err != nil || !c.Found || len(c.ColumnsMissing) > 0 {
		return out, bad, err
	}
	columns, err := tdb.columnNames(ctx, "measure_filter")
	if err != nil {
		return out, bad, err
	}
	for i := range columns {
		columns[i] = sqliteASCIIIdentifierFold(columns[i])
	}
	optional := resourceOptionalSelects(columns, []string{"name", "type", "source_arg_set_id"})
	rows, err := tdb.db.QueryContext(ctx, `SELECT id,`+strings.Join(optional, ",")+` FROM measure_filter`)
	if err != nil {
		return out, bad, err
	}
	defer rows.Close()
	seen := map[int64]bool{}
	retainedBytes := 0
	for rows.Next() {
		if err = ctx.Err(); err != nil {
			return out, bad, err
		}
		var id, name, kind, arg any
		if err = rows.Scan(&id, &name, &kind, &arg); err != nil {
			return out, bad, err
		}
		fid, ok := traceDBStrictSQLiteInt(id)
		if !ok {
			if poison, possible := measurePoisonEquivalentInt(id); possible && referenced[poison] {
				bad[poison] = true
				delete(out, poison)
			}
			continue
		}
		if !referenced[fid] {
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
		f := tracewire.MeasureFilter{ID: measureSQLScalar(id, true), Name: measureSQLScalar(name, traceDBStringSliceContains(columns, "name")), Type: measureSQLScalar(kind, traceDBStringSliceContains(columns, "type")), SourceArgSetID: measureSQLScalar(arg, traceDBStringSliceContains(columns, "source_arg_set_id"))}
		retainedBytes += len(f.Name.Value) + len(f.Type.Value) + len(f.SourceArgSetID.Value)
		if retainedBytes > 32<<20 {
			return out, bad, fmt.Errorf("generic measure referenced metadata exceeds retention bound; no partial export")
		}
		out[fid] = f
	}
	return out, bad, rows.Err()
}

func exportTraceDBMeasureIntervals(ctx context.Context, tdb *traceDB, sink *traceDBRowSink) (TraceDBCoverage, error) {
	c, err := tdb.inspectCoverage(ctx, "measurement", "measure", []string{"ts", "value", "filter_id"})
	c.SourceTables = []string{"measure", "measure_filter"}
	if err != nil || !c.Found || len(c.ColumnsMissing) > 0 {
		return c, err
	}
	referenced, err := referencedMeasureFilters(ctx, tdb)
	if err != nil {
		return c, err
	}
	filters, bad, err := loadMeasureFilters(ctx, tdb, referenced)
	if err != nil {
		return c, err
	}
	stable, source, err := traceDBHiddenRowIDExpr(ctx, tdb.db, "measure")
	if err != nil {
		c.Skipped = "physical row identity unavailable"
		return c, nil
	}
	c.FieldSources = map[string]string{"row_id": source, "interval": "measure.ts/dur; exact signed SQLite scalars; no duration extension", "filter": "strict unique measure_filter.id only; source_arg_set_id is a raw reference, not a resource identity"}
	columns, err := tdb.columnNames(ctx, "measure")
	if err != nil {
		return c, err
	}
	for i := range columns {
		columns[i] = sqliteASCIIIdentifierFold(columns[i])
	}
	optional := resourceOptionalSelects(columns, []string{"dur", "type"})
	rows, err := tdb.db.QueryContext(ctx, `SELECT `+stable+`,ts,value,filter_id,`+strings.Join(optional, ",")+` FROM measure ORDER BY `+stable)
	if err != nil {
		return c, err
	}
	defer rows.Close()
	for rows.Next() {
		if err = ctx.Err(); err != nil {
			return c, err
		}
		var id, ts, value, filter, dur, kind any
		if err = rows.Scan(&id, &ts, &value, &filter, &dur, &kind); err != nil {
			return c, err
		}
		rowID, _ := traceDBStrictSQLiteInt(id)
		r := tracewire.MeasureInterval{RowID: rowID, StartNS: measureSQLScalar(ts, true), DurationNS: measureSQLScalar(dur, traceDBStringSliceContains(columns, "dur")), Value: measureSQLScalar(value, true), FilterID: measureSQLScalar(filter, true), MeasureType: measureSQLScalar(kind, traceDBStringSliceContains(columns, "type")), FilterStatus: "unknown"}
		if fid, ok := r.FilterID.Integer(); ok {
			if bad[fid] {
				r.FilterStatus = "ambiguous"
			} else if f, found := filters[fid]; found {
				r.FilterStatus = "observed_unique"
				r.Filter = &f
			}
		}
		line, e := tracewire.FormatMeasureInterval(r)
		if e != nil {
			return c, e
		}
		if e = addTraceDBTypedCommentRow(sink, r.TimestampNS(), line); e != nil {
			return c, e
		}
		c.RowsEmitted++
	}
	return c, rows.Err()
}

func referencedMeasureFilters(ctx context.Context, tdb *traceDB) (map[int64]bool, error) {
	ids := map[int64]bool{}
	rows, err := tdb.db.QueryContext(ctx, `SELECT filter_id FROM measure`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw any
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if id, ok := traceDBStrictSQLiteInt(raw); ok {
			ids[id] = true
			if len(ids) > 65536 {
				return nil, fmt.Errorf("generic measure filter-reference retention limit; no partial export")
			}
		}
	}
	return ids, rows.Err()
}

func measurePoisonEquivalentInt(raw any) (int64, bool) {
	if v, ok := raw.(string); ok {
		n, err := strconv.ParseInt(v, 10, 64)
		return n, err == nil && strconv.FormatInt(n, 10) == v
	}
	if v, ok := raw.(float64); ok && !math.IsNaN(v) && !math.IsInf(v, 0) && math.Trunc(v) == v && v >= -9223372036854775808.0 && v < 9223372036854775808.0 {
		return int64(v), true
	}
	return 0, false
}
