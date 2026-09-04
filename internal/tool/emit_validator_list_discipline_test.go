package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// emit_validator_list_discipline_test.go — V2-4 (§40.51) behavioural pins:
// every emit validator that walks a model list reports EVERY violation in one
// reject (message content only; the accept/reject verdict is unchanged and a
// single violation keeps its historical text byte-identical). Red on the
// 3080934fd tree via `go test -overlay` of the pre-V2-4 executor / runtime /
// full-emit / patch-validator files; green after the collectors landed.

// The patch transaction reject names every independent structural violation
// and the typed repair leads with the precedence row, unions the fields and
// counts the list.
func TestEmitAnswerDocumentPatch_StructureRejectSummaryNamesEveryViolation(t *testing.T) {
	bus := newPatchTestBusContext()
	// Duplicate / unknown replace / citation-mode mistakes are absorbed by the
	// executor's transactional-tolerance normalizers before Apply; the arms
	// that still reach the structure gate are exercised here.
	params, _ := json.Marshal(map[string]any{
		"unchanged_block_ids": []string{"phantom", "ghost"},
		"model_block_order":   []string{"s1"},
	})
	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if res.Success {
		t.Fatalf("structurally invalid patch must be rejected: %+v", res)
	}
	for _, want := range []string{
		"patch: 3 structural violations — fix ALL of them in this one patch: ",
		`[1] patch: unchanged_block_ids["phantom"] not present in previous emit`,
		`[2] patch: unchanged_block_ids["ghost"] not present in previous emit`,
		`[3] patch: model_block_order must list every model-authored previous block exactly once: got 1 ids, want 2`,
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("reject summary must list every violation, missing %q:\n%s", want, res.Summary)
		}
	}
	if res.Repair == nil || res.Repair.Code != types.ToolRepairCodeAnswerDocPatchStructure {
		t.Fatalf("repair must key on the typed kinds (generic structure code), got %+v", res.Repair)
	}
	for _, field := range []string{"unchanged_block_ids", "replace_blocks", "model_block_order"} {
		if !mutationRepairStringSliceContains(res.Repair.Fields, field) {
			t.Fatalf("repair fields must union every violation's fields, missing %q: %+v", field, res.Repair.Fields)
		}
	}
	if res.Repair.Metadata[types.ToolRepairMetaAnswerDocPatchViolationCount] != "3" {
		t.Fatalf("repair metadata must carry the violation count, got %+v", res.Repair.Metadata)
	}
	if !strings.Contains(res.Repair.Hint, "fix ALL of them in one resubmission") {
		t.Fatalf("multi-violation repair hint must teach the one-resubmission rule: %s", res.Repair.Hint)
	}
	// A single violation keeps the historical summary byte-for-byte (the
	// transaction-state prefix is the existing annotate tail).
	single, _ := json.Marshal(map[string]any{"unchanged_block_ids": []string{"phantom"}})
	res, _ = (&EmitAnswerDocumentPatch{}).Execute(newPatchTestBusContext(), single)
	if !strings.HasSuffix(res.Summary, `mutation apply rejected: patch: unchanged_block_ids["phantom"] not present in previous emit`) {
		t.Fatalf("single-violation summary drifted: %s", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != types.ToolRepairCodeAnswerDocPatchStructure || res.Repair.Metadata[types.ToolRepairMetaAnswerDocPatchViolationCount] != "1" {
		t.Fatalf("single structural violation must still carry a typed repair, got %+v", res.Repair)
	}
}

// replace_blocks AND add_blocks normalize errors are listed in one reject.
func TestConvertEmitBlocksToTyped_ListsReplaceAndAddErrorsTogether(t *testing.T) {
	bus := newPatchTestBusContext()
	params := json.RawMessage(`{
		"replace_blocks":[{"id":"s1","kind":"nope","text":"x"}],
		"add_blocks":[{"id":"n1","kind":"nope","text":"y"},{"id":"n2","kind":"nope","text":"z"}]
	}`)
	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if res.Success {
		t.Fatalf("invalid block kinds must be rejected: %+v", res)
	}
	for _, want := range []string{
		"3 block(s) failed validation",
		"[1] emit_answer_document_patch: replace_blocks[0]: kind=\"nope\"",
		"[2] emit_answer_document_patch: add_blocks[0]: kind=\"nope\"",
		"[3] emit_answer_document_patch: add_blocks[1]: kind=\"nope\"",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("reject must list every block error across both lists, missing %q:\n%s", want, res.Summary)
		}
	}
	_, violations := convertEmitBlocksToTyped("t", []emitAnswerBlockV2{{ID: "a", Kind: "nope"}}, "add_blocks")
	if len(violations) != 1 || emitBlockViolationsMessage(violations) != violations[0] {
		t.Fatalf("a single block error must stay byte-identical, got %v", violations)
	}
}

