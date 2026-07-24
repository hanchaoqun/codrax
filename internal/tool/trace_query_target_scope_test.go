package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceQueryTargetScopeSchemaAndQueryPropagation(t *testing.T) {
	schema := string((&TraceQuery{}).Parameters())
	if !strings.Contains(schema, `"target_scope"`) ||
		!strings.Contains(schema, `"enum":["thread","process"]`) ||
		!strings.Contains(schema, "unknown membership is never guessed") {
		t.Fatalf("target_scope schema contract missing:\n%s", schema)
	}
	q := traceQueryBuildQuery(nil, traceQueryParams{
		View: "frame_root_cause_bundle", PID: FlexInt(100), TargetScope: "process",
	}, "path", "trace.systrace", 1, 2)
	if q.PID != 100 || q.TargetScope != tracequery.TargetScopeProcess {
		t.Fatalf("target scope did not propagate to engine query: %+v", q)
	}
	params := traceQueryRefinementPreferredParams(tracequery.Result{View: q.View}, q, traceQueryParams{}, "path")
	if params["target_scope"] != "process" {
		t.Fatalf("refinement dropped explicit process scope: %+v", params)
	}
}

func TestTraceQueryRejectsUnscopedOrUnsupportedProcessScope(t *testing.T) {
	tool := &TraceQuery{}
	for _, tc := range []struct {
		params string
		want   string
	}{
		{`{"view":"frame_timeline","target_scope":"process"}`, "explicit positive pid=<process_id>"},
		{`{"view":"wakeup_chain","pid":100,"target_scope":"process"}`, "frame/span discovery scope only"},
	} {
		result, err := tool.Execute(nil, json.RawMessage(tc.params))
		if err != nil {
			t.Fatalf("unexpected execution error for %s: %v", tc.params, err)
		}
		if result.Success || !strings.Contains(result.Summary, tc.want) {
			t.Fatalf("process-scope validation drifted for %s: %+v", tc.params, result)
		}
	}
}
