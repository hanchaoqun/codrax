package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// emit_answer_document_patch_test.go — R16-c2 tool-level lock tests.
// The underlying ApplyPatch logic + 11 reject cases are pinned in
// internal/types/answer_document_v2_patch_test.go; this file pins
// the tool dispatch surface (peek prev emit, decode params,
// merge, write Mutable).

// helper: build a fresh BusContext with a prev emit.
func newPatchTestBusContext() *types.BusContext {
	bus := &types.BusContext{Mutable: &types.MutableState{}}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations:     []types.Citation{{File: "x.go", Line: 10}},
		Blocks: []types.AnswerBlock{
			{
				ID: "s1", Kind: types.BlockSummary,
				SurfaceRole: types.SurfacePrincipal,
				Text:        "summary text",
				FacetIDs:    []string{"current_code_path"},
				ClaimUses: []types.RenderedClaimUse{
					{ClaimForm: types.ClaimDefinitionFact, EvidenceID: "ev1"},
				},
			},
			{
				ID: "list1", Kind: types.BlockOrderedList,
				ClaimUses: []types.RenderedClaimUse{
					{ClaimForm: types.ClaimCallEdge},
				},
				Items: []types.AnswerBlockItem{
					{ID: "i1", Label: "A", CitationRef: 0},
				},
			},
		},
	})
	return bus
}

// TestEmitAnswerDocumentPatch_NoPrevRejects pins the "first
// dispatch must use emit_answer_document" guard.
func TestEmitAnswerDocumentPatch_NoPrevRejects(t *testing.T) {
	bus := &types.BusContext{Mutable: &types.MutableState{}}
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, json.RawMessage(`{"unchanged_block_ids":["x"]}`))
	if res.Success {
		t.Error("no prev emit must reject")
	}
	if !strings.Contains(res.Summary, "no previous emit") {
		t.Errorf("error must name the missing prev: %q", res.Summary)
	}
}

func TestEmitAnswerDocumentPatch_EmptyRetryStatePrevRejects(t *testing.T) {
	mut := types.NewMutableState("retry")
	mut.SetRetryState(&types.RetryState{
		Attempt:      1,
		PrevEmitJSON: []byte(`{"blocks":[]}`),
	})
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, json.RawMessage(`{"unchanged_block_ids":["x"]}`))
	if res.Success {
		t.Error("empty retry-state previous emit must reject")
	}
	if !strings.Contains(res.Summary, "no previous emit") {
		t.Errorf("error must name the missing usable prev: %q", res.Summary)
	}
}

// TestEmitAnswerDocumentPatch_EmptyPatchRejects pins the "every
// retry must declare some change" invariant at the tool layer.
func TestEmitAnswerDocumentPatch_EmptyPatchRejects(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, json.RawMessage(`{}`))
	if res.Success {
		t.Error("empty patch must reject")
	}
}

