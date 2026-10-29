package types

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func nativeProjectionHMC312(t *testing.T, path, envelope string) ObservationRecord {
	t.Helper()
	r := measurementProviderRecord(t)
	r.Predicate, r.Role = "thread_timeline_observation", AnswerAggregateRoleSupportingCoverage
	r.ID = "trace_query:" + envelope + "#thread_timeline_observation"
	r.SourceRef.Path, r.SourceRef.PayloadRef, r.SourceRef.RawRef = path, envelope, envelope+".txt"
	r.SourceRef.QueryScopeID = envelope + ":query=" + envelope
	r.SourceRef.QueryTargetPID, r.SourceRef.QueryTargetScope = 100, "thread"
	r.SourceRef.QueryLineRangeKnown = true
	p := RuntimeMeasurementPublication{Version: 1, ObservationID: r.ID, Source: r.SourceRef, Tables: []RuntimeMeasurementTable{{ObservationID: r.ID, View: RuntimeMeasurementTimeline, DefaultPresentation: true,
		Label: "retained partition", Columns: []string{"state", "duration (ms)"}, Rows: [][]string{{"sleep", "31"}, {"runnable", "1"}}, Notes: []string{"source interval end is exclusive"}}}}
	raw, _ := json.Marshal(p)
	r.RichNotes = []string{TraceNoteKeyRuntimeMeasurement + "=" + string(raw)}
	return r
}

func nativeProjectionReplayHMC312(t *testing.T) (string, TraceQueryWindowReplayRef) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "capture.trace")
	if err := os.WriteFile(path, []byte("exact capture generation"), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := filegeneration.FromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, TraceQueryWindowReplayRef{source: TraceQuerySourceReadRef{path: path, identity: id, generation: &traceSourceReadGeneration{}}}
}

func projectionKeyHMC312(t *testing.T, r ObservationRecord, path string, replay TraceQueryWindowReplayRef) string {
	t.Helper()
	p, ok := DecodeRuntimeMeasurementPublication(r)
	if !ok {
		return ""
	}
	qualifyRuntimeMeasurementEquivalentFact(&p, r, path, replay)
	return p.Tables[0].nativeFactIdentity
}

func TestHMC312NativeMeasurementEquivalentFactsExactBoundaries(t *testing.T) {
	path, replay := nativeProjectionReplayHMC312(t)
	native := json.RawMessage(`{"partition":"one","intervals":[{"start":1.0100000001,"end":1.0410000001,"end_known":true}]}`)
	base := PublishRuntimeMeasurementFactIdentity(nativeProjectionHMC312(t, path, "a.json"), native)
	want := projectionKeyHMC312(t, base, path, replay)
	if want == "" {
		t.Fatal("positive projection not qualified")
	}
	for _, mode := range []string{"query_alias", "source", "target", "target_selector", "window", "unknown_window", "lines", "clock", "native_precision", "native_end_known", "native_member", "column", "column_order", "row", "row_order", "notes", "label", "default", "preview", "member_authority", "population_authority", "future_predicate", "no_receipt", "generation", "json", "mutation"} {
		t.Run(mode, func(t *testing.T) {
			r := nativeProjectionHMC312(t, path, "b.json")
			facts, current := native, replay
			r = rewriteRawProjectionHMC268(t, r, func(r *ObservationRecord, p *RuntimeMeasurementPublication) {
				switch mode {
				case "source":
					r.SourceRef.Path += ".other"
				case "target":
					r.SourceRef.QueryTargetPID = 200
				case "target_selector":
					r.SourceRef.QueryTargetThread = "app-main"
				case "window":
					r.SourceRef.QueryWindowEndTs = 2
				case "unknown_window":
					r.SourceRef.QueryWindowKnown = false
				case "lines":
					r.SourceRef.QueryLineStart = 2
				case "clock":
					r.SourceRef.CanonicalTimeDomain = "other-clock"
				case "native_precision":
					facts = json.RawMessage(`{"partition":"one","intervals":[{"start":1.0100000002,"end":1.0410000001,"end_known":true}]}`)
				case "native_end_known":
					facts = json.RawMessage(`{"partition":"one","intervals":[{"start":1.0100000001,"end":1.0410000001,"end_known":false}]}`)
				case "native_member":
					facts = json.RawMessage(`{"partition":"two","intervals":[{"start":1.0100000001,"end":1.0410000001,"end_known":true}]}`)
				case "column":
					p.Tables[0].Columns[1] = "request residence (ms)"
				case "column_order":
					p.Tables[0].Columns[0], p.Tables[0].Columns[1] = p.Tables[0].Columns[1], p.Tables[0].Columns[0]
				case "row":
					p.Tables[0].Rows[0][0] = "running"
				case "row_order":
					p.Tables[0].Rows[0], p.Tables[0].Rows[1] = p.Tables[0].Rows[1], p.Tables[0].Rows[0]
				case "notes":
					p.Tables[0].Notes = []string{"end is inclusive"}
				case "label":
					p.Tables[0].Label = "other measurement"
				case "default":
					p.Tables[0].DefaultPresentation = false
				case "preview":
					p.Tables[0].Rows = p.Tables[0].Rows[:1]
					p.Tables[0].Notes = []string{"one retained row omitted from preview"}
				case "member_authority":
					p.Tables[0].MemberSet = &RuntimeMeasurementMemberSet{}
				case "population_authority":
					p.Tables[0].DisplayPopulation = &RuntimeMeasurementDisplayPopulation{}
				case "future_predicate":
					r.Predicate = "future_timeline_observation"
				}
			})
			r = PublishRuntimeMeasurementFactIdentity(r, facts)
			if mode == "no_receipt" {
				current = TraceQueryWindowReplayRef{}
			}
			if mode == "generation" {
				_, other := nativeProjectionReplayHMC312(t)
				current.source.identity = other.source.identity
			}
			if mode == "json" {
				data, _ := json.Marshal(r)
				var copy ObservationRecord
				if err := json.Unmarshal(data, &copy); err != nil {
					t.Fatal(err)
				}
				r = FinalizeRuntimeMeasurementFactIdentity(copy)
			}
			if mode == "mutation" {
				r = rewriteRawProjectionHMC268(t, r, func(_ *ObservationRecord, p *RuntimeMeasurementPublication) { p.Tables[0].Rows[0][1] = "35" })
				r = FinalizeRuntimeMeasurementFactIdentity(r)
			}
			key := projectionKeyHMC312(t, r, path, current)
			if (key == want) != (mode == "query_alias") {
				t.Fatalf("display equivalence crossed %s: base=%q current=%q", mode, want, key)
			}
		})
	}
}

