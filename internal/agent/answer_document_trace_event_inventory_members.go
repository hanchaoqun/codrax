package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

// These IDs address display objects, not unique physical events. Sharing is
// lossless only for the full row before clipping and the same source/clock
// envelope. Query membership (including repeated members) remains independent.
// No source generation is inferred when the producer did not provide one.
type traceInventoryMembers struct {
	rows []types.TraceEventSearchInventoryRow
	refs [][]string
}

type traceInventoryDisplayKey struct{ source, row [sha256.Size]byte }

func traceEventInventoryMembers(records []types.ObservationRecord) traceInventoryMembers {
	keys := make([][]traceInventoryDisplayKey, len(records))
	maxRows := 0
	for n, record := range records {
		source := record.SourceRef
		// Only request/result locators are removed from the sharing key. Keep
		// physical carrier, capture, commit, excerpt and clock provenance.
		source.ToolCallID, source.RawRef, source.PayloadRef, source.RowSetRef = "", "", "", ""
		source.QueryScopeID = ""
		source.QueryWindowKnown, source.QueryWindowStartTs, source.QueryWindowEndTs = false, 0, 0
		source.QueryTargetPID, source.QueryTargetThread, source.QueryTargetScope = 0, "", ""
		source.QueryLineRangeKnown, source.QueryLineStart, source.QueryLineEnd = false, 0, 0
		encodedSource, _ := json.Marshal(source)
		sourceKey := sha256.Sum256(encodedSource)
		for _, row := range record.EventSearchInventory.Rows {
			encoded, _ := json.Marshal(row)
			keys[n] = append(keys[n], traceInventoryDisplayKey{sourceKey, sha256.Sum256(encoded)})
		}
		if len(keys[n]) > maxRows {
			maxRows = len(keys[n])
		}
	}
	selected := make(map[traceInventoryDisplayKey]string)
	out := traceInventoryMembers{refs: make([][]string, len(records))}
	// Fairness is by query and original row order, not a heuristic relevance
	// score. Previously selected objects do not consume another display slot.
	for row := 0; row < maxRows && len(out.rows) < traceEventInventoryPromptRowLimit; row++ {
		for n, record := range records {
			if row >= len(keys[n]) || selected[keys[n][row]] != "" {
				continue
			}
			selected[keys[n][row]] = fmt.Sprintf("row-%d", len(out.rows)+1)
			out.rows = append(out.rows, record.EventSearchInventory.Rows[row])
			if len(out.rows) == traceEventInventoryPromptRowLimit {
				break
			}
		}
	}
	// Revisit every member: a late member may reference an object selected by
	// an earlier query. Never infer membership from source/time proximity.
	for n := range records {
		out.refs[n] = []string{}
		for _, key := range keys[n] {
			if id := selected[key]; id != "" {
				out.refs[n] = append(out.refs[n], id)
			}
		}
	}
	return out
}

func traceEventInventoryPromptRowCounts(records []types.ObservationRecord) []int {
	members := traceEventInventoryMembers(records)
	counts := make([]int, len(records))
	for n := range records {
		counts[n] = len(members.refs[n])
	}
	return counts
}
