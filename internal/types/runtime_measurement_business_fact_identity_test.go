package types

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func TestHMC230BusinessFactIdentityExactBoundaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.trace")
	if err := os.WriteFile(path, []byte("paired markers and states"), 0600); err != nil {
		t.Fatal(err)
	}
	identity, err := filegeneration.FromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	replay := TraceQueryWindowReplayRef{source: TraceQuerySourceReadRef{path: path, identity: identity, generation: &traceSourceReadGeneration{}}}
	fixture := func(id string) (RuntimeMeasurementPublication, ObservationRecord) {
		source := ObservationSourceRef{Path: path, PayloadRef: "/results/" + id, RawRef: "/results/" + id + ".txt", QueryScopeID: id + ":query=original", QueryWindowKnown: true, QueryWindowStartTs: 1, QueryWindowEndTs: 2, QueryTargetPID: 7, QueryTargetThread: "worker", QueryTargetScope: "thread"}
		record := ObservationRecord{ID: id, Predicate: TraceBusinessSpanPredicate, Subject: "worker-7", Object: "BusinessWork", SourceRef: source, Span: ObservationSpan{LineStart: 10, LineEnd: 20, StartTs: 1.1, EndTs: 1.2}}
		table := RuntimeMeasurementTable{ObservationID: id, View: RuntimeMeasurementSummary, Label: "native business", Columns: []string{"state", "duration (ms)"}, Rows: [][]string{{"running", "5"}}, Notes: []string{"partial: uncovered interval remains unknown"}, DefaultPresentation: true}
		return RuntimeMeasurementPublication{Version: 1, ObservationID: id, Source: source, Tables: []RuntimeMeasurementTable{table}}, record
	}
	for _, mode := range []string{"query_envelope_view_target", "source", "owner", "business", "span_time", "span_line", "invalid_span", "window", "line_scope", "clock", "value", "notes", "unit", "membership", "display_population", "private_coverage", "generation", "no_receipt", "wrong_predicate", "missing_original"} {
		t.Run(mode, func(t *testing.T) {
			a, ar := fixture("first")
			b, br := fixture("second")
			second, original := replay, path
			b.Source.QueryScopeID += ":different-view"
			b.Source.QueryTargetPID, b.Source.QueryTargetThread, b.Source.QueryTargetScope = 8, "other-search-selector", "process"
			switch mode {
			case "source":
				b.Source.Path += ".other"
			case "owner":
				br.Subject = "worker-8"
			case "business":
				br.Object = "DifferentWork"
			case "span_time":
				br.Span.StartTs += 0.001
			case "span_line":
				br.Span.LineStart++
			case "invalid_span":
				br.Span.LineEnd = br.Span.LineStart
			case "window":
				b.Source.QueryWindowEndTs += 0.001
			case "line_scope":
				b.Source.QueryLineStart = 8
			case "clock":
				b.Source.TimeDomain = "different-clock"
			case "value":
				b.Tables[0].Rows[0][1] = "8"
			case "notes":
				b.Tables[0].Notes[0] = "complete"
			case "unit":
				b.Tables[0].Columns[1] = "duration (seconds)"
			case "membership":
				b.Tables[0].MemberSet = &RuntimeMeasurementMemberSet{}
			case "display_population":
				b.Tables[0].DisplayPopulation = &RuntimeMeasurementDisplayPopulation{}
			case "private_coverage":
				b.Tables[0].coverageScope = &runtimeMeasurementCoverageScope{path: path}
			case "generation":
				other := filepath.Join(t.TempDir(), "other.trace")
				if err := os.WriteFile(other, []byte("paired markers and states"), 0600); err != nil {
					t.Fatal(err)
				}
				second.source.identity, err = filegeneration.FromPath(other)
				if err != nil {
					t.Fatal(err)
				}
			case "no_receipt":
				second = TraceQueryWindowReplayRef{}
			case "wrong_predicate":
				br.Predicate = "measurements_observation"
			case "missing_original":
				original = ""
			}
			qualifyRuntimeBusinessMeasurementIdentity(&a, ar, path, replay)
			qualifyRuntimeBusinessMeasurementIdentity(&b, br, original, second)
			if a.Tables[0].nativeFactIdentity == "" {
				t.Fatal("business fixture not qualified")
			}
			if got := a.Tables[0].nativeFactIdentity == b.Tables[0].nativeFactIdentity; got != (mode == "query_envelope_view_target") {
				t.Fatalf("%s reused=%v", mode, got)
			}
			if a.Tables[0].HasCompleteNativeDisplayPopulation() || a.Tables[0].CoversMemberSet(nil) {
				t.Fatal("business display minted population authority")
			}
			raw, _ := json.Marshal(a)
			var restored RuntimeMeasurementPublication
			if err := json.Unmarshal(raw, &restored); err != nil {
				t.Fatal(err)
			}
			if restored.Tables[0].nativeFactIdentity != "" {
				t.Fatal("JSON restored current business display witness")
			}
		})
	}
}
