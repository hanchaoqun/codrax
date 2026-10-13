package types

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRuntimeEventNativeFieldsUnknownAndIndependentGroupsHMC236(t *testing.T) {
	r := runtimeEventRecordHMC236(t)
	statuses := []string{"known", "known", "unavailable", "invalid", "omitted", "absent"}
	r.EventSearchInventory.Rows = append(r.EventSearchInventory.Rows, r.EventSearchInventory.Rows[0], r.EventSearchInventory.Rows[0])
	r.EventSearchInventory.Coverage.Emitted, r.EventSearchInventory.Coverage.MatchedTotal = 6, 6
	for n := range r.EventSearchInventory.Rows {
		row := &r.EventSearchInventory.Rows[n]
		row.Line, row.LocalLine = n+1, n+1
		row.JankEvent = nil
		v := ""
		field := TraceEventSemanticField{Key: "marker.name", Type: "text", Status: statuses[n]}
		if n < 2 {
			field.Value = &v
		} else {
			field.IssueReason = "source_unavailable"
		}
		row.Semantics = &TraceEventSemantics{SchemaVersion: 1, Fields: []TraceEventSemanticField{field}}
		if n == 4 {
			row.Semantics.Fields[0] = OmitTraceEventSemanticValue(TraceEventSemanticField{Key: "marker.name", Type: "text", Status: "known", Value: semanticTestValue(strings.Repeat("large", 300))}, "value_exceeds_limit")
		}
		if n == 5 {
			row.Semantics = nil
		}
	}
	c := runtimeEventContractHMC236(t, r)
	s := &AnswerRuntimeMeasurementReceipt{ObservationID: r.ID, View: RuntimeMeasurementMembers, EventSelection: &RuntimeEventSelection{GroupBy: []string{"marker.name"}}}
	if !BindRuntimeMeasurementReceipt(s, c) {
		t.Fatal("generic semantic field could not group")
	}
	if len(s.BoundTable.Rows) != 5 || s.BoundTable.Rows[0][1] != "2" || s.BoundTable.Rows[0][0] != "\"\"（已知空值）" || !strings.HasPrefix(s.BoundTable.Rows[1][0], "未知") || !strings.HasPrefix(s.BoundTable.Rows[2][0], "无效") || !strings.HasPrefix(s.BoundTable.Rows[3][0], "已省略") || s.BoundTable.Rows[4][0] != "未提供" {
		t.Fatalf("unknown/empty groups collapsed: %#v", s.BoundTable.Rows)
	}
	unknown := &AnswerRuntimeMeasurementReceipt{ObservationID: r.ID, View: RuntimeMeasurementMembers, EventSelection: &RuntimeEventSelection{FieldKeys: []string{"emitter.tid", "source.trace_time_seconds"}}}
	if !BindRuntimeMeasurementReceipt(unknown, c) || !strings.HasPrefix(unknown.BoundTable.Rows[0][0], "未知") || !strings.HasPrefix(unknown.BoundTable.Rows[0][1], "未知") {
		t.Fatalf("legacy coordinates upgraded: %+v", unknown.BoundTable)
	}
}

func TestRuntimeEventDecoderReconstructsAllNativeFieldsHMC236(t *testing.T) {
	for _, field := range []string{"appid", "group_count", "query", "coverage", "source", "extra_table", "extra_field", "duplicate_key", "legacy_producer"} {
		t.Run(field, func(t *testing.T) {
			r := runtimeEventRecordHMC236(t)
			p, _ := BuildTraceEventRuntimeMeasurementPublication(r)
			switch field {
			case "appid":
				found := false
				for n := range p.Tables[0].EventData.Rows[3].Fields {
					f := &p.Tables[0].EventData.Rows[3].Fields[n]
					if f.Key == "jank.appid" {
						f.Value, found = semanticTestValue("27599"), true
					}
				}
				if !found {
					t.Fatal("fixture has no appid field to tamper")
				}
			case "group_count":
				p.Tables[0].Rows[0][0] = "9"
			case "query":
				p.Tables[0].EventData.Query.TimeEnd = 4
			case "coverage":
				p.Tables[0].EventData.Coverage.MatchedTotal++
			case "source":
				p.Source.Path = "/other"
			case "extra_table":
				p.Tables = append(p.Tables, p.Tables[0])
			case "extra_field":
				p.Tables[0].EventData.Fields = append(p.Tables[0].EventData.Fields, RuntimeEventFieldDescriptor{Key: "cause", Label: "root cause", Type: "text"})
			case "legacy_producer":
				r.Predicate = "io_inflight"
			}
			raw, _ := json.Marshal(p)
			if field == "duplicate_key" {
				raw = []byte(strings.Replace(string(raw), `"event_data":{`, `"event_data":{"version":1,`, 1))
			}
			r.RichNotes = []string{TraceNoteKeyRuntimeMeasurement + "=" + string(raw)}
			if _, ok := DecodeRuntimeMeasurementPublication(r); ok {
				t.Fatal("altered native publication accepted")
			}
		})
	}
}

