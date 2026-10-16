package tool

import (
	"fmt"
	"strconv"
	"strings"

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
		if item.PID < 0 || item.PID > types.RuntimeTargetMaxPID {
			return nil, fmt.Sprintf("runtime_thread_lookups[%d].pid is outside the supported TID range 1..%d; omit pid for a name-only lookup. Do not substitute a containing process PID or an artifact-discovered identity", i, types.RuntimeTargetMaxPID)
		}
		if item.PID == 0 && item.Thread == "" {
			return nil, fmt.Sprintf("runtime_thread_lookups[%d].pid/thread identity is missing; submit the user-named TID or exact thread selector with its current-request source_quote. A quote alone does not create an identity", i)
		}
		if pid, _, ok := tracequery.ParseThreadSelectorIdentity(item.Thread); ok && item.PID > 0 && pid != item.PID {
			return nil, fmt.Sprintf("runtime_thread_lookups[%d].thread contains TID %d, which conflicts with submitted pid=%d; correct the conflicting identity fields from the current request, not by changing only source_quote", i, pid, item.PID)
		}
		if item.SourceQuote == "" {
			return nil, runtimeThreadLookupQuoteRepair(i, "is missing")
		}
		if !sourceQuotePresentInCurrentRequest(raw, item.SourceQuote) {
			return nil, runtimeThreadLookupQuoteRepair(i, "is not a contiguous verbatim current-request phrase")
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

// This diagnostic reports only a failed provenance span. Structurally sound
// input is still unaccepted until the complete object is submitted again.
func runtimeThreadLookupQuoteRepair(index int, issue string) string {
	return fmt.Sprintf("runtime_thread_lookups[%d].source_quote %s. Keep the submitted pid/thread for this quote-only repair; those fields are structurally valid but not yet accepted. Copy a contiguous current-request phrase that contains the submitted name/TID; a shared verbatim phrase may support multiple independently submitted identities. If no current-request phrase supports that identity, withdraw the unproven lookup row rather than deleting its identity fields. Re-emit the complete object, preserving other identities, scopes and dimensions. Do not invent a quote from artifact/model prose or infer a replacement identity from numbers", index, issue)
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
		return item.Thread != "" && entityNamedInQuote(item.SourceQuote, item.Thread)
	}
	if entityNamedInQuote(item.SourceQuote, strconv.Itoa(pid)) {
		return true
	}
	// A canonical thread selector such as ui-10 also names TID 10. Parse
	// complete identity surfaces, not numeric substrings in arbitrary names.
	for _, surface := range diagramParticipantIdentitySurfaces(item.SourceQuote) {
		if quotedPID, _, ok := tracequery.ParseThreadSelectorIdentity(surface); ok && quotedPID == pid {
			return true
		}
	}
	return false
}
