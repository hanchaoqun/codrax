package types

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestHMC229NativeDisplayPopulationTypedQualification(t *testing.T) {
	for _, mode := range []string{"complete", "omitted", "row_count", "duplicate_ids", "blank_id", "different_query", "unknown_window", "line_window", "model_claim", "summary", "duplicate_json"} {
		t.Run(mode, func(t *testing.T) {
			r := measurementProviderRecord(t)
			p, _ := DecodeRuntimeMeasurementPublication(r)
			table := &p.Tables[0]
			table.View, table.Rows = RuntimeMeasurementMembers, [][]string{{"0", "unavailable"}, {"9007199254740993", "NULL"}}
			table.DisplayPopulation = &RuntimeMeasurementDisplayPopulation{QueryScopeID: r.SourceRef.QueryScopeID, RowIDs: []string{"source-row-1", "source-row-2"}, MatchedRows: 2}
			switch mode {
			case "omitted":
				table.DisplayPopulation.OmittedRows = 1
			case "row_count":
				table.DisplayPopulation.MatchedRows = 3
			case "duplicate_ids":
				table.DisplayPopulation.RowIDs[1] = table.DisplayPopulation.RowIDs[0]
			case "blank_id":
				table.DisplayPopulation.RowIDs[1] = " "
			case "different_query":
				table.DisplayPopulation.QueryScopeID = "other"
			case "unknown_window":
				r.SourceRef.QueryWindowKnown = false
			case "line_window":
				r.SourceRef.QueryLineRangeKnown, r.SourceRef.QueryLineStart, r.SourceRef.QueryLineEnd = true, 1, 9
			case "model_claim":
				r.ClaimAuthority = ObservationClaimAuthorityModelInference
			case "summary":
				table.View = RuntimeMeasurementSummary
			}
			p.Source = r.SourceRef
			raw, _ := json.Marshal(p)
			if mode == "duplicate_json" {
				raw = []byte(strings.Replace(string(raw), `"matched_rows":2`, `"matched_rows":2,"matched_rows":2`, 1))
			}
			r.RichNotes = []string{TraceNoteKeyRuntimeMeasurement + "=" + string(raw)}
			decoded, ok := DecodeRuntimeMeasurementPublication(r)
			if !ok || !reflect.DeepEqual(decoded.Tables[0].Rows, table.Rows) {
				t.Fatal("unqualified population lost its display values")
			}
			got := decoded.Tables[0]
			if got.HasCompleteNativeDisplayPopulation() != (mode == "complete") || got.MemberSet != nil || got.CoversMemberSet(nil) {
				t.Fatalf("wrong authority for %s: %+v", mode, got)
			}
			if mode != "complete" {
				return
			}
			clone := got.Clone()
			clone.DisplayPopulation.RowIDs[0] = "changed"
			if got.DisplayPopulation.RowIDs[0] == "changed" {
				t.Fatal("clone shares population identities")
			}
			encoded, _ := json.Marshal(got)
			var public RuntimeMeasurementTable
			if err := json.Unmarshal(encoded, &public); err != nil {
				t.Fatal(err)
			}
			if public.HasCompleteNativeDisplayPopulation() {
				t.Fatal("public JSON forged a private display credential")
			}
			if qualifyRuntimeMeasurementNativeDisplay(got, r.SourceRef, false, nil).HasCompleteNativeDisplayPopulation() {
				t.Fatal("display ignored current original source")
			}
		})
	}
}
