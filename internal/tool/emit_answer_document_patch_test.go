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

func TestEmitAnswerDocumentPatch_QuarantinesUnknownSchemaMetadata(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"unchanged_block_ids": ["s1", "list1"],
		"add_blocks": [
			{"id": "scope_note", "kind": "caveat", "text": "Scope note.", "metadata": {"dropped": true}}
		],
		"claim_uses": [{"claim_form": "definition"}],
		"metadata": {"model": "local"}
	}`))
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("unknown patch metadata should be quarantined instead of forcing retry: %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 3 || doc.Blocks[2].ID != "scope_note" {
		t.Fatalf("merged document missing quarantined add block: %+v", doc)
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

func TestEmitAnswerDocumentPatch_NormalizesRemoveThenAddSameExistingBlockToReplace(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	params := json.RawMessage(`{
		"remove_block_ids": ["list1"],
		"add_blocks": [{
			"id": "list1",
			"kind": "ordered_list",
			"claim_uses": [{"claim_form": "call_edge"}],
			"items": [{"id": "i1", "label": "A", "text": "rewritten", "citation_ref": 0}]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if !res.Success {
		t.Fatalf("remove+add same existing block should normalize to replace_blocks; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if len(doc.Blocks) != 2 || doc.Blocks[1].ID != "list1" {
		t.Fatalf("normalized same-id remove+add should replace in place, got %+v", doc.Blocks)
	}
	if got := doc.Blocks[1].Items[0].Text; got != "rewritten" {
		t.Fatalf("replacement payload not applied: %q", got)
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

func TestEmitAnswerDocumentPatch_UsesRejectedDraftAsBase(t *testing.T) {
	bus := &types.BusContext{Mutable: &types.MutableState{}}
	bus.Mutable.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations:     []types.Citation{{File: "x.go", Line: 10}},
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "previous rejected summary"},
			{
				ID:    "list1",
				Kind:  types.BlockOrderedList,
				Items: []types.AnswerBlockItem{{ID: "i1", Label: "A", CitationRef: 0}},
			},
		},
	})
	tool := &EmitAnswerDocumentPatch{}
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1", "list1"],
		"add_blocks": [{"id":"scope_caveat","kind":"caveat","text":"Scope note"}]
	}`)
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch should apply against structurally valid rejected draft; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 3 {
		t.Fatalf("merged document not persisted from rejected-draft patch base: %+v", doc)
	}
	if doc.Blocks[2].Kind != types.BlockCaveat || doc.Blocks[0].Text != "previous rejected summary" {
		t.Fatalf("patch did not preserve base blocks and append caveat: %+v", doc.Blocks)
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

func TestEmitAnswerDocumentPatch_NormalizesReplaceCitationsForPreservedBlocks(t *testing.T) {
	bus := newPatchTestBusContext()
	params := json.RawMessage(`{
		"unchanged_block_ids": ["list1"],
		"replace_blocks": [{
			"id": "s1",
			"kind": "summary",
			"text": "fixed lead",
			"items": [{"id":"lead_cite", "citation_ref": 1}]
		}],
		"replace_citations": [
			{"file":"x.go", "line":10},
			{"file":"z.go", "line":99}
		]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("replace_citations with preserved citation-bearing blocks should normalize, got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Citations) != 2 {
		t.Fatalf("normalized citation pool = %+v, want inherited old + appended new", doc)
	}
	if doc.Citations[0].File != "x.go" || doc.Citations[1].File != "z.go" {
		t.Fatalf("unexpected citation pool after normalization: %+v", doc.Citations)
	}
	if got := doc.Blocks[0].Items[0].CitationRef; got != 1 {
		t.Fatalf("replacement block citation_ref should be remapped to appended citation index 1, got %d", got)
	}
	if got := doc.Blocks[1].Items[0].CitationRef; got != 0 {
		t.Fatalf("preserved block citation_ref should still point at inherited pool index 0, got %d", got)
	}
}

func TestEmitAnswerDocumentPatch_NormalizesCitationRefsBeforePoolRangeGate(t *testing.T) {
	bus := &types.BusContext{
		Mutable: types.NewMutableState("页面分配时序图"),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentTrace},
		},
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: []types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "mm/page_alloc.c",
			LineStart:       4687,
			AnchorKind:      types.AnchorDefinition,
			Subject:         "__alloc_pages_slowpath",
			AnchorSymbol:    "__alloc_pages_slowpath",
			Summary:         "慢速路径主入口。",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "mm/page_alloc.c",
			LineStart:       4782,
			AnchorKind:      types.AnchorCall,
			Subject:         "__alloc_pages_slowpath",
			Object:          "get_page_from_freelist",
			Summary:         "__alloc_pages_slowpath calls get_page_from_freelist.",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "mm/page_alloc.c",
			LineStart:       5226,
			AnchorKind:      types.AnchorCall,
			Object:          "get_page_from_freelist",
			Summary:         "首次分配尝试调用快速路径。",
			GroundingStatus: types.GroundingGrounded,
		},
	}})
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations: []types.Citation{
			{File: "mm/page_alloc.c", Line: 4687},
			{File: "mm/page_alloc.c", Line: 4782},
			{File: "mm/page_alloc.c", Line: 5226},
		},
		Blocks: []types.AnswerBlock{
			{
				ID:          "s1",
				Kind:        types.BlockSummary,
				SurfaceRole: types.SurfacePrincipal,
				Text:        "页面分配先走快速路径，失败后进入慢速路径。",
				FacetIDs:    []string{"current_code_path"},
				ClaimUses:   []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge, FacetID: "current_code_path"}},
			},
			{
				ID:          "hops",
				Kind:        types.BlockOrderedList,
				SurfaceRole: types.SurfacePrincipal,
				FacetIDs:    []string{"current_code_path", "principal_path_edge"},
				ClaimUses:   []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge, FacetID: "principal_path_edge"}},
				Items: []types.AnswerBlockItem{
					{ID: "fast", Label: "get_page_from_freelist (快速路径)", Text: "快速路径核心函数。", CitationRef: 2},
					{ID: "slow", Label: "__alloc_pages_slowpath (慢速路径)", Text: "慢速路径主入口。", CitationRef: 0},
				},
			},
			{
				ID:   "caveat1",
				Kind: types.BlockCaveat,
				Text: "仅基于当前已收集的 mm/page_alloc.c 证据说明快慢路径。",
			},
		},
	})
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1", "caveat1"],
		"replace_blocks": [{
			"id": "hops",
			"kind": "ordered_list",
			"surface_role": "principal",
			"facet_ids": ["current_code_path", "principal_path_edge"],
			"claim_uses": [{"claim_form": "call_edge", "facet_id": "principal_path_edge"}],
			"items": [
				{"id":"fast", "label":"get_page_from_freelist (快速路径)", "text":"快速路径核心函数。", "citation_ref":3},
				{"id":"slow", "label":"__alloc_pages_slowpath (慢速路径)", "text":"慢速路径主入口。", "citation_ref":0},
				{"id":"retry", "label":"get_page_from_freelist (慢速路径重试)", "text":"慢速路径重新尝试 get_page_from_freelist。", "citation_ref":1}
			]
		}]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch path should normalize item citation_ref before citation-pool range reject, got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil {
		t.Fatal("patch path produced nil doc")
	}
	var hops *types.AnswerBlock
	for i := range doc.Blocks {
		if doc.Blocks[i].ID == "hops" {
			hops = &doc.Blocks[i]
			break
		}
	}
	if hops == nil || len(hops.Items) != 3 {
		t.Fatalf("missing normalized hops block: %+v", doc)
	}
	if got := hops.Items[0].CitationRef; got != 2 {
		t.Fatalf("out-of-range fast-path citation_ref should be remapped to existing matching citation index 2, got %d", got)
	}
	if got := hops.Items[2].CitationRef; got != 1 {
		t.Fatalf("row-text decorated retry label should keep the cited slowpath call site, got %d", got)
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

func TestEmitAnswerDocumentPatch_DiagramSingletonEdgeAnchorPromotedAndRepaired(t *testing.T) {
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
				"edge_anchors": {"fromNode":"A","toNode":"B","relationKind":"Call"}
			}
		}]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must promote and repair singleton diagram.edge_anchors; got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 || len(doc.Blocks[0].EdgeAnchors) != 1 {
		t.Fatalf("promoted singleton edge anchor not persisted: %+v", doc)
	}
	if got := doc.Blocks[0].EdgeAnchors[0].RelationKind; got != types.DiagramRelCall {
		t.Fatalf("relation_kind = %q, want call", got)
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

func TestEmitAnswerDocumentPatch_RepairsSingletonArrayShapes(t *testing.T) {
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
			"items": {"id": "i1", "label": "A", "text": "entry"},
			"columns": "Component",
			"facet_ids": "current_code_path",
			"claim_uses": {"claimForm": "definitionFact", "facetId": "current_code_path"},
			"edge_anchors": {"fromNode": "A", "toNode": "B", "relationKind": "Call"}
		}]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must repair singleton array shapes; got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 {
		t.Fatalf("repaired patch did not persist added block: %+v", doc)
	}
	added := doc.Blocks[1]
	if len(added.Items) != 1 || len(added.Columns) != 1 ||
		len(added.FacetIDs) != 1 || len(added.ClaimUses) != 1 ||
		len(added.EdgeAnchors) != 1 {
		t.Fatalf("singleton fields not repaired into arrays: %+v", added)
	}
	if added.ClaimUses[0].ClaimForm != types.ClaimDefinitionFact ||
		added.EdgeAnchors[0].RelationKind != types.DiagramRelCall {
		t.Fatalf("singleton annotation aliases not normalized: %+v / %+v", added.ClaimUses, added.EdgeAnchors)
	}
}

