package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Only display references are shared. The inventory must have actually
// published its complete metadata, and the full accepted source envelope must
// match byte-for-byte after canonical encoding. Query/window/clock/generation
// differences remain distinct. No observation, member, note or authority is
// removed, and no abbreviated prompt string is used to establish a match.
func traceEventInventorySourceReferences(records []types.ObservationRecord, published map[string]bool) map[string]string {
	byID := make(map[string]string, len(records))
	conflicts := make(map[string]bool)
	for _, record := range records {
		encoded, err := json.Marshal(record.SourceRef)
		if err != nil || record.ID == "" {
			continue
		}
		key := string(encoded)
		if prior, exists := byID[record.ID]; exists && prior != key {
			conflicts[record.ID] = true
		}
		byID[record.ID] = key
	}
	sources := make(map[string]string)
	for _, record := range records {
		if !published[record.ID] || conflicts[record.ID] || !types.IsValidTraceEventSearchInventoryRecord(record) {
			continue
		}
		key := byID[record.ID]
		if key != "" && sources[key] == "" {
			sources[key] = record.ID
		}
	}
	refs := make(map[string]string)
	for _, record := range records {
		if !conflicts[record.ID] && sources[byID[record.ID]] != "" {
			refs[record.ID] = sources[byID[record.ID]]
		}
	}
	return refs
}

// Dispatch-local aliases avoid copying a long result ID for every mirror.
// Every alias resolves to a complete inventory source that is present in this
// same prompt, not to a path inferred from a display label or external state.
func traceEventInventoryPromptSourceAliases(records []types.ObservationPromptRecord, sources map[string]string) (map[string]string, string) {
	aliases := make(map[string]string)
	refs := make(map[string]string)
	var b strings.Builder
	for _, record := range records {
		inventory := sources[record.ID]
		if inventory == "" || record.Claim == types.TraceEventSearchInventoryPredicate {
			continue
		}
		alias := aliases[inventory]
		if alias == "" {
			alias = fmt.Sprintf("inventory_source_%d", len(aliases)+1)
			aliases[inventory] = alias
			fmt.Fprintf(&b, "- %s: complete source of inventory observation_id=%q.\n", alias, inventory)
		}
		refs[record.ID] = alias
	}
	if len(refs) == 0 {
		return nil, ""
	}
	return refs, "- inventory_source aliases below reference complete source objects in Trace Event Search Inventories. Only identical source envelopes share a display reference; each row's own span, claim, notes and authority remain separate.\n" + b.String() + "\n"
}
