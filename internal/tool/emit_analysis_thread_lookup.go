package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeThreadLookupSchema() map[string]any {
	return map[string]any{
		"type": "array", "maxItems": 8, "description": types.RuntimeThreadLookupTeaching,
		"items": map[string]any{
			"type": "object", "required": []string{"source_quote"},
			"properties": map[string]any{
				"pid":          map[string]any{"type": "integer", "minimum": 1, "maximum": types.RuntimeTargetMaxPID, "description": "Explicit source thread TID, not its containing process."},
				"thread":       map[string]any{"type": "string", "description": "Exact user-named thread selector; omit if only TID is known."},
				"source_quote": map[string]any{"type": "string", "description": "One exact current-request phrase containing this lookup identity and its role."},
			},
		},
	}
}

func parseRuntimeThreadLookups(raw string, in []types.RuntimeThreadLookup) ([]types.RuntimeThreadLookup, string) {
	if len(in) > 8 {
		return nil, "runtime_thread_lookups exceeds 8 entries; preserve only explicit thread lookup identities"
	}
	out := make([]types.RuntimeThreadLookup, 0, len(in))
	seen := map[types.RuntimeThreadLookup]bool{}
	for i, item := range in {
		item.Thread, item.SourceQuote = strings.TrimSpace(item.Thread), strings.TrimSpace(item.SourceQuote)
		if item.PID < 0 || item.PID > types.RuntimeTargetMaxPID || (item.PID == 0 && item.Thread == "") ||
			item.SourceQuote == "" || !sourceQuotePresentInCurrentRequest(raw, item.SourceQuote) {
			return nil, fmt.Sprintf("runtime_thread_lookups[%d] needs a valid thread identity and exact current-request source_quote; do not repair it with artifact/model prose", i)
		}
		if !seen[item] {
			out = append(out, item)
			seen[item] = true
		}
	}
	return out, ""
}
