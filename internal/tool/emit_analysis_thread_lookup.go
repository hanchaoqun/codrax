package tool

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/tracequery"
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
				"source_quote": map[string]any{"type": "string", "description": "One exact current-request phrase containing this thread name or TID and its lookup role. Generic phrases such as a request for all threads cannot authorize identities discovered in the artifact."},
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
		if !runtimeThreadLookupIdentityInQuote(item) {
			return nil, fmt.Sprintf("runtime_thread_lookups[%d].source_quote must contain that exact thread name or TID; remove artifact-discovered identities instead of relabeling them as user lookup inputs", i)
		}
		if !seen[item] {
			out = append(out, item)
			seen[item] = true
		}
	}
	return out, ""
}

// Validate only the typed locator's own verbatim provenance span. This is not
// intent classification or keyword scanning over the question/model answer.
// A generic quote proves no identity, and TID 10 is not proved by token 310.
func runtimeThreadLookupIdentityInQuote(item types.RuntimeThreadLookup) bool {
	pid := item.PID
	if parsed, _, ok := tracequery.ParseThreadSelectorIdentity(item.Thread); ok {
		if pid > 0 && pid != parsed {
			return false
		}
		pid = parsed
	}
	if pid <= 0 {
		return item.Thread != "" && strings.Contains(item.SourceQuote, item.Thread)
	}
	literal := strconv.Itoa(pid)
	quote := item.SourceQuote
	for offset := 0; offset < len(quote); {
		rel := strings.Index(quote[offset:], literal)
		if rel < 0 {
			return false
		}
		start := offset + rel
		end := start + len(literal)
		before, _ := utf8.DecodeLastRuneInString(quote[:start])
		after, _ := utf8.DecodeRuneInString(quote[end:])
		if !unicode.IsDigit(before) && !unicode.IsDigit(after) {
			return true
		}
		offset = end
	}
	return false
}
