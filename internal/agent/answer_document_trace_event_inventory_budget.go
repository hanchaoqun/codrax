package agent

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Reserve the existing fair row share before summaries, but return bytes that
// the selected rows do not need. Fixed half-budget partitions otherwise omit
// exact source locators while leaving unused space in the same section. This
// does not change the total budget, query/member roster or omission contract.
func traceEventInventorySummaryBudget(remaining, queries int, rows []types.TraceEventSearchInventoryRow) int {
	reserved := 0
	if len(rows) > 0 {
		rowShare := remaining / 2
		perRow := rowShare/len(rows) - 64
		for _, row := range rows {
			bounded := traceEventInventoryBoundedPromptObject(traceEventInventoryPromptRow(row), perRow)
			encoded, _ := json.Marshal(bounded)
			reserved += len(encoded) + 64
		}
		if reserved > rowShare {
			reserved = rowShare
		}
	}
	return (remaining - reserved) / queries
}

func traceEventInventoryPromptRow(row types.TraceEventSearchInventoryRow) types.TraceEventSearchInventoryRow {
	if len(row.Raw) > traceEventInventoryPromptRawLimit {
		end := traceEventInventoryPromptRawLimit
		for end > 0 && !utf8.RuneStart(row.Raw[end]) {
			end--
		}
		row.Raw, row.RawTruncated = row.Raw[:end], true
	}
	return row
}

type traceInventoryBudgetField struct {
	parent      map[string]any
	key, path   string
	bytes       int
	replacement map[string]any
}

// This is a display-only object, deliberately not a modified typed receipt.
// Numeric coordinates/counts and booleans stay exact. Pathological strings or
// filter/caveat arrays can be omitted, but never shortened into another valid
// source or filter value. The same object declares the omitted field paths and
// a digest of the omitted-values JSON. The accepted ledger remains untouched.
func traceEventInventoryBoundedPromptObject(value any, budget int) map[string]any {
	raw, _ := json.Marshal(value)
	var out map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	_ = decoder.Decode(&out)
	if len(raw) <= budget {
		return out
	}
	// Retain the original envelope before any display-only value reductions.
	var originalFields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &originalFields)
	originalSemantics := originalFields["semantics"]
	var fields []traceInventoryBudgetField
	var visit func(map[string]any, string)
	visit = func(object map[string]any, prefix string) {
		for key, value := range object {
			path := prefix + "/" + key
			switch nested := value.(type) {
			case map[string]any:
				if path == "/semantics" {
					fields = append(fields, traceInventorySemanticBudgetFields(nested)...)
					continue
				}
				visit(nested, path)
			case string, []any:
				if path == "/inventory/rows" || strings.HasPrefix(path, "/jank_event/values/") {
					continue
				}
				encoded, _ := json.Marshal(nested)
				fields = append(fields, traceInventoryBudgetField{parent: object, key: key, path: path, bytes: len(encoded)})
			}
		}
	}
	visit(out, "")
	sort.Slice(fields, func(i, j int) bool {
		if fields[i].bytes != fields[j].bytes {
			return fields[i].bytes > fields[j].bytes
		}
		return fields[i].path < fields[j].path
	})
	omitted := map[string]any{}
	var paths []string
	for _, field := range fields {
		omitted[field.path] = field.parent[field.key]
		delete(field.parent, field.key)
		for key, value := range field.replacement {
			field.parent[key] = value
		}
		paths = append(paths, field.path)
		original, _ := json.Marshal(omitted)
		out["prompt_metadata_omission"] = map[string]any{
			"fields": paths, "sha256": fmt.Sprintf("%x", sha256.Sum256(original)), "original_json_bytes": len(original),
		}
		encoded, _ := json.Marshal(out)
		if len(encoded) <= budget {
			return out
		}
	}
	// Even an omission roster needs a bound when every free-form field was
	// enormous. At this point every eligible string/array has been removed;
	// describe that exact set by type instead of emitting an oversized list
	// of paths. Exact numeric/boolean rulers and nanosecond values remain.
	original, _ := json.Marshal(omitted)
	out["prompt_metadata_omission"] = map[string]any{
		"all_free_form_string_and_array_fields": true,
		"field_count":                           len(paths), "sha256": fmt.Sprintf("%x", sha256.Sum256(original)), "original_json_bytes": len(original),
	}
	encoded, _ := json.Marshal(out)
	if len(encoded) <= budget {
		return out
	}
	// Keep the typed roster atomic if even compact omission metadata cannot
	// fit it. Bind this final fallback to the ORIGINAL projection, not its
	// already reduced display copy. Exact numeric strings are never truncated.
	if _, ok := out["semantics"]; ok {
		for path := range omitted {
			if strings.HasPrefix(path, "/semantics/") {
				delete(omitted, path)
			}
		}
		omitted["/semantics"] = json.RawMessage(originalSemantics)
		delete(out, "semantics")
		original, _ = json.Marshal(omitted)
		out["prompt_metadata_omission"] = map[string]any{
			"all_free_form_string_and_array_fields": true, "semantic_projection_omitted": true,
			"field_count": len(omitted), "sha256": fmt.Sprintf("%x", sha256.Sum256(original)), "original_json_bytes": len(original),
		}
	}
	return out
}
