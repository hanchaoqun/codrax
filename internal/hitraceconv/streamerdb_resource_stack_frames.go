package hitraceconv

import (
	"context"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func loadTraceDBResourceFrames(ctx context.Context, tdb *traceDB) (*traceDBResourceFrames, error) {
	c, err := tdb.inspectCoverage(ctx, "resource_stack.frames", "native_hook_frame", []string{"callchain_id", "depth"})
	out := &traceDBResourceFrames{coverage: c, byChain: map[int64][]tracewire.ResourceFrame{}}
	if err != nil || !c.Found || len(c.ColumnsMissing) > 0 {
		return out, err
	}
	stable, _, err := traceDBHiddenRowIDExpr(ctx, tdb.db, "native_hook_frame")
	if err != nil {
		out.coverage.Skipped = "stable_resource_frame_row_unavailable=1"
		return out, nil
	}
	dict, err := loadResourceStackDictionary(ctx, tdb)
	if err != nil {
		return out, err
	}
	columns, err := traceDBColumnNames(ctx, tdb.db, "native_hook_frame")
	if err != nil {
		return out, err
	}
	if err := out.inspectColumns(ctx, tdb, columns); err != nil {
		return out, err
	}
	selects := resourceOptionalSelects(columns, []string{"id", "ip", "symbol_id", "file_id", "offset", "symbol_offset", "vaddr"})
	rows, err := tdb.db.QueryContext(ctx, `SELECT `+stable+`,callchain_id,depth,`+strings.Join(selects, ",")+` FROM native_hook_frame WHERE typeof(callchain_id)='integer' AND callchain_id IN (SELECT callchain_id FROM native_hook WHERE typeof(callchain_id)='integer' AND callchain_id BETWEEN 0 AND 4294967294) ORDER BY `+stable)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		var rowRaw, chainRaw, depthRaw any
		var raw [7]any
		if err := rows.Scan(&rowRaw, &chainRaw, &depthRaw, &raw[0], &raw[1], &raw[2], &raw[3], &raw[4], &raw[5], &raw[6]); err != nil {
			return out, err
		}
		row, rowOK := traceDBStrictSQLiteInt(rowRaw)
		chain, chainOK := traceDBStrictSQLiteInt(chainRaw)
		if !rowOK || !chainOK || chain < 0 || chain >= math.MaxUint32 {
			continue
		}
		f := tracewire.ResourceFrame{RowID: row, SourceID: resourceSQLScalar(raw[0], selects[0] != "NULL"), Depth: resourceSQLScalar(depthRaw, true), IP: resourceSQLScalar(raw[1], selects[1] != "NULL"), SymbolID: resourceSQLScalar(raw[2], selects[2] != "NULL"), FileID: resourceSQLScalar(raw[3], selects[3] != "NULL"), Offset: resourceSQLScalar(raw[4], selects[4] != "NULL"), SymbolOffset: resourceSQLScalar(raw[5], selects[5] != "NULL"), VAddr: resourceSQLText(raw[6], selects[6] != "NULL")}
		f.Symbol, f.Library = resolveResourceText(dict, f.SymbolID), resolveResourceText(dict, f.FileID)
		out.decoded(f)
		out.byChain[chain] = append(out.byChain[chain], f)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	for chain := range out.byChain {
		sort.SliceStable(out.byChain[chain], func(i, j int) bool {
			a, ao := out.byChain[chain][i].Depth.Integer()
			b, bo := out.byChain[chain][j].Depth.Integer()
			if ao != bo {
				return ao
			}
			if a != b {
				return a < b
			}
			return out.byChain[chain][i].RowID < out.byChain[chain][j].RowID
		})
	}
	out.available = true
	return out, nil
}

func resourceSQLText(raw any, present bool) tracewire.ResourceText {
	if !present {
		return tracewire.ResourceText{Status: "unavailable"}
	}
	if raw == nil {
		return tracewire.ResourceText{Status: "null"}
	}
	s, ok := raw.(string)
	if !ok || !utf8.ValidString(s) || len(s) > 4096 {
		return tracewire.ResourceText{Status: "invalid"}
	}
	return tracewire.ResourceText{Status: "known", Value: s}
}

func loadResourceStackDictionary(ctx context.Context, tdb *traceDB) (map[int64]tracewire.ResourceText, error) {
	c, err := tdb.inspectCoverage(ctx, "resource_stack.dictionary", "data_dict", []string{"id", "data"})
	if err != nil || !c.Found || len(c.ColumnsMissing) > 0 {
		return nil, err
	}
	columns, err := traceDBColumnNames(ctx, tdb.db, "native_hook_frame")
	if err != nil {
		return nil, err
	}
	var references []string
	for _, name := range []string{"symbol_id", "file_id"} {
		if traceDBStringSliceContains(columns, name) {
			references = append(references, `SELECT `+quoteSQLiteIdent(name)+` FROM native_hook_frame WHERE typeof(`+quoteSQLiteIdent(name)+`)='integer' AND callchain_id IN (SELECT callchain_id FROM native_hook WHERE typeof(callchain_id)='integer' AND callchain_id BETWEEN 0 AND 4294967294)`)
		}
	}
	if len(references) == 0 {
		return nil, nil
	}
	rows, err := tdb.db.QueryContext(ctx, `SELECT id,data FROM data_dict WHERE id IN (`+strings.Join(references, ` UNION `)+`)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]tracewire.ResourceText{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var idRaw, textRaw any
		if err := rows.Scan(&idRaw, &textRaw); err != nil {
			return nil, err
		}
		id, valid := traceDBStrictSQLiteInt(idRaw)
		if !valid || id < 0 || id > math.MaxUint32 {
			continue
		}
		if _, exists := out[id]; exists {
			out[id] = tracewire.ResourceText{Status: "ambiguous"}
		} else {
			out[id] = resourceSQLText(textRaw, true)
		}
	}
	return out, rows.Err()
}

func resolveResourceText(dict map[int64]tracewire.ResourceText, id tracewire.ResourceScalar) tracewire.ResourceText {
	if n, ok := id.Integer(); ok && n >= 0 && n <= math.MaxUint32 {
		if s, exists := dict[n]; exists {
			return s
		}
	}
	return tracewire.ResourceText{Status: "unavailable"}
}
