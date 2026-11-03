package types

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func TestHMC341OpaqueCoordinateDisplayReuseKeepsOriginalBoundaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.trace")
	if err := os.WriteFile(path, []byte("current source"), 0600); err != nil {
		t.Fatal(err)
	}
	identity, err := filegeneration.FromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	replay := TraceQueryWindowReplayRef{source: TraceQuerySourceReadRef{path: path, identity: identity, generation: &traceSourceReadGeneration{}}}
	publication := func(envelope, digit string) RuntimeMeasurementPublication {
		id := "trace_query:publication-" + strings.Repeat(digit, 64) + "#measurements"
		scope := envelope + ":frame=one:query=exact-query:query_lines=0..0"
		table := RuntimeMeasurementTable{ObservationID: id, View: RuntimeMeasurementMembers, Label: "raw", Columns: []string{"value"}, Rows: [][]string{{"42"}}, Notes: []string{"complete native population"},
			DisplayPopulation: &RuntimeMeasurementDisplayPopulation{QueryScopeID: scope, RowIDs: []string{"source-row-1"}, MatchedRows: 1}, nativeTraceDisplayPopulation: true}
		return RuntimeMeasurementPublication{Version: 1, ObservationID: id, Source: ObservationSourceRef{Path: path, PayloadRef: "/results/" + envelope, RawRef: "/results/" + envelope + ".txt", QueryScopeID: scope, QueryWindowKnown: true, QueryWindowStartTs: 1, QueryWindowEndTs: 2}, Tables: []RuntimeMeasurementTable{table}}
	}
	for _, mode := range []string{"exact", "query", "window", "value", "notes", "incomplete", "short_digest", "invalid_hex", "missing_suffix", "unknown_namespace", "no_current_source"} {
		t.Run(mode, func(t *testing.T) {
			a, b := publication("a.json", "a"), publication("b.json", "b")
			second := replay
			switch mode {
			case "query":
				b.Source.QueryScopeID += ":other-filter"
				b.Tables[0].DisplayPopulation.QueryScopeID = b.Source.QueryScopeID
			case "window":
				b.Source.QueryWindowEndTs = 1.5
			case "value":
				b.Tables[0].Rows[0][0] = "43"
			case "notes":
				b.Tables[0].Notes = append(b.Tables[0].Notes, "other scope")
			case "incomplete":
				b.Tables[0].DisplayPopulation.OmittedRows = 1
			case "short_digest":
				b.ObservationID = "trace_query:publication-ab#measurements"
			case "invalid_hex":
				b.ObservationID = "trace_query:publication-" + strings.Repeat("z", 64) + "#measurements"
			case "missing_suffix":
				b.ObservationID = "trace_query:publication-" + strings.Repeat("b", 64) + "#"
			case "unknown_namespace":
				b.ObservationID = "trace_query:alias-" + strings.Repeat("b", 64) + "#measurements"
			case "no_current_source":
				second = TraceQueryWindowReplayRef{}
			}
			b.Tables[0].ObservationID = b.ObservationID
			qualifyRuntimeMeasurementFactIdentity(&a, path, replay)
			qualifyRuntimeMeasurementFactIdentity(&b, path, second)
			if a.Tables[0].nativeFactIdentity == "" {
				t.Fatal("current complete opaque source population lost display identity")
			}
			if equal := a.Tables[0].nativeFactIdentity == b.Tables[0].nativeFactIdentity; equal != (mode == "exact") {
				t.Fatalf("display reuse crossed %s boundary: %v", mode, equal)
			}
			if a.Tables[0].CoversMemberSet(nil) || b.Tables[0].CoversMemberSet(nil) {
				t.Fatal("display normalization acquired member-set completion authority")
			}
		})
	}
}
