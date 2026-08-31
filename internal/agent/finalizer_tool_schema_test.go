package agent

import (
	"encoding/json"
	"reflect"
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

func TestFinalizerToolSchemas_DocumentDescriptionPublishesProjectedKindRoster(t *testing.T) {
	agent := finalizerSchemaTestAgent()
	sk := finalizerSchemaTestSkill()
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState("call-chain finalizer dispatch"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
			PredicateAxis: types.AxisCall,
		}},
	}
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
	var root map[string]any
	if err := json.Unmarshal(docSchema.Parameters, &root); err != nil {
		t.Fatalf("decode projected schema: %v", err)
	}
	props := root["properties"].(map[string]any)
	blockItems := props["blocks"].(map[string]any)["items"].(map[string]any)
	blockProps := blockItems["properties"].(map[string]any)
	kindEnum := blockProps["kind"].(map[string]any)["enum"].([]any)
	for _, raw := range kindEnum {
		kind := raw.(string)
		if !strings.Contains(docSchema.Description, "`"+kind+"`") {
			t.Fatalf("description omitted live kind %q:\n%s", kind, docSchema.Description)
		}
	}
	if _, diagramExposed := blockProps["diagram"]; !diagramExposed &&
		strings.Contains(strings.Split(docSchema.Description, ". Do not substitute")[0], "`diagram`") {
		t.Fatalf("description advertised diagram in its exact live roster while schema omitted it:\n%s", docSchema.Description)
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
	if !strings.Contains(patchSchema.Description, "`replace_snippets` is only for code snippets") ||
		!strings.Contains(patchSchema.Description, "block items, diagrams, evidence_ids") {
		t.Fatalf("retry patch teaching does not distinguish snippet and block operations:\n%s", patchSchema.Description)
	}
	if strings.Contains(patchSchema.Description, "failure_ref") || strings.Contains(patchSchema.Description, "addition_ref") {
		t.Fatalf("retry dispatch without a live lease must not teach generation-scoped refs:\n%s", patchSchema.Description)
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
	var rawRoot map[string]any
	if err := json.Unmarshal(patchSchema.Parameters, &rawRoot); err != nil {
		t.Fatalf("decode no-lease patch schema: %v", err)
	}
	rawProps := rawRoot["properties"].(map[string]any)
	rawEdgeProps := rawProps["diagram_edge_edits"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	for _, field := range []string{"failure_ref", "addition_ref"} {
		if _, ok := rawEdgeProps[field]; ok {
			t.Fatalf("agent no-lease dispatch leaked generation-scoped field %q", field)
		}
	}
	if _, ok := rawProps["diagram_participant_edits"]; ok {
		t.Fatal("agent no-lease dispatch leaked unavailable participant cleanup")
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
	if _, exposed := rawProps["model_block_order"]; exposed {
		t.Fatal("one-model-block retry must omit the meaningless model_block_order operation")
	}
	for _, field := range []string{"replace_blocks", "add_blocks"} {
		itemProps := paramsSchema.Properties[field].Items.Properties
		for _, required := range []string{"id", "kind", "diagram", "edge_anchors", "participant_boundaries"} {
			if _, ok := itemProps[required]; !ok {
				t.Fatalf("patch %s must expose projected block field %q: %v", field, required, itemProps)
			}
		}
	}
	snippetProps := paramsSchema.Properties["replace_snippets"].Items.Properties
	for _, field := range []string{"file", "start_line", "end_line", "language", "code"} {
		if _, ok := snippetProps[field]; !ok {
			t.Fatalf("replace_snippets schema omitted canonical snippet field %q: %v", field, snippetProps)
		}
	}
	for _, forbidden := range []string{"block_id", "id", "kind", "items", "diagram", "evidence_ids"} {
		if _, ok := snippetProps[forbidden]; ok {
			t.Fatalf("replace_snippets schema leaked answer-block field %q: %v", forbidden, snippetProps)
		}
	}
}

func TestFinalizerToolSchemas_RetryProjectsExactModelOwnedBlockOrderRoster(t *testing.T) {
	agent := finalizerSchemaTestAgent()
	sk := finalizerSchemaTestSkill()
	mut := types.NewMutableState("retry block-order dispatch")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "summary"},
		{ID: "system", Kind: types.BlockCaveat, Text: "system", SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace},
		{ID: "diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n A->>B: call"}},
		{ID: "roster", Kind: types.BlockBulletList, Items: []types.AnswerBlockItem{{ID: "m", Text: "member"}}},
	}})
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
		t.Fatal("accepted previous document must expose patch tool")
	}
	var root map[string]any
	if err := json.Unmarshal(patchSchema.Parameters, &root); err != nil {
		t.Fatalf("decode patch schema: %v", err)
	}
	order := root["properties"].(map[string]any)["model_block_order"].(map[string]any)
	if order["minItems"] != float64(3) || order["maxItems"] != float64(3) {
		t.Fatalf("model order length must be exact: %+v", order)
	}
	got := order["items"].(map[string]any)["enum"].([]any)
	want := []any{"summary", "diagram", "roster"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("model order roster=%v want=%v; system block must stay hidden", got, want)
	}
}

