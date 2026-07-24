package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryCursorTargetKindFollowsExplicitScopeOnly(t *testing.T) {
	// CURSORKIND (NW-02 残余①, 2026-07-24): schema 明文裸 pid=exact thread
	// TID(target_scope 缺省=thread),cursor 车道不得默认铸 process kind——
	// Batch1 起 Kind 承载 scope 进 frame bundle,pid-only 默认 process 会把
	// exact-TID 调查静默升级成 process scope(fail-closed 假 unproven)。
	for _, tc := range []struct {
		blob string
		want types.RuntimeTargetKind
	}{
		{`{"pid":200}`, types.RuntimeTargetKindThread},
		{`{"pid":200,"thread":"worker"}`, types.RuntimeTargetKindThread},
		{`{"pid":200,"target_scope":"process"}`, types.RuntimeTargetKindProcess},
		{`{"pid":200,"thread":"worker","target_scope":"process"}`, types.RuntimeTargetKindProcess},
	} {
		ctx := &types.BusContext{
			Mutable:    types.NewMutableState("q"),
			AnalysisIR: &types.AnalysisIR{},
		}
		var p traceQueryParams
		if err := json.Unmarshal([]byte(tc.blob), &p); err != nil {
			t.Fatalf("params %s: %v", tc.blob, err)
		}
		traceQueryRecordExplicitRuntimeTarget(ctx, p)
		targets := ctx.AnalysisIR.RequestModel.RuntimeTargets
		if len(targets) != 1 {
			t.Fatalf("params %s must record exactly one cursor target, got %+v", tc.blob, targets)
		}
		if targets[0].Kind != tc.want {
			t.Fatalf("params %s recorded kind=%q, want %q", tc.blob, targets[0].Kind, tc.want)
		}
	}
}

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

func TestTraceQueryRejectsInvalidTargetScopeInsteadOfNormalizingToThread(t *testing.T) {
	result, err := (&TraceQuery{}).Execute(nil, json.RawMessage(
		`{"view":"frame_root_cause_bundle","pid":100,"target_scope":"application"}`,
	))
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}
	if result.Success ||
		!strings.Contains(result.Summary, `invalid target_scope="application"`) ||
		!strings.Contains(result.Summary, "thread or process") {
		t.Fatalf("invalid scope must fail closed at the tool boundary: %+v", result)
	}
}
