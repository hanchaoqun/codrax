package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ── B3-T5 emit_answer_document V2 路径单测 ─────────────────────────

// helper: build a fresh BusContext with a non-nil Mutable so
// emit_answer_document_v2 has somewhere to write.
func newV2TestBusContext() *types.BusContext {
	return &types.BusContext{
		Mutable: &types.MutableState{},
	}
}

// helper: minimal V2 emit JSON with one summary block.
func minimalV2EmitJSON() json.RawMessage {
	return json.RawMessage(`{
		"document_model": "v2",
		"blocks": [
			{"id": "b1", "kind": "summary", "text": "Hello"}
		]
	}`)
}

func TestEmitAnswerDocumentV2_NoDocumentModelDoesNotWriteV2(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	// Legacy V1-shaped emit (no document_model field, V1 carrier
	// retired). Must NOT write the V2 carrier — when the V1 carrier
	// existed this routed to V1; today it produces nothing.
	v1JSON := json.RawMessage(`{"shape": "explanation", "summary": "hi"}`)
	res, err := tool.Execute(bus, v1JSON)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	_ = res
	if bus.Mutable.AnswerDocumentV2() != nil {
		t.Errorf("legacy emit must not write V2 carrier; got %+v", bus.Mutable.AnswerDocumentV2())
	}
}

func TestEmitAnswerDocumentV2_EmptyDocumentModelDoesNotWriteV2(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	// Explicit empty document_model — pre-PR5 this routed V1 (B3
	// design); with V1 retired, the call still must not write V2.
	emptyJSON := json.RawMessage(`{"document_model": "", "shape": "explanation", "summary": "hi"}`)
	res, err := tool.Execute(bus, emptyJSON)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	_ = res
	if bus.Mutable.AnswerDocumentV2() != nil {
		t.Errorf("empty document_model must not write V2 carrier; got %+v", bus.Mutable.AnswerDocumentV2())
	}
}

func TestEmitAnswerDocumentV2_AcceptsValidV2(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	res, err := tool.Execute(bus, minimalV2EmitJSON())
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected V2 emit to succeed; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil {
		t.Fatal("V2 doc not written to Mutable")
	}
	if doc.DocumentModel != "v2" {
		t.Errorf("DocumentModel=%q want \"v2\"", doc.DocumentModel)
	}
	if len(doc.Blocks) != 1 || doc.Blocks[0].ID != "b1" || doc.Blocks[0].Kind != types.BlockSummary {
		t.Errorf("blocks not deserialised: got %+v", doc.Blocks)
	}
}

func TestEmitAnswerDocumentV2_RejectsV1FieldsAtTopLevel(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	// document_model="v2" with V1 'shape' field at top-level → reject.
	mixed := json.RawMessage(`{
		"document_model": "v2",
		"shape": "explanation",
		"blocks": [{"id":"b1","kind":"summary","text":"x"}]
	}`)
	res, _ := tool.Execute(bus, mixed)
	if res.Success {
		t.Fatal("expected mixed V1+V2 emit to be rejected")
	}
	if !strings.Contains(res.Summary, "V1 field") {
		t.Errorf("error message should name V1 field; got %q", res.Summary)
	}
}

func TestEmitAnswerDocumentV2_RejectsUnknownDocumentModel(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	bad := json.RawMessage(`{"document_model": "v3", "blocks":[]}`)
	res, _ := tool.Execute(bus, bad)
	if res.Success {
		t.Fatal("expected document_model=v3 to be rejected")
	}
	if !strings.Contains(res.Summary, "v3") {
		t.Errorf("error message should name the bad value; got %q", res.Summary)
	}
}

func TestEmitAnswerDocumentV2_RejectsEmptyBlocks(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	emptyBlocks := json.RawMessage(`{"document_model": "v2", "blocks": []}`)
	res, _ := tool.Execute(bus, emptyBlocks)
	if res.Success {
		t.Fatal("expected empty blocks[] to be rejected")
	}
}