func TestFinalizerToolSchemas_LiveRelationLeaseUsesMatchingExecutableDescriptionAndParameters(t *testing.T) {
	agent := finalizerSchemaTestAgent()
	sk := finalizerSchemaTestSkill()
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "kept"},
		{
			ID: "flow", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramSequence, Language: "mermaid",
				Body: "sequenceDiagram\n A->>B: model wording",
			},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "A", ToNode: "B", FromIdentity: "A.run", ToIdentity: "B.run",
				RelationKind: types.DiagramRelCall, VisibleLabel: "model wording",
			}},
		},
	}}
	lease := types.NewAnswerDiagramRelationRepairLease(doc, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "flow", Issue: "call_edge_unproven", FromNode: "A", ToNode: "B",
		FromIdentity: "A.run", ToIdentity: "B.run", RelationKind: types.DiagramRelCall,
	}}, nil)
	if lease == nil {
		t.Fatal("test setup: expected live relation lease")
	}
	mut := types.NewMutableState("live relation retry")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	mut.SetAnswerDiagramRelationRepairLease(lease)
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
		t.Fatal("live relation retry must expose emit_answer_document_patch")
	}
	if !strings.Contains(patchSchema.Description, "current schema is the sole capability authority") ||
		!strings.Contains(patchSchema.Description, "only unrelated existing blocks") {
		t.Fatalf("agent dispatch leaked the broad compatibility description into a narrow lease: %q", patchSchema.Description)
	}
	var root map[string]any
	if err := json.Unmarshal(patchSchema.Parameters, &root); err != nil {
		t.Fatalf("decode live patch parameters: %v", err)
	}
	props := root["properties"].(map[string]any)
	for _, field := range []string{"add_blocks"} {
		if _, ok := props[field]; ok {
			t.Fatalf("agent dispatch leaked unavailable whole mutation %q", field)
		}
	}
	removeIDs := props["remove_block_ids"].(map[string]any)["items"].(map[string]any)["enum"].([]any)
	if len(removeIDs) != 1 || removeIDs[0] != "summary" {
		t.Fatalf("live relation retry removal roster=%v, want only unrelated summary block", removeIDs)
	}
	replaceBlocks, ok := props["replace_blocks"].(map[string]any)
	if !ok {
		t.Fatal("live relation retry hid replacement of unrelated existing blocks")
	}
	replaceItem := replaceBlocks["items"].(map[string]any)
	replaceProps := replaceItem["properties"].(map[string]any)
	idEnum := replaceProps["id"].(map[string]any)["enum"].([]any)
	if len(idEnum) != 1 || idEnum[0] != "summary" {
		t.Fatalf("live relation retry replacement roster=%v, want only unrelated summary block", idEnum)
	}
	branches := props["diagram_edge_edits"].(map[string]any)["items"].(map[string]any)["oneOf"].([]any)
	if len(branches) != 1 {
		t.Fatalf("agent dispatch must expose only remove for an evidence-negative relation, got %+v", branches)
	}
	branchProps := branches[0].(map[string]any)["properties"].(map[string]any)
	if actions := branchProps["action"].(map[string]any)["enum"].([]any); len(actions) != 1 || actions[0] != "remove" {
		t.Fatalf("evidence-negative lease action roster=%v, want remove-only", actions)
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
