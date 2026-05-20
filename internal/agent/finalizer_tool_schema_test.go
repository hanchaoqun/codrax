package agent

import (
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

func TestFinalizerToolSchemas_DocumentBlocksWarnAgainstStringifiedArrays(t *testing.T) {
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
	for _, want := range []string{
		"native JSON array",
		"do not JSON-encode or quote",
	} {
		if !strings.Contains(docSchema.Description, want) {
			t.Fatalf("emit_answer_document description missing %q:\n%s", want, docSchema.Description)
		}
	}
	params := string(docSchema.Parameters)
	for _, want := range []string{
		"native JSON array",
		"not a JSON-encoded string",
	} {
		if !strings.Contains(params, want) {
			t.Fatalf("emit_answer_document parameters missing %q:\n%s", want, params)
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
