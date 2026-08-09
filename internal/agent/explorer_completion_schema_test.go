package agent

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func explorerCompletionSchemaForTest() llm.ToolSchema {
	return llm.ToolSchema{
		Name:       explorerCompletionToolName,
		Parameters: (&tool.EmitInvestigationComplete{}).Parameters(),
	}
}

func explorerSchemaTopLevelPropertyForTest(t *testing.T, schema llm.ToolSchema, property string) bool {
	t.Helper()
	var root struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema.Parameters, &root); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	_, ok := root.Properties[property]
	return ok
}

func explorerTypedRelationAuthorityResultForTest() types.ToolResult {
	return types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{{
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Subject:         "target-7",
			Predicate:       "self_runnable_two_ruler",
			RichNotes: []string{
				types.TraceNoteKeySelfTwoRulerWallEffs + "=3.956,1.193",
				types.TraceNoteKeySelfTwoRulerWallRanks + "=4,13",
				types.TraceNoteKeySelfTwoRulerWallSubtotal + "=5.149",
				types.TraceNoteKeySelfTwoRulerEdgeEffs + "=1.648",
				types.TraceNoteKeySelfTwoRulerEdgeRanks + "=10",
				types.TraceNoteKeySelfTwoRulerEdgeSubtotal + "=1.648",
			},
		}},
	}
}

func TestExplorerCompletionSchemaOmitsRelationClaimsWithoutTypedAuthority(t *testing.T) {
	original := explorerCompletionSchemaForTest()
	originalBytes := append([]byte(nil), original.Parameters...)
	schemas := []llm.ToolSchema{{Name: "read_file"}, original}

	got := (&explorerEvaluator{}).FilterToolSchemas(&types.AgentContext{
		Stage:   types.StageExplore,
		Mutable: types.NewMutableState("schema projection"),
	}, schemas)
	if len(got) != len(schemas) || explorerSchemaTopLevelPropertyForTest(t, got[1], "relation_claims") {
		t.Fatalf("relation_claims must be absent without copyable typed authority")
	}
	if !reflect.DeepEqual([]byte(schemas[1].Parameters), originalBytes) ||
		!explorerSchemaTopLevelPropertyForTest(t, schemas[1], "relation_claims") {
		t.Fatalf("schema projection mutated the shared input schema")
	}
}

func TestExplorerCompletionSchemaKeepsRelationClaimsWithTypedAuthority(t *testing.T) {
	mut := types.NewMutableState("schema projection")
	mut.AppendDispatchToolResult(explorerTypedRelationAuthorityResultForTest())
	schema := explorerCompletionSchemaForTest()

	got := (&explorerEvaluator{}).FilterToolSchemas(&types.AgentContext{
		Stage:   types.StageExplore,
		Mutable: mut,
	}, []llm.ToolSchema{schema})
	if len(got) != 1 || !explorerSchemaTopLevelPropertyForTest(t, got[0], "relation_claims") {
		t.Fatalf("relation_claims must remain available when a copyable typed authority exists")
	}
	if !reflect.DeepEqual([]byte(got[0].Parameters), []byte(schema.Parameters)) {
		t.Fatalf("authority-present schema should remain byte-identical")
	}
}

func TestExplorerCompletionSchemaDoesNotTreatUnrelatedTraceRowAsAuthority(t *testing.T) {
	mut := types.NewMutableState("schema projection")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{{
			Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "root_cause_background",
		}},
	})
	got := (&explorerEvaluator{}).FilterToolSchemas(&types.AgentContext{
		Stage: types.StageExplore, Mutable: mut,
	}, []llm.ToolSchema{explorerCompletionSchemaForTest()})
	if explorerSchemaTopLevelPropertyForTest(t, got[0], "relation_claims") {
		t.Fatalf("an unrelated typed trace row must not mint relation-copy authority")
	}
}

func TestExplorerCompletionSchemaMalformedInputFailsOpen(t *testing.T) {
	schemas := []llm.ToolSchema{{Name: explorerCompletionToolName, Parameters: json.RawMessage(`{"type":`)}}
	got := (&explorerEvaluator{}).FilterToolSchemas(&types.AgentContext{
		Stage: types.StageExplore, Mutable: types.NewMutableState("schema projection"),
	}, schemas)
	if len(got) != 1 || !reflect.DeepEqual([]byte(got[0].Parameters), []byte(schemas[0].Parameters)) {
		t.Fatalf("malformed input schema must fail open without mutation")
	}
}
