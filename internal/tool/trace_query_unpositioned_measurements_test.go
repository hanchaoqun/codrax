package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

func unpositionedMeasurementQuery(t *testing.T, path, view string) (types.ToolResult, tracequery.Result) {
	t.Helper()
	ctx, _, _ := hmc17NamedPathContext(t)
	params := map[string]any{"source": "path", "path": path, "view": view}
	if view == "event_search" {
		params["event_types"] = []string{"measure_interval"}
	}
	args, _ := json.Marshal(params)
	r, err := (&TraceQuery{}).Execute(ctx, args)
	if err != nil || !r.Success {
		t.Fatalf("public query: %v / %s", err, r.Summary)
	}
	for _, observation := range r.Observations {
		if observation.SourceRef.PayloadRef == "" {
			continue
		}
		data, err := os.ReadFile(observation.SourceRef.PayloadRef)
		var native tracequery.Result
		if err == nil && json.Unmarshal(data, &native) == nil {
			return r, native
		}
	}
	t.Fatal("query payload not retained")
	return r, tracequery.Result{}
}

func unpositionedMeasurementRecord(t *testing.T, r types.ToolResult) types.ObservationRecord {
	t.Helper()
	for _, record := range r.Observations {
		if record.Predicate == "unpositioned_measurements_observation" {
			return record
		}
	}
	t.Fatal("native unpositioned raw measurement table absent")
	return types.ObservationRecord{}
}

func TestUnpositionedMeasurementsPublicRawValueWithoutInventedTime(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_dual_measurements/current.data")
	_, timed := unpositionedMeasurementQuery(t, path, "measurements")
	if timed.Measurements == nil || timed.Measurements.TotalRows != 9 || timed.Measurements.UnpositionedRows != 1 {
		t.Fatalf("fixture lacks independent 9 timed + 1 unknown population: %+v", timed.Measurements)
	}
	r, raw := unpositionedMeasurementQuery(t, path, "event_search")
	if len(raw.Events) != 10 || raw.Events[0].PluginFields == nil || raw.Events[0].PluginFields.Measure == nil || raw.Events[0].PluginFields.Measure.Value.Value != "55" {
		t.Fatal("fixture did not return the exact unknown-time original")
	}
	record := unpositionedMeasurementRecord(t, r)
	p, ok := types.DecodeRuntimeMeasurementPublication(record)
	if !ok || len(p.Tables) != 1 || p.Tables[0].View != types.RuntimeMeasurementMembers || len(p.Tables[0].Rows) != 1 || p.Tables[0].MemberSet != nil || !p.Tables[0].DefaultPresentation {
		t.Fatalf("unpositioned raw display acquired invalid authority: %+v", p)
	}
	if record.Span.StartTs != 0 || record.Span.EndTs != 0 {
		t.Fatal("query sorting/window coordinates became record time")
	}
	row := p.Tables[0].Rows[0]
	if len(row) != 10 || row[7] != "55 (integer)" || row[8] != "未知（NULL）" || row[9] != "50000000 (integer)" || !strings.Contains(row[0], " / 9 / ") {
		t.Fatalf("raw value, signed time type, duration or source row changed: %+v", row)
	}
	if !strings.Contains(strings.Join(p.Tables[0].Notes, " "), "不计入任何时间窗") {
		t.Fatal("missing unknown-time boundary")
	}
}

func TestUnpositionedMeasurementsPublicPreservesScalarTypes(t *testing.T) {
	var body strings.Builder
	for i, start := range []tracewire.MeasureScalar{{StorageClass: "null"}, {StorageClass: "text", Value: "123"}, {StorageClass: "real", Value: "1.5"}, {StorageClass: "integer", Value: "-1"}, {StorageClass: "integer", Value: "0"}} {
		value := tracewire.MeasureScalar{StorageClass: "integer", Value: "9007199254740993"}
		if i == 1 {
			value = tracewire.MeasureScalar{StorageClass: "text", Value: "0"}
		}
		row := tracewire.MeasureInterval{RowID: int64(i + 1), StartNS: start, DurationNS: tracewire.MeasureScalar{StorageClass: "integer", Value: "-1"}, Value: value,
			FilterID: tracewire.MeasureScalar{StorageClass: "null"}, MeasureType: tracewire.MeasureScalar{StorageClass: "null"}, FilterStatus: "unknown"}
		line, err := tracewire.FormatMeasureInterval(row)
		if err != nil {
			t.Fatal(err)
		}
		body.WriteString(line + "\n")
	}
	path := filepath.Join(t.TempDir(), "raw.systrace")
	if err := os.WriteFile(path, []byte(body.String()), 0600); err != nil {
		t.Fatal(err)
	}
	r, _ := unpositionedMeasurementQuery(t, path, "event_search")
	p, ok := types.DecodeRuntimeMeasurementPublication(unpositionedMeasurementRecord(t, r))
	if !ok || len(p.Tables[0].Rows) != 3 {
		t.Fatalf("zero/negative integer timestamp classified as unknown: %+v", p)
	}
	for i, row := range p.Tables[0].Rows {
		if row[9] != "-1 (integer)" {
			t.Fatal("signed unknown duration erased", row)
		}
		want := "9007199254740993 (integer)"
		if i == 1 {
			want = `"0" (text)`
		}
		if row[7] != want {
			t.Fatal("integer precision/storage type lost", row)
		}
	}
}

func TestUnpositionedMeasurementsPublicScopeAndNoPopulationAuthority(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_dual_measurements/current.data")
	result, native := unpositionedMeasurementQuery(t, path, "event_search")
	record := unpositionedMeasurementRecord(t, result)
	start, end, known := types.TraceObservationContinuousQueryWindow(record.SourceRef)
	if !known {
		start, end = 0, 5
	}
	input := types.ObservationLedgerInput{ToolResults: []types.ToolResult{result}, RequestModel: &types.RequestModel{RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "explicit interval"}}}
	choices := types.BuildRuntimeMeasurementContract(input).Choices()
	if len(choices) != 1 || choices[0].DefaultPresentation || choices[0].MemberSet != nil {
		t.Fatalf("scan scope promoted unknown source time or completion: %+v", choices)
	}
	p, _ := types.DecodeRuntimeMeasurementPublication(record)
	p.Tables[0].MemberSet = &types.RuntimeMeasurementMemberSet{PopulationID: "invented", RowIDs: []string{"9"}, TotalRows: 1, Complete: true}
	encoded, _ := json.Marshal(p)
	bad := record
	bad.RichNotes = []string{types.TraceNoteKeyRuntimeMeasurement + "=" + string(encoded)}
	if _, ok := types.DecodeRuntimeMeasurementPublication(bad); ok {
		t.Fatal("unpositioned display gained MemberSet authority")
	}
	for _, mutate := range []func(*tracequery.Result){
		func(r *tracequery.Result) { r.SourcePath += ".other" },
		func(r *tracequery.Result) { r.Events[0].SourcePath += ".other" },
		func(r *tracequery.Result) { r.Events[0].LocalLine++ },
		func(r *tracequery.Result) {
			r.Events = append(r.Events, r.Events[0])
			r.EventSearchCoverage.Emitted++
			r.EventSearchCoverage.MatchedTotal++
		},
	} {
		data, _ := json.Marshal(native)
		var copy tracequery.Result
		_ = json.Unmarshal(data, &copy)
		mutate(&copy)
		if got := traceQueryUnpositionedMeasurementsObservations(copy, record.SourceRef, "scope", "now"); len(got) != 0 {
			t.Fatal("ambiguous source/row identity published")
		}
	}
}
