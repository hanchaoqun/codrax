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
	// Positions address each query's projected rows before display clipping.
	// They retain multiplicity when several members share a display object.
	positions [][]int
}

type traceInventoryDisplayKey struct{ source, row [sha256.Size]byte }

func traceEventInventoryMembers(records []types.ObservationRecord) traceInventoryMembers {
	keys := make([][]traceInventoryDisplayKey, len(records))
	priorities := make([][]int, len(records))
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
		priorities[n] = traceEventInventoryPreviewOrder(len(keys[n]))
	}
	selected := make(map[traceInventoryDisplayKey]string)
	out := traceInventoryMembers{refs: make([][]string, len(records)), positions: make([][]int, len(records))}
	// Fairness remains by query. Within each finite retained member set,
	// prioritize its boundaries then split positional gaps so a second display
	// limit does not silently become another head-only preview. This is neither
	// random sampling nor a statement that displayed rows represent the source.
	for rank := 0; rank < maxRows && len(out.rows) < traceEventInventoryPromptRowLimit; rank++ {
		for n, record := range records {
			if rank >= len(priorities[n]) {
				continue
			}
			row := priorities[n][rank]
			if selected[keys[n][row]] != "" {
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
		for position, key := range keys[n] {
			if id := selected[key]; id != "" {
				out.refs[n] = append(out.refs[n], id)
				out.positions[n] = append(out.positions[n], position)
			}
		}
	}
	return out
}

// A deterministic, prefix-stable allocation order over retained positions.
// Member references are rebuilt in original order after allocation, so the
// budget cannot reverse chronology or misaddress a query-local selector ID.
func traceEventInventoryPreviewOrder(count int) []int {
	if count == 0 {
		return nil
	}
	out := []int{0}
	if count == 1 {
		return out
	}
	out = append(out, count-1)
	type gap struct{ left, right int }
	gaps := []gap{{0, count - 1}}
	for n := 0; n < len(gaps); n++ {
		g := gaps[n]
		if g.right-g.left <= 1 {
			continue
		}
		middle := g.left + (g.right-g.left+1)/2
		out = append(out, middle)
		gaps = append(gaps, gap{g.left, middle}, gap{middle, g.right})
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
