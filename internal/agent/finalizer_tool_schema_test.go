package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestFinalizerToolSchemas_HidePatchWithoutPatchBase(t *testing.T) {
	agent := finalizerSchemaTestAgent()
	sk := finalizerSchemaTestSkill()
	ctx := &types.AgentContext{Mutable: types.NewMutableState("first finalizer dispatch")}

	names := finalizerSchemaToolNames(agent.buildToolSchemas(sk, ctx))
	if !names["emit_answer_document"] {
		t.Fatalf("finalizer must still expose full emit tool, got %v", names)
	}
	if names["emit_answer_document_patch"] {
		t.Fatalf("patch tool must be hidden until a successful previous answer document exists, got %v", names)
	}
}

func TestFinalizerToolSchemas_ExposePatchWithRetryBase(t *testing.T) {
	agent := finalizerSchemaTestAgent()
	sk := finalizerSchemaTestSkill()
	mut := types.NewMutableState("retry finalizer dispatch")
	mut.SetRetryState(&types.RetryState{
		Attempt:      1,
		PrevEmitJSON: []byte(`{"blocks":[{"id":"s1","kind":"summary","text":"kept"}]}`),
	})
	ctx := &types.AgentContext{Mutable: mut}

	names := finalizerSchemaToolNames(agent.buildToolSchemas(sk, ctx))
	if !names["emit_answer_document"] || !names["emit_answer_document_patch"] {
		t.Fatalf("retry with patch base should expose both full emit and patch tools, got %v", names)
	}
}

func TestFinalizerToolSchemas_HidePatchWithInvalidRetryBase(t *testing.T) {
	agent := finalizerSchemaTestAgent()
	sk := finalizerSchemaTestSkill()
	mut := types.NewMutableState("retry finalizer dispatch")
	mut.SetRetryState(&types.RetryState{
		Attempt:      1,
		PrevEmitJSON: []byte(`{"blocks":[]}`),
	})
	ctx := &types.AgentContext{Mutable: mut}

	names := finalizerSchemaToolNames(agent.buildToolSchemas(sk, ctx))
	if names["emit_answer_document_patch"] {
		t.Fatalf("patch tool must be hidden when retry state lacks a usable previous document, got %v", names)
	}
}

func TestFinalizerToolSchemas_DocumentBlocksUseOneSchemaNearCarrierTeaching(t *testing.T) {
	agent := finalizerSchemaTestAgent()
	sk := finalizerSchemaTestSkill()
	ctx := &types.AgentContext{Mutable: types.NewMutableState("first finalizer dispatch")}

	var docSchema *llm.ToolSchema
	for _, schema := range agent.buildToolSchemas(sk, ctx) {
		if schema.Name == "emit_answer_document" {
			s := schema
			docSchema = &s
			break
		}
	}
	if docSchema == nil {
		t.Fatal("finalizer must expose emit_answer_document")
	}
	if strings.Count(docSchema.Description, types.AnswerDocumentJSONShapeFirstTeaching) != 1 {
		t.Fatalf("tool description must carry the canonical JSON teaching exactly once:\n%s", docSchema.Description)
	}
	if !strings.Contains(docSchema.Description, "projected tool schema as the only authority") {
		t.Fatalf("tool description must assign field ownership to the projected schema:\n%s", docSchema.Description)
	}
	var paramsSchema struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(docSchema.Parameters, &paramsSchema); err != nil {
		t.Fatalf("decode projected schema: %v", err)
	}
	if paramsSchema.Properties["blocks"].Type != "array" {
		t.Fatalf("projected schema must make blocks an array structurally: %+v", paramsSchema.Properties["blocks"])
	}
	params := string(docSchema.Parameters)
	for _, duplicateCarrierTeaching := range []string{"native JSON array", "JSON-encoded string"} {
		if strings.Contains(params, duplicateCarrierTeaching) {
			t.Fatalf("parameter schema must express array shape structurally, not repeat prose %q:\n%s", duplicateCarrierTeaching, params)
		}
	}
}

func TestFinalizerToolSchemas_RetryPatchKeepsJSONShapeStructuralAndNonContradictory(t *testing.T) {
	agent := finalizerSchemaTestAgent()
	sk := finalizerSchemaTestSkill()
	mut := types.NewMutableState("retry finalizer dispatch")
	mut.SetRetryState(&types.RetryState{
		Attempt:      1,
		PrevEmitJSON: []byte(`{"blocks":[{"id":"s1","kind":"summary","text":"kept"}]}`),
	})
	ctx := &types.AgentContext{Mutable: mut}

	var patchSchema *llm.ToolSchema
	for _, schema := range agent.buildToolSchemas(sk, ctx) {
		if schema.Name == "emit_answer_document_patch" {
			s := schema
			patchSchema = &s
			break
		}
	}
	if patchSchema == nil {
		t.Fatal("retry dispatch must expose emit_answer_document_patch")
	}
	// The full tool owns the one compact carrier teaching. The patch tool's
	// projected function schema must express its delta containers structurally,
	// not copy another prose schema that can drift or contradict the full tool.
	for _, duplicate := range []string{
		types.AnswerDocumentJSONShapeFirstTeaching,
		"native JSON array",
		"JSON-encoded string",
	} {
		if strings.Contains(patchSchema.Description, duplicate) {
			t.Fatalf("patch description must not duplicate JSON carrier teaching %q:\n%s", duplicate, patchSchema.Description)
		}
	}
	var paramsSchema struct {
		Properties map[string]struct {
			Type  string `json:"type"`
			Items struct {
				Type       string                     `json:"type"`
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(patchSchema.Parameters, &paramsSchema); err != nil {
		t.Fatalf("decode patch schema: %v", err)
	}
	for _, field := range []string{
		"unchanged_block_ids", "replace_blocks", "add_blocks", "remove_block_ids",
		"replace_citations", "append_citations", "replace_missing_requested_roles",
		"replace_caveats", "replace_snippets",
	} {
		property, ok := paramsSchema.Properties[field]
		if !ok || property.Type != "array" || property.Items.Type == "" {
			t.Fatalf("patch field %q must carry its array/item shape structurally: %+v", field, property)
		}
	}
	for _, field := range []string{"replace_blocks", "add_blocks"} {
		itemProps := paramsSchema.Properties[field].Items.Properties
		for _, required := range []string{"id", "kind", "diagram", "edge_anchors", "participant_boundaries"} {
			if _, ok := itemProps[required]; !ok {
				t.Fatalf("patch %s must expose projected block field %q: %v", field, required, itemProps)
			}
		}
	}
}

func finalizerSchemaTestAgent() *BaseAgent {
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitAnswerDocument{})
	registry.Register(&tool.EmitAnswerDocumentPatch{})
	deps := &Dependencies{Tools: registry, MaxIterations: 1}
	return NewFinalizerAgent(deps).(*BaseAgent)
}

func finalizerSchemaTestSkill() *skill.Config {
	return &skill.Config{
		Name:            "answer-document-skill",
		ToolSuggestions: []string{"emit_answer_document", "emit_answer_document_patch"},
	}
}

func finalizerSchemaToolNames(schemas []llm.ToolSchema) map[string]bool {
	out := make(map[string]bool, len(schemas))
	for _, schema := range schemas {
		out[schema.Name] = true
	}
	return out
}
