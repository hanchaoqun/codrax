package agent

import "github.com/hanchaoqun/codrax/internal/types"

// This writing digest deliberately separates the producer's exact query count
// from the finite, possibly target/window-projected members available here.
// It is not another inventory, aggregate claim, or publication authority.
type traceInventoryQuerySummary struct {
	NativeRetainedRows        int                       `json:"native_retained_rows"`
	ProfileRows               int                       `json:"profile_rows"`
	ProfileCoversQueryMatches bool                      `json:"profile_covers_query_matches"`
	RetainedEventKinds        []traceInventoryEventKind `json:"retained_event_kinds"`
}

type traceInventoryEventKind struct {
	EventType string `json:"event_type"`
	EventName string `json:"event_name"`
	Members   int    `json:"members"`
}

func traceEventInventoryQuerySummary(record types.ObservationRecord, nativeRows, displayRows int) *traceInventoryQuerySummary {
	i := record.EventSearchInventory
	complete := i.Coverage.ScopeComplete && i.Coverage.EnumerationComplete
	// Fully displayed complete results already carry every member beside their
	// count. Only incomplete layers need this extra explanation; do not spend
	// shared context on a second copy of a complete query's metadata.
	if complete && displayRows == i.Coverage.MatchedTotal {
		return nil
	}
	out := &traceInventoryQuerySummary{
		NativeRetainedRows: nativeRows, ProfileRows: len(i.Rows),
		ProfileCoversQueryMatches: complete && len(i.Rows) == i.Coverage.MatchedTotal,
		RetainedEventKinds:        []traceInventoryEventKind{},
	}
	// Equality of existing typed labels only. Do not parse raw preview text,
	// infer business categories, or extrapolate this distribution to omitted
	// members. Keep first-occurrence order and each occurrence's multiplicity.
	seen := map[[2]string]int{}
	for _, row := range i.Rows {
		key := [2]string{row.EventType, row.EventName}
		if n, exists := seen[key]; exists {
			out.RetainedEventKinds[n].Members++
			continue
		}
		seen[key] = len(out.RetainedEventKinds)
		out.RetainedEventKinds = append(out.RetainedEventKinds, traceInventoryEventKind{row.EventType, row.EventName, 1})
	}
	return out
}
