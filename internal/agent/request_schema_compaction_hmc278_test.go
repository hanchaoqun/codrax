package agent

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC278RealNativeQuerySchemaBudgetAndIdentity(t *testing.T) {
	ctx, path := hmc277MeasurementContext(t, "hmosperf_io_inflight", "window_stats", 1, 1.01)
	// Equal values do not make two independently executed query identities one.
	first := ctx.Mutable.TurnAArtifacts().ToolResults
	raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "platform": "harmonyos", "time_start": 1, "time_end": 1.01})
	second, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
	if err != nil || !second.Success {
		t.Fatalf("second native query: %v %s", err, second.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: append(first, second)})
	before, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
	projection := hmc218CausalProjection(t, ctx)
	ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "Preserved accepted answer"}}})
	reg := tool.NewRegistry()
	reg.Register(&tool.EmitAnswerDocument{})
	reg.Register(&tool.EmitAnswerDocumentPatch{})
	base := NewBaseAgent(types.AgentFinalizer, &Dependencies{Tools: reg}, nil)
	canonical := base.buildToolSchemas(&skill.Config{ToolSuggestions: []string{"emit_answer_document", "emit_answer_document_patch"}}, ctx)
	wire := llm.CompactToolSchemas(canonical)
	if len(wire) != 2 || llm.EstimateToolSchemasBytes(wire) >= llm.EstimateToolSchemasBytes(canonical)*3/4 {
		t.Fatalf("real repair catalog not compact: tools=%d, bytes %d => %d", len(wire), llm.EstimateToolSchemasBytes(canonical), llm.EstimateToolSchemasBytes(wire))
	}
	decode := func(raw []byte) map[string]any {
		var result map[string]any
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	for i, projected := range wire {
		root := decode(projected.Parameters)
		defs, _ := root["$defs"].(map[string]any)
		delete(root, "$defs")
		var expand func(any) any
		expand = func(v any) any {
			switch n := v.(type) {
			case map[string]any:
				if ref, ok := n["$ref"].(string); ok && len(n) == 1 && strings.HasPrefix(ref, "#/$defs/") {
					value, exists := defs[strings.TrimPrefix(ref, "#/$defs/")]
					if !exists {
						t.Fatalf("dangling ref %s", ref)
					}
					return expand(value)
				}
				for k, child := range n {
					n[k] = expand(child)
				}
			case []any:
				for i, child := range n {
					n[i] = expand(child)
				}
			}
			return v
		}
		if !reflect.DeepEqual(expand(root), decode(canonical[i].Parameters)) {
			t.Fatalf("%s lost exact identity or constraints", projected.Name)
		}
		t.Logf("%s parameters %d => %d bytes", projected.Name, len(canonical[i].Parameters), len(projected.Parameters))
	}
	after, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
	if !bytes.Equal(before, after) || !bytes.Equal(projection, hmc218CausalProjection(t, ctx)) {
		t.Fatal("transport changed original facts, queries or causal projection")
	}
}
