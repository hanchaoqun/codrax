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

func TestEmitAnswerDocumentV2_NoDocumentModelRoutesV1(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	// V1-shaped emit (no document_model field) must NOT touch V2.
	v1JSON := json.RawMessage(`{"shape": "explanation", "summary": "hi"}`)
	res, err := tool.Execute(bus, v1JSON)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	_ = res
	if bus.Mutable.AnswerDocumentV2() != nil {
		t.Errorf("V1 emit must not write V2 carrier; got %+v", bus.Mutable.AnswerDocumentV2())
	}
}

func TestEmitAnswerDocumentV2_EmptyDocumentModelRoutesV1(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	// Explicit empty document_model still routes V1 (B3 design).
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
