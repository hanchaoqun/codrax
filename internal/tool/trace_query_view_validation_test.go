package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryRejectsUnknownViewBeforePublishingEvidence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "view.systrace")
	if err := os.WriteFile(path, []byte(" app-10 (10) [000] .... 1.100000: tracing_mark_write: B|10|work\n app-10 (10) [000] .... 1.200000: tracing_mark_write: E|10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"storage_latency_by_layer", "invented_statistics_view"} {
		t.Run(view, func(t *testing.T) {
			params, _ := json.Marshal(map[string]any{"path": path, "view": view, "time_start": 1, "time_end": 14})
			got, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
			if err != nil {
				t.Fatal(err)
			}
			if got.Success || got.RawRef != "" || len(got.Observations) != 0 || got.EnumerationAuthority != nil || got.TraceEvidenceAuthority != nil {
				t.Fatalf("unknown view published a successful result or evidence: %+v", got)
			}
			if got.Repair == nil || got.Repair.Code != "tool_param_invalid_enum_value" || !reflect.DeepEqual(got.Repair.Fields, []string{"view"}) {
				t.Fatalf("missing precise view repair: %+v", got.Repair)
			}
			if got.Repair.Metadata["value"] != view || got.Repair.Metadata["allowed"] != strings.Join(tracequery.CanonicalViewNames(), ", ") {
				t.Fatalf("repair did not use the engine's closed view universe: %+v", got.Repair)
			}
		})
	}
}

func TestTraceQueryUnknownViewPrecedesBusinessReferenceResolution(t *testing.T) {
	got, err := (&TraceQuery{}).Execute(nil, json.RawMessage(`{"view":"invented_statistics_view","business_span_ref":"expired-ref","time_start":0,"time_end":14}`))
	if err != nil || got.Success || got.Repair == nil || got.Repair.Code != "tool_param_invalid_enum_value" {
		t.Fatalf("unknown enum must be rejected at the parameter boundary, got=%+v err=%v", got, err)
	}
}

func TestTraceQueryViewValidationPreservesDefaultsAliasesAndWindow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "view.systrace")
	if err := os.WriteFile(path, []byte(" app-10 (10) [000] .... 1.100000: tracing_mark_write: B|10|work\n app-10 (10) [000] .... 1.200000: tracing_mark_write: E|10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ view, want string }{
		{"", "event_search"},
		{"  ", "event_search"},
		{"EVENT-SEARCH", "event_search"},
		{"state_churn", "window_stats"},
		{"perf_bundle", "trace_perf_bundle"},
		{"causal_impact", "wakeup_chain"},
		{"frame_bundle", "frame_root_cause_bundle"},
		{"window_sweep", "window_sweep"},
	} {
		t.Run(tc.view, func(t *testing.T) {
			params, _ := json.Marshal(map[string]any{"path": path, "view": tc.view, "time_start": 1, "time_end": 14})
			got, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
			if err != nil || !got.Success || !strings.Contains(got.Summary, "view="+tc.want+" ") || !strings.Contains(got.Summary, "time_start=1.000000 time_end=14.000000") {
				t.Fatalf("valid view/default/explicit window changed: err=%v result=%+v", err, got)
			}
		})
	}
}

func TestTraceQueryUnknownViewDoesNotPrepareOrRegisterAnything(t *testing.T) {
	dir := t.TempDir()
	mutable := types.NewMutableState("inspect the trace")
	mutable.RecordTraceQueryCallWindow(types.TraceQueryCallWindow{TimeStart: 2, TimeEnd: 3})
	beforeWindows := mutable.TraceQueryCallWindows()
	beforeRequest := mutable.RequestModel()
	ctx := &types.BusContext{
		RepoRoot: dir, WorkDir: dir, Mutable: mutable,
		AttachedHitrace: " app-10 (10) [000] .... 1.100000: tracing_mark_write: B|10|work\n",
	}
	params := json.RawMessage(`{"source":"attached_trace","view":"invented_statistics_view","pid":41,"time_start":1,"time_end":14,"business_span_ref":"unknown-ref"}`)
	for i := 0; i < 2; i++ {
		got, err := (&TraceQuery{}).Execute(ctx, params)
		if err != nil || got.Success || got.Repair == nil || got.Repair.Code != "tool_param_invalid_enum_value" {
			t.Fatalf("unknown view was not rejected first: %+v err=%v", got, err)
		}
		if got.ReusedFromRunMemo || got.RawRef != "" || len(got.TraceBusinessSpanRefs) != 0 || len(got.TraceBusinessSpanCandidates) != 0 || got.TraceQuerySourceRead.Path() != "" {
			t.Fatalf("unknown view acquired publication or memo state: %+v", got)
		}
	}
	if !reflect.DeepEqual(beforeWindows, mutable.TraceQueryCallWindows()) || !reflect.DeepEqual(beforeRequest, mutable.RequestModel()) {
		t.Fatal("unknown view seeded a supplement window or runtime target")
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatalf("unknown view materialized an attachment or result: files=%v err=%v", files, err)
	}
}

func TestTraceQueryWindowSweepViewValidationKeepsCancellation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "view.systrace")
	if err := os.WriteFile(path, []byte(" app-10 (10) [000] .... 1.100000: tracing_mark_write: B|10|work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	params, _ := json.Marshal(map[string]any{"path": path, "view": "window_sweep", "time_start": 1, "time_end": 14})
	got, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir, Ctx: canceled}, params)
	if err != nil || got.Success || got.Repair != nil || !strings.Contains(got.Summary, "context canceled") || len(got.Observations) != 0 || got.EnumerationAuthority != nil {
		t.Fatalf("valid streaming view lost its cancellation path: %+v err=%v", got, err)
	}
}

func TestTraceQuerySchemaViewEnumMatchesEngineUniverse(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Enum    []string          `json:"enum"`
			Aliases map[string]string `json:"x-codrax-enum-aliases"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&TraceQuery{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	got := schema.Properties["view"].Enum
	sort.Strings(got)
	if !reflect.DeepEqual(got, tracequery.CanonicalViewNames()) {
		t.Fatalf("tool view schema drifted from engine universe: %v", got)
	}
	for alias, canonical := range schema.Properties["view"].Aliases {
		raw, _ := json.Marshal(map[string]string{"view": alias})
		normalized := applyStructuredPayloadCompat("trace_query", raw, (&TraceQuery{}).Parameters())
		var p traceQueryParams
		if err := json.Unmarshal(normalized, &p); err != nil || p.View != canonical || tracequery.ValidateViewName(p.View) != nil {
			t.Errorf("documented alias %q no longer reaches supported %q: view=%q err=%v", alias, canonical, p.View, err)
		}
	}
}
