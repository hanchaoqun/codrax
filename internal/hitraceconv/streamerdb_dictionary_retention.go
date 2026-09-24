package hitraceconv

import (
	"context"
	"fmt"
)

// References are only a retention plan. They never prove that a consumer row
// or dictionary entry is admissible. All consumers keep their existing gates.
func (tdb *traceDB) extendedDictionaryReferences(ctx context.Context) (map[int64]bool, error) {
	refs := map[int64]bool{}
	for _, source := range []struct{ table, column string }{
		{"app_startup", "start_name"}, {"hisys_all_event", "domain_id"}, {"hisys_all_event", "event_name_id"},
	} {
		found, err := tdb.tableExists(ctx, source.table)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		found, err = tdb.columnExists(ctx, source.table, source.column)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		if err := tdb.appendDictionaryReferences(ctx, refs, "SELECT "+quoteSQLiteIdent(source.column)+" FROM "+quoteSQLiteIdent(source.table)); err != nil {
			return nil, err
		}
	}
	return refs, nil
}

func (tdb *traceDB) appendDictionaryReferences(ctx context.Context, refs map[int64]bool, query string) (err error) {
	rows, err := tdb.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer func() { err = traceDBJoinPreservingSingle(err, rows.Close()) }()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		var raw any
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		if id, ok := traceDBStrictSQLiteInt(raw); ok {
			refs[id] = true
		}
	}
	return traceDBJoinPreservingSingle(rows.Err(), ctx.Err())
}

func (tdb *traceDB) argDictionaryReferences(ctx context.Context, dataTypes traceDBArgDataTypeAuthority) (refs map[int64]bool, err error) {
	refs = map[int64]bool{}
	rows, err := tdb.db.QueryContext(ctx, "SELECT argset, key, datatype, value FROM args")
	if err != nil {
		return nil, err
	}
	defer func() { err = traceDBJoinPreservingSingle(err, rows.Close()) }()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var argsetRaw, keyRaw, datatypeRaw, valueRaw any
		if err := rows.Scan(&argsetRaw, &keyRaw, &datatypeRaw, &valueRaw); err != nil {
			return nil, err
		}
		argset, valid := traceDBStrictSQLiteInt(argsetRaw)
		if !valid || argset < 0 {
			continue
		}
		// Even a rejected datatype may need its key to localize poison to one
		// canonical arg, instead of incorrectly poisoning the whole argset.
		if key, valid := traceDBStrictSQLiteInt(keyRaw); valid && key >= 0 {
			refs[key] = true
		}
		datatype, valid := traceDBStrictSQLiteInt(datatypeRaw)
		if valid && datatype != 0 && traceDBArgDataTypeAllowed(dataTypes, datatype) {
			if value, valid := traceDBStrictSQLiteInt(valueRaw); valid && value >= 0 {
				refs[value] = true
			}
		}
	}
	return refs, traceDBJoinPreservingSingle(rows.Err(), ctx.Err())
}

func traceDBDictionaryRetentionMetrics(refs map[int64]bool, retained int) map[string]int64 {
	return map[string]int64{"dictionary_references": int64(len(refs)), "dictionary_entries_retained": int64(retained)}
}

// The argument policy has always required ordered auditing of every physical
// row. Preserve that order and its rejection accounting, but keep only the
// current typed identity plus values/poison for actual consumer references.
func (tdb *traceDB) loadArgDictionary(ctx context.Context, refs map[int64]bool, coverage *TraceDBCoverage) (dict map[int64]string, invalid map[int64]bool, err error) {
	dict, invalid = map[int64]string{}, map[int64]bool{}
	rows, err := tdb.db.QueryContext(ctx, "SELECT id, data FROM data_dict ORDER BY typeof(id), id, data")
	if err != nil {
		return nil, nil, err
	}
	defer func() { err = traceDBJoinPreservingSingle(err, rows.Close()) }()
	var key int64
	var haveKey, poisoned bool
	var value string
	accepted, rejected := 0, 0
	flush := func() {
		if !haveKey {
			return
		}
		if poisoned {
			if refs[key] {
				invalid[key] = true
			}
		} else {
			accepted++
			if refs[key] {
				dict[key] = value
			}
		}
	}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		var idRaw, dataRaw any
		if err := rows.Scan(&idRaw, &dataRaw); err != nil {
			return nil, nil, err
		}
		id, idOK := traceDBStrictSQLiteInt(idRaw)
		text, textOK := traceDBStrictArgText(dataRaw, true)
		if !idOK || id < 0 {
			rejected++
			continue
		}
		if haveKey && key == id {
			// Any second physical row permanently poisons this typed identity,
			// including a bad first value or an identical repeated value.
			poisoned = true
			rejected++
			continue
		}
		flush()
		key, haveKey, poisoned, value = id, true, !textOK, ""
		if !textOK {
			rejected++
		} else if refs[id] {
			value = text
		}
	}
	if err := traceDBJoinPreservingSingle(rows.Err(), ctx.Err()); err != nil {
		return nil, nil, err
	}
	flush()
	coverage.RowsEmitted = accepted // Global audited valid identities, not the retained subset.
	coverage.Metrics = traceDBDictionaryRetentionMetrics(refs, len(dict))
	coverage.FieldSources = map[string]string{"dictionary_retention": "all physical rows audited; only referenced argument keys/text values retained; no identity coercion or dictionary truncation"}
	if rejected > 0 {
		coverage.Skipped = fmt.Sprintf("%d data_dict row(s) rejected: invalid or duplicate typed identity", rejected)
	}
	return dict, invalid, nil
}

// Global counts use only INTEGER identity and storage-class metadata. Sorting
// these narrow groups avoids sorting/copying all unreferenced TEXT payloads.
// The original immutable read-only connection and its resource budget apply.
func (tdb *traceDB) sharedDictionaryPopulation(ctx context.Context) (valid, duplicates int, err error) {
	err = tdb.db.QueryRowContext(ctx, `SELECT
 COALESCE(SUM(CASE WHEN n=1 AND text_n=1 THEN 1 ELSE 0 END),0),
 COALESCE(SUM(n-1),0)
 FROM (SELECT COUNT(*) AS n, SUM(CASE WHEN typeof(data)='text' THEN 1 ELSE 0 END) AS text_n
       FROM data_dict WHERE typeof(id)='integer' GROUP BY id)`).Scan(&valid, &duplicates)
	return valid, duplicates, err
}