// The full-emit twin lists every blocks[] normalize error at once.
func TestEmitAnswerDocumentV2_BlocksRejectListsEveryInvalidBlock(t *testing.T) {
	bus := newV2TestBusContext()
	res, err := (&EmitAnswerDocument{}).Execute(bus, json.RawMessage(`{"blocks":[
		{"id":"a","kind":"nope","text":"x"},
		{"id":"b","kind":"summary","text":"ok"},
		{"id":"c","kind":"nope","text":"y"}
	]}`))
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if res.Success {
		t.Fatalf("invalid block kinds must be rejected: %+v", res)
	}
	for _, want := range []string{"2 block(s) failed validation", `[1] blocks[0]: kind="nope"`, `[2] blocks[2]: kind="nope"`} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("full-emit reject must list every invalid block, missing %q:\n%s", want, res.Summary)
		}
	}
}

func listDisciplineFieldEditSchema() json.RawMessage {
	return json.RawMessage(`{"properties":{
		"block_field_edits_v1":{"items":{"oneOf":[{"properties":{
			"field":{"const":"surface_role"},"block_id":{"enum":["s1"]},"value":{"enum":["principal"]}}}]}},
		"block_receipt_edits_v1":{"items":{"oneOf":[{"properties":{
			"field":{"const":"runtime_work_relation"},"block_id":{"enum":["s1"]},
			"value":{"oneOf":[{"properties":{"observation_id":{"const":"obs-1"},"conclusion":{"const":"supports"}}}]}}}]}}
	}}`)
}

func TestValidateAnswerDocumentPatchFieldEditsAgainstSchema_ListsEveryIndex(t *testing.T) {
	raw := json.RawMessage(`{"block_field_edits_v1":[
		{"block_id":"s1","field":"nope","value":"principal"},
		{"block_id":"s1","field":"surface_role","value":"principal"},
		{"block_id":"zz","field":"surface_role","value":"principal"},
		{"block_id":"s1","field":"surface_role","value":"bad"}
	]}`)
	violations := validateAnswerDocumentPatchFieldEditsAgainstSchema(raw, listDisciplineFieldEditSchema())
	if len(violations) != 3 {
		t.Fatalf("every failing entry must be listed, got %d: %+v", len(violations), violations)
	}
	wantReasons := []string{"field_not_published", "block_id_not_published", "value_not_published"}
	wantIndexes := []int{0, 2, 3}
	for i, v := range violations {
		if v.Reason != wantReasons[i] || v.Index != wantIndexes[i] {
			t.Fatalf("violation[%d] = %+v, want index=%d reason=%s", i, v, wantIndexes[i], wantReasons[i])
		}
	}
	tail := answerDocumentPatchFieldEditSchemaViolationTail(violations)
	for _, want := range []string{"this patch has 3 such violations", "[1] block_field_edits_v1[0]: reason=field_not_published", "[3] block_field_edits_v1[3]: reason=value_not_published"} {
		if !strings.Contains(tail, want) {
			t.Fatalf("tail missing %q: %s", want, tail)
		}
	}
	if answerDocumentPatchFieldEditSchemaViolationTail(violations[:1]) != "" {
		t.Fatal("a single violation must add no tail (byte-identical summary)")
	}
	repair := answerDocumentPatchFieldEditSchemaRepairAll(violations)
	for _, field := range []string{"block_field_edits_v1[0].field", "block_field_edits_v1[2].field", "block_field_edits_v1[3].field", "block_field_edits_v1[3].value"} {
		if !mutationRepairStringSliceContains(repair.Fields, field) {
			t.Fatalf("repair fields must union every entry, missing %q: %+v", field, repair.Fields)
		}
	}
	if repair.Metadata[types.ToolRepairMetaAnswerDocPatchViolationCount] != "3" {
		t.Fatalf("repair must count the list: %+v", repair.Metadata)
	}
}

