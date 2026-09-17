package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryJankEventFieldFiltersPublicExecution(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jank.systrace")
	trace := strings.Join([]string{
		`emitter-100 (100) [000] .... 34579.591000123: print: B|27599|jank_event_sync: start_ts=25175823383662, end_ts=25175970920781, jank_frames=1, appid=27599`,
		`emitter-200 (200) [000] .... 34579.592000123: print: B|27599|jank_event_sync: start_ts=25175823383662, end_ts=25175970920781, jank_frames=2, appid=27599`,
		`emitter-300 (300) [000] .... 34579.593000123: print: B|27599|jank_event_sync: start_ts=9007199254740993, end_ts=9007199254740995, jank_frames=8, appid=27599`,
		`emitter-300 (300) [000] .... 34579.594000123: print: B|42|jank_event_sync: start_ts=25175823383662, end_ts=25175970920781, jank_frames=8, appid=42`,
		`emitter-300 (300) [000] .... 34579.595000123: print: B|27599|jank_event_sync: start_ts=25175823383662, end_ts=25175970920781, jank_frames=bad, appid=27599`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeTargets: []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 777, Source: "user_explicit", Confidence: 1}},
	}}}
	params := json.RawMessage(`{"source":"path","path":"jank.systrace","view":"event_search","event_field_filters":[{"field":"jank_frames","op":"gte","value":"2"},{"field":"appid","op":"eq","value":27599}],"limit":1}`)
	result, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !result.Success {
		t.Fatalf("typed jank query failed: %v %+v", err, result)
	}
	for _, want := range []string{"matched_total=2", "emitted=1", "jank_frames=2", "event_field_filters=", "trace_query_target_inheritance_skipped=event_field_filters"} {
		if !strings.Contains(result.Summary, want) {
			t.Errorf("missing %q:\n%s", want, result.Summary)
		}
	}
	if result.Refinement == nil || !strings.Contains(result.Refinement.PreferredParams["event_field_filters"], `"appid"`) {
		t.Fatalf("continuation lost typed conditions: %+v", result.Refinement)
	}
	for _, field := range result.Refinement.RequiredFields {
		if field == "pattern" || field == "event_types" {
			t.Errorf("redundant required selector %s", field)
		}
	}
	// No appid-to-header-PID promotion: explicit emitter selectors still AND.
	explicit := json.RawMessage(`{"source":"path","path":"jank.systrace","view":"event_search","pid":300,"event_field_filters":[{"field":"jank_frames","op":"gte","value":2},{"field":"appid","op":"eq","value":"27599"},{"field":"start_ts","op":"eq","value":9007199254740993}]}`)
	result, err = (&TraceQuery{}).Execute(ctx, explicit)
	if err != nil || !result.Success || !strings.Contains(result.Summary, "matched_total=1") || !strings.Contains(result.Summary, "9007199254740993") {
		t.Fatalf("exact native integer/emitter query failed: %v %+v", err, result)
	}
	if strings.Contains(result.Summary, "trace_query_target_inheritance_skipped=event_field_filters") {
		t.Fatal("explicit emitter filter was ignored")
	}
}

func TestTraceQueryJankEventFieldFiltersRejectInvalidContracts(t *testing.T) {
	for _, payload := range []string{
		`{"view":"window_stats","event_field_filters":[{"field":"jank_frames","op":"gte","value":2}]}`,
		`{"view":"event_search","event_field_filters":[{"field":"invented","op":"gte","value":2}]}`,
		`{"view":"event_search","event_field_filters":[{"field":"jank_frames","op":"regex","value":2}]}`,
		`{"view":"event_search","event_field_filters":[{"field":"jank_frames","op":"gte"}]}`,
		`{"view":"event_search","event_field_filters":[{"field":"jank_frames","op":"gte","value":2.5}]}`,
		`{"view":"event_search","event_field_filters":[{"field":"start_ts","op":"eq","value":9223372036854775808}]}`,
	} {
		result, err := (&TraceQuery{}).Execute(&types.BusContext{}, json.RawMessage(payload))
		if result.Success || !strings.Contains(result.Summary, "event_field_filters") {
			t.Errorf("invalid contract not precisely rejected: %s: %v %+v", payload, err, result)
		}
	}
}

