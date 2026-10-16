package agent

import (
	"encoding/json"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC249CatalogPlanActualExplorerSchemaAndPublicDispatch(t *testing.T) {
	bus := traceCapabilitiesDiscoveryBus(t, false)
	bus.WorkDir = t.TempDir()
	reg := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(reg)
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: reg}, &stubEvaluator{})
	ctx := ctxbuilder.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
	schemas := base.buildToolSchemas(traceTeachingSkill(t, "explore-skill"), ctx)
	var schema json.RawMessage
	for _, candidate := range schemas {
		if candidate.Name == "trace_catalog" {
			schema = candidate.Parameters
		}
	}
	if len(schema) == 0 {
		t.Fatal("catalog absent from current explorer surface")
	}
	for _, raw := range []string{
		`{"queries":[{"pid":101}]}`,
		`{"queries":[{"view":"resource_stack","target_scope":"all_processes"}]}`,
		`{"queries":[{"view":"wakeup_chain","max_depth":10000}]}`,
		`{"queries":[{"view":"resource_stack","path":"different.data"}]}`,
		`{"queries":[{"catalog_query_ref":{"catalog_id":"prior","query_id":"prior"}}]}`,
	} {
		// This checks the actual schema advertised by BaseAgent, not a
		// replacement runtime policy for schema-bypassed direct Execute.
		if err := toolparam.Validate(json.RawMessage(raw), schema); err == nil {
			t.Fatalf("actual explorer schema accepted invalid plan: %s", raw)
		}
	}
	raw := json.RawMessage(`{"queries":[{"view":"resource_stack","pid":101,"time_start":10,"time_end":10.05}]}`)
	if err := toolparam.Validate(raw, schema); err != nil {
		t.Fatal(err)
	}
	result, mcp := base.executeTool(ctx, llm.ToolCall{Name: "trace_catalog", Params: raw}, toolSchemaNameSet(schemas))
	if mcp != nil || result == nil || !result.Success {
		t.Fatalf("valid advertised plan failed current public dispatch: %+v %+v", result, mcp)
	}
	if len(result.Observations) != 0 || len(ctx.Mutable.TraceCatalogs()) != 1 {
		t.Fatal("planning produced runtime evidence or failed to retain current catalog")
	}
}
