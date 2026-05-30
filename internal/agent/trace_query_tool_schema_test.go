package agent

import (
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
}

func hasToolSchema(schemas []llm.ToolSchema, name string) bool {
	for _, schema := range schemas {
		if schema.Name == name {
			return true
		}
	}
	return false
}
