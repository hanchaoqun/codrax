package agent

import "github.com/hanchaoqun/codrax/internal/types"

// A compact navigation descriptor, not a new measurement or source credential.
// The unit still carries its exact observation/query address. Full source path,
// version and producer notes remain in the native table, not inferred here.
type investigationNativeReferenceContext struct {
	ReferenceIndex      *int     `json:"reference_index,omitempty"`
	Label               string   `json:"label"`
	Columns             []string `json:"columns"`
	OriginalSource      string   `json:"original_source,omitempty"`
	QueryWindow         string   `json:"query_window,omitempty"`
	QuerySelection      string   `json:"query_selection,omitempty"`
	Role                string   `json:"role,omitempty"`
	RoleBasis           string   `json:"role_basis,omitempty"`
	UseLimit            string   `json:"use_limit,omitempty"`
	ThreadTID           int      `json:"tid,omitempty"`
	ScopeDetailsOmitted bool     `json:"scope_details_omitted,omitempty"`
}

func investigationNativeTableContext(table types.RuntimeMeasurementTable) (investigationNativeReferenceContext, bool) {
	source, current := table.NativeSourceTitleBasis()
	if !current {
		return investigationNativeReferenceContext{}, false
	}
	return investigationNativeReferenceContext{
		Label: table.Label, Columns: append([]string(nil), table.Columns...),
		OriginalSource: source.OriginalSource, QueryWindow: source.QueryWindow,
		QuerySelection: source.QuerySelection, Role: source.Role, RoleBasis: source.RoleBasis,
		UseLimit: source.ReferenceLimit, ThreadTID: source.ReferenceTID,
	}, true
}

type investigationNativeReferenceLimit struct {
	Refs []int `json:"refs"`
	investigationNativeReferenceLimitBasis
}

type investigationNativeReferenceLimitBasis struct {
	QueryWindow    string `json:"query_window,omitempty"`
	QuerySelection string `json:"query_selection,omitempty"`
	ThreadTID      int    `json:"tid,omitempty"`
	UseLimit       string `json:"use_limit"`
}

// Share identical teaching within a unit, not source identities or data. A
// different exact query window or thread always retains its own limit. The
// reference positions resolve to the unit's original native_refs entries.
func investigationNativeReferenceLimits(in []investigationNativeReferenceContext) []investigationNativeReferenceLimit {
	var out []investigationNativeReferenceLimit
	positions := map[investigationNativeReferenceLimitBasis]int{}
	for _, context := range in {
		if context.ReferenceIndex == nil || context.UseLimit == "" {
			continue
		}
		basis := investigationNativeReferenceLimitBasis{context.QueryWindow, context.QuerySelection, context.ThreadTID, context.UseLimit}
		index, found := positions[basis]
		if !found {
			index = len(out)
			positions[basis] = index
			out = append(out, investigationNativeReferenceLimit{investigationNativeReferenceLimitBasis: basis})
		}
		out[index].Refs = append(out[index].Refs, *context.ReferenceIndex)
	}
	return out
}

func investigationNativeReferenceNavigation(in []investigationNativeReferenceContext) []investigationNativeReferenceContext {
	out := append([]investigationNativeReferenceContext(nil), in...)
	for i := range out {
		if out[i].ReferenceIndex != nil && out[i].UseLimit != "" {
			// Already delivered with the unit, not a second scope description.
			out[i].QueryWindow, out[i].QuerySelection, out[i].UseLimit, out[i].ThreadTID = "", "", "", 0
		}
	}
	return out
}

// Current accepted ownership is checked by the caller; private row/source/table
// binding is independently rechecked here. A restored JSON address, stale source
// or another unit's selector cannot mint a current scope descriptor.
func investigationNativeReferenceContexts(h *types.InvestigationNativeFactHandoff, contract *types.RuntimeMeasurementContract) map[int][]investigationNativeReferenceContext {
	if !types.InvestigationNativeRowReferencesCurrent(h, contract) {
		return nil
	}
	selected, ok := types.BindInvestigationNativeFactHandoff(h, contract)
	if !ok {
		return nil
	}
	tables := map[string]types.RuntimeMeasurementTable{}
	for _, receipt := range selected {
		tables[receipt.ObservationID+"\x00"+string(receipt.View)] = *receipt.BoundTable
	}
	out := map[int][]investigationNativeReferenceContext{}
	for i, unit := range h.InterpretationUnits {
		var contexts []investigationNativeReferenceContext
		for j, ref := range unit.NativeRefs {
			table := tables[ref.ObservationID+"\x00"+string(ref.View)]
			context, current := investigationNativeTableContext(table)
			if !current {
				return nil
			}
			index := j
			context.ReferenceIndex = &index
			contexts = append(contexts, context)
		}
		if len(contexts) > 0 {
			out[i] = contexts
		}
	}
	return out
}

// Choose a complete optional descriptor profile, never truncate a source path,
// column label or an accepted interpretation to make navigation metadata fit.
func compactInvestigationNativeReferenceContexts(in []investigationNativeReferenceContext) []investigationNativeReferenceContext {
	out := make([]investigationNativeReferenceContext, len(in))
	for i, context := range in {
		out[i] = investigationNativeReferenceContext{ReferenceIndex: context.ReferenceIndex, Label: context.Label, Columns: context.Columns, ScopeDetailsOmitted: true}
	}
	return out
}
