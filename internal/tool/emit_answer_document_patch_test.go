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
	bus.Mutable.SetAnswerDocumentV2(&types.AnswerDocumentV2{
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
				Items: []types.AnswerBlockItem{
					{ID: "i1", Label: "A", CitationRef: 0,
						ClaimUse: &types.RenderedClaimUse{ClaimForm: types.ClaimCallEdge}},
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
	// Item-level claim_use preservation
	if doc.Blocks[1].Items[0].ClaimUse == nil ||
		doc.Blocks[1].Items[0].ClaimUse.ClaimForm != types.ClaimCallEdge {
		t.Error("item-level ClaimUse lost on unchanged block")
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