func TestTraceQueryJankSchemaAndTeachingSingleSource(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			MaxItems    int    `json:"maxItems"`
			Description string `json:"description"`
			Items       struct {
				AdditionalProperties bool     `json:"additionalProperties"`
				Required             []string `json:"required"`
				Properties           map[string]struct {
					Enum []string `json:"enum"`
				} `json:"properties"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&TraceQuery{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	field, ok := schema.Properties["event_field_filters"]
	if !ok || field.MaxItems != tracequery.EventFieldFilterLimit || field.Items.AdditionalProperties ||
		!reflect.DeepEqual(field.Items.Required, []string{"field", "op", "value"}) ||
		!reflect.DeepEqual(field.Items.Properties["field"].Enum, tracequery.EventFieldFilterFields()) ||
		!reflect.DeepEqual(field.Items.Properties["op"].Enum, tracequery.EventFieldFilterOps()) ||
		field.Description != skill.TraceJankQueryContract {
		t.Fatalf("tool schema drifted from engine/teaching: %+v", field)
	}
	registry := skill.NewRegistry()
	skill.RegisterDefaults(registry)
	explorer, err := registry.Get("explore-skill")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, item := range explorer.WorkflowTierB {
		if item.Body == field.Description {
			count++
			if !item.AppliesTo.RequiresTrace {
				t.Fatal("jank teaching must use typed trace selector")
			}
		}
	}
	if count != 1 {
		t.Fatalf("want one shared trace teaching item, got %d", count)
	}
}

func TestTraceQueryJankHeaderWindowEmptyResultAndTypedPattern(t *testing.T) {
	dir := t.TempDir()
	trace := "writer-10 (10) [000] .... 1.000000: print: B|20|jank_event_sync: start_ts=25175823383662, end_ts=25175970920781, jank_frames=2, appid=30\n" +
		"writer-10 (10) [000] .... 2.000000: print: B|20|jank_event_sync: start_ts=25175823383662, end_ts=25175970920781, jank_frames=8, appid=30\n" +
		"writer-10 (10) [000] .... 3.000000: print: B|20|jank_event_sync: start_ts=0, end_ts=1, jank_frames=bad, appid=30\n"
	if err := os.WriteFile(filepath.Join(dir, "jank.systrace"), []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	for _, tt := range []struct{ name, filters, selectors, count string }{
		{"header_window", `[{"field":"jank_frames","op":"gte","value":2}]`, `,"time_start":0.9,"time_end":1.1`, "1"},
		{"native_range", `[{"field":"start_ts","op":"gte","value":"25175823383662"},{"field":"end_ts","op":"lte","value":"25175970920781"}]`, "", "2"},
		{"typed_pattern", `[{"field":"jank_frames","op":"gte","value":2}]`, `,"pattern":"trace_mark"`, "2"},
		{"empty", `[{"field":"jank_frames","op":"gt","value":8}]`, "", "0"},
		// Legacy name matching is intentionally broad, not a thread identity witness.
		{"legacy_thread_name", `[{"field":"jank_frames","op":"gte","value":2}]`, `,"thread":"jank_event_sync"`, "2"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			payload := `{"source":"path","path":"jank.systrace","view":"event_search","event_field_filters":` + tt.filters + tt.selectors + `}`
			result, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(payload))
			if err != nil || !result.Success || !strings.Contains(result.Summary, "matched_total="+tt.count) {
				t.Fatalf("wrong query result: %v %+v", err, result)
			}
			if tt.name == "empty" && !strings.Contains(result.Summary, "jank_event_fields_invalid=true rows=1") {
				t.Fatal("empty query hid malformed marker disclosure")
			}
			if tt.name == "header_window" && !strings.Contains(result.Summary, "start_ts_ns=25175823383662") {
				t.Fatal("header window replaced native payload clock")
			}
		})
	}
}
