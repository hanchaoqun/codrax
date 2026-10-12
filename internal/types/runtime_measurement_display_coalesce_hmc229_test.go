package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestHMC229NativeDisplayWitnessCoalescesOnlyIdenticalFacts(t *testing.T) {
	record := measurementProviderRecord(t)
	publication, _ := DecodeRuntimeMeasurementPublication(record)
	table := &publication.Tables[0]
	table.View = RuntimeMeasurementMembers
	table.DisplayPopulation = &RuntimeMeasurementDisplayPopulation{QueryScopeID: record.SourceRef.QueryScopeID, RowIDs: []string{"original-row"}, MatchedRows: 1}
	raw, _ := json.Marshal(publication)
	record.RichNotes = []string{TraceNoteKeyRuntimeMeasurement + "=" + string(raw)}
	witnessed, ok := DecodeRuntimeMeasurementPublication(record)
	if !ok || !witnessed.Tables[0].HasCompleteNativeDisplayPopulation() {
		t.Fatal("fixture lacks producer display population")
	}
	for _, mode := range []string{"missing_display_witness", "both_missing", "source", "query", "window", "value", "row_identity", "coverage_scope", "log_private_witness"} {
		for _, reverse := range []bool{false, true} {
			t.Run(mode+map[bool]string{false: "/forward", true: "/reverse"}[reverse], func(t *testing.T) {
				strong, weak := witnessed, witnessed
				strong.Tables = []RuntimeMeasurementTable{witnessed.Tables[0].Clone()}
				weak.Tables = []RuntimeMeasurementTable{witnessed.Tables[0].Clone()}
				weak.Tables[0].nativeTraceDisplayPopulation = false
				switch mode {
				case "both_missing":
					strong.Tables[0].nativeTraceDisplayPopulation = false
				case "source":
					weak.Source.Path += ".other"
				case "query":
					weak.Source.QueryScopeID += ".other"
				case "window":
					weak.Source.QueryWindowEndTs++
				case "value":
					weak.Tables[0].Rows[0][0] = "different"
				case "row_identity":
					weak.Tables[0].DisplayPopulation.RowIDs[0] = "different"
				case "coverage_scope":
					weak.Tables[0].coverageScope = &runtimeMeasurementCoverageScope{path: record.SourceRef.Path, windowKnown: true}
				case "log_private_witness":
					weak.Tables[0].nativeLogDisplayPopulation = true
				}
				inputs := []RuntimeMeasurementPublication{weak, strong}
				if reverse {
					inputs[0], inputs[1] = inputs[1], inputs[0]
				}
				got := coalesceRuntimeMeasurementPublications(inputs)
				compatible := mode == "missing_display_witness" || mode == "both_missing"
				if !compatible {
					if len(got) != 0 {
						t.Fatalf("conflicting %s retained a selectable table", mode)
					}
					return
				}
				if len(got) != 1 || !reflect.DeepEqual(got[0].Rows, strong.Tables[0].Rows) {
					t.Fatalf("absence of display witness removed identical facts: %+v", got)
				}
				if got[0].HasCompleteNativeDisplayPopulation() != (mode == "missing_display_witness") || got[0].MemberSet != nil || got[0].CoversMemberSet(nil) {
					t.Fatal("coalescing manufactured or lost authority")
				}
				if weak.Tables[0].nativeTraceDisplayPopulation {
					t.Fatal("coalescing changed the unqualified input")
				}
			})
		}
	}
}
