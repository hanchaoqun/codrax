package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNormalizeMisroutedPatchBlockOperationsExpandsExactPartialBlock(t *testing.T) {
	prev := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "chain", Kind: types.BlockOrderedList, Title: "old title",
		Items: []types.AnswerBlockItem{{ID: "old", Label: "kept", CitationRef: 0}},
		RuntimeWorkRelation: &types.AnswerRuntimeWorkRelationReceipt{
			ObservationID: "obs-semantic", Conclusion: types.RuntimeWorkRelationConclusionRelationUnproven,
		},
		ConceptualTerminalResolution: &types.AnswerConceptualTerminalResolutionReceipt{
			EvidenceID: "ev-terminal", Conclusion: types.ConceptualTerminalResolutionDestinationUnproven,
		},
	}}}
	raw := json.RawMessage(`{"replace_snippets":[{"block_id":"chain","title":"new title"}],"unchanged_block_ids":["summary"]}`)

	repaired, ids, violation := normalizeMisroutedPatchBlockOperations(raw, prev)
	if violation != "" || len(ids) != 1 || ids[0] != "chain" {
		t.Fatalf("partial block operation was not remapped: ids=%v violation=%q", ids, violation)
	}
	var envelope struct {
		ReplaceBlocks   []emitAnswerBlockV2 `json:"replace_blocks"`
		ReplaceSnippets []emitCodeSnippetV2 `json:"replace_snippets"`
	}
	if err := json.Unmarshal(repaired, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.ReplaceSnippets) != 0 || len(envelope.ReplaceBlocks) != 1 {
		t.Fatalf("operation remained on snippet carrier: %+v", envelope)
	}
	block := envelope.ReplaceBlocks[0]
	if block.ID != "chain" || block.Kind != string(types.BlockOrderedList) || block.Title != "new title" ||
		len(block.Items) != 1 || block.Items[0].Label != "kept" || block.Items[0].CitationRef == nil || block.Items[0].CitationRef.Int() != 0 ||
		block.RuntimeWorkRelation == nil || block.RuntimeWorkRelation.ObservationID != "obs-semantic" ||
		block.ConceptualTerminalResolution == nil || block.ConceptualTerminalResolution.EvidenceID != "ev-terminal" {
		t.Fatalf("partial expansion did not preserve the exact prior block fields: %+v", block)
	}
}

func TestNormalizeMisroutedPatchBlockOperationsMovesFullBlockByteShape(t *testing.T) {
	prev := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "old"}}}
	raw := json.RawMessage(`{"replace_snippets":[{"id":"summary","kind":"summary","text":"new"}]}`)
	repaired, ids, violation := normalizeMisroutedPatchBlockOperations(raw, prev)
	if violation != "" || len(ids) != 1 || ids[0] != "summary" {
		t.Fatalf("full block was not remapped: ids=%v violation=%q", ids, violation)
	}
	if strings.Contains(string(repaired), `"replace_snippets"`) || !strings.Contains(string(repaired), `"replace_blocks"`) {
		t.Fatalf("full block carrier was not moved: %s", repaired)
	}
}

func TestNormalizeMisroutedPatchBlockOperationsLeavesRealSnippetsAlone(t *testing.T) {
	prev := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "old"}}}
	raw := json.RawMessage(`{"replace_snippets":[{"file":"main.go","start_line":1,"end_line":2,"language":"go","code":"package main"}]}`)
	repaired, ids, violation := normalizeMisroutedPatchBlockOperations(raw, prev)
	if violation != "" || len(ids) != 0 || string(repaired) != string(raw) {
		t.Fatalf("real snippet was reclassified: ids=%v violation=%q repaired=%s", ids, violation, repaired)
	}
}

func TestNormalizeMisroutedPatchBlockOperationsRejectsAmbiguousShapes(t *testing.T) {
	prev := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "old"},
		{ID: "list", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{{Label: "old"}}},
	}}
	cases := []struct {
		name string
		raw  string
	}{
		{"simultaneous replace", `{"replace_blocks":[{"id":"summary","kind":"summary","text":"x"}],"replace_snippets":[{"block_id":"list","items":[{"label":"x"}]}]}`},
		{"mixed snippet field", `{"replace_snippets":[{"block_id":"list","code":"x","items":[{"label":"x"}]}]}`},
		{"mixed selectors", `{"replace_snippets":[{"id":"summary","kind":"summary","text":"x"},{"block_id":"list","items":[{"label":"x"}]}]}`},
		{"unknown nested field", `{"replace_snippets":[{"block_id":"list","items":[{"label":"x","mystery":"lost"}]}]}`},
		{"duplicate target", `{"replace_snippets":[{"block_id":"list","items":[{"label":"x"}]},{"block_id":"list","title":"again"}]}`},
		{"unknown target", `{"replace_snippets":[{"block_id":"missing","items":[{"label":"x"}]}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repaired, ids, violation := normalizeMisroutedPatchBlockOperations(json.RawMessage(tc.raw), prev)
			if violation == "" || len(ids) != 0 || string(repaired) != tc.raw {
				t.Fatalf("ambiguous shape did not fail closed: ids=%v violation=%q repaired=%s", ids, violation, repaired)
			}
		})
	}
}

func TestNormalizeMisroutedPatchBlockOperationsRejectsSystemGeneratedTarget(t *testing.T) {
	prev := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "runtime-trace", Kind: types.BlockSection, Text: "system-owned",
		SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace,
	}}}
	raw := json.RawMessage(`{"replace_snippets":[{"block_id":"runtime-trace","text":"model replacement"}]}`)
	repaired, ids, violation := normalizeMisroutedPatchBlockOperations(raw, prev)
	if violation == "" || len(ids) != 0 || string(repaired) != string(raw) {
		t.Fatalf("system-generated block target did not fail closed: ids=%v violation=%q repaired=%s", ids, violation, repaired)
	}
}

func TestEmitAnswerDocumentPatchRepairsStringWrappedPartialBlockOperation(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "lead"},
			{ID: "chain", Kind: types.BlockOrderedList, Title: "chain", Items: []types.AnswerBlockItem{{ID: "old", Label: "old"}}},
		},
	})
	params := json.RawMessage(`{"replace_snippets":"[{\"block_id\":\"chain\",\"items\":[{\"id\":\"new\",\"label\":\"new\"}]}]","unchanged_block_ids":["summary"]}`)
	result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
	if err != nil || !result.Success {
		t.Fatalf("string-wrapped partial block operation was not recovered: err=%v result=%+v", err, result)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 || len(doc.Blocks[1].Items) != 1 || doc.Blocks[1].Items[0].Label != "new" {
		t.Fatalf("remapped block mutation did not reach the document: %+v", doc)
	}
}

func TestEmitAnswerDocumentPatchRejectsConflictingMisroutedBlockOperation(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "old"}},
	})
	params := json.RawMessage(`{"replace_blocks":[{"id":"summary","kind":"summary","text":"one"}],"replace_snippets":[{"block_id":"summary","text":"two"}]}`)
	result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
	if err != nil || result.Success || result.Repair == nil || result.Repair.Code != "answer_doc_patch_block_operation_misrouted" {
		t.Fatalf("conflicting block operation did not fail with typed repair: err=%v result=%+v", err, result)
	}
	if got := bus.Mutable.AnswerDocumentV2(); got == nil || got.Blocks[0].Text != "old" {
		t.Fatalf("rejected conflict mutated the accepted document: %+v", got)
	}
}
