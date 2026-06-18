package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryToolSchemaLazyRuntimeExposure(t *testing.T) {
	reg := toolpkg.NewRegistry()
	reg.Register(&toolpkg.TraceQuery{})
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: reg}, nil)
	sk := &skill.Config{Name: "explore-skill", ToolSuggestions: []string{"trace_query"}}

	codeOnly := &types.AgentContext{Stage: types.StageExplore, Objective: "explain internal/tool grep implementation"}
	if hasToolSchema(base.buildToolSchemas(sk, codeOnly), "trace_query") {
		t.Fatal("trace_query must not be exposed for ordinary code-only exploration")
	}

	traceCtx := &types.AgentContext{Stage: types.StageExplore, Objective: "analyze sample.systrace", AttachedHitrace: "app-1 (1) [000] .... 1.0: sched_switch: prev_comm=app prev_pid=1 prev_state=S ==> next_comm=idle next_pid=0"}
	if !hasToolSchema(base.buildToolSchemas(sk, traceCtx), "trace_query") {
		t.Fatal("trace_query should be exposed when an attached trace exists")
	}

	pathCtx := &types.AgentContext{Stage: types.StageExplore, Objective: "analyze record_trace.systrace for wakeup chain"}
	if !hasToolSchema(base.buildToolSchemas(sk, pathCtx), "trace_query") {
		t.Fatal("trace_query should be exposed when the request names an explicit trace artifact path")
	}

	perfTraceCtx := &types.AgentContext{Stage: types.StageExplore, Objective: "analyze capture.perftrace for sampled CPU hotspots"}
	if !hasToolSchema(base.buildToolSchemas(sk, perfTraceCtx), "trace_query") {
		t.Fatal("trace_query should be exposed for explicit perftrace paths")
	}

	bundleCtx := &types.AgentContext{Stage: types.StageExplore, Objective: "analyze capture.tracebundle.json for trace plus perf context"}
	if !hasToolSchema(base.buildToolSchemas(sk, bundleCtx), "trace_query") {
		t.Fatal("trace_query should be exposed for explicit tracebundle paths")
	}

	dir := t.TempDir()
	perfData := filepath.Join(dir, "perf.data")
	if err := os.WriteFile(perfData, []byte("PERFILE2\x00\x00\x00\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	perfDataCtx := &types.AgentContext{Stage: types.StageExplore, RepoRoot: dir, Objective: "analyze ./perf.data for running hotspots"}
	if !hasToolSchema(base.buildToolSchemas(sk, perfDataCtx), "trace_query") {
		t.Fatal("trace_query should be exposed for an explicit existing perf.data path")
	}

	perfDataCodeCtx := &types.AgentContext{Stage: types.StageExplore, Objective: "explain the raw perf.data parser implementation"}
	if hasToolSchema(base.buildToolSchemas(sk, perfDataCodeCtx), "trace_query") {
		t.Fatal("bare perf.data source-code discussion must not expose trace_query")
	}

	simpleperfProto := filepath.Join(dir, "simpleperf_proto_without_suffix")
	if err := os.WriteFile(simpleperfProto, []byte("SIMPLEPERF\x00\x00\x00\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	simpleperfCtx := &types.AgentContext{Stage: types.StageExplore, RepoRoot: dir, Objective: "analyze simpleperf_proto_without_suffix for sampled CPU hotspots"}
	if !hasToolSchema(base.buildToolSchemas(sk, simpleperfCtx), "trace_query") {
		t.Fatal("trace_query should be exposed for an explicit existing suffixless SIMPLEPERF artifact path")
	}
}

func hasToolSchema(schemas []llm.ToolSchema, name string) bool {
	for _, schema := range schemas {
		if schema.Name == name {
			return true
		}
	}
	return false
}

// TestTraceQueryViewTeachingsMatchToolSchemaEnum pins the shared prompt
// teaching table against the tool's view enum: every schema view has exactly
// one teaching row, in the same order, so prompt sites rendered from the
// table can never teach a view the tool rejects (or silently skip one it
// accepts).
func TestTraceQueryViewTeachingsMatchToolSchemaEnum(t *testing.T) {
	var params struct {
		Properties struct {
			View struct {
				Enum []string `json:"enum"`
			} `json:"view"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&toolpkg.TraceQuery{}).Parameters(), &params); err != nil {
		t.Fatalf("unmarshal trace_query parameters: %v", err)
	}
	enum := params.Properties.View.Enum
	if len(enum) == 0 {
		t.Fatal("trace_query schema view enum is empty")
	}
	rows := skill.TraceQueryViewTeachings()
	if len(rows) != len(enum) {
		t.Fatalf("teaching table has %d rows, schema enum has %d views:\ntable=%v\nenum=%v", len(rows), len(enum), rows, enum)
	}
	for i, view := range enum {
		if rows[i].View != view {
			t.Fatalf("teaching table row %d is %q, schema enum has %q — keep the table in schema enum order", i, rows[i].View, view)
		}
	}
}
