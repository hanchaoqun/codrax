package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// V2 carrier dispatch for emit_answer_document. B3 落地 — handled
// behind a document_model="v2" gate; V1 path is unchanged.
//
// Schema validation discipline (R2 / R12 from
// docs/migration/block_only_carrier.md §3):
//   - document_model MUST be exactly "v2"
//   - blocks[] MUST be present and non-empty
//   - every block.kind MUST be in AllAnswerBlockKinds()
//   - every block.id MUST be non-empty
//   - V1 fields (shape / steps / symbols / value / boolean /
//     symbols_completeness) MUST NOT appear at the top level —
//     V1-or-V2 hybrid emits are rejected to keep the carriers
//     mutually exclusive
//   - summary at top level (V1 field) MUST NOT appear

// emitAnswerDocumentV2Params mirrors AnswerDocumentV2 one-to-one for
// JSON unmarshalling. We DO NOT reuse types.AnswerDocumentV2
// directly so the tool layer can run schema-level checks (forbid V1
// fields) before constructing the typed value. CitationRef is
// transported as FlexInt to accept either int or string forms.
type emitAnswerDocumentV2Params struct {
	DocumentModel string                 `json:"document_model"`
	Blocks        []emitAnswerBlockV2    `json:"blocks"`
	Citations     []types.Citation       `json:"citations,omitempty"`
	ExactResolution *types.AnswerExactResolution `json:"exact_resolution,omitempty"`
	Caveats       []string               `json:"caveats,omitempty"`
	Snippets      []types.CodeSnippet    `json:"snippets,omitempty"`
}

type emitAnswerBlockV2 struct {
	ID          string                     `json:"id"`
	Kind        string                     `json:"kind"`
	Title       string                     `json:"title,omitempty"`
	Text        string                     `json:"text,omitempty"`
	Items       []emitAnswerBlockItemV2    `json:"items,omitempty"`
	Diagram     *emitAnswerDiagramV2       `json:"diagram,omitempty"`
	ClaimUses   []types.RenderedClaimUse   `json:"claim_uses,omitempty"`
	EdgeAnchors []types.DiagramEdgeAnchor  `json:"edge_anchors,omitempty"`
	FacetIDs    []string                   `json:"facet_ids,omitempty"`
	SurfaceRole string                     `json:"surface_role,omitempty"`
}

type emitAnswerBlockItemV2 struct {
	ID          string                  `json:"id,omitempty"`
	Label       string                  `json:"label,omitempty"`
	Text        string                  `json:"text,omitempty"`
	CitationRef FlexInt                 `json:"citation_ref,omitempty"`
	ClaimUse    *types.RenderedClaimUse `json:"claim_use,omitempty"`
}

type emitAnswerDiagramV2 struct {
	Kind      string                   `json:"kind"`
	Language  string                   `json:"language,omitempty"`
	Body      string                   `json:"body"`
	ClaimUses []types.RenderedClaimUse `json:"claim_uses,omitempty"`
}