// TestEmitAnswerDocumentPatch_PureUnchangedAppliesAndPreserves is
// the **R16 load-bearing tool-level test**. LLM declares "all
// blocks unchanged"; tool clones them verbatim; result has every
// typed annotation field preserved.
func TestEmitAnswerDocumentPatch_PureUnchangedAppliesAndPreserves(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	params := json.RawMessage(`{"unchanged_block_ids":["s1","list1"]}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if !res.Success {
		t.Fatalf("apply must succeed; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil {
		t.Fatal("merged doc not written to Mutable")
	}
	if len(doc.Blocks) != 2 {
		t.Errorf("block count = %d, want 2", len(doc.Blocks))
	}
	// Block-level claim_use preservation
	if len(doc.Blocks[0].ClaimUses) == 0 {
		t.Error("s1 ClaimUses dropped by patch (load-bearing R16 invariant)")
	}
	if doc.Blocks[0].FacetIDs == nil || doc.Blocks[0].FacetIDs[0] != "current_code_path" {
		t.Errorf("s1 FacetIDs lost; got %+v", doc.Blocks[0].FacetIDs)
	}
	// list1 block-level claim_uses preservation
	if len(doc.Blocks[1].ClaimUses) != 1 || doc.Blocks[1].ClaimUses[0].ClaimForm != types.ClaimCallEdge {
		t.Errorf("list1 ClaimUses lost on unchanged block; got %+v", doc.Blocks[1].ClaimUses)
	}
}

// TestEmitAnswerDocumentPatch_ReplaceBlock pins the typical retry
// flow: replace one block, leave others untouched.
func TestEmitAnswerDocumentPatch_ReplaceBlock(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	params := json.RawMessage(`{
		"unchanged_block_ids": ["list1"],
		"replace_blocks": [{
			"id": "s1",
			"kind": "summary",
			"surface_role": "principal",
			"text": "fixed summary",
			"facet_ids": ["current_code_path"],
			"claim_uses": [{"claim_form": "definition_fact", "evidence_id": "ev1"}]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if !res.Success {
		t.Fatalf("must succeed; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc.Blocks[0].Text != "fixed summary" {
		t.Errorf("s1 not replaced; got Text=%q", doc.Blocks[0].Text)
	}
	if doc.Blocks[1].ID != "list1" {
		t.Errorf("list1 not preserved at original position")
	}
}

// TestEmitAnswerDocumentPatch_AddNewBlock pins add path through
// tool layer (kind validation + id uniqueness).
func TestEmitAnswerDocumentPatch_AddNewBlock(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	params := json.RawMessage(`{
		"add_blocks": [{
			"id": "diag1",
			"kind": "diagram",
			"diagram": {"kind": "flow", "language": "mermaid", "body": "flowchart TD\nA-->B"}
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if !res.Success {
		t.Fatalf("add must succeed; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if len(doc.Blocks) != 3 {
		t.Errorf("block count after add = %d, want 3", len(doc.Blocks))
	}
	if doc.Blocks[2].ID != "diag1" {
		t.Error("added block at wrong position")
	}
}

func TestEmitAnswerDocumentPatch_NormalizesReplaceNewBlockToAdd(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	params := json.RawMessage(`{
		"replace_blocks": [{
			"id": "diag1",
			"kind": "diagram",
			"facet_ids": ["diagram_spine"],
			"diagram": {"kind": "flow", "language": "mermaid", "body": "flowchart TD\nA-->B"}
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if !res.Success {
		t.Fatalf("new block misfiled under replace_blocks should be normalized to add_blocks; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if len(doc.Blocks) != 3 || doc.Blocks[2].ID != "diag1" {
		t.Fatalf("normalized block should append at tail, got %+v", doc.Blocks)
	}
}

func TestEmitAnswerDocumentPatch_NormalizesAddExistingBlockToReplace(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	params := json.RawMessage(`{
		"add_blocks": [{
			"id": "s1",
			"kind": "summary",
			"surface_role": "principal",
			"text": "fixed summary",
			"facet_ids": ["current_code_path"],
			"claim_uses": [{"claim_form": "definition_fact", "evidence_id": "ev1"}]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if !res.Success {
		t.Fatalf("existing block misfiled under add_blocks should be normalized to replace_blocks; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if len(doc.Blocks) != 2 || doc.Blocks[0].ID != "s1" || doc.Blocks[0].Text != "fixed summary" {
		t.Fatalf("normalized existing block should replace in place, got %+v", doc.Blocks)
	}
}

// TestEmitAnswerDocumentPatch_RecoverFromRetryState confirms the
// fallback path: when AnswerDocumentV2 was cleared (e.g. by
// ResetForFallback), the tool falls back to RetryState.PrevEmitJSON.
func TestEmitAnswerDocumentPatch_RecoverFromRetryState(t *testing.T) {
	bus := &types.BusContext{Mutable: &types.MutableState{}}
	prevDoc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "from retry state"},
		},
	}
	prevJSON, _ := json.Marshal(prevDoc)
	bus.Mutable.SetRetryState(&types.RetryState{
		Attempt:      1,
		PrevEmitJSON: prevJSON,
	})
	// AnswerDocumentV2 NOT set on Mutable — must fall back.
	tool := &EmitAnswerDocumentPatch{}
	params := json.RawMessage(`{"unchanged_block_ids":["s1"]}`)
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("must recover from RetryState.PrevEmitJSON; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 || doc.Blocks[0].ID != "s1" {
		t.Errorf("recovery + apply broken; got %+v", doc)
	}
}

// TestEmitAnswerDocumentPatch_RejectsInvalidKind covers tool-layer
// kind validation (mirrors the V2 emit gate).
func TestEmitAnswerDocumentPatch_RejectsInvalidKind(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	params := json.RawMessage(`{"add_blocks":[{"id":"new1","kind":"bogus_kind"}]}`)
	res, _ := tool.Execute(bus, params)
	if res.Success {
		t.Error("invalid kind must reject at tool layer")
	}
}

// TestEmitAnswerDocumentPatch_DiagramRequiresPayload covers the
// diagram-kind sanity check at tool layer.
func TestEmitAnswerDocumentPatch_DiagramRequiresPayload(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	// kind=diagram with no diagram payload
	params := json.RawMessage(`{"add_blocks":[{"id":"d1","kind":"diagram"}]}`)
	res, _ := tool.Execute(bus, params)
	if res.Success {
		t.Error("kind=diagram without payload must reject")
	}
}

// TestEmitAnswerDocumentPatch_ToolMetadata pins Name + Description
// presence + Schema validity.
func TestEmitAnswerDocumentPatch_ToolMetadata(t *testing.T) {
	tool := &EmitAnswerDocumentPatch{}
	if tool.Name() != "emit_answer_document_patch" {
		t.Errorf("Name = %q", tool.Name())
	}
	desc := tool.Description()
	for _, want := range []string{
		"DELTA",
		"unchanged_block_ids",
		"replace_blocks",
		"add_blocks",
		"remove_block_ids",
	} {
		if !strings.Contains(desc, want) {
			t.Errorf("Description missing %q", want)
		}
	}
	var schemaObj map[string]interface{}
	if err := json.Unmarshal(tool.Parameters(), &schemaObj); err != nil {
		t.Errorf("Schema not valid JSON: %v", err)
	}
}

// TestEmitAnswerDocumentPatch_StringWrappedAddBlocks confirms the
// flat-mode tolerance fix-up: when the LLM stringifies the
// `add_blocks` array (same MiniMax streaming bug that hits the full
// emit's `blocks` field), the patch tool now silently re-parses
// instead of forcing an LLM retry.
func TestEmitAnswerDocumentPatch_StringWrappedAddBlocks(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	// Seed a previous emit so the patch has something to apply against.
	prev := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
		},
	}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)

	// Stringified add_blocks — exact MiniMax bug shape.
	params := json.RawMessage(`{"add_blocks": "[{\"id\":\"sec_extra\",\"kind\":\"section\",\"title\":\"More\",\"text\":\"detail\",\"surface_role\":\"principal\",\"claim_uses\":[{\"claim_form\":\"definition_fact\"}]}]","unchanged_block_ids":["s1"]}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must auto-repair stringified add_blocks; got Success=false: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil {
		t.Fatal("repaired patch did not write merged doc")
	}
	if len(doc.Blocks) != 2 {
		t.Errorf("expected 2 blocks after patch (s1 unchanged + sec_extra added); got %d", len(doc.Blocks))
	}
}

// TestEmitAnswerDocumentPatch_StringWrappedReplaceBlocks confirms
// the same fix-up for replace_blocks (the more common patch field).
func TestEmitAnswerDocumentPatch_StringWrappedReplaceBlocks(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	prev := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "old summary"},
		},
	}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)

	params := json.RawMessage(`{"replace_blocks": "[{\"id\":\"s1\",\"kind\":\"summary\",\"text\":\"new summary\"}]"}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must auto-repair stringified replace_blocks; got Success=false: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) == 0 || doc.Blocks[0].Text != "new summary" {
		t.Errorf("repaired replace_blocks did not apply; got %+v", doc)
	}
}

func TestEmitAnswerDocumentPatch_StringWrappedReplaceCitationsWithExtraCloser(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
		},
		Citations: []types.Citation{{File: "old.go", Line: 1}},
	})
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1"],
		"replace_citations": "[{\"file\":\"internal/agent/sub_explorer.go\",\"line\":31}]]"
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must auto-repair overclosed stringified replace_citations; got Success=false: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Citations) != 1 || doc.Citations[0].File != "internal/agent/sub_explorer.go" || doc.Citations[0].Line != 31 {
		t.Fatalf("repaired replace_citations did not apply; got %+v", doc)
	}
}

func TestEmitAnswerDocumentPatch_StringWrappedNestedDiagramAndExactResolution(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
		},
	})
	params := json.RawMessage(`{
		"remove_block_ids": ["s1"],
		"add_blocks": [{
			"id": "d1",
			"kind": "diagram",
			"diagram": "{\"kind\":\"sequence\",\"language\":\"mermaid\",\"body\":\"sequenceDiagram\\nA->>B: hi\"}",
			"facet_ids": "[\"current_code_path\"]"
		}],
		"replace_exact_resolution": "{\"status\":\"exact_match\",\"anchor\":\"SubExplorer.Name\"}"
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must auto-repair stringified object/nested fields; got Success=false: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || doc.ExactResolution == nil || doc.ExactResolution.Anchor != "SubExplorer.Name" {
		t.Fatalf("replace_exact_resolution not repaired: %+v", doc)
	}
	if len(doc.Blocks) != 1 || doc.Blocks[0].Diagram == nil || len(doc.Blocks[0].FacetIDs) != 1 {
		t.Fatalf("nested diagram/facet_ids not repaired: %+v", doc)
	}
}

func TestEmitAnswerDocumentPatch_DiagramEdgeAnchorsPromotedToBlock(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
		},
	})
	params := json.RawMessage(`{
		"remove_block_ids": ["s1"],
		"add_blocks": [{
			"id": "d1",
			"kind": "diagram",
			"diagram": {
				"kind": "sequence",
				"language": "mermaid",
				"body": "sequenceDiagram\nA->>B: hi",
				"edge_anchors": [{"from_node":"A","to_node":"B","relation_kind":"call"}]
			}
		}]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must promote diagram.edge_anchors to block edge_anchors; got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 || len(doc.Blocks[0].EdgeAnchors) != 1 {
		t.Fatalf("promoted edge anchors not persisted: %+v", doc)
	}
}

func TestEmitAnswerDocumentPatch_RepairsAnnotationCamelCaseShape(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
		},
	})
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1"],
		"add_blocks": [{
			"id": "d1",
			"kind": "diagram",
			"diagram": {"kind": "sequence", "language": "mermaid", "body": "sequenceDiagram\nA->>B: hi"},
			"claim_uses": [{"claimForm": "returnFact", "facetId": "current_code_path"}],
			"edge_anchors": [{"fromNode": "A", "toNode": "B", "relationKind": "Call", "claimForm": "callEdge"}]
		}]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must auto-repair camelCase annotation shape; got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 {
		t.Fatalf("repaired patch did not persist added block: %+v", doc)
	}
	added := doc.Blocks[1]
	if got := added.ClaimUses; len(got) != 1 ||
		got[0].ClaimForm != types.ClaimReturnFact ||
		got[0].FacetID != "current_code_path" {
		t.Fatalf("claim_uses not normalized: %+v", got)
	}
	if got := added.EdgeAnchors; len(got) != 1 ||
		got[0].RelationKind != types.DiagramRelCall ||
		got[0].ClaimForm != types.ClaimCallEdge {
		t.Fatalf("edge_anchors not normalized: %+v", got)
	}
}

func TestEmitAnswerDocumentPatch_StringCitationAndSnippetLineNumbers(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
		},
	})
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1"],
		"append_citations": [{"file":"internal/agent/sub_explorer.go","line":"31","line_end":"33"}],
		"replace_snippets": [{"file":"internal/agent/sub_explorer.go","start_line":"31","end_line":"33","code":"func (s *SubExplorer) Name() string { return \"explorer\" }"}]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must accept string line numbers; got Success=false: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Citations) != 1 || doc.Citations[0].Line != 31 || doc.Citations[0].LineEnd != 33 {
		t.Fatalf("append_citations line numbers not parsed: %+v", doc)
	}
	if len(doc.Snippets) != 1 || doc.Snippets[0].StartLine != 31 || doc.Snippets[0].EndLine != 33 {
		t.Fatalf("replace_snippets line numbers not parsed: %+v", doc.Snippets)
	}
}