func TestHMC312NativeMeasurementFinalizationCannotRepairChangedFacts(t *testing.T) {
	path, replay := nativeProjectionReplayHMC312(t)
	base := PublishRuntimeMeasurementFactIdentity(nativeProjectionHMC312(t, path, "a.json"), json.RawMessage(`{"intervals":[1,2]}`))
	for _, mode := range []string{"decoration", "target", "query", "window", "path", "table", "native_missing", "span", "role"} {
		t.Run(mode, func(t *testing.T) {
			r := rewriteRawProjectionHMC268(t, base, func(r *ObservationRecord, p *RuntimeMeasurementPublication) {
				switch mode {
				case "decoration":
					r.SourceRef.TimeDomain, r.SourceRef.ClockAlignment = "trace_seconds", "identity"
					r.Span.Selector = "source lines"
					r.SupportRefs = []string{"current"}
				case "target":
					r.SourceRef.QueryTargetPID = 200
				case "query":
					r.SourceRef.QueryScopeID += ":another"
				case "window":
					r.SourceRef.QueryWindowEndTs = 2
				case "path":
					r.SourceRef.Path += ".other"
				case "table":
					p.Tables[0].Rows[0][0] = "running"
				case "native_missing":
					r.runtimeMeasurementFactIdentity = nil
				case "span":
					r.Span.EndTs = 2
				case "role":
					r.Role = AnswerAggregateRolePrincipalAnswer
				}
			})
			r = FinalizeRuntimeMeasurementFactIdentity(r)
			if (projectionKeyHMC312(t, r, path, replay) != "") != (mode == "decoration") {
				t.Fatal("finalizer repaired facts or lost permitted decoration")
			}
			if projectionKeyHMC312(t, base, path, replay) == "" {
				t.Fatal("immutable prior receipt mutated")
			}
		})
	}
}

func TestHMC312NativeDisplayWitnessPrivateImmutableAndCloned(t *testing.T) {
	field, ok := reflect.TypeOf(ObservationRecord{}).FieldByName("runtimeMeasurementFactIdentity")
	if !ok || field.PkgPath == "" || field.Type != reflect.TypeOf((*runtimeMeasurementFactWitness)(nil)) {
		t.Fatal("display witness must remain producer-private, outside JSON observation fields")
	}
	witness := reflect.TypeOf(runtimeMeasurementFactWitness{})
	if witness.NumField() != 3 {
		t.Fatal("classify any new witness field before using immutable clone semantics")
	}
	for i := 0; i < witness.NumField(); i++ {
		if witness.Field(i).Type.Kind() != reflect.String || witness.Field(i).PkgPath == "" {
			t.Fatal("witness must contain only private immutable strings")
		}
	}
	path, replay := nativeProjectionReplayHMC312(t)
	original := PublishRuntimeMeasurementFactIdentity(nativeProjectionHMC312(t, path, "a.json"), json.RawMessage(`{"intervals":[1,2]}`))
	m := NewMutableState("native projection display")
	m.SetTurnAArtifacts(TurnAArtifacts{ToolResults: []ToolResult{{ToolName: "trace_query", Success: true, Observations: []ObservationRecord{original}}}})
	copy := m.TurnAArtifacts().ToolResults[0].Observations[0]
	if projectionKeyHMC312(t, copy, path, replay) == "" {
		t.Fatal("accepted artifact clone lost display witness")
	}
	copy = rewriteRawProjectionHMC268(t, copy, func(_ *ObservationRecord, p *RuntimeMeasurementPublication) { p.Tables[0].Notes = []string{"changed"} })
	if projectionKeyHMC312(t, copy, path, replay) != "" || projectionKeyHMC312(t, m.TurnAArtifacts().ToolResults[0].Observations[0], path, replay) == "" {
		t.Fatal("changed clone retained authority or mutated original")
	}
	data, _ := json.Marshal(original)
	if string(data) == "" {
		t.Fatal("JSON witness fixture")
	}
	var restored ObservationRecord
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.runtimeMeasurementFactIdentity != nil {
		t.Fatal("serialized observation recovered a private display witness")
	}
}
