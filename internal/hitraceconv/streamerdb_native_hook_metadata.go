package hitraceconv

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// These are optional observations, not admission, execution or pairing keys.
// The first two columns retain the pre-existing wire order and semantics.
type traceDBNativeHookMetadata struct {
	present  [4]bool
	selects  [4]string
	subtypes map[int64]traceDBNativeHookSubtype
}

type traceDBNativeHookSubtype struct {
	jsonName string
	reason   string
}

func loadTraceDBNativeHookMetadata(ctx context.Context, tdb *traceDB, coverage *TraceDBCoverage) (traceDBNativeHookMetadata, error) {
	metadata := traceDBNativeHookMetadata{}
	for i, column := range []string{"heap_size", "callchain_id", "addr", "sub_type_id"} {
		present, err := tdb.columnExists(ctx, "native_hook", column)
		if err != nil {
			return metadata, err
		}
		metadata.present[i] = present
		metadata.selects[i] = "NULL"
		if present {
			metadata.selects[i] = quoteSQLiteIdent(column)
			coverage.ColumnsPresent = appendTraceDBCoverageColumn(coverage.ColumnsPresent, column)
		}
	}
	if !metadata.present[3] {
		return metadata, nil
	}
	var err error
	metadata.subtypes, err = loadTraceDBNativeHookSubtypes(ctx, tdb)
	return metadata, err
}

// OpenHarmony 5c5afb0c: data_dict.id is uint32 CurrentRow() published as int64;
// native_hook.sub_type_id is DataIndex, with INVALID_UINT64 published as NULL.
// The shared int64/TEXT display dictionary has a different ID/value contract;
// do not reuse it or normalize a negative row ID into this namespace. The sealed DB
// owns both tables; there is no process-global cache or cross-capture fallback.
func loadTraceDBNativeHookSubtypes(ctx context.Context, tdb *traceDB) (map[int64]traceDBNativeHookSubtype, error) {
	coverage, err := tdb.inspectCoverage(ctx, "resource.subtype", "data_dict", []string{"id", "data"})
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) != 0 {
		return nil, err
	}
	// Keep only referenced IDs, rather than retaining another full copy of a
	// potentially large symbol dictionary. Validate both storage classes below;
	// SQL comparison/affinity is not identity authority.
	rows, err := tdb.db.QueryContext(ctx, `SELECT id, data FROM data_dict
		WHERE id IN (SELECT sub_type_id FROM native_hook
			WHERE typeof(sub_type_id) = 'integer' AND sub_type_id BETWEEN 0 AND 4294967295)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64]traceDBNativeHookSubtype)
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var idRaw, nameRaw any
		if err := rows.Scan(&idRaw, &nameRaw); err != nil {
			return nil, err
		}
		id, valid := traceDBStrictSQLiteInt(idRaw)
		if !valid || id < 0 || id > math.MaxUint32 {
			continue
		}
		if _, duplicate := out[id]; duplicate {
			// Even identical duplicate rows cannot identify a unique source row.
			out[id] = traceDBNativeHookSubtype{reason: "unresolvable_sub_type_id"}
			continue
		}
		item := traceDBNativeHookSubtype{jsonName: "null"}
		if nameRaw != nil {
			name, valid := nameRaw.(string)
			if !valid || !utf8.ValidString(name) || len(name) > maxTraceDBIdentityDisplayBytes {
				item = traceDBNativeHookSubtype{reason: "invalid_optional_sub_type_name"}
			} else {
				// A JSON string preserves empty/whitespace/control characters. A
				// literal pipe would still break the outer I|pid|name grammar.
				encoded, err := json.Marshal(name)
				if err != nil {
					return nil, err
				}
				item.jsonName = strings.ReplaceAll(string(encoded), "|", `\u007c`)
			}
		}
		out[id] = item
	}
	return out, rows.Err()
}

func (metadata traceDBNativeHookMetadata) append(name string, raw [4]any, skipped map[string]int) string {
	for i, column := range []string{"heap_size", "callchain_id", "addr", "sub_type_id"} {
		if !metadata.present[i] {
			continue
		}
		value := "null"
		var integer int64
		if raw[i] != nil {
			var valid bool
			integer, valid = traceDBStrictSQLiteInt(raw[i])
			if !valid || (i == 0 && integer < 0) {
				skipped["invalid_optional_"+column]++
				continue
			}
			value = strconv.FormatInt(integer, 10)
		}
		switch i {
		case 0, 1:
			name += " source_" + column + "=" + value
		case 2:
			// Upstream uint64 addr is projected to SQLite int64 without sentinel
			// filtering. All bits survive, including -1; neither address validity
			// nor allocation/free pairing follows from this raw observation.
			bits := "null"
			if raw[i] != nil {
				bits = fmt.Sprintf("0x%016x", uint64(integer))
			}
			name += " source_addr_i64=" + value + " source_addr_bits_hex=" + bits
		case 3:
			name += " source_sub_type_id=" + value
			if raw[i] == nil {
				continue
			}
			if integer < 0 || integer > math.MaxUint32 {
				skipped["invalid_sub_type_id"]++
				continue
			}
			item, found := metadata.subtypes[integer]
			if !found {
				skipped["unresolvable_sub_type_id"]++
			} else if item.reason != "" {
				skipped[item.reason]++
			} else {
				name += " source_sub_type_name=" + item.jsonName
			}
		}
	}
	return name
}