func TestValidateAnswerDocumentPatchReceiptEditsAgainstSchema_ListsEveryIndex(t *testing.T) {
	raw := json.RawMessage(`{"block_receipt_edits_v1":[
		{"block_id":"s1","field":"other","value":{"observation_id":"obs-1","conclusion":"supports"}},
		{"block_id":"s1","field":"runtime_work_relation","value":{"observation_id":"obs-1","conclusion":"supports"}},
		{"block_id":"s1","field":"runtime_work_relation","value":{"observation_id":"obs-9","conclusion":"supports"}}
	]}`)
	violations := validateAnswerDocumentPatchReceiptEditsAgainstSchema(raw, listDisciplineFieldEditSchema())
	if len(violations) != 2 || violations[0].Index != 0 || violations[1].Index != 2 || violations[1].Reason != "value_not_published" {
		t.Fatalf("every failing receipt entry must be listed, got %+v", violations)
	}
	if tail := answerDocumentPatchReceiptEditSchemaViolationTail(violations); !strings.Contains(tail, "[2] block_receipt_edits_v1[2]: reason=value_not_published") {
		t.Fatalf("tail must list the second entry: %s", tail)
	}
	if repair := answerDocumentPatchReceiptEditSchemaRepairAll(violations); !mutationRepairStringSliceContains(repair.Fields, "block_receipt_edits_v1[2]") {
		t.Fatalf("repair fields must union every entry: %+v", repair.Fields)
	}
}

func TestSplitCompanionDispositionViolations_ListEveryPair(t *testing.T) {
	prev := &types.AnswerDocumentV2{DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "p1", Kind: types.BlockSection, Text: "one"},
			{ID: "p1-diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "graph TD\nA-->B"}},
			{ID: "p2", Kind: types.BlockSection, Text: "two"},
			{ID: "p2-diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "graph TD\nC-->D"}},
		},
		BlockCompanionLineages: []types.AnswerBlockCompanionLineage{
			{Kind: types.AnswerBlockCompanionLineageFusedDiagramSplit, VisibleBlockID: "p1", DiagramBlockID: "p1-diagram"},
			{Kind: types.AnswerBlockCompanionLineageFusedDiagramSplit, VisibleBlockID: "p2", DiagramBlockID: "p2-diagram"},
		},
	}
	failures := splitCompanionDispositionViolations(prev, &emitAnswerDocumentPatchParams{RemoveBlockIDs: []string{"p1", "p2"}})
	if len(failures) != 2 || failures[0].RemovedBlockID != "p1" || failures[1].RemovedBlockID != "p2" {
		t.Fatalf("every undisposed pair must be listed, got %+v", failures)
	}
	repair := splitCompanionDispositionRepairAll(failures)
	if repair.Metadata["removed_block_ids"] != "p1,p2" || repair.Metadata["companion_block_ids"] != "p1-diagram,p2-diagram" {
		t.Fatalf("repair metadata must list every pair: %+v", repair.Metadata)
	}
	if tail := splitCompanionDispositionViolationTail(failures); !strings.Contains(tail, `[2] removed "p2" without disposing sibling "p2-diagram"`) {
		t.Fatalf("tail must list the second pair: %s", tail)
	}
	if splitCompanionDispositionViolationTail(failures[:1]) != "" {
		t.Fatal("a single pair must add no tail")
	}
}

func TestLocalDiagramLeaseWholeBlockMutationViolationsListEveryOperation(t *testing.T) {
	prev := atomicPatchTestDocument()
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "call_edge_unproven",
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence,
		}}, nil)
	if lease == nil {
		t.Fatal("test setup: expected live local lease")
	}
	violations := localDiagramLeaseWholeBlockMutationViolations(&emitAnswerDocumentPatchParams{
		ReplaceBlocks:       []emitAnswerBlockV2{{ID: "diag"}},
		BlockReceiptEditsV1: []types.AnswerBlockReceiptEditV1{{BlockID: "diag", Field: types.AnswerBlockReceiptFieldRuntimeWorkRelation}},
		RemoveBlockIDs:      []string{"diag"},
	}, lease, prev, nil)
	if len(violations) != 3 ||
		violations[0].Issue != "whole_replace_not_authorized" ||
		violations[1].Issue != "block_receipt_edit_not_authorized" ||
		violations[2].Issue != "whole_remove_not_authorized" {
		t.Fatalf("every unauthorized whole-block operation must be listed in operation order, got %+v", violations)
	}
	if tail := localDiagramLeaseWholeBlockMutationViolationTail(violations); !strings.Contains(tail, `[3] block="diag" operation=whole_remove_not_authorized`) {
		t.Fatalf("tail must list every operation: %s", tail)
	}
}

