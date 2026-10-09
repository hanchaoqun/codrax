package tool

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestProcessMeasurementsPublicNativePreparationAndReceipt(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_process_measurements/capture.data")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, _, _ := hmc17NamedPathContext(t)
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "process_measurements", "time_start": 1, "time_end": 2})
	result, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !result.Success {
		t.Fatalf("default process preparation/query failed: %v %s", err, result.Summary)
	}
	var found bool
	for _, r := range result.Observations {
		if r.Predicate != "process_measurements_observation" {
			continue
		}
		publication, ok := types.DecodeRuntimeMeasurementPublication(r)
		if !ok {
			t.Fatalf("native row publication absent: %+v", r)
		}
		encoded, _ := json.Marshal(publication.Tables)
		for _, want := range []string{"app.alpha", "app.beta", "9007199254740993", "unavailable"} {
			if !strings.Contains(string(encoded), want) {
				t.Errorf("lost native value or owner %q: %s", want, encoded)
			}
		}
		if strings.Contains(string(encoded), "9900") {
			t.Fatal("included right boundary")
		}
		if r.SourceRef.QueryTargetThread != "" || r.SourceRef.QueryWindowStartTs != 1 || r.SourceRef.QueryWindowEndTs != 2 {
			t.Fatalf("changed owner/window: %+v", r.SourceRef)
		}
		found = true
	}
	if !found {
		t.Fatal("no source-bound process measurement table")
	}
	after, err := os.ReadFile(path)
	if err != nil || sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("source database mutated")
	}
}

func TestProcessMeasurementsPublicOwnerScope(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_process_measurements/capture.data")
	for _, tc := range []struct {
		name  string
		focus []types.RuntimeTarget
		pid   int
		rows  int
	}{
		{"thread is not process", []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Source: "user_explicit", Confidence: 1}}, 0, 11},
		{"typed process", []types.RuntimeTarget{{Kind: types.RuntimeTargetKindProcess, PID: 100, Source: "user_explicit", Confidence: 1}}, 100, 7},
		{"multiple owners", []types.RuntimeTarget{{Kind: types.RuntimeTargetKindProcess, PID: 100, Source: "user_explicit", Confidence: 1}, {Kind: types.RuntimeTargetKindProcess, PID: 200, Source: "user_explicit", Confidence: 1}}, 0, 11},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _, _ := hmc17NamedPathContext(t)
			ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeTargets: tc.focus}}
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "process_measurements", "time_start": 1, "time_end": 2})
			out, err := (&TraceQuery{}).Execute(ctx, params)
			if err != nil || !out.Success {
				t.Fatalf("query: %v %s", err, out.Summary)
			}
			found := false
			for _, r := range out.Observations {
				if r.Predicate != TraceProcessMeasurementsPredicate {
					continue
				}
				p, ok := types.DecodeRuntimeMeasurementPublication(r)
				if !ok || r.SourceRef.QueryTargetPID != tc.pid || r.SourceRef.QueryTargetScope != "process" || r.SourceRef.QueryTargetThread != "" {
					t.Fatalf("bad process authority: %+v", r)
				}
				for _, table := range p.Tables {
					if table.MemberSet != nil {
						t.Fatal("display acquired population-completion authority")
					}
					if table.View == types.RuntimeMeasurementMembers && len(table.Rows) != tc.rows {
						t.Fatalf("rows=%d, want %d", len(table.Rows), tc.rows)
					}
				}
				for _, mutate := range []func(*types.ObservationRecord){
					func(r *types.ObservationRecord) { r.SourceRef.Path += ".other" },
					func(r *types.ObservationRecord) { r.SourceRef.QueryWindowEndTs = 3 },
					func(r *types.ObservationRecord) { r.SourceRef.QueryTargetPID = 999 },
					func(r *types.ObservationRecord) { r.SourceRef.QueryTargetThread = "worker" },
					func(r *types.ObservationRecord) { r.Producer = "model" },
					func(r *types.ObservationRecord) {
						r.RichNotes = append(append([]string(nil), r.RichNotes...), r.RichNotes[0])
					},
				} {
					bad := r
					mutate(&bad)
					if _, ok := types.DecodeRuntimeMeasurementPublication(bad); ok {
						t.Fatal("accepted rebound/ambiguous display receipt")
					}
				}
				found = true
			}
			if !found {
				t.Fatal("missing native process observation")
			}
		})
	}
	for _, args := range []string{`{"view":"process_measurements","thread":"worker"}`, `{"view":"process_measurements","pid":100,"target_scope":"thread"}`} {
		out, err := (&TraceQuery{}).Execute(nil, json.RawMessage(args))
		if err != nil || out.Success || out.Repair == nil || out.Repair.Code != "trace_query_process_owned_view" || out.RawRef != "" || len(out.Observations) != 0 {
			t.Fatalf("read before selector repair: %v %+v", err, out)
		}
	}
	if !strings.Contains(string((&TraceQuery{}).Parameters()), tracequery.ProcessMeasurementsTeaching) {
		t.Fatal("schema lost single-source teaching")
	}
}