func TestRuntimeEventRebindKeepsScopeWarningsAndDisplayIdentityHMC236(t *testing.T) {
	r := runtimeEventRecordHMC236(t)
	c := runtimeEventContractHMC236(t, r)
	c.Tables[0] = runtimeMeasurementUnverifiedWindowTable(c.Tables[0])
	ref := AnswerRuntimeMeasurementReceipt{ObservationID: r.ID, View: RuntimeMeasurementMembers, EventSelection: &RuntimeEventSelection{RowIDs: []string{"event-1"}, FieldKeys: []string{"jank.appid"}}}
	if !BindRuntimeMeasurementReceipt(&ref, c) {
		t.Fatal("qualified supplement failed")
	}
	if got := strings.Join(ref.BoundTable.Notes, " "); !strings.Contains(got, "cannot substitute") || strings.Count(got, "本视图：") != 1 {
		t.Fatalf("rebind lost scope warning/duplicated summary: %s", got)
	}
	full := AnswerRuntimeMeasurementReceipt{ObservationID: r.ID, View: RuntimeMeasurementMembers}
	if RuntimeMeasurementDisplayKey(ref, c) == RuntimeMeasurementDisplayKey(full, c) {
		t.Fatal("subset can suppress full facts")
	}
	if ref.BoundTable.DefaultPresentation {
		t.Fatal("scope warning recovered default authority")
	}
	clone := c.Clone()
	*clone.Tables[0].EventData.Rows[0].Fields[0].Value = "999"
	if reflect.DeepEqual(clone.Tables[0].EventData, c.Tables[0].EventData) {
		t.Fatal("contract clone aliases event fields")
	}
}

func TestRuntimeEventKnownZeroAndSourceScopeHMC236(t *testing.T) {
	r := runtimeEventRecordHMC236(t)
	r.EventSearchInventory.Rows = nil
	r.EventSearchInventory.Coverage.MatchedTotal, r.EventSearchInventory.Coverage.Emitted = 0, 0
	c := runtimeEventContractHMC236(t, r)
	ref := &AnswerRuntimeMeasurementReceipt{ObservationID: r.ID, View: RuntimeMeasurementMembers}
	if !BindRuntimeMeasurementReceipt(ref, c) || len(ref.BoundTable.Rows) != 0 || !strings.Contains(strings.Join(ref.BoundTable.Notes, " "), "查询匹配：0") {
		t.Fatal("known empty result became unavailable or invented rows")
	}
	p, _ := BuildTraceEventRuntimeMeasurementPublication(r)
	raw, _ := json.Marshal(p)
	r.RichNotes = []string{TraceNoteKeyRuntimeMeasurement + "=" + string(raw)}
	in := measurementProviderInput(r)
	start, end := 1.1, 1.5
	in.RequestModel = &RequestModel{RuntimeArtifactScopeProfile: &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, Confidence: 1, SourceQuote: "1.1 to 1.5"}}
	if BuildRuntimeMeasurementContract(in).Active() {
		t.Fatal("broader event query became current narrow-window facts")
	}
	var restored RuntimeMeasurementTable
	tableRaw, _ := json.Marshal(p.Tables[0])
	if json.Unmarshal(tableRaw, &restored) != nil || restored.IsValid() {
		t.Fatal("public EventData restored private binding authority")
	}
}

func TestRuntimeEventDoesNotAutomaticallyDuplicateWideInventoryHMC236(t *testing.T) {
	r := runtimeEventRecordHMC236(t)
	c := runtimeEventContractHMC236(t, r)
	rm := &RequestModel{RuntimeQuestionProfile: &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeBoundedFactSet, FactFamilies: []RuntimeQuestionFactFamily{RuntimeQuestionFactCountOrDuration}}}
	if c.Tables[0].DefaultPresentation || len(RuntimeNativeFactDisplaySelections(rm, c)) != 0 {
		t.Fatal("new optional event selector duplicated the writing inventory as a wide automatic table")
	}
	s := &AnswerRuntimeMeasurementReceipt{ObservationID: r.ID, View: RuntimeMeasurementMembers, EventSelection: &RuntimeEventSelection{FieldKeys: []string{"jank.appid"}}}
	if !BindRuntimeMeasurementReceipt(s, c) || len(s.BoundTable.Rows) != 4 {
		t.Fatal("optional non-default lane lost selection capability")
	}
}
