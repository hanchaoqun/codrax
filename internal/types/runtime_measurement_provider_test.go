package types

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func measurementProviderRecord(t *testing.T) ObservationRecord {
	t.Helper()
	r := ObservationRecord{ID: "native-query-a/group-1", Producer: "trace_query", Predicate: "io_inflight",
		Origin: AnswerEvidenceOriginRuntimeArtifact, GroundingPolicy: ClaimGroundingHard,
		ProvenanceLane: ObservationProvenanceArtifactSpan,
		SourceRef: ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, Path: "/capture/a", PayloadRef: "/result/a.json",
			QueryScopeID: "query-a", QueryWindowKnown: true, QueryWindowStartTs: 0, QueryWindowEndTs: 1}}
	p := RuntimeMeasurementPublication{Version: 1, ObservationID: r.ID, Source: r.SourceRef,
		Tables: []RuntimeMeasurementTable{{ObservationID: r.ID, View: RuntimeMeasurementSummary, Label: "native",
			Columns: []string{"measured", "unknown"}, Rows: [][]string{{"0", "unavailable"}}}}}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	r.RichNotes = []string{TraceNoteKeyRuntimeMeasurement + "=" + string(data)}
	return r
}

func measurementProviderInput(r ObservationRecord) ObservationLedgerInput {
	return ObservationLedgerInput{ToolResults: []ToolResult{{ToolName: "trace_query", Success: true, Observations: []ObservationRecord{r}}}}
}

func TestRuntimeMeasurementProviderStrictSourceAndShape(t *testing.T) {
	original := measurementProviderRecord(t)
	if _, ok := DecodeRuntimeMeasurementPublication(original); !ok {
		t.Fatal("valid receipt rejected")
	}
	for _, field := range []string{"source", "payload", "query", "window", "lines", "clock", "id", "producer", "policy", "duplicate", "empty_duplicate", "extra", "trailing"} {
		t.Run(field, func(t *testing.T) {
			r := original
			r.RichNotes = append([]string(nil), original.RichNotes...)
			switch field {
			case "source":
				r.SourceRef.Path = "/capture/b"
			case "payload":
				r.SourceRef.PayloadRef = "/result/b.json"
			case "query":
				r.SourceRef.QueryScopeID = "query-b"
			case "window":
				r.SourceRef.QueryWindowEndTs = 2
			case "lines":
				r.SourceRef.QueryLineRangeKnown, r.SourceRef.QueryLineStart = true, 9
			case "clock":
				r.SourceRef.TimeDomain = "unrelated"
			case "id":
				r.ID = "other"
			case "producer":
				r.Producer = "model"
			case "policy":
				r.GroundingPolicy = ""
			case "duplicate":
				r.RichNotes = append(r.RichNotes, r.RichNotes[0])
			case "empty_duplicate":
				r.RichNotes = append([]string{TraceNoteKeyRuntimeMeasurement + "="}, r.RichNotes...)
			case "extra":
				r.RichNotes[0] = strings.Replace(r.RichNotes[0], "\"version\":1", "\"version\":1,\"injected\":true", 1)
			case "trailing":
				r.RichNotes[0] += " {}"
			}
			if _, ok := DecodeRuntimeMeasurementPublication(r); ok {
				t.Fatal("inconsistent publication accepted")
			}
		})
	}
}

func TestRuntimeMeasurementProviderRepeatedConflictingAndScope(t *testing.T) {
	r := measurementProviderRecord(t)
	in := measurementProviderInput(r)
	in.ToolResults = append(in.ToolResults, in.ToolResults[0])
	if c := BuildRuntimeMeasurementContract(in); len(c.Choices()) != 1 || !reflect.DeepEqual(c.Tables[0].Rows, [][]string{{"0", "unavailable"}}) {
		t.Fatal("identical repeat lost or numeric null confused with zero")
	}
	conflict := r
	conflict.RichNotes = []string{strings.Replace(r.RichNotes[0], "\"0\"", "\"9\"", 1)}
	in.ToolResults = append(in.ToolResults, measurementProviderInput(conflict).ToolResults[0])
	if c := BuildRuntimeMeasurementContract(in); c.Active() {
		t.Fatal("conflicting same selector silently chose one publication")
	}
	start, end := .2, .8
	in = measurementProviderInput(r)
	in.RequestModel = &RequestModel{RuntimeArtifactScopeProfile: &RuntimeArtifactScopeProfile{
		RequestedScope: RuntimeArtifactScopeExplicitWindow, SourceQuote: "0.2–0.8 seconds", TimeStart: &start, TimeEnd: &end, Confidence: 1}}
	if BuildRuntimeMeasurementContract(in).Active() {
		t.Fatal("wider window borrowed as narrower statistics")
	}
	// Line precedence is part of the publication identity, not recovered from
	// the simultaneous time arguments or a member's actual endpoints.
	p, _ := DecodeRuntimeMeasurementPublication(r)
	r.SourceRef.QueryLineRangeKnown, r.SourceRef.QueryLineStart, r.SourceRef.QueryLineEnd = true, 2, 8
	p.Source = r.SourceRef
	data, _ := json.Marshal(p)
	r.RichNotes = []string{TraceNoteKeyRuntimeMeasurement + "=" + string(data)}
	in.ToolResults = measurementProviderInput(r).ToolResults
	c := BuildRuntimeMeasurementContract(in)
	if !c.Active() || !strings.Contains(c.Tables[0].Label, "unverified") || !strings.Contains(strings.Join(c.Tables[0].Notes, " "), "cannot substitute") {
		t.Fatal("line-only query erased or promoted to selected time window")
	}
}

func TestRuntimeMeasurementProviderFreshCachedSemanticView(t *testing.T) {
	r := measurementProviderRecord(t)
	mut := NewMutableState("read measurements")
	rm := RequestModel{Language: "en", Intent: IntentExplain, Scenario: ScenarioGeneric}
	mut.SetRequestModel(rm)
	bus := &BusContext{Mutable: mut, AnalysisIR: &AnalysisIR{RequestModel: rm}}
	if BuildAnswerSemanticViewForBusContext(bus).RuntimeMeasurementContract.Active() {
		t.Fatal("empty view invented measurements")
	}
	mut.SetTurnAArtifacts(TurnAArtifacts{ToolResults: measurementProviderInput(r).ToolResults})
	if !BuildAnswerSemanticViewForBusContext(bus).RuntimeMeasurementContract.Active() {
		t.Fatal("cached view hid new supply")
	}
	mut.SetTurnAArtifacts(TurnAArtifacts{ToolResults: []ToolResult{{ToolName: "trace_query", Success: true}}})
	if BuildAnswerSemanticViewForBusContext(bus).RuntimeMeasurementContract.Active() {
		t.Fatal("cached view retained stale supply")
	}
}
