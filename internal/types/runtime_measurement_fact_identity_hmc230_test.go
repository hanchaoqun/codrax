package types

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func TestHMC230NativeFactIdentityFullPublicationBoundaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.trace")
	if err := os.WriteFile(path, []byte("source generation"), 0600); err != nil {
		t.Fatal(err)
	}
	identity, err := filegeneration.FromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	replay := TraceQueryWindowReplayRef{source: TraceQuerySourceReadRef{path: path, identity: identity, generation: &traceSourceReadGeneration{}}}
	publication := func(envelope string) RuntimeMeasurementPublication {
		id := "trace_query:" + envelope + ":frame=one#measurements"
		scope := envelope + ":frame=one:query=exact-query:query_lines=0..0"
		member := RuntimeMeasurementTable{ObservationID: id, View: RuntimeMeasurementMembers, Label: "raw", Columns: []string{"row", "value (unit unknown)"}, Rows: [][]string{{"physical row 1", "42 (integer)"}}, Notes: []string{"full query population"}, DefaultPresentation: true,
			DisplayPopulation: &RuntimeMeasurementDisplayPopulation{QueryScopeID: scope, RowIDs: []string{"row-1"}, MatchedRows: 1}, nativeTraceDisplayPopulation: true}
		summary := member.Clone()
		summary.View, summary.DisplayPopulation, summary.DefaultPresentation, summary.nativeTraceDisplayPopulation = RuntimeMeasurementSummary, nil, false, false
		return RuntimeMeasurementPublication{Version: 1, ObservationID: id, Source: ObservationSourceRef{Path: path, PayloadRef: "/results/" + envelope, RawRef: "/results/" + envelope + ".txt", QueryScopeID: scope, QueryWindowKnown: true, QueryWindowStartTs: 1, QueryWindowEndTs: 2}, Tables: []RuntimeMeasurementTable{member, summary}}
	}
	for _, mode := range []string{"exact", "source", "frame", "query", "window", "row_identity", "value", "field", "unit", "notes", "completeness", "sibling_value", "missing_view", "default", "member_authority", "no_current_receipt", "generation", "missing_witness"} {
		t.Run(mode, func(t *testing.T) {
			a, b := publication("a.json"), publication("b.json")
			second := replay
			switch mode {
			case "source":
				b.Source.Path += ".other"
			case "frame":
				b.Source.QueryScopeID += ":frame=two"
				b.Tables[0].DisplayPopulation.QueryScopeID = b.Source.QueryScopeID
			case "query":
				b.Source.QueryScopeID += ":filter=other"
				b.Tables[0].DisplayPopulation.QueryScopeID = b.Source.QueryScopeID
			case "window":
				b.Source.QueryWindowEndTs = 1.5
			case "row_identity":
				b.Tables[0].DisplayPopulation.RowIDs[0] = "row-2"
			case "value":
				b.Tables[0].Rows[0][1] = "42 (text)"
			case "field":
				b.Tables[0].Columns[0] = "different field"
			case "unit":
				b.Tables[0].Columns[1] = "value (Hz)"
			case "notes":
				b.Tables[1].Notes = append(b.Tables[1].Notes, "different diagnostic")
			case "completeness":
				b.Tables[0].DisplayPopulation.OmittedRows = 1
			case "sibling_value":
				b.Tables[1].Rows[0][1] = "different"
			case "missing_view":
				b.Tables = b.Tables[:1]
			case "default":
				b.Tables[0].DefaultPresentation = false
			case "member_authority":
				b.Tables[0].coverageScope = &runtimeMeasurementCoverageScope{path: path}
			case "no_current_receipt":
				second = TraceQueryWindowReplayRef{}
			case "missing_witness":
				b.Tables[0].nativeTraceDisplayPopulation = false
			case "generation":
				other := filepath.Join(t.TempDir(), "other.trace")
				if err := os.WriteFile(other, []byte("source generation"), 0600); err != nil {
					t.Fatal(err)
				}
				second.source.identity, err = filegeneration.FromPath(other)
				if err != nil {
					t.Fatal(err)
				}
			}
			qualifyRuntimeMeasurementFactIdentity(&a, path, replay)
			qualifyRuntimeMeasurementFactIdentity(&b, path, second)
			if a.Tables[0].nativeFactIdentity == "" {
				t.Fatal("positive fixture not qualified")
			}
			equal := a.Tables[0].nativeFactIdentity == b.Tables[0].nativeFactIdentity
			if equal != (mode == "exact") {
				t.Fatalf("%s reused=%v", mode, equal)
			}
			if mode == "exact" {
				contract := &RuntimeMeasurementContract{Tables: coalesceRuntimeMeasurementPublications([]RuntimeMeasurementPublication{a, b, a})}
				rm := &RequestModel{RuntimeQuestionProfile: &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeBoundedFactSet, FactFamilies: []RuntimeQuestionFactFamily{RuntimeQuestionFactOtherObservedValue}}}
				if len(contract.Choices()) != 4 || len(RuntimeNativeFactDisplaySelections(rm, contract)) != 1 {
					t.Fatal("display reuse removed original selectors or repeated exact default population")
				}
			}
			for _, p := range []RuntimeMeasurementPublication{a, b} {
				for _, table := range p.Tables {
					if table.CoversMemberSet(nil) {
						t.Fatal("display reuse minted completion")
					}
				}
			}
			raw, _ := json.Marshal(a)
			var restored RuntimeMeasurementPublication
			if err := json.Unmarshal(raw, &restored); err != nil {
				t.Fatal(err)
			}
			if restored.Tables[0].nativeFactIdentity != "" {
				t.Fatal("JSON restored display identity")
			}
		})
	}
}

func TestHMC230FactIdentityWitnessCoalesceKeepsConflictSticky(t *testing.T) {
	for _, conflicting := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			table := RuntimeMeasurementTable{ObservationID: "same", View: RuntimeMeasurementSummary, Columns: []string{"value"}, Rows: [][]string{{"42"}}, nativeFactIdentity: "current-generation-a"}
			a := RuntimeMeasurementPublication{Version: 1, ObservationID: "same", Tables: []RuntimeMeasurementTable{table}}
			b := a
			b.Tables = []RuntimeMeasurementTable{table.Clone()}
			b.Tables[0].nativeFactIdentity = ""
			if conflicting {
				b.Tables[0].nativeFactIdentity = "current-generation-b"
			}
			publications := []RuntimeMeasurementPublication{a, b, a}
			if reverse {
				publications = []RuntimeMeasurementPublication{b, a, a}
			}
			got := coalesceRuntimeMeasurementPublications(publications)
			if conflicting {
				if len(got) != 0 {
					t.Fatal("later repetition revived a conflicting generation")
				}
			} else if len(got) != 1 || got[0].nativeFactIdentity != "current-generation-a" {
				t.Fatal("identical unqualified copy erased independently current display witness")
			}
		}
	}
}