func TestEmitAnswerDocumentPatch_RepairsTopLevelSingletonArrayShapes(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
		},
	})
	params := json.RawMessage(`{
		"unchanged_block_ids": "s1",
		"add_blocks": {"id": "extra", "kind": "summary", "text": "extra", "items": {"id": "c0", "citation_ref": 0}},
		"append_citations": {"file": "a.go", "line": 1},
		"replace_caveats": "bounded scope"
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must repair top-level singleton array shapes; got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 || len(doc.Citations) != 1 || len(doc.Caveats) != 1 {
		t.Fatalf("top-level singleton fields not repaired into arrays: %+v", doc)
	}
}

func TestEmitAnswerDocumentPatch_RepairsSingleFacetIDsInsideClaimUse(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
		},
	})
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1"],
		"add_blocks": [{
			"id": "detail",
			"kind": "section",
			"text": "detail",
			"claim_uses": [{"claim_form": "definition_fact", "facet_ids": ["current_code_path"]}]
		}]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must repair single claim_uses[].facet_ids value; got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 || len(doc.Blocks[1].ClaimUses) != 1 {
		t.Fatalf("repaired patch did not persist claim use: %+v", doc)
	}
	if got := doc.Blocks[1].ClaimUses[0].FacetID; got != "current_code_path" {
		t.Fatalf("facet_id = %q, want current_code_path", got)
	}
}

func TestEmitAnswerDocumentPatch_HoistsItemLevelClaimUses(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
		},
	})
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1"],
		"add_blocks": [{
			"id": "list",
			"kind": "ordered_list",
			"items": [{
				"id": "i1",
				"label": "ContractCheck",
				"text": "checks the final answer contract",
				"claim_use": {"claimForm": "definitionFact", "facetId": "current_code_path"}
			}]
		}]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must hoist item-level claim_use; got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 {
		t.Fatalf("repaired patch did not persist added block: %+v", doc)
	}
	got := doc.Blocks[1].ClaimUses
	if len(got) != 1 || got[0].ClaimForm != types.ClaimDefinitionFact || got[0].FacetID != "current_code_path" {
		t.Fatalf("item-level claim_use was not hoisted and normalized: %+v", got)
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