func TestEmitAnswerDocumentV2_RejectsMissingBlockID(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	bad := json.RawMessage(`{
		"document_model": "v2",
		"blocks": [{"kind": "summary", "text": "x"}]
	}`)
	res, _ := tool.Execute(bus, bad)
	if res.Success {
		t.Fatal("expected missing block id to be rejected")
	}
	if !strings.Contains(res.Summary, "id is required") {
		t.Errorf("error message should mention id requirement; got %q", res.Summary)
	}
}

func TestEmitAnswerDocumentV2_RejectsInvalidBlockKind(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	bad := json.RawMessage(`{
		"document_model": "v2",
		"blocks": [{"id":"b1","kind":"bogus_kind","text":"x"}]
	}`)
	res, _ := tool.Execute(bus, bad)
	if res.Success {
		t.Fatal("expected invalid block kind to be rejected")
	}
	if !strings.Contains(res.Summary, "AnswerBlockKind") {
		t.Errorf("error message should reference AnswerBlockKind; got %q", res.Summary)
	}
}

func TestEmitAnswerDocumentV2_RejectsDuplicateBlockIDs(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	dup := json.RawMessage(`{
		"document_model": "v2",
		"blocks": [
			{"id":"x","kind":"summary","text":"a"},
			{"id":"x","kind":"summary","text":"b"}
		]
	}`)
	res, _ := tool.Execute(bus, dup)
	if res.Success {
		t.Fatal("expected duplicate block ids to be rejected")
	}
}

func TestEmitAnswerDocumentV2_DiagramBlockRequiresPayload(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	noPayload := json.RawMessage(`{
		"document_model": "v2",
		"blocks": [{"id":"d1","kind":"diagram","text":"x"}]
	}`)
	res, _ := tool.Execute(bus, noPayload)
	if res.Success {
		t.Fatal("expected diagram block without payload to be rejected")
	}
}

// V1/V2 mutual exclusion: V2 emit must not pollute V1's
// AnswerDocument field, AND V1 emit (with shape) must not pollute
// V2's AnswerDocumentV2 field.
func TestEmitAnswerDocumentV2_MutualExclusionWithV1(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}

	// 1. V2 emit → V1 must remain nil.
	if _, err := tool.Execute(bus, minimalV2EmitJSON()); err != nil {
		t.Fatal(err)
	}
	if bus.Mutable.AnswerDocumentV2() == nil {
		t.Error("V2 emit did not write V2 carrier")
	}

	// 2. New bus, V1 emit → V2 must remain nil.
	bus2 := newV2TestBusContext()
	v1 := json.RawMessage(`{"shape":"explanation","summary":"text"}`)
	if _, err := tool.Execute(bus2, v1); err != nil {
		t.Fatal(err)
	}
	if bus2.Mutable.AnswerDocumentV2() != nil {
		t.Error("V1 emit polluted V2 carrier")
	}
}

// ── B5-F1 flat-mode tolerance: blocks[] arrived as JSON-encoded string ─