func TestValidateMergedV2Doc_ListsEveryBlockViolation(t *testing.T) {
	err := validateMergedV2Doc(&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "", Kind: types.BlockSummary, Text: "a"},
		{ID: "d", Kind: types.BlockDiagram},
		{ID: "d", Kind: types.BlockSummary, Text: "dup"},
	}})
	if err == nil {
		t.Fatal("merged doc invariants must reject")
	}
	for _, want := range []string{"merged doc has 3 violations", "[1] merged blocks[0]: id is empty", "[2] merged blocks[1]: kind=diagram requires", `[3] merged doc: duplicate id "d"`} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("merged-doc reject must list every violation, missing %q: %v", want, err)
		}
	}
	single := validateMergedV2Doc(&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "", Kind: types.BlockSummary, Text: "a"}}})
	if single == nil || single.Error() != "merged blocks[0]: id is empty" {
		t.Fatalf("a single violation must stay byte-identical, got %v", single)
	}
}

func TestBindReceipts_ListEveryUnboundBlock(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "a", Kind: types.BlockSummary, RuntimeWorkRelation: &types.AnswerRuntimeWorkRelationReceipt{ObservationID: "x"}},
		{ID: "b", Kind: types.BlockSummary},
		{ID: "c", Kind: types.BlockSummary, RuntimeWorkRelation: &types.AnswerRuntimeWorkRelationReceipt{ObservationID: "y"}},
	}}
	err := bindRuntimeWorkRelationReceipts(doc, nil)
	if err == nil || !strings.Contains(err.Error(), "[1] blocks[0].runtime_work_relation") || !strings.Contains(err.Error(), "[2] blocks[2].runtime_work_relation") {
		t.Fatalf("every unbound receipt must be listed, got %v", err)
	}
}

// The repair table is a closed set over the typed violation kinds.
func TestAnswerDocumentPatchStructureRepairTableCoversEveryKind(t *testing.T) {
	seen := map[types.AnswerDocumentPatchViolationKind]int{}
	for _, row := range answerDocumentPatchStructureRepairTable {
		seen[row.kind]++
		if row.code == "" || row.hint == "" || len(row.fields) == 0 {
			t.Fatalf("row %s is incomplete: %+v", row.kind, row)
		}
	}
	known := map[types.AnswerDocumentPatchViolationKind]bool{}
	for _, kind := range types.AllAnswerDocumentPatchViolationKinds() {
		known[kind] = true
		if seen[kind] != 1 {
			t.Fatalf("kind %s must have exactly one repair row, found %d", kind, seen[kind])
		}
	}
	for kind := range seen {
		if !known[kind] {
			t.Fatalf("repair row %s is not a member of the closed kind set", kind)
		}
	}
}

// Parity pin for the tool-side serial gate (census row
// tool/normalizeCompletionAggregateFacts → serial_with_collector): the
// accumulate walker's first element is byte-identical to the serial reject.
func TestNormalizeCompletionAggregateFacts_CollectFirstElementMatchesSerialGate(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("q")}
	facts := []types.AnswerAggregateFact{
		{Kind: types.AnswerAggregateTotalCount, Label: "call sites", Value: "not-a-number"},
		{Kind: types.AnswerAggregateKind("bogus_kind"), Label: "x", Value: "1"},
	}
	_, _, serialErr := normalizeCompletionAggregateFacts(bus, "resolved", facts)
	if serialErr == nil {
		t.Fatal("serial gate must reject the witness facts")
	}
	violations := completionAggregateFactsCollectViolations(bus, facts)
	if len(violations) < 2 {
		t.Fatalf("collector must report more than the first failing entry, got %v", violations)
	}
	if violations[0] != serialErr.Error() {
		t.Fatalf("collector first element must match the serial gate error:\n serial: %s\n walker: %s", serialErr.Error(), violations[0])
	}
}
