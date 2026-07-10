package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryTraceMarkActionsSchemaAndExactExecution(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Type  string `json:"type"`
			Items struct {
				Type string   `json:"type"`
				Enum []string `json:"enum"`
			} `json:"items"`
			Split bool `json:"x-codrax-split-string-array"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&TraceQuery{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	actionSchema, ok := schema.Properties["trace_mark_actions"]
	if !ok || actionSchema.Type != "array" || actionSchema.Items.Type != "string" || !actionSchema.Split || !reflect.DeepEqual(actionSchema.Items.Enum, tracequery.TraceMarkActionNames()) {
		t.Fatalf("trace_mark_actions schema drift: %+v", actionSchema)
	}

	dir := t.TempDir()
	tracePath := filepath.Join(dir, "action-filter.systrace")
	trace := strings.Join([]string{
		` app-10 (10) [000] .... 1.000000: tracing_mark_write: C|10|counter|1|S|10|`,
		` app-10 (10) [000] .... 1.001000: tracing_mark_write: S|10|first-async|7`,
		` app-10 (10) [000] .... 1.002000: tracing_mark_write: S|bad|malformed|8`,
		` app-10 (10) [000] .... 1.003000: tracing_mark_write: S|10|second-async|9`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{
		"source":             "path",
		"path":               tracePath,
		"view":               "event_search",
		"trace_mark_actions": "s", // schema enum compat canonicalizes to S
		"time_start":         1.0,
		"time_end":           1.01,
		"limit":              1,
	})
	res, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("exact action query failed: %s", res.Summary)
	}
	for _, want := range []string{"trace_mark_actions=S", "matched_events=1", "event_search_stream_compacted=true; matched 2 row(s)", "S|10|first-async|7", "trace_mark_integrity_degraded=true", "invalid_payload_pid"} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("action result missing %q:\n%s", want, res.Summary)
		}
	}
	for _, forbidden := range []string{"C|10|counter|1|S|10|", "second-async"} {
		if strings.Contains(res.Summary, forbidden) {
			t.Errorf("action result admitted compacted/non-S-prefix row %q:\n%s", forbidden, res.Summary)
		}
	}
	if res.Refinement == nil || res.Refinement.PreferredParams["trace_mark_actions"] != "S" {
		t.Fatalf("refinement did not preserve exact action filter: %+v", res.Refinement)
	}
	for _, field := range res.Refinement.RequiredFields {
		if field == "pattern" || field == "event_types" {
			t.Fatalf("action-filtered event_search must not demand redundant %s: %+v", field, res.Refinement)
		}
	}
	foundActionSuggestion := false
	for _, suggestion := range res.Refinement.ParamNarrowingSuggestions {
		if suggestion.Param == "trace_mark_actions" && suggestion.Suggested == "S" {
			foundActionSuggestion = true
		}
	}
	if !foundActionSuggestion {
		t.Fatalf("refinement narrowing rows lost action filter: %+v", res.Refinement.ParamNarrowingSuggestions)
	}
}

func TestTraceQueryTraceMarkActionsFailLoudBoundaries(t *testing.T) {
	cases := []map[string]any{
		{"view": "event_search", "trace_mark_actions": []string{"X"}},
		{"view": "event_search", "trace_mark_actions": []string{"S", "S"}},
		{"view": "window_stats", "trace_mark_actions": []string{"S"}},
		{"view": "event_search", "trace_mark_actions": []string{"S"}, "event_types": []string{"sched_switch"}},
		{"view": "event_search", "trace_mark_actions": []string{"S"}, "event_types": []string{"trace_mark", "sched_switch"}},
	}
	for _, params := range cases {
		raw, _ := json.Marshal(params)
		res, err := (&TraceQuery{}).Execute(&types.BusContext{}, raw)
		if err != nil {
			t.Fatal(err)
		}
		if res.Success || !strings.Contains(res.Summary, "rejected trace_mark_actions") {
			t.Errorf("invalid action shape did not fail loud params=%v result=%+v", params, res)
		}
	}
}