// TestEmitAnswerDocumentV2_FlatModeBlocksAsString covers the
// streaming-artefact path where the LLM stringified the blocks
// array. The repair re-parses the embedded JSON array and the emit
// is accepted byte-equivalently.
func TestEmitAnswerDocumentV2_FlatModeBlocksAsString(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	// blocks rendered as a JSON-encoded string instead of an array.
	flat := json.RawMessage(`{
		"document_model": "v2",
		"blocks": "[{\"id\":\"b1\",\"kind\":\"summary\",\"text\":\"Hello\"}]"
	}`)
	res, err := tool.Execute(bus, flat)
	if err != nil {
		t.Fatalf("flat-mode emit error: %v", err)
	}
	if !res.Success {
		t.Fatalf("flat-mode emit should succeed via repair; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil {
		t.Fatal("flat-mode repair did not write V2 carrier")
	}
	if len(doc.Blocks) != 1 || doc.Blocks[0].ID != "b1" || doc.Blocks[0].Kind != types.BlockSummary {
		t.Errorf("flat-mode block did not deserialise: %+v", doc.Blocks)
	}
}

// TestRepairBlocksAsString_NoTrigger pins the negative cases — the
// repair must NOT fire when blocks is already an array, when the
// embedded string is not a JSON array, or when blocks is absent.
func TestRepairBlocksAsString_NoTrigger(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"already_array", `{"blocks":[{"id":"b1","kind":"summary"}]}`},
		{"absent", `{"document_model":"v2"}`},
		{"string_not_array", `{"blocks":"hello"}`},
		{"empty", ``},
		{"malformed_outer", `{not json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := repairBlocksAsString(json.RawMessage(tc.raw))
			if ok {
				t.Errorf("%s: repair fired unexpectedly", tc.name)
			}
		})
	}
}

// TestRepairBlocksAsString_PreservesOtherFields confirms the patch
// surgically swaps blocks while every other top-level key passes
// through verbatim.
func TestRepairBlocksAsString_PreservesOtherFields(t *testing.T) {
	raw := json.RawMessage(`{
		"document_model": "v2",
		"caveats": ["a","b"],
		"blocks": "[{\"id\":\"b1\",\"kind\":\"summary\"}]"
	}`)
	patched, ok := repairBlocksAsString(raw)
	if !ok {
		t.Fatal("repair did not fire")
	}
	if !strings.Contains(string(patched), `"document_model":"v2"`) {
		t.Errorf("document_model lost: %s", patched)
	}
	if !strings.Contains(string(patched), `"caveats":["a","b"]`) {
		t.Errorf("caveats lost: %s", patched)
	}
	// Critically: blocks must now be a real JSON array.
	if !strings.Contains(string(patched), `"blocks":[`) {
		t.Errorf("blocks not converted to array: %s", patched)
	}
}

// TestEmitAnswerDocumentV2_FlatModeRejectsMixedV1FieldsAfterRepair
// confirms the repair does NOT bypass downstream V1↔V2 mutual
// exclusion: even if blocks-as-string is repaired, top-level V1
// fields still trigger rejection.
func TestEmitAnswerDocumentV2_FlatModeRejectsMixedV1FieldsAfterRepair(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	mixed := json.RawMessage(`{
		"document_model": "v2",
		"shape": "explanation",
		"blocks": "[{\"id\":\"b1\",\"kind\":\"summary\"}]"
	}`)
	res, _ := tool.Execute(bus, mixed)
	if res.Success {
		t.Fatal("flat-mode emit with V1 'shape' field must still reject")
	}
}

// ── R4.1 nested-array string-mode tolerance tests ──────────────

// TestRepairNestedArraysAsString_ItemsAsString covers the
// per-block items[] string-encoded case (extending B5-F1's
// blocks[] coverage).
func TestRepairNestedArraysAsString_ItemsAsString(t *testing.T) {
	raw := json.RawMessage(`{
		"document_model": "v2",
		"blocks": [
			{"id": "list1", "kind": "ordered_list",
			 "items": "[{\"id\":\"i1\",\"label\":\"A\"},{\"id\":\"i2\",\"label\":\"B\"}]"}
		]
	}`)
	patched, paths, ok := repairNestedArraysAsString(raw)
	if !ok {
		t.Fatal("repair must fire")
	}
	if len(paths) != 1 || paths[0] != "blocks[0].items" {
		t.Errorf("paths = %v, want [blocks[0].items]", paths)
	}
	// Confirm blocks[0].items is now a real array (not string)
	if !strings.Contains(string(patched), `"items":[{`) {
		t.Errorf("items[] not converted to array: %s", patched)
	}
}

// TestRepairNestedArraysAsString_ClaimUsesAsString covers the
// per-block claim_uses[] string-encoded case.
func TestRepairNestedArraysAsString_ClaimUsesAsString(t *testing.T) {
	raw := json.RawMessage(`{
		"document_model": "v2",
		"blocks": [
			{"id": "s1", "kind": "summary",
			 "claim_uses": "[{\"claim_form\":\"definition_fact\"}]"}
		]
	}`)
	_, paths, ok := repairNestedArraysAsString(raw)
	if !ok {
		t.Fatal("repair must fire")
	}
	if len(paths) != 1 || paths[0] != "blocks[0].claim_uses" {
		t.Errorf("paths = %v, want [blocks[0].claim_uses]", paths)
	}
}

// TestRepairNestedArraysAsString_DiagramClaimUses covers the
// nested-deeper diagram.claim_uses[] case.
func TestRepairNestedArraysAsString_DiagramClaimUses(t *testing.T) {
	raw := json.RawMessage(`{
		"document_model": "v2",
		"blocks": [
			{"id": "d1", "kind": "diagram",
			 "diagram": {"kind": "flow", "body": "flowchart TD\nA-->B",
			   "claim_uses": "[{\"claim_form\":\"call_edge\"}]"}}
		]
	}`)
	_, paths, ok := repairNestedArraysAsString(raw)
	if !ok {
		t.Fatal("repair must fire")
	}
	if len(paths) != 1 || paths[0] != "blocks[0].diagram.claim_uses" {
		t.Errorf("paths = %v, want [blocks[0].diagram.claim_uses]", paths)
	}
}

// TestRepairNestedArraysAsString_MultipleAtOnce confirms the
// repair handles multiple nested-string sites in one emit.
func TestRepairNestedArraysAsString_MultipleAtOnce(t *testing.T) {
	raw := json.RawMessage(`{
		"document_model": "v2",
		"blocks": [
			{"id": "list1", "kind": "ordered_list",
			 "items": "[{\"id\":\"i1\",\"label\":\"A\"}]",
			 "claim_uses": "[{\"claim_form\":\"call_edge\"}]"},
			{"id": "d1", "kind": "diagram",
			 "diagram": {"kind": "flow", "body": "x",
			   "claim_uses": "[{\"claim_form\":\"definition_fact\"}]"}}
		]
	}`)
	_, paths, ok := repairNestedArraysAsString(raw)
	if !ok {
		t.Fatal("repair must fire")
	}
	if len(paths) != 3 {
		t.Errorf("expected 3 repair paths; got %v", paths)
	}
}

// TestRepairNestedArraysAsString_NoTrigger confirms the negative
// cases (already arrays, absent fields, non-array strings, etc.)
// don't trigger the repair.
func TestRepairNestedArraysAsString_NoTrigger(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"already_array", `{"blocks":[{"id":"b1","kind":"summary","items":[{"id":"i1"}]}]}`},
		{"items_absent", `{"blocks":[{"id":"b1","kind":"summary"}]}`},
		{"items_not_string", `{"blocks":[{"id":"b1","kind":"summary","items":42}]}`},
		{"items_string_not_array", `{"blocks":[{"id":"b1","kind":"summary","items":"plain text"}]}`},
		{"empty", ``},
		{"malformed", `{not json`},
		{"blocks_string_top_level", `{"blocks":"[{}]"}`}, // covered by repairBlocksAsString, NOT this one
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, ok := repairNestedArraysAsString(json.RawMessage(tc.raw))
			if ok {
				t.Errorf("repair fired unexpectedly for %s", tc.name)
			}
		})
	}
}

// TestEmitAnswerDocumentV2_NestedFlatModeAccepts pins end-to-end
// behaviour: tool Execute path repairs nested string + completes
// the emit successfully.
func TestEmitAnswerDocumentV2_NestedFlatModeAccepts(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	flat := json.RawMessage(`{
		"document_model": "v2",
		"blocks": [
			{"id": "list1", "kind": "ordered_list",
			 "items": "[{\"id\":\"i1\",\"label\":\"first\"},{\"id\":\"i2\",\"label\":\"second\"}]"}
		]
	}`)
	res, err := tool.Execute(bus, flat)
	if err != nil {
		t.Fatalf("flat-mode nested emit error: %v", err)
	}
	if !res.Success {
		t.Fatalf("nested string repair must succeed; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 {
		t.Fatalf("doc not written; got %+v", doc)
	}
	if got := len(doc.Blocks[0].Items); got != 2 {
		t.Errorf("items not deserialised; got %d, want 2", got)
	}
}
