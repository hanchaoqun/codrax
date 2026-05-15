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
		"blocks": [
			{"id": "b1", "kind": "summary", "text": "Hello"}
		]
	}`)
}

func TestEmitAnswerDocumentV2_LegacyV1PayloadDoesNotWriteV2(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	// Legacy V1-shaped emit. Must NOT write the V2 carrier. The
	// deciding factor is the retired V1 top-level payload, not the
	// absence of any document_model field.
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

func TestEmitAnswerDocumentV2_LegacyHybridPayloadDoesNotWriteV2(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	// Even if a legacy caller includes an empty document_model field,
	// the retired V1 top-level payload still must not write V2.
	emptyJSON := json.RawMessage(`{"document_model": "", "shape": "explanation", "summary": "hi"}`)
	res, err := tool.Execute(bus, emptyJSON)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	_ = res
	if bus.Mutable.AnswerDocumentV2() != nil {
		t.Errorf("legacy hybrid payload must not write V2 carrier; got %+v", bus.Mutable.AnswerDocumentV2())
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

func TestEmitAnswerDocumentV2_RejectsMissingModelSurfaceTerm(t *testing.T) {
	bus := newV2TestBusContext()
	bus.Mutable.AppendEvidence([]types.EvidenceItem{{
		ID:           "ev-index",
		Kind:         types.EvidenceDirect,
		Producer:     EmitEvidenceProducer,
		Subject:      "Index",
		AnchorSymbol: "Index",
		Source:       "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
		LineStart:    7,
		Scope:        types.ScopeLine,
		SurfaceTerms: []string{"Index.ets"},
	}})
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [{
			"id": "entry_list",
			"kind": "ordered_list",
			"items": [{
				"id": "e1",
				"label": "Index",
				"text": "struct Index with @Entry + @Component",
				"citation_ref": 0
			}]
		}],
		"citations": [{
			"file": "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
			"line": 7
		}]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if res.Success {
		t.Fatal("expected missing surface term to reject")
	}
	if !strings.Contains(res.Summary, "surface_terms") || !strings.Contains(res.Summary, "Index.ets") {
		t.Fatalf("rejection should name missing model surface term, got %q", res.Summary)
	}
}

func TestEmitAnswerDocumentV2_AcceptsPreservedModelSurfaceTerm(t *testing.T) {
	bus := newV2TestBusContext()
	bus.Mutable.AppendEvidence([]types.EvidenceItem{{
		ID:           "ev-index",
		Kind:         types.EvidenceDirect,
		Producer:     EmitEvidenceProducer,
		Subject:      "Index",
		AnchorSymbol: "Index",
		Source:       "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
		LineStart:    7,
		Scope:        types.ScopeLine,
		SurfaceTerms: []string{"Index.ets"},
	}})
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [{
			"id": "entry_list",
			"kind": "ordered_list",
			"items": [{
				"id": "e1",
				"label": "Index",
				"text": "struct Index, original source label Index.ets, with @Entry + @Component",
				"citation_ref": 0
			}]
		}],
		"citations": [{
			"file": "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
			"line": 7
		}]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected surface term preserving emit to succeed; got %+v", res)
	}
}

func TestEmitAnswerDocumentV2_DoesNotRequireUnrelatedPathSurfaceTerm(t *testing.T) {
	bus := newV2TestBusContext()
	bus.Mutable.AppendEvidence([]types.EvidenceItem{{
		ID:           "ev-normalize",
		Kind:         types.EvidenceDirect,
		Producer:     EmitEvidenceProducer,
		Subject:      "analyzerGraphForNormalize",
		AnchorSymbol: "analyzerGraphForNormalize",
		Source:       "internal/agent/analyzer.go",
		LineStart:    1625,
		Scope:        types.ScopeLine,
		SurfaceTerms: []string{"internal/types/enumeration_boundary.go"},
	}})
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [{
			"id": "trace_steps",
			"kind": "ordered_list",
			"items": [{
				"id": "e1",
				"label": "analyzerGraphForNormalize",
				"text": "calls normalizer.Normalize and returns a TermGraph.",
				"citation_ref": 0
			}]
		}],
		"citations": [{
			"file": "internal/agent/analyzer.go",
			"line": 1625
		}]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("unrelated path-like surface term should not be hard-required; got %+v", res)
	}
}

func TestEmitAnswerDocumentV2_RejectsMissingPathSurfaceTermWhenItNamesItem(t *testing.T) {
	bus := newV2TestBusContext()
	bus.Mutable.AppendEvidence([]types.EvidenceItem{{
		ID:           "ev-widget",
		Kind:         types.EvidenceDirect,
		Producer:     EmitEvidenceProducer,
		Subject:      "Widget",
		AnchorSymbol: "Widget",
		Source:       "src/components/Widget.ets",
		LineStart:    3,
		Scope:        types.ScopeLine,
		SurfaceTerms: []string{"src/components/Widget.ets"},
	}})
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [{
			"id": "entry_list",
			"kind": "ordered_list",
			"items": [{
				"id": "e1",
				"label": "Widget",
				"text": "declares the component entry.",
				"citation_ref": 0
			}]
		}],
		"citations": [{
			"file": "src/components/Widget.ets",
			"line": 3
		}]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if res.Success {
		t.Fatal("expected missing item-identifying path surface term to reject")
	}
	if !strings.Contains(res.Summary, "src/components/Widget.ets") {
		t.Fatalf("rejection should name missing path-like surface term, got %q", res.Summary)
	}
}

func TestEmitAnswerDocumentV2_RejectsV1FieldsAtTopLevel(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	// Legacy top-level V1 field mixed into the block-only carrier → reject.
	mixed := json.RawMessage(`{
		"shape": "explanation",
		"blocks": [{"id":"b1","kind":"summary","text":"x"}]
	}`)
	res, _ := tool.Execute(bus, mixed)
	if res.Success {
		t.Fatal("expected mixed V1+V2 emit to be rejected")
	}
	// v3.1 (2026-05-05): the rejection text no longer mentions
	// internal version concepts ("V1") — it only names the offending
	// top-level field and explains the answer must live in blocks[].
	if !strings.Contains(res.Summary, "shape") || !strings.Contains(res.Summary, "blocks[]") {
		t.Errorf("error message should name the offending field and the blocks-only contract; got %q", res.Summary)
	}
}

// v3.1 (2026-05-05) — TestEmitAnswerDocumentV2_RejectsUnknownDocumentModel
// removed. The system runs only one carrier and document_model is no
// longer surfaced to the LLM nor validated by the executor; whatever
// value the LLM emits (or omits) is silently accepted. Future
// migration to a second carrier would re-introduce a typed rejection.

func TestEmitAnswerDocumentV2_RejectsEmptyBlocks(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	emptyBlocks := json.RawMessage(`{"blocks": []}`)
	res, _ := tool.Execute(bus, emptyBlocks)
	if res.Success {
		t.Fatal("expected empty blocks[] to be rejected")
	}
}

func TestEmitAnswerDocumentV2_RejectsMissingBlockID(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	bad := json.RawMessage(`{
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
		"blocks": [{"id":"b1","kind":"bogus_kind","text":"x"}]
	}`)
	res, _ := tool.Execute(bus, bad)
	if res.Success {
		t.Fatal("expected invalid block kind to be rejected")
	}
	// G2 (post_v2_runtime_gap_remediation, 2026-05-04): the unified
	// NormalizeEmitAnswerBlock helper uses LLM-facing prose
	// ("block kind" instead of the Go type name "AnswerBlockKind",
	// per R4). The error message MUST still name the offending kind
	// verbatim AND the allowed list, so the LLM can fix the emit.
	if !strings.Contains(res.Summary, `"bogus_kind"`) || !strings.Contains(res.Summary, "block kind") {
		t.Errorf("error message must name the bad value + the contract concept; got %q", res.Summary)
	}
}

func TestEmitAnswerDocumentV2_RejectsDuplicateBlockIDs(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	dup := json.RawMessage(`{
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

func TestEmitAnswerDocumentV2_FlatModeTopLevelCitationsAsString(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	flat := json.RawMessage(`{
		"blocks": [{"id":"b1","kind":"summary","text":"Hello"}],
		"citations": "[{\"file\":\"internal/agent/sub_explorer.go\",\"line\":31}]"
	}`)
	res, err := tool.Execute(bus, flat)
	if err != nil {
		t.Fatalf("flat-mode emit error: %v", err)
	}
	if !res.Success {
		t.Fatalf("top-level citations string should succeed via repair; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil {
		t.Fatal("flat-mode repair did not write V2 carrier")
	}
	if len(doc.Citations) != 1 || doc.Citations[0].File != "internal/agent/sub_explorer.go" || doc.Citations[0].Line != 31 {
		t.Fatalf("citations did not deserialise from JSON string: %+v", doc.Citations)
	}
}

func TestEmitAnswerDocumentV2_FlatModeTopLevelCitationsStringWithExtraCloser(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	flat := json.RawMessage(`{
		"blocks": [{"id":"b1","kind":"summary","text":"Hello"}],
		"citations": "[{\"file\":\"internal/agent/sub_explorer.go\",\"line\":31}]]"
	}`)
	res, err := tool.Execute(bus, flat)
	if err != nil {
		t.Fatalf("flat-mode emit error: %v", err)
	}
	if !res.Success {
		t.Fatalf("overclosed top-level citations string should succeed via repair; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Citations) != 1 || doc.Citations[0].Line != 31 {
		t.Fatalf("citations did not deserialise from overclosed JSON string: %+v", doc)
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
		"caveats": ["a","b"],
		"blocks": "[{\"id\":\"b1\",\"kind\":\"summary\"}]"
	}`)
	patched, ok := repairBlocksAsString(raw)
	if !ok {
		t.Fatal("repair did not fire")
	}
	if !strings.Contains(string(patched), `"caveats":["a","b"]`) {
		t.Errorf("caveats lost: %s", patched)
	}
	// Critically: blocks must now be a real JSON array.
	if !strings.Contains(string(patched), `"blocks":[`) {
		t.Errorf("blocks not converted to array: %s", patched)
	}
}

// TestRepairBlocksAsString_WholeDocumentStringify covers the v2 repair
// extension (Plan 2 v2 follow-up, 2026-05-05) — the LLM JSON.stringify'd
// the entire document body, producing
//
//	{"blocks": "[blocks_array], \"citations\": [...], \"caveats\": [...]"}
//
// where the encoded string contains the blocks array followed by the
// rest of the top-level fields. Recovery: wrap the encoded string with
// `{"blocks": ... }` so the whole-doc payload becomes a parseable
// top-level object, then merge.
func TestRepairBlocksAsString_WholeDocumentStringify(t *testing.T) {
	raw := json.RawMessage(`{"blocks": "[{\"id\":\"b1\",\"kind\":\"summary\",\"text\":\"hi\"}], \"citations\": [{\"file\":\"x.go\",\"line\":1}], \"caveats\": [\"c1\"]"}`)
	patched, ok := repairBlocksAsString(raw)
	if !ok {
		t.Fatal("repair did not fire on whole-document stringify")
	}
	if !strings.Contains(string(patched), `"blocks":[`) {
		t.Errorf("blocks not converted to array: %s", patched)
	}
	if !strings.Contains(string(patched), `"citations":[`) {
		t.Errorf("citations not lifted to top level: %s", patched)
	}
	if !strings.Contains(string(patched), `"caveats":["c1"]`) {
		t.Errorf("caveats not lifted to top level: %s", patched)
	}
	// Validate full Decode round-trip.
	var p emitAnswerDocumentV2Params
	if err := json.Unmarshal(patched, &p); err != nil {
		t.Fatalf("repaired payload no longer parses: %v", err)
	}
	if len(p.Blocks) != 1 || p.Blocks[0].ID != "b1" {
		t.Errorf("blocks lost during repair: %+v", p.Blocks)
	}
	if len(p.Citations) != 1 {
		t.Errorf("citations lost during repair: %+v", p.Citations)
	}
}

func TestRepairBlocksAsString_WholeDocumentStringifyWithTrailingBrace(t *testing.T) {
	raw := json.RawMessage(`{"blocks": "[{\"id\":\"b1\",\"kind\":\"summary\",\"text\":\"hi\"}], \"citations\": [{\"file\":\"x.go\",\"line\":1}], \"caveats\": [\"c1\"]}"}`)
	patched, ok := repairBlocksAsString(raw)
	if !ok {
		t.Fatal("repair did not fire on whole-document stringify with trailing object brace")
	}
	var p emitAnswerDocumentV2Params
	if err := json.Unmarshal(patched, &p); err != nil {
		t.Fatalf("repaired payload no longer parses: %v\npayload=%s", err, patched)
	}
	if len(p.Blocks) != 1 || p.Blocks[0].ID != "b1" {
		t.Fatalf("blocks lost during repair: %+v", p.Blocks)
	}
	if len(p.Citations) != 1 || p.Citations[0].File != "x.go" || p.Citations[0].Line != 1 {
		t.Fatalf("citations not lifted to typed citation pool: %+v", p.Citations)
	}
	if len(p.Caveats) != 1 || p.Caveats[0] != "c1" {
		t.Fatalf("caveats not lifted to top level: %+v", p.Caveats)
	}
}

func TestRepairBlocksAsString_BraceFallbackSkipsCitationObjects(t *testing.T) {
	raw := json.RawMessage(`{"blocks": "[{\"id\":\"b1\",\"kind\":\"summary\",\"text\":\"hi\"}], \"citations\": [{\"file\":\"x.go\",\"line\":1}], trailing"}`)
	patched, ok := repairBlocksAsString(raw)
	if !ok {
		t.Fatal("brace fallback did not recover the valid answer block")
	}
	var p emitAnswerDocumentV2Params
	if err := json.Unmarshal(patched, &p); err != nil {
		t.Fatalf("brace fallback produced invalid payload: %v\npayload=%s", err, patched)
	}
	if len(p.Blocks) != 1 || p.Blocks[0].ID != "b1" {
		t.Fatalf("brace fallback should keep only block-shaped objects, got %+v", p.Blocks)
	}
}

func TestRepairBlocksAsString_BraceFallbackReportsDroppedDiagram(t *testing.T) {
	raw := json.RawMessage(`{"blocks": "[{\"id\":\"sum1\",\"kind\":\"summary\",\"text\":\"hi\"},{\"id\":\"diag1\",\"kind\":\"diagram\",\"diagram\":{\"kind\":\"sequence\",\"language\":\"mermaid\",\"body\":\"sequenceDiagram\\n    User->>Agent: \"ask\"}}], \"citations\": [{\"file\":\"x.go\",\"line\":1}], trailing"}`)
	patched, report, ok := repairBlocksAsStringDetailed(raw)
	if !ok {
		t.Fatal("brace fallback did not recover the valid answer block")
	}
	var p emitAnswerDocumentV2Params
	if err := json.Unmarshal(patched, &p); err != nil {
		t.Fatalf("brace fallback produced invalid payload: %v\npayload=%s", err, patched)
	}
	if len(p.Blocks) != 1 || p.Blocks[0].ID != "sum1" {
		t.Fatalf("expected only valid summary block to survive structured recovery, got %+v", p.Blocks)
	}
	if report.Lossless {
		t.Fatalf("malformed diagram recovery must be marked lossy: %+v", report)
	}
	if !report.DroppedVisiblePayload {
		t.Fatalf("dropped visible payload not reported: %+v", report)
	}
	if len(report.Attachments) != 1 {
		t.Fatalf("expected recovered diagram attachment, got %+v", report.Attachments)
	}
	if got := report.Attachments[0].Body; !strings.Contains(got, "sequenceDiagram") || !strings.Contains(got, "User->>Agent") {
		t.Fatalf("diagram body not preserved in attachment: %q", got)
	}
}

func TestEmitAnswerDocumentV2_BraceFallbackPreservesDroppedDiagramAttachment(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	raw := json.RawMessage(`{"blocks": "[{\"id\":\"sum1\",\"kind\":\"summary\",\"text\":\"hi\"},{\"id\":\"diag1\",\"kind\":\"diagram\",\"diagram\":{\"kind\":\"sequence\",\"language\":\"mermaid\",\"body\":\"sequenceDiagram\\n    User->>Agent: \"ask\"}}], \"citations\": [{\"file\":\"x.go\",\"line\":1}], trailing"}`)
	res, err := tool.Execute(bus, raw)
	if err != nil {
		t.Fatalf("emit error: %v", err)
	}
	if !res.Success {
		t.Fatalf("lossy brace fallback should still accept the valid structured doc with attachment preserved; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 || doc.Blocks[0].Kind != types.BlockSummary {
		t.Fatalf("structured summary doc not written: %+v", doc)
	}
	attachments := bus.Mutable.AnswerDisplayAttachments()
	if len(attachments) != 1 {
		t.Fatalf("recovered display attachment not stored: %+v", attachments)
	}
	if !strings.Contains(attachments[0].Body, "sequenceDiagram") {
		t.Fatalf("stored attachment lost diagram body: %+v", attachments[0])
	}
}

func TestEmitAnswerDocumentV2_RejectedAttemptsAccumulateVisibleDiagrams(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}

	first := json.RawMessage(`{
		"blocks": [
			{"id":"d1","kind":"diagram","diagram":{"kind":"flow","language":"mermaid","body":"flowchart TD\n  A --> B"}},
			{"id":"bad1","kind":"diagram"}
		]
	}`)
	res, err := tool.Execute(bus, first)
	if err != nil {
		t.Fatalf("first emit error: %v", err)
	}
	if res.Success {
		t.Fatalf("first malformed emit should fail")
	}

	second := json.RawMessage(`{
		"blocks": [
			{"id":"d2","kind":"diagram","diagram":{"kind":"sequence","language":"mermaid","body":"sequenceDiagram\n  User->>Agent: ask"}},
			{"id":"bad2","kind":"diagram"}
		]
	}`)
	res, err = tool.Execute(bus, second)
	if err != nil {
		t.Fatalf("second emit error: %v", err)
	}
	if res.Success {
		t.Fatalf("second malformed emit should fail")
	}

	attachments := bus.Mutable.AnswerDisplayAttachments()
	if len(attachments) != 2 {
		t.Fatalf("rejected attempts should accumulate unique diagrams, got %+v", attachments)
	}
	if !strings.Contains(attachments[0].Body, "flowchart TD") || !strings.Contains(attachments[1].Body, "sequenceDiagram") {
		t.Fatalf("visible diagram bodies not preserved in order: %+v", attachments)
	}
}

func TestEmitAnswerDocumentV2_RejectedAttemptPreservesVisibleDraftText(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	raw := json.RawMessage(`{
		"blocks": [
			{"id":"s1","kind":"summary","text":"Visible answer body"},
			{"id":"s2","kind":"not_a_real_kind","text":"Malformed but visible"}
		],
		"citations": [{"file":"internal/agent/sub_explorer.go","line":24}]
	}`)
	res, err := tool.Execute(bus, raw)
	if err != nil {
		t.Fatalf("emit error: %v", err)
	}
	if res.Success {
		t.Fatal("invalid block kind should reject the structured emit")
	}
	attachments := bus.Mutable.AnswerDisplayAttachments()
	if len(attachments) == 0 {
		t.Fatal("rejected visible draft should be preserved as a display attachment")
	}
	joined := ""
	for _, att := range attachments {
		joined += "\n" + att.Body
	}
	for _, want := range []string{
		"Visible answer body",
		"Malformed but visible",
		"internal/agent/sub_explorer.go:24",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("recovered draft lost %q; attachments=%+v", want, attachments)
		}
	}
}

// TestEmitAnswerDocumentV2_WholeDocumentStringifyAccepted is the
// end-to-end version: a finalizer-shaped LLM emit where the whole
// answer body was JSON.stringify'd into the blocks key MUST now be
// accepted instead of failing 6 retries before answer quality
// degrades (qf_arch run-2 forensic, 2026-05-05).
func TestEmitAnswerDocumentV2_WholeDocumentStringifyAccepted(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	flat := json.RawMessage(`{"blocks": "[{\"id\":\"sum1\",\"kind\":\"summary\",\"text\":\"abc\"}], \"citations\": [{\"file\":\"x.go\",\"line\":1}]"}`)
	res, err := tool.Execute(bus, flat)
	if err != nil {
		t.Fatalf("emit error: %v", err)
	}
	if !res.Success {
		t.Fatalf("whole-document stringify should be repaired and accepted; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 {
		t.Fatalf("answer document not written: %+v", doc)
	}
	if len(doc.Citations) != 1 {
		t.Errorf("citations not preserved through repair: %+v", doc.Citations)
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

func TestRepairNestedArraysAsString_ItemsAsStringWithExtraCloser(t *testing.T) {
	raw := json.RawMessage(`{
		"blocks": [
			{"id": "list1", "kind": "ordered_list",
			 "items": "[{\"id\":\"i1\",\"label\":\"A\"}]]"}
		]
	}`)
	patched, paths, ok := repairNestedArraysAsString(raw)
	if !ok {
		t.Fatal("repair must fire")
	}
	if len(paths) != 1 || paths[0] != "blocks[0].items" {
		t.Errorf("paths = %v, want [blocks[0].items]", paths)
	}
	if !strings.Contains(string(patched), `"items":[{`) {
		t.Errorf("items[] not converted to array: %s", patched)
	}
}

// TestRepairNestedArraysAsString_ClaimUsesAsString covers the
// per-block claim_uses[] string-encoded case.
func TestRepairNestedArraysAsString_ClaimUsesAsString(t *testing.T) {
	raw := json.RawMessage(`{
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

func TestRepairNestedArraysAsString_FacetIDsAndEdgeAnchorsAsString(t *testing.T) {
	raw := json.RawMessage(`{
		"blocks": [
			{"id": "d1", "kind": "diagram",
			 "facet_ids": "[\"current_code_path\"]",
			 "edge_anchors": "[{\"from_node\":\"A\",\"to_node\":\"B\",\"relation_kind\":\"call\"}]"}
		]
	}`)
	patched, paths, ok := repairNestedArraysAsString(raw)
	if !ok {
		t.Fatal("repair must fire")
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 repair paths; got %v", paths)
	}
	if !strings.Contains(string(patched), `"facet_ids":["current_code_path"]`) ||
		!strings.Contains(string(patched), `"edge_anchors":[{`) {
		t.Fatalf("structured block arrays not repaired: %s", patched)
	}
}

func TestRepairNestedArraysAsString_DiagramObjectAsString(t *testing.T) {
	raw := json.RawMessage(`{
		"blocks": [
			{"id": "d1", "kind": "diagram",
			 "diagram": "{\"kind\":\"sequence\",\"language\":\"mermaid\",\"body\":\"sequenceDiagram\\nA->>B: hi\"}"}
		]
	}`)
	patched, paths, ok := repairNestedArraysAsString(raw)
	if !ok {
		t.Fatal("repair must fire")
	}
	if len(paths) != 1 || paths[0] != "blocks[0].diagram" {
		t.Errorf("paths = %v, want [blocks[0].diagram]", paths)
	}
	if !strings.Contains(string(patched), `"diagram":{`) {
		t.Fatalf("diagram object not repaired: %s", patched)
	}
}

// TestRepairNestedArraysAsString_MultipleAtOnce confirms the
// repair handles multiple nested-string sites in one emit.
func TestRepairNestedArraysAsString_MultipleAtOnce(t *testing.T) {
	raw := json.RawMessage(`{
		"blocks": [
			{"id": "list1", "kind": "ordered_list",
			 "items": "[{\"id\":\"i1\",\"label\":\"A\"}]",
			 "claim_uses": "[{\"claim_form\":\"call_edge\"}]"},
			{"id": "list2", "kind": "bullet_list",
			 "items": "[{\"id\":\"i2\",\"label\":\"B\"}]"}
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

func TestEmitAnswerDocumentV2_RepairsAnnotationCamelCaseShape(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [{
			"id": "s1",
			"kind": "summary",
			"text": "lead",
			"claim_uses": [{"claimForm": "assignmentFact", "facetId": "current_code_path", "evidenceId": "ev1"}]
		}, {
			"id": "d1",
			"kind": "diagram",
			"diagram": {"kind": "sequence", "language": "mermaid", "body": "sequenceDiagram\nA->>B: hi"},
			"edge_anchors": [{"fromNode": "A", "toNode": "B", "relationKind": "Call", "claimForm": "callEdge"}]
		}]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("camelCase annotation shape should be repaired; got %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 {
		t.Fatalf("expected repaired doc with two blocks; got %+v", doc)
	}
	if got := doc.Blocks[0].ClaimUses; len(got) != 1 ||
		got[0].ClaimForm != types.ClaimAssignmentFact ||
		got[0].FacetID != "current_code_path" ||
		got[0].EvidenceID != "ev1" {
		t.Fatalf("claim_uses not normalized: %+v", got)
	}
	if got := doc.Blocks[1].EdgeAnchors; len(got) != 1 ||
		got[0].FromNode != "A" ||
		got[0].ToNode != "B" ||
		got[0].RelationKind != types.DiagramRelCall ||
		got[0].ClaimForm != types.ClaimCallEdge {
		t.Fatalf("edge_anchors not normalized: %+v", got)
	}
}

func TestEmitAnswerDocumentV2_RejectsConflictingAnnotationAlias(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [{
			"id": "s1",
			"kind": "summary",
			"text": "lead",
			"claim_uses": [{"claim_form": "call_edge", "claimForm": "returnFact"}]
		}]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if res.Success {
		t.Fatal("conflicting alias/canonical fields must stay rejected")
	}
	if !strings.Contains(res.Summary, `claimForm`) {
		t.Fatalf("rejection should name unrepaired alias; got %s", res.Summary)
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

func TestEmitAnswerDocumentV2_StringWrappedExactResolutionAndDiagram(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"exact_resolution": "{\"status\":\"exact_match\",\"anchor\":\"SubExplorer.Name\"}",
		"blocks": [{
			"id": "d1",
			"kind": "diagram",
			"diagram": "{\"kind\":\"sequence\",\"language\":\"mermaid\",\"body\":\"sequenceDiagram\\nA->>B: hi\"}"
		}]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("emit error: %v", err)
	}
	if !res.Success {
		t.Fatalf("string-wrapped object fields should be repaired and accepted; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || doc.ExactResolution == nil || doc.ExactResolution.Anchor != "SubExplorer.Name" {
		t.Fatalf("exact_resolution not repaired: %+v", doc)
	}
	if len(doc.Blocks) != 1 || doc.Blocks[0].Diagram == nil || doc.Blocks[0].Diagram.Body == "" {
		t.Fatalf("diagram not repaired: %+v", doc)
	}
}

func TestEmitAnswerDocumentV2_StringCitationAndSnippetLineNumbers(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	payload := json.RawMessage(`{
		"blocks": [{"id": "s1", "kind": "summary", "text": "Hello"}],
		"citations": [{"file": "internal/agent/sub_explorer.go", "line": "31", "line_end": "33"}],
		"snippets": [{"file": "internal/agent/sub_explorer.go", "start_line": "31", "end_line": "33", "code": "func (s *SubExplorer) Name() string { return \"explorer\" }"}]
	}`)
	res, err := tool.Execute(bus, payload)
	if err != nil {
		t.Fatalf("emit error: %v", err)
	}
	if !res.Success {
		t.Fatalf("string line numbers should be repaired and accepted; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Citations) != 1 || doc.Citations[0].Line != 31 || doc.Citations[0].LineEnd != 33 {
		t.Fatalf("citation line numbers not parsed: %+v", doc)
	}
	if len(doc.Snippets) != 1 || doc.Snippets[0].StartLine != 31 || doc.Snippets[0].EndLine != 33 {
		t.Fatalf("snippet line numbers not parsed: %+v", doc.Snippets)
	}
}

// TestEmitAnswerDocumentV2_NestedFlatModeAccepts pins end-to-end
// behaviour: tool Execute path repairs nested string + completes
// the emit successfully.
func TestEmitAnswerDocumentV2_NestedFlatModeAccepts(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	flat := json.RawMessage(`{
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

// TestEmitAnswerDocumentV2_CitationRefInsideClaimUseRemapped pins
// Phase 1-B (V2 runtime eval followup, 2026-05-04): when the LLM
// places `citation_ref` inside `claim_use` (the u3a-1 forensic
// outlier), the strict-decode error MUST surface the correct
// paths so the next retry sees concrete relocation guidance. A
// bare `unknown field "citation_ref"` reject was burning 5-7 retry
// iters; the remap reduces that to ≤ 2.
func TestEmitAnswerDocumentV2_CitationRefInsideClaimUseRemapped(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	misplaced := json.RawMessage(`{
		"blocks": [{
			"id": "b1",
			"kind": "summary",
			"text": "x",
			"claim_uses": [{"claim_form": "definition_fact", "citation_ref": 0}]
		}],
		"citations": [{"file": "f.go", "line": 1}]
	}`)
	res, _ := tool.Execute(bus, misplaced)
	if res.Success {
		t.Fatal("expected misplaced citation_ref reject")
	}
	for _, want := range []string{
		`field "citation_ref"`,
		"items[i].citation_ref",
		"NOT inside",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("error message missing %q (Phase 1-B remap broken); got: %s", want, res.Summary)
		}
	}
}

// TestEmitAnswerDocumentV2_FacetIDsInsideClaimUseRemapped pins the
// recurring V2 retry failure where the model copies block.facet_ids
// into claim_uses[] as a plural field. The schema only allows
// singular facet_id on each claim annotation object; plural facet_ids
// belongs on the block itself.
func TestEmitAnswerDocumentV2_FacetIDsInsideClaimUseRemapped(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	misplaced := json.RawMessage(`{
		"blocks": [{
			"id": "b1",
			"kind": "summary",
			"text": "x",
			"claim_uses": [{"claim_form": "definition_fact", "facet_ids": ["resolved_literal_or_symbol"]}]
		}]
	}`)
	res, _ := tool.Execute(bus, misplaced)
	if res.Success {
		t.Fatal("expected misplaced facet_ids reject")
	}
	for _, want := range []string{
		`field "facet_ids"`,
		"blocks[i].facet_ids",
		"blocks[i].claim_uses[j].facet_id",
		"NOT inside",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("error message missing %q; got: %s", want, res.Summary)
		}
	}
}

// TestEmitAnswerDocumentV2_ValueFieldInScalarBlockRemapped pins
// V1-carrier-residue bug fix (u3a forensic, 2026-05-04): scalar
// blocks no longer carry block.value{literal, citation_ref};
// literal goes in block.text and citation in items[0].citation_ref.
// LLMs trained on V1 mental model still emit value{...} on a
// scalar block — strict-decode rejects "unknown field value" and
// the remap MUST direct to the V2 location.
func TestEmitAnswerDocumentV2_ValueFieldInScalarBlockRemapped(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	misplaced := json.RawMessage(`{
		"document_model": "v2",
		"blocks": [{
			"id": "v1",
			"kind": "scalar",
			"surface_role": "principal",
			"value": {"literal": "42", "citation_ref": 0}
		}],
		"citations": [{"file": "f.go", "line": 1}]
	}`)
	res, _ := tool.Execute(bus, misplaced)
	if res.Success {
		t.Fatal("expected misplaced value field reject")
	}
	for _, want := range []string{
		`field "value"`,
		"blocks[i].text",
		"blocks[i].items=",
		"NOT inside",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("error message missing %q; got: %s", want, res.Summary)
		}
	}
}

// Boolean/decision block parallel: LLM emits boolean{...} at block
// level (V1 mental model); V2 needs verdict in block.text and
// citation in items[0].
func TestEmitAnswerDocumentV2_BooleanFieldInDecisionBlockRemapped(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	misplaced := json.RawMessage(`{
		"document_model": "v2",
		"blocks": [{
			"id": "d1",
			"kind": "decision",
			"surface_role": "principal",
			"boolean": {"decision": "yes", "rationale": "because", "citation_ref": 0}
		}],
		"citations": [{"file": "f.go", "line": 1}]
	}`)
	res, _ := tool.Execute(bus, misplaced)
	if res.Success {
		t.Fatal("expected misplaced boolean field reject")
	}
	for _, want := range []string{
		`field "boolean"`,
		"blocks[i].text",
		"NOT inside",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("error message missing %q; got: %s", want, res.Summary)
		}
	}
}

// TestEmitAnswerDocumentV2_FromNodeInsideClaimUseRemapped pins
// Phase 1-B source-fix follow-on (2026-05-04): from_node /
// to_node moved out of RenderedClaimUse into block-level
// edge_anchors[]. LLMs trained on the old shape may still try to
// place them inside claim_use; the remap directs to the new path.
func TestEmitAnswerDocumentV2_FromNodeInsideClaimUseRemapped(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	misplaced := json.RawMessage(`{
		"document_model": "v2",
		"blocks": [{
			"id": "d1",
			"kind": "diagram",
			"diagram": {"kind":"sequence","language":"mermaid","body":"sequenceDiagram\n  A->>B: invoke\n"},
			"claim_uses": [{"claim_form": "call_edge", "from_node": "A", "to_node": "B"}]
		}],
		"citations": []
	}`)
	res, _ := tool.Execute(bus, misplaced)
	if res.Success {
		t.Fatal("expected misplaced from_node/to_node reject")
	}
	for _, want := range []string{
		`field "from_node"`,
		"blocks[i].edge_anchors[j].from_node",
		"NOT inside",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("error message missing %q; got: %s", want, res.Summary)
		}
	}
}

func TestEmitAnswerDocumentV2_DiagramEdgeAnchorsPromotedToBlock(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	params := json.RawMessage(`{
		"blocks": [{
			"id": "d1",
			"kind": "diagram",
			"diagram": {
				"kind": "sequence",
				"language": "mermaid",
				"body": "sequenceDiagram\nA->>B: call",
				"edge_anchors": [{"from_node": "A", "to_node": "B", "relation_kind": "call"}]
			}
		}],
		"citations": []
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("diagram.edge_anchors should be structurally promoted, got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 || len(doc.Blocks[0].EdgeAnchors) != 1 {
		t.Fatalf("promoted edge anchors not persisted: %+v", doc)
	}
	if got := doc.Blocks[0].EdgeAnchors[0]; got.FromNode != "A" || got.ToNode != "B" || got.RelationKind != "call" {
		t.Fatalf("edge anchor changed during promotion: %+v", got)
	}
}

// TestRepairBlocksAsString_Path_C_ControlCharRecovery confirms
// Path C's structured fallback: when the LLM's stringified blocks
// array contains UNESCAPED control characters inside string values
// (raw \n in a diagram body, etc.), Path A/B both fail because
// json.Unmarshal rejects raw control bytes inside string literals.
// Path C runs a deterministic state-machine pass that re-escapes
// the control chars and retries the same parse.
//
// This test simulates the MiniMax streaming bug where the model
// stringified an array but emitted single-escape \n instead of
// double-escape \\n inside the string.
func TestRepairBlocksAsString_Path_C_ControlCharRecovery(t *testing.T) {
	// Crafted payload: blocks key holds a JSON-encoded string whose
	// content is a valid JSON array EXCEPT one diagram body has a
	// raw newline (0x0A) inside a string literal — invalid per RFC
	// 7159 §7.
	rawWithControlChar := []byte(`{"blocks": "[{\"id\":\"d1\",\"kind\":\"diagram\",\"diagram\":{\"body\":\"flowchart TD
    A --> B\"}}]"}`)
	// Sanity: confirm Path A/B would reject this (raw newline inside
	// the inner JSON string makes json.Unmarshal fail).
	repaired, ok := repairBlocksAsString(rawWithControlChar)
	if !ok {
		t.Fatalf("Path C must rescue control-char-corrupted blocks; got ok=false")
	}
	// The repaired payload must round-trip through the strict V2
	// schema decoder.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(repaired, &probe); err != nil {
		t.Fatalf("repaired payload is not valid JSON: %v\n%s", err, string(repaired))
	}
	blocks, ok := probe["blocks"]
	if !ok || len(blocks) == 0 || blocks[0] != '[' {
		t.Fatalf("repaired blocks is not a real array: got %s", string(blocks))
	}
	var inner []json.RawMessage
	if err := json.Unmarshal(blocks, &inner); err != nil {
		t.Fatalf("repaired blocks array does not parse: %v", err)
	}
	if len(inner) != 1 {
		t.Fatalf("expected 1 block, got %d", len(inner))
	}
	// And the diagram body must round-trip with the newline preserved
	// (escaped as \n in the JSON value, but a literal newline in the
	// decoded Go string).
	var blk struct {
		Diagram struct {
			Body string `json:"body"`
		} `json:"diagram"`
	}
	if err := json.Unmarshal(inner[0], &blk); err != nil {
		t.Fatalf("decode block: %v", err)
	}
	if !strings.Contains(blk.Diagram.Body, "flowchart TD") || !strings.Contains(blk.Diagram.Body, "A --> B") {
		t.Errorf("diagram body lost content; got %q", blk.Diagram.Body)
	}
	if !strings.Contains(blk.Diagram.Body, "\n") {
		t.Errorf("diagram body must preserve the newline as a real \\n char; got %q", blk.Diagram.Body)
	}
}

// TestNormalizeControlCharsInJSONStrings_Atomic exercises the
// state-machine in isolation. The recovery contract:
//
//   - raw \n / \r / \t / \f / \b inside a string become escape pairs
//   - other 0x00–0x1F bytes inside strings become \uXXXX
//   - bytes OUTSIDE strings pass through verbatim (so structural
//     braces / brackets / commas are unaffected)
//   - a string literal already-escaped inputs are byte-identical and
//     `changed=false` (fast path)
func TestNormalizeControlCharsInJSONStrings_Atomic(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   string
		change bool
	}{
		{
			name:   "no controls — fast path",
			in:     `[{"a":"b"}]`,
			want:   `[{"a":"b"}]`,
			change: false,
		},
		{
			name:   "raw newline inside string",
			in:     "{\"body\":\"line1\nline2\"}",
			want:   `{"body":"line1\nline2"}`,
			change: true,
		},
		{
			name:   "tab + carriage return",
			in:     "{\"body\":\"a\tb\rc\"}",
			want:   `{"body":"a\tb\rc"}`,
			change: true,
		},
		{
			name:   "newline outside string preserved",
			in:     "[\n  {\"a\":\"b\"}\n]",
			want:   "[\n  {\"a\":\"b\"}\n]",
			change: false,
		},
		{
			name:   "escaped quote inside string ignored",
			in:     `{"a":"x\"y"}`,
			want:   `{"a":"x\"y"}`,
			change: false,
		},
		{
			name:   "low byte gets uXXXX escape",
			in:     "{\"a\":\"x\x07y\"}",
			want:   `{"a":"x\u0007y"}`,
			change: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := normalizeControlCharsInJSONStrings(tc.in)
			if got != tc.want {
				t.Errorf("got %q\nwant %q", got, tc.want)
			}
			if changed != tc.change {
				t.Errorf("changed=%v want %v", changed, tc.change)
			}
		})
	}
}