// executeAnswerDocumentV2 handles document_model="v2" emits. It
// validates the V2 schema, rejects mixed V1+V2 fields, and writes
// the typed AnswerDocumentV2 to MutableState via
// SetAnswerDocumentV2. The result Summary string mirrors V1's tone
// so finalize_preview / orchestrator hooks render consistent
// per-call text.
func executeAnswerDocumentV2(toolName string, ctx *types.BusContext, raw json.RawMessage, now time.Time) (types.ToolResult, error) {
	// First pass: detect retired top-level fields (shape / steps /
	// symbols / value / boolean / summary / symbols_completeness).
	// The answer payload lives entirely inside blocks[]; these
	// top-level fields would silently shadow the block contract and
	// must be moved into the appropriate block kind.
	if violation := detectV1FieldsInV2Emit(raw); violation != "" {
		return failEmit(toolName, now,
			"top-level field %q is not accepted; the answer is expressed through blocks[] only — move any answer payload into the appropriate block kind",
			violation)
	}

	// Flat-mode tolerance. Some LLMs emit nested arrays as JSON
	// strings ("[{...}]") instead of real arrays — typically a
	// streaming artefact where the model ran the array through
	// JSON.stringify before placing it in the tool-call. We
	// repair top-level blocks[] AND every nested array on each
	// block (items / claim_uses / diagram.claim_uses) so the
	// LLM gets accepted instead of forced to retry, while a WARN
	// log surfaces the recovery for operator visibility.
	if repaired, ok := repairBlocksAsString(raw); ok {
		logging.Warning("[emit_answer_document] blocks[] arrived as JSON-encoded string; re-parsed via flat-mode tolerance path")
		raw = repaired
	}
	if repaired, paths, ok := repairNestedArraysAsString(raw); ok {
		logging.Warning("[emit_answer_document] nested arrays arrived as JSON-encoded strings (paths: %s); re-parsed via flat-mode tolerance path",
			strings.Join(paths, ", "))
		raw = repaired
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var p emitAnswerDocumentV2Params
	if err := dec.Decode(&p); err != nil {
		err = RemapStrictDecodeError(err, answerDocumentV2MisplacedHints)
		return failEmit(toolName, now, "invalid params: %v", err)
	}

	// v3.1 (2026-05-05): document_model is no longer surfaced to the
	// LLM. The system runs only one carrier so any value (or absence)
	// the LLM happens to emit is silently accepted — the dispatcher
	// always routes to this executor and the typed model is hardcoded
	// downstream. Future migration to a second carrier would re-introduce
	// validation here.
	if len(p.Blocks) == 0 {
		return failEmit(toolName, now,
			"blocks[] is required and must be non-empty")
	}

	// Build typed AnswerDocumentV2 — per-block validation lives in
	// NormalizeEmitAnswerBlock (shared with the patch path so a typed
	// annotation field added to AnswerBlock surfaces in BOTH paths).
	// Merged-doc invariants (block-id uniqueness, diagram payload,
	// max blocks) are enforced inside ApplyAndPersistMutation so
	// both paths share them.
	doc := &types.AnswerDocumentV2{
		DocumentModel:   "v2",
		Citations:       p.Citations,
		ExactResolution: p.ExactResolution,
		Caveats:         p.Caveats,
		Snippets:        p.Snippets,
	}
	for i, raw := range p.Blocks {
		blk, err := NormalizeEmitAnswerBlock(raw, fmt.Sprintf("blocks[%d]", i))
		if err != nil {
			return failEmit(toolName, now, "%s", err.Error())
		}
		doc.Blocks = append(doc.Blocks, blk)
	}

	// v3 B4 (2026-05-04): route the full-emit write through the
	// unified mutation runtime — same chokepoint as the patch path,
	// merged-doc validation + persist + telemetry shared.
	mutation := types.NewReplaceAllMutation(doc)
	return ApplyAndPersistMutation(ctx, toolName, mutation, nil, now)
}

// detectV1FieldsInV2Emit scans the raw JSON for any top-level field
// that belongs to V1's shape-specific schema. Returns the first
// offending field name, or "" when none are present. The check is
// intentionally a flat regex-free string match because the field
// list is short and explicit.
func detectV1FieldsInV2Emit(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return ""
	}
	v1Fields := []string{
		"shape",
		"steps",
		"symbols",
		"value",
		"boolean",
		"symbols_completeness",
		"summary",
	}
	for _, f := range v1Fields {
		if _, present := probe[f]; present {
			return f
		}
	}
	return ""
}

// repairBlocksAsString detects the "blocks[] arrived as JSON
// string" failure mode and returns a re-encoded RawMessage with
// blocks[] inlined as a real array.
//
// Trigger condition: top-level object whose `blocks` field decodes
// as a JSON string AND that string itself decodes as a JSON array.
// Anything else returns (_, false) and the caller proceeds with the
// original raw JSON unchanged — a hard parse error then surfaces
// the original schema violation to the LLM via the existing
// rejection path.
//
// The repair is conservative: only `blocks` is patched; every other
// top-level key passes through verbatim. This means downstream
// V1-field detection (detectV1FieldsInV2Emit) still fires after the
// repair, so we cannot soften any of the V1↔V2 mutual-exclusion
// invariants.
func repairBlocksAsString(raw json.RawMessage) (json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, false
	}
	rawBlocks, ok := probe["blocks"]
	if !ok {
		return nil, false
	}
	// blocks must be encoded as a JSON string for this repair path.
	if len(rawBlocks) == 0 || rawBlocks[0] != '"' {
		return nil, false
	}
	var encoded string
	if err := json.Unmarshal(rawBlocks, &encoded); err != nil {
		return nil, false
	}
	trimmed := strings.TrimSpace(encoded)
	if !strings.HasPrefix(trimmed, "[") {
		return nil, false
	}
	// Path A: pure-array stringify ("[{...},{...}]"). Repair just
	// the blocks key, preserve every other top-level field.
	var inner []json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &inner); err == nil {
		probe["blocks"] = mustMarshal(inner)
		patched, err := json.Marshal(probe)
		if err != nil {
			return nil, false
		}
		return patched, true
	}
	// Path B: whole-document stringify — the LLM put the entire
	// answer body inside the blocks string, so trimmed looks like
	//   "[{...blocks...}], \"citations\": [{...}], \"caveats\": [...]"
	// Recovery: wrap the encoded string with `{"blocks":` ... `}`
	// to form a fresh top-level object, parse, then merge with the
	// outer probe (outer wins on key conflict so caller-side keys
	// are not silently dropped).
	wrapped := []byte(`{"blocks": ` + trimmed + `}`)
	var fullDoc map[string]json.RawMessage
	if err := json.Unmarshal(wrapped, &fullDoc); err != nil {
		return nil, false
	}
	innerBlocks, ok := fullDoc["blocks"]
	if !ok || len(innerBlocks) == 0 || innerBlocks[0] != '[' {
		return nil, false
	}
	merged := make(map[string]json.RawMessage, len(probe)+len(fullDoc))
	for k, v := range fullDoc {
		merged[k] = v
	}
	for k, v := range probe {
		if k == "blocks" {
			continue
		}
		merged[k] = v
	}
	patched, err := json.Marshal(merged)
	if err != nil {
		return nil, false
	}
	return patched, true
}

