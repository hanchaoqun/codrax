package agent

import (
	"encoding/json"
	"errors"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Keep the actual source-literal emission between reading historical navigation
// and resuming it. This is the path the direct read->resume HMC-247 case omits.
func TestHMC252CheckpointReadEnvelopeActualExplorer(t *testing.T) {
	bus, _, _ := checkpointPreRead250(t)
	bus.AnalysisIR = nil // Exercise explicit read, not automatic pre-read.
	ctx := ctxbuilder.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
	stop := errors.New("bounded checkpoint envelope inspection finished")
	call := func(name string, params any) (llm.Response, error) {
		raw, err := json.Marshal(params)
		if err != nil {
			t.Fatal(err)
		}
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: name, Name: name, Params: raw}}, StopReason: "tool_use"}, nil
	}
	model := &toolSurfaceTransitionLLM247{}
	model.step = func(i int, _ []llm.Message, schemas []llm.ToolSchema) (llm.Response, error) {
		switch i {
		case 0:
			return call("read_file", map[string]string{"path": "checkpoint.json"})
		case 1:
			return call("emit_evidence", map[string]any{"items": []map[string]any{{
				"source": "checkpoint.json", "line_start": 2, "scope": "line", "evidence_kind": "direct",
				"anchor_kind": "string_literal", "anchor_symbol": "schema_version", "subject": "schema_version",
				"summary": "The saved navigation file declares its schema_version field; this is not current query execution.",
			}}})
		case 2:
			items := ctx.Mutable.EmittedEvidence()
			if len(items) != 1 || items[0].GroundingStatus != types.GroundingGrounded || items[0].GroundingTier != types.TierLineText {
				results := ctx.Mutable.DispatchToolResults()
				t.Fatalf("actual emit lost the read gutter: %+v last=%+v", items, results[len(results)-1])
			}
			if !hasToolSchema(schemas, "trace_catalog") {
				t.Fatal("valid source-literal emission stranded the admitted resume entrance")
			}
			if hasToolSchema(schemas, "trace_query") || len(ctx.Mutable.TraceCatalogs()) != 0 {
				t.Fatal("historical read/source literal created native execution authority")
			}
			for _, r := range ctx.Mutable.DispatchToolResults() {
				if r.ToolName == "read_file" && r.Refinement != nil && r.Refinement.ReasonCode == "historical_catalog_navigation" {
					return call("trace_catalog", r.Refinement.PreferredParams)
				}
			}
			t.Fatal("actual read lost typed resume navigation")
		case 3:
			if !hasToolSchema(schemas, "trace_query") {
				t.Fatal("actual resume did not expose admitted native query")
			}
			catalogs := ctx.Mutable.TraceCatalogs()
			if len(catalogs) != 1 {
				t.Fatal("actual resume did not install exactly one live catalog")
			}
			for _, q := range catalogs[0].Snapshot().Queries {
				if q.Outcome != tracecatalog.OutcomeNotExecuted || len(q.Attempts) != 0 {
					t.Fatal("historical execution was restored as current")
				}
			}
			results := ctx.Mutable.DispatchToolResults()
			var page struct {
				Calls map[string]json.RawMessage `json:"query_calls"`
			}
			if err := json.Unmarshal([]byte(results[len(results)-1].Summary), &page); err != nil || len(page.Calls) == 0 {
				t.Fatalf("actual resume omitted current calls: %v", err)
			}
			for _, raw := range page.Calls {
				return call("trace_query", raw)
			}
		case 4:
			return llm.Response{}, stop
		}
		return llm.Response{}, errors.New("unexpected model fixture round")
	}
	registry := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(registry)
	registry.Register(&toolpkg.EmitEvidence{})
	registry.Register(&toolpkg.EmitInvestigationComplete{})
	sk := traceTeachingSkill(t, "explore-skill")
	// Match cmd/root.go bootstrap; the defaults omit structured emitters.
	sk.ToolSuggestions = append(sk.ToolSuggestions, "emit_evidence", "emit_investigation_complete")
	_, err := NewExplorerAgent(&Dependencies{LLM: model, Tools: registry, MaxIterations: 5}).Execute(ctx, sk)
	if !errors.Is(err, stop) || model.calls != 5 {
		t.Fatalf("real loop failed before query: calls=%d err=%v", model.calls, err)
	}
	queries := 0
	for _, result := range ctx.Mutable.DispatchToolResults() {
		if result.ToolName == "trace_query" && result.Success {
			queries++
		}
	}
	if queries != 1 {
		t.Fatalf("successful current native queries=%d want=1", queries)
	}
}
