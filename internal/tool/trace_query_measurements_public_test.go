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

func TestMeasurementsPublicNativeSourceAndNoInheritedOwner(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_measurements/capture.data")
	ctx, _, _ := hmc17NamedPathContext(t)
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeTargets: []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Source: "user_explicit", Confidence: 1}, {Kind: types.RuntimeTargetKindProcess, PID: 200, Source: "user_explicit", Confidence: 1}}}}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": tracequery.ViewMeasurements, "time_start": 1, "time_end": 2})
	out, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !out.Success {
		t.Fatalf("native default query: %v / %s", err, out.Summary)
	}
	for _, word := range []string{"gpufreq", "gpu_state", "gpuload", "source_arg_set_id", "量测类型", "9007199254740993"} {
		if !strings.Contains(out.Summary, word) {
			t.Errorf("explorer readable facts missing %q", word)
		}
	}
	var observation *types.ObservationRecord
	for _, r := range out.Observations {
		if r.Predicate == TraceMeasurementsPredicate {
			copy := r
			observation = &copy
		}
	}
	if observation == nil {
		t.Fatal("no native raw receipt")
	}
	r := *observation
	p, ok := types.DecodeRuntimeMeasurementPublication(r)
	if !ok || r.SourceRef.QueryTargetPID != 0 || r.SourceRef.QueryTargetThread != "" {
		t.Fatal("inherited thread/process selector", r)
	}
	if len(p.Tables) != 3 {
		t.Fatal(p)
	}
	for _, table := range p.Tables {
		if table.MemberSet != nil {
			t.Fatal("raw source acquired population authority")
		}
		if table.View == types.RuntimeMeasurementMembers && len(table.Rows) != 13 {
			t.Fatal("fixture oracle expected13 window records", table)
		}
	}
	for _, mutate := range []func(*types.ObservationRecord){func(r *types.ObservationRecord) { r.SourceRef.Path += ".other" }, func(r *types.ObservationRecord) { r.SourceRef.QueryWindowEndTs = 3 }, func(r *types.ObservationRecord) { r.SourceRef.QueryTargetPID = 1 }, func(r *types.ObservationRecord) { r.Producer = "model" }, func(r *types.ObservationRecord) {
		r.RichNotes = append(append([]string(nil), r.RichNotes...), r.RichNotes[0])
	}} {
		bad := r
		mutate(&bad)
		if _, ok := types.DecodeRuntimeMeasurementPublication(bad); ok {
			t.Fatal("rebound receipt admitted")
		}
	}
}

func TestMeasurementsPublicEventInventorySourceTime(t *testing.T) {
	dir := t.TempDir()
	var body strings.Builder
	for i, start := range []tracewire.MeasureScalar{{StorageClass: "integer", Value: "-1"}, {StorageClass: "integer", Value: "0"}, {StorageClass: "null"}} {
		null := tracewire.MeasureScalar{StorageClass: "null"}
		r := tracewire.MeasureInterval{RowID: int64(i + 1), StartNS: start, DurationNS: null, Value: null, FilterID: null, MeasureType: null, FilterStatus: "unknown"}
		line, err := tracewire.FormatMeasureInterval(r)
		if err != nil {
			t.Fatal(err)
		}
		body.WriteString(line + "\n")
	}
	path := filepath.Join(dir, "raw.systrace")
	if err := os.WriteFile(path, []byte(body.String()), 0600); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "event_types": []string{"measure_interval"}})
	out, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil || !out.Success {
		t.Fatal(err, out)
	}
	inv := requireEventSearchInventory(t, out).EventSearchInventory
	if len(inv.Rows) != 3 {
		t.Fatal(inv)
	}
	if !inv.Rows[0].SourceTimeKnown || inv.Rows[0].SourceTimeSeconds != -1e-9 || !inv.Rows[1].SourceTimeKnown || inv.Rows[1].SourceTimeSeconds != 0 || inv.Rows[2].SourceTimeKnown {
		t.Fatal("sort coordinate replaced source time", inv.Rows)
	}
	for _, r := range inv.Rows {
		if r.CPUKnown == nil || *r.CPUKnown || r.EmitterTIDKnown == nil || *r.EmitterTIDKnown || r.EmitterTGIDKnown == nil || *r.EmitterTGIDKnown {
			t.Fatal("raw resource created thread identity", r)
		}
	}
}

func TestMeasurementsPublicRejectsSelectorsBeforeRead(t *testing.T) {
	for _, args := range []string{`{"view":"measurements","pid":7}`, `{"view":"measurements","thread":"worker"}`, `{"view":"measurements","target_scope":"thread"}`, `{"view":"measurements","patterns":["freq"]}`, `{"view":"measurements","event_names":["name"]}`, `{"view":"measurements","span_name":"frame"}`} {
		out, err := (&TraceQuery{}).Execute(nil, json.RawMessage(args))
		if err != nil || out.Success || len(out.Observations) != 0 || out.RawRef != "" || out.Repair == nil || out.Repair.Code != "trace_query_raw_measurements_view" || !strings.Contains(out.Summary, "measurements accepts") {
			t.Fatal("unsupported selector not rejected before source access", args, out, err)
		}
	}
	if !strings.Contains(string((&TraceQuery{}).Parameters()), tracequery.MeasurementsTeaching) {
		t.Fatal("schema missing shared precise teaching")
	}
}
