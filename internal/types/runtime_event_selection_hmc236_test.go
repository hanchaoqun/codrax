package types

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func runtimeEventRecordHMC236(t *testing.T) ObservationRecord {
	t.Helper()
	r := testTraceEventSearchInventoryRecord()
	r.SourceRef.QueryWindowKnown, r.SourceRef.QueryWindowStartTs, r.SourceRef.QueryWindowEndTs = true, 1, 2
	i := r.EventSearchInventory
	i.Query.TimeStart, i.Query.TimeEnd, i.Query.TimeStartSet, i.Query.TimeEndSet = 1, 2, true, true
	i.Coverage.ScopeKind, i.Coverage.MatchedTotal, i.Coverage.Emitted = "selected_window", 4, 4
	first := i.Rows[0]
	i.Rows = nil
	for n, app := range []int64{27599, 41002, 27599, 41002} {
		row := first
		row.Line, row.SourcePath, row.LocalLine, row.TraceTimeSeconds = n+1, r.SourceRef.Path, n+1, 1+float64(n)/10
		row.JankEvent = &TraceEventSearchJankEvent{TimeDomainStatus: TraceJankSourceClock, Values: &TraceEventSearchJankValues{StartTSNS: 9007199254740993 + int64(n), EndTSNS: 9007199254741093 + int64(n), JankFrames: int64(n + 3), AppID: app, ReportedDurationNS: 100}}
		i.Rows = append(i.Rows, row)
	}
	if !IsValidTraceEventSearchInventoryRecord(r) {
		t.Fatal("bad fixture")
	}
	return r
}

func runtimeEventContractHMC236(t *testing.T, r ObservationRecord) *RuntimeMeasurementContract {
	t.Helper()
	p, ok := BuildTraceEventRuntimeMeasurementPublication(r)
	if !ok || len(p.Tables) != 1 {
		t.Fatal("native inventory did not publish")
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	r.RichNotes = []string{TraceNoteKeyRuntimeMeasurement + "=" + string(data)}
	decoded, ok := DecodeRuntimeMeasurementPublication(r)
	if !ok {
		t.Fatal("published native inventory did not decode")
	}
	return &RuntimeMeasurementContract{Tables: decoded.Tables}
}

func TestRuntimeEventSelectionExactMembersAndGroupHMC236(t *testing.T) {
	r := runtimeEventRecordHMC236(t)
	c := runtimeEventContractHMC236(t, r)
	selectRows := &AnswerRuntimeMeasurementReceipt{ObservationID: r.ID, View: RuntimeMeasurementMembers,
		EventSelection: &RuntimeEventSelection{RowIDs: []string{"event-4", "event-2"}, FieldKeys: []string{"jank.appid", "jank.start_ts_ns"}}}
	if !BindRuntimeMeasurementReceipt(selectRows, c) || !selectRows.IsBound() {
		t.Fatal("selector did not bind")
	}
	if got := selectRows.BoundTable.Rows; !reflect.DeepEqual(got, [][]string{{"41002", "9007199254740994"}, {"41002", "9007199254740996"}}) {
		t.Fatalf("row order or exact values lost: %#v", got)
	}
	selectRows.EventSelection.GroupBy = []string{"jank.appid"}
	if selectRows.IsBound() {
		t.Fatal("selector mutation retained binding")
	}
	selectRows.EventSelection.FieldKeys = nil
	if !BindRuntimeMeasurementReceipt(selectRows, c) || !reflect.DeepEqual(selectRows.BoundTable.Rows, [][]string{{"41002", "2"}}) {
		t.Fatalf("selected group wrong: %+v", selectRows.BoundTable)
	}
	all := &AnswerRuntimeMeasurementReceipt{ObservationID: r.ID, View: RuntimeMeasurementMembers, EventSelection: &RuntimeEventSelection{GroupBy: []string{"jank.appid"}}}
	if !BindRuntimeMeasurementReceipt(all, c) || !reflect.DeepEqual(all.BoundTable.Rows, [][]string{{"27599", "2"}, {"41002", "2"}}) {
		t.Fatalf("native groups wrong: %+v", all.BoundTable)
	}
	if all.BoundTable.MemberSet != nil || all.BoundTable.coverageScope != nil {
		t.Fatal("event facts acquired completion authority")
	}
}

func TestRuntimeEventSelectionRejectsForgedOrAmbiguousHMC236(t *testing.T) {
	r := runtimeEventRecordHMC236(t)
	c := runtimeEventContractHMC236(t, r)
	for _, s := range []*RuntimeEventSelection{
		{RowIDs: []string{"event-99"}}, {RowIDs: []string{"event-1", "event-1"}},
		{FieldKeys: []string{"root_cause"}}, {FieldKeys: []string{"jank.appid", "jank.appid"}},
		{GroupBy: []string{"jank.appid", "jank.appid"}}, {GroupBy: []string{"invented"}},
		{FieldKeys: []string{"jank.appid"}, GroupBy: []string{"jank.appid"}},
	} {
		ref := &AnswerRuntimeMeasurementReceipt{ObservationID: r.ID, View: RuntimeMeasurementMembers, EventSelection: s}
		if BindRuntimeMeasurementReceipt(ref, c) || ref.IsBound() {
			t.Fatalf("invalid selector accepted: %+v", s)
		}
	}
	legacy := &AnswerRuntimeMeasurementReceipt{ObservationID: "native-query-a/group-1", View: RuntimeMeasurementSummary, EventSelection: &RuntimeEventSelection{}}
	if BindRuntimeMeasurementReceipt(legacy, BuildRuntimeMeasurementContract(measurementProviderInput(measurementProviderRecord(t)))) {
		t.Fatal("event selector borrowed legacy measurement")
	}
	p, _ := BuildTraceEventRuntimeMeasurementPublication(r)
	p.Tables[0].Rows[0][0] = "forged"
	raw, _ := json.Marshal(p)
	r.RichNotes = []string{TraceNoteKeyRuntimeMeasurement + "=" + string(raw)}
	if _, ok := DecodeRuntimeMeasurementPublication(r); ok {
		t.Fatal("forged displayed value accepted")
	}
}

func TestRuntimeEventSelectionPartialAndCloneHMC236(t *testing.T) {
	r := runtimeEventRecordHMC236(t)
	r.EventSearchInventory.Coverage.MatchedTotal = 99
	r.EventSearchInventory.Coverage.EnumerationComplete, r.EventSearchInventory.RowsComplete = false, false
	c := runtimeEventContractHMC236(t, r)
	ref := &AnswerRuntimeMeasurementReceipt{ObservationID: r.ID, View: RuntimeMeasurementMembers, EventSelection: &RuntimeEventSelection{GroupBy: []string{"jank.appid"}}}
	if !BindRuntimeMeasurementReceipt(ref, c) {
		t.Fatal("partial retained rows disappeared")
	}
	if got := strings.Join(ref.BoundTable.Notes, " "); !strings.Contains(got, "99") || !strings.Contains(got, "完整") {
		t.Fatalf("partial scope undisclosed: %s", got)
	}
	cloned := ref.Clone()
	cloned.EventSelection.GroupBy[0] = "jank.jank_frames"
	cloned.BoundTable.EventData.Rows[0].Fields[0].Value = nil
	if !ref.IsBound() || cloned.IsBound() {
		t.Fatal("clone aliased selection or accepted stale binding")
	}
	raw, _ := json.Marshal(ref)
	var restored AnswerRuntimeMeasurementReceipt
	if json.Unmarshal(raw, &restored) != nil || restored.IsBound() || restored.EventSelection == nil {
		t.Fatal("JSON restored authority or lost selector")
	}
}
