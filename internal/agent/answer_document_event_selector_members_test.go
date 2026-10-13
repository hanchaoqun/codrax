package agent

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEventSelectorMembersKeepExactDuplicateOccurrencesHMC236(t *testing.T) {
	ctx := traceEventInventoryPublicContext(traceEventInventoryPublicResults(t, 30, "2"))
	var r types.ObservationRecord
	for _, record := range answerDocObservationLedger(ctx).Records {
		if types.IsValidTraceEventSearchInventoryRecord(record) {
			r = record
			break
		}
	}
	i := types.CloneTraceEventSearchInventory(r.EventSearchInventory)
	first := i.Rows[0]
	other := types.CloneTraceEventSearchInventory(i).Rows[0]
	other.JankEvent.Values.AppID++ // Same source line/time/name, different full row.
	i.Rows = []types.TraceEventSearchInventoryRow{first, other, first}
	i.Coverage.Emitted, i.Coverage.MatchedTotal = 3, 3
	i.RowsComplete, i.HandoffRowsOmitted = true, 0
	r.EventSearchInventory, r.RichNotes = i, nil
	p, valid := types.BuildTraceEventRuntimeMeasurementPublication(r)
	if !valid {
		t.Fatal("duplicate occurrence fixture failed native publication")
	}
	raw, _ := json.Marshal(p)
	r.RichNotes = []string{types.TraceNoteKeyRuntimeMeasurement + "=" + string(raw)}
	index := traceEventInventorySelectorMembers(r)
	if index == nil {
		t.Fatal("verified publication did not supply selector membership")
	}
	before, _ := json.Marshal(r)
	projected := r
	projected.EventSearchInventory = types.CloneTraceEventSearchInventory(i)
	projected.EventSearchInventory.Rows = []types.TraceEventSearchInventoryRow{first, first}
	if got := index.selectIDs(projected, []int{0, 1}); !reflect.DeepEqual(got, []string{"event-1", "event-3"}) {
		t.Fatalf("duplicate full-row queue merged or renumbered members: %v", got)
	}
	if got := index.selectIDs(r, []int{1}); !reflect.DeepEqual(got, []string{"event-2"}) {
		t.Fatalf("same-line different payload borrowed selector: %v", got)
	}
	members := traceEventInventoryMembers([]types.ObservationRecord{r})
	if len(members.rows) != 2 || !reflect.DeepEqual(members.refs[0], []string{"row-1", "row-2", "row-1"}) ||
		!reflect.DeepEqual(index.selectIDs(r, members.positions[0]), []string{"event-1", "event-2", "event-3"}) {
		t.Fatal("shared display storage changed occurrence multiplicity")
	}
	for name, change := range map[string]func(*types.ObservationRecord){
		"source":   func(r *types.ObservationRecord) { r.SourceRef.Path += ".other" },
		"window":   func(r *types.ObservationRecord) { r.SourceRef.QueryWindowEndTs++ },
		"query":    func(r *types.ObservationRecord) { r.EventSearchInventory.Query.Limit++ },
		"query id": func(r *types.ObservationRecord) { r.EventSearchInventory.QueryScopeID += "other" },
		"raw tail": func(r *types.ObservationRecord) { r.EventSearchInventory.Rows[0].Raw += "different" },
		"too many repeats": func(r *types.ObservationRecord) {
			r.EventSearchInventory.Rows = append(r.EventSearchInventory.Rows, r.EventSearchInventory.Rows[0])
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := r
			changed.EventSearchInventory = types.CloneTraceEventSearchInventory(i)
			change(&changed)
			if got := index.selectIDs(changed, []int{0}); got != nil {
				t.Fatalf("changed query/member acquired stale selector mapping: %v", got)
			}
		})
	}
	after, _ := json.Marshal(r)
	if string(before) != string(after) {
		t.Fatal("mapping changed native receipt")
	}
}