func TestProcessMeasurementsReceiptBudgetAndEmptyStatus(t *testing.T) {
	integer := func(v int64) tracewire.ProcessMeasureScalar {
		return tracewire.ProcessMeasureScalar{Status: "known", StorageClass: "integer", Value: strconv.FormatInt(v, 10)}
	}
	pid := 100
	p := tracequery.ProcessMeasurementsResult{Status: "available", SourcePath: "/capture.systrace", Window: tracequery.ProcessMeasurementsWindow{StartTs: 1, EndTs: 2}, TargetScope: "process"}
	for i := 0; i < 64; i++ {
		left, right := int64(1e9+i), int64(1e9+i+1)
		rec := tracewire.ProcessMeasureInterval{RowID: int64(i + 1), StartNS: integer(left), DurationNS: integer(1), Value: integer(9007199254740993), FilterID: integer(int64(i)), IPID: integer(1), NameKnown: true, Name: strings.Repeat("指标", 500) + strconv.Itoa(i), OwnerStatus: "known", PID: &pid, ProcessName: "app"}
		p.Rows = append(p.Rows, tracequery.ProcessMeasurementRow{SourcePath: p.SourcePath, Line: i + 1, SourceLine: i + 1, Record: rec, ClippedStartNS: &left, ClippedEndNS: &right, Selection: "interval_overlap", Unit: "unknown"})
	}
	p.TotalRows = len(p.Rows)
	before, _ := json.Marshal(p)
	ref := types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: p.SourcePath, PayloadRef: "payload", QueryScopeID: "scope", QueryWindowKnown: true, QueryWindowStartTs: 1, QueryWindowEndTs: 2, QueryTargetScope: "process"}
	rows := traceQueryProcessMeasurementsObservations(&p, ref, "scope", "now")
	if len(rows) != 1 || len(rows[0].RichNotes[0]) > (64<<10)+len(types.TraceNoteKeyRuntimeMeasurement)+1 {
		t.Fatal("unbounded publication")
	}
	publication, ok := types.DecodeRuntimeMeasurementPublication(rows[0])
	if !ok {
		t.Fatal("bounded receipt invalid")
	}
	kept := len(publication.Tables[1].Rows)
	if kept <= 0 || kept >= len(p.Rows) || len(publication.Tables[2].Rows) != kept {
		t.Fatal("budget did not omit coupled whole rows")
	}
	for i, row := range publication.Tables[1].Rows {
		if row[2] != p.Rows[i].Record.Name || row[5] != "9007199254740993" {
			t.Fatal("budget truncated individual value")
		}
	}
	after, _ := json.Marshal(p)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("receipt changed original query")
	}
	for _, status := range []string{"available", "unavailable"} {
		p.Status, p.Rows, p.TotalRows = status, nil, 0
		rows := traceQueryProcessMeasurementsObservations(&p, ref, "scope", "now")
		if len(rows) != 1 {
			t.Fatal("empty query lost status")
		}
		publication, ok := types.DecodeRuntimeMeasurementPublication(rows[0])
		if !ok || !strings.Contains(strings.Join(publication.Tables[0].Notes, "\n"), "查询状态："+status) {
			t.Fatal("empty and unavailable conflated")
		}
	}
}
