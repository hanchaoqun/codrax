package types

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRuntimeEventRequestWindowProjectsBeforeEverySelectionHMC237(t *testing.T) {
	r := runtimeEventRecordHMC236(t)
	// Interleaved excluded rows ensure the remaining event IDs are original
	// query IDs, not a new ordinal that can bind fields to the wrong app.
	for n, ts := range []float64{.99, 1, 1.1, 1.2} {
		r.EventSearchInventory.Rows[n].TraceTimeSeconds = ts
	}
	c := runtimeEventContractHMC236(t, r)
	original, _ := json.Marshal(c.Tables[0])
	start, end := 1.0, 1.2
	scope := &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1..1.2s"}
	projected, ok := scopeRuntimeEventTable(c.Tables[0], r, scope)
	if !ok || len(projected.EventData.Rows) != 2 || projected.EventData.Rows[0].ID != "event-2" || projected.EventData.Rows[1].ID != "event-3" || projected.nativeEventRequestExcluded != 2 {
		t.Fatalf("request projection changed IDs or window topology: %+v", projected)
	}
	if projected.EventData.Coverage.MatchedTotal != 4 || projected.EventData.HandoffRowsOmitted != 0 || !projected.EventData.RowsComplete {
		t.Fatal("request exclusions rewrote original query coverage")
	}
	private := &RuntimeMeasurementContract{Tables: []RuntimeMeasurementTable{projected}}
	group := &AnswerRuntimeMeasurementReceipt{ObservationID: r.ID, View: RuntimeMeasurementMembers, EventSelection: &RuntimeEventSelection{GroupBy: []string{"jank.appid"}}}
	if !BindRuntimeMeasurementReceipt(group, private) || !reflect.DeepEqual(group.BoundTable.Rows, [][]string{{"41002", "1"}, {"27599", "1"}}) || !strings.Contains(strings.Join(group.BoundTable.Notes, " "), "另有2条查询记录") {
		t.Fatalf("group did not use the same projected rows: %+v", group.BoundTable)
	}
	outside := &AnswerRuntimeMeasurementReceipt{ObservationID: r.ID, View: RuntimeMeasurementMembers, EventSelection: &RuntimeEventSelection{RowIDs: []string{"event-1"}}}
	if BindRuntimeMeasurementReceipt(outside, private) {
		t.Fatal("explicit selector recovered an out-of-request member")
	}
	after, _ := json.Marshal(c.Tables[0])
	if string(original) != string(after) {
		t.Fatal("display projection mutated original publication")
	}
	cloned := projected.Clone()
	if cloned.nativeEventRequestExcluded != 2 {
		t.Fatal("clone lost request selection qualifier")
	}
	var restored RuntimeMeasurementTable
	raw, _ := json.Marshal(projected)
	if json.Unmarshal(raw, &restored) != nil || restored.IsValid() {
		t.Fatal("persisted display acquired native authority")
	}
}

func TestRuntimeEventRequestScopeKeepsBusinessAndZeroDistinctHMC237(t *testing.T) {
	start, end := 0.0, 1.0
	scope := &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "0..1s"}
	if !TraceEventRowWithinRequestedWindows(TraceEventSearchInventoryRow{TraceTimeSeconds: 0}, scope) || TraceEventRowWithinRequestedWindows(TraceEventSearchInventoryRow{TraceTimeSeconds: 1}, scope) {
		t.Fatal("zero or right boundary changed")
	}
	unknown := TraceEventSearchInventoryRow{EventType: "measure_interval", TraceTimeSeconds: 0}
	if TraceEventRowWithinRequestedWindows(unknown, scope) {
		t.Fatal("unknown timestamp promoted from synthetic zero")
	}
	business := &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeBoundedSelector, SourceQuote: "business start"}
	if !TraceEventRowWithinRequestedWindows(TraceEventSearchInventoryRow{TraceTimeSeconds: 12}, business) {
		t.Fatal("business predicate silently added report window")
	}
}