func mustMarshal(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// repairNestedArraysAsString detects the same "JSON-encoded string
// where an array is expected" failure mode for the nested arrays
// inside each block (items, claim_uses, diagram.claim_uses) and
// returns a re-encoded RawMessage with each affected array inlined
// as a real array. Mirrors repairBlocksAsString conservatively —
// only the named fields are patched; everything else passes through
// verbatim so downstream V1-field detection + schema validation
// still fire on whatever else may be wrong.
//
// Returns (raw, [list of repaired paths], true) on at least one
// repair, or (raw, nil, false) when nothing was repaired.
//
// Paths use dotted-bracket notation so the WARN log can name the
// exact site (e.g. blocks[0].items, blocks[2].claim_uses,
// blocks[1].diagram.claim_uses).
func repairNestedArraysAsString(raw json.RawMessage) (json.RawMessage, []string, bool) {
	if len(raw) == 0 {
		return nil, nil, false
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, nil, false
	}
	rawBlocks, ok := probe["blocks"]
	if !ok || len(rawBlocks) == 0 || rawBlocks[0] != '[' {
		return nil, nil, false
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(rawBlocks, &blocks); err != nil {
		return nil, nil, false
	}
	var paths []string
	repaired := false
	for i, blk := range blocks {
		var blkObj map[string]json.RawMessage
		if err := json.Unmarshal(blk, &blkObj); err != nil {
			continue
		}
		for _, field := range []string{"items", "claim_uses"} {
			if r, ok := repairBlockArrayField(blkObj, field); ok {
				blkObj[field] = r
				paths = append(paths, fmt.Sprintf("blocks[%d].%s", i, field))
				repaired = true
			}
		}
		// Diagram.claim_uses — one level deeper.
		if rawDiag, ok := blkObj["diagram"]; ok && len(rawDiag) > 0 && rawDiag[0] == '{' {
			var diagObj map[string]json.RawMessage
			if err := json.Unmarshal(rawDiag, &diagObj); err == nil {
				if r, ok := repairBlockArrayField(diagObj, "claim_uses"); ok {
					diagObj["claim_uses"] = r
					if patched, err := json.Marshal(diagObj); err == nil {
						blkObj["diagram"] = patched
						paths = append(paths, fmt.Sprintf("blocks[%d].diagram.claim_uses", i))
						repaired = true
					}
				}
			}
		}
		if patched, err := json.Marshal(blkObj); err == nil {
			blocks[i] = patched
		}
	}
	if !repaired {
		return raw, nil, false
	}
	if patchedBlocks, err := json.Marshal(blocks); err == nil {
		probe["blocks"] = patchedBlocks
	}
	out, err := json.Marshal(probe)
	if err != nil {
		return raw, nil, false
	}
	return out, paths, true
}

// repairBlockArrayField detects a JSON-encoded string at obj[field]
// where a JSON array is expected, and returns the re-encoded array
// RawMessage. Returns (raw, false) when the field is absent OR is
// already a real array OR the string content cannot be decoded as
// an array. Conservative: any decode failure leaves the field
// untouched so the regular validator surfaces the real schema
// violation.
func repairBlockArrayField(obj map[string]json.RawMessage, field string) (json.RawMessage, bool) {
	rawField, ok := obj[field]
	if !ok || len(rawField) == 0 {
		return nil, false
	}
	if rawField[0] != '"' {
		// Already a real array (or another shape) — nothing to do.
		return nil, false
	}
	var encoded string
	if err := json.Unmarshal(rawField, &encoded); err != nil {
		return nil, false
	}
	trimmed := strings.TrimSpace(encoded)
	if !strings.HasPrefix(trimmed, "[") {
		return nil, false
	}
	var inner []json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &inner); err != nil {
		return nil, false
	}
	out, err := json.Marshal(inner)
	if err != nil {
		return nil, false
	}
	return out, true
}

// summarizeV2Blocks builds a short " (kinds=summary,ordered_list,…)"
// snippet for the tool result Summary. Helps operators eyeball the
// shape of the V2 emit without reading the full doc.
func summarizeV2Blocks(blocks []types.AnswerBlock) string {
	if len(blocks) == 0 {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		parts = append(parts, string(b.Kind))
	}
	return " (kinds=" + strings.Join(parts, ",") + ")"
}

// answerDocumentV2MisplacedHints lists field-name patterns
// historically seen as `unknown field` strict-decode rejects in
// V2 emit calls — typically caused by the LLM placing a sibling
// field inside a nested object whose description grew large
// enough to look like a "metadata bag". The remap rewrites the
// strict-decode error to surface the correct paths so the next
// retry sees concrete relocation guidance instead of a bare
// field-name reject.
//
// Provenance:
//   - citation_ref → claim_use / claim_uses: u3a-1 forensic
//     2026-05-04 (7 retry iters before recovery). Phase 3-C3
//     added from_node/to_node to claim_use; LLMs increasingly
//     guessed citation_ref into the same nested object.
//
// Adding new entries: confirm via grep + real-eval log that the
// LLM mistake is recurring (not a one-off), then add the pattern
// here and a regression test.
var answerDocumentV2MisplacedHints = []MisplacedFieldHint{
	{
		Field:          "citation_ref",
		ContainerNames: []string{"claim_use", "claim_uses"},
		CorrectPaths: []string{
			"items[i].citation_ref (anchor scalar / decision blocks via a one-element items=[{citation_ref:N}])",
		},
	},
	// V1 carrier residue: scalar blocks used to carry value{literal,
	// citation_ref}, decision blocks used to carry boolean{decision,
	// rationale, citation_ref}. V2 carrier puts the literal / verdict
	// in block.text and the citation on a one-element items[].
	// LLMs trained on the old shape may still emit value / boolean
	// at block top level; the remap directs them to the V2 location.
	{
		Field:          "value",
		ContainerNames: []string{"block (top-level)"},
		CorrectPaths: []string{
			"blocks[i].text (put the literal here directly, e.g. text=\"42\")",
			"blocks[i].items=[{id:\"v\", citation_ref: N}] (anchor citation here)",
		},
	},
	{
		Field:          "boolean",
		ContainerNames: []string{"block (top-level)"},
		CorrectPaths: []string{
			"blocks[i].text (put the verdict + rationale here directly, e.g. text=\"yes — the function returns ...\")",
			"blocks[i].items=[{id:\"d\", citation_ref: N}] (anchor citation here)",
		},
	},
	{
		Field:          "literal",
		ContainerNames: []string{"value (the value field is rejected; this targets the nested literal)"},
		CorrectPaths: []string{
			"blocks[i].text (the literal goes directly in block.text on a scalar block)",
		},
	},
	{
		Field:          "decision",
		ContainerNames: []string{"boolean (the boolean field is rejected; this targets the nested decision)"},
		CorrectPaths: []string{
			"blocks[i].text (the verdict and rationale go directly in block.text on a decision block)",
		},
	},
	{
		Field:          "rationale",
		ContainerNames: []string{"boolean (the boolean field is rejected; this targets the nested rationale)"},
		CorrectPaths: []string{
			"blocks[i].text (the verdict and rationale go directly in block.text on a decision block)",
		},
	},
	// Phase 1-B source-fix follow-on (2026-05-04): from_node /
	// to_node previously lived inside RenderedClaimUse and were
	// removed for source-level information-density reduction.
	// LLMs trained on the old shape may still place them inside
	// claim_use; remap to the new edge_anchors[] location.
	{
		Field:          "from_node",
		ContainerNames: []string{"claim_use", "claim_uses"},
		CorrectPaths: []string{
			"blocks[i].edge_anchors[j].from_node",
		},
	},
	{
		Field:          "to_node",
		ContainerNames: []string{"claim_use", "claim_uses"},
		CorrectPaths: []string{
			"blocks[i].edge_anchors[j].to_node",
		},
	},
	// G3 (post_v2_runtime_gap_remediation, 2026-05-04). relation_kind
	// is typed-only on EdgeAnchor; LLMs that have not yet learned the
	// new schema may misplace it inside claim_use (alongside its
	// neighbour fields from_node / to_node).
	{
		Field:          "relation_kind",
		ContainerNames: []string{"claim_use", "claim_uses"},
		CorrectPaths: []string{
			"blocks[i].edge_anchors[j].relation_kind",
		},
	},
}
