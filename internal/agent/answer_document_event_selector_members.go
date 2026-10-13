package agent

import (
	"encoding/json"
	"reflect"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Link the original query-local selector IDs to the existing shared display
// pool. Exact full-row matching is performed before raw/metadata clipping;
// equal line numbers, appids or names alone never establish this association.
type traceEventSelectorMembers struct {
	source types.ObservationSourceRef
	query  types.TraceEventSearchInventoryQuery
	rows   map[string][]string
}

func traceEventInventorySelectorMembers(record types.ObservationRecord, bridges ...traceEventPublicationBridge) *traceEventSelectorMembers {
	publication, valid := traceEventInventoryVerifiedPublication(record, bridges...)
	if !valid || len(publication.Tables) != 1 {
		return nil
	}
	data := publication.Tables[0].EventData
	if data == nil || len(data.Rows) != len(record.EventSearchInventory.Rows) {
		return nil
	}
	index := &traceEventSelectorMembers{source: record.SourceRef, query: record.EventSearchInventory.Query, rows: map[string][]string{}}
	for n, row := range record.EventSearchInventory.Rows {
		encoded, err := json.Marshal(row)
		if err != nil || data.Rows[n].ID == "" {
			return nil
		}
		index.rows[string(encoded)] = append(index.rows[string(encoded)], data.Rows[n].ID)
	}
	return index
}

func (index *traceEventSelectorMembers) selectIDs(record types.ObservationRecord, positions []int) []string {
	if index == nil || record.EventSearchInventory == nil || !reflect.DeepEqual(index.source, record.SourceRef) ||
		!reflect.DeepEqual(index.query, record.EventSearchInventory.Query) || record.EventSearchInventory.QueryScopeID != index.source.QueryScopeID {
		return nil
	}
	used := make(map[string]int, len(index.rows))
	ids := make([]string, 0, len(record.EventSearchInventory.Rows))
	for _, row := range record.EventSearchInventory.Rows {
		encoded, err := json.Marshal(row)
		if err != nil {
			return nil
		}
		key := string(encoded)
		queue := index.rows[key]
		if used[key] >= len(queue) {
			return nil // no partial or guessed correspondence
		}
		ids = append(ids, queue[used[key]])
		used[key]++
	}
	out := make([]string, 0, len(positions))
	for _, position := range positions {
		if position < 0 || position >= len(ids) {
			return nil
		}
		out = append(out, ids[position])
	}
	return out
}
