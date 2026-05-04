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
	ID          string                  `json:"id"`
	Kind        string                  `json:"kind"`
	Title       string                  `json:"title,omitempty"`
	Text        string                  `json:"text,omitempty"`
	Items       []emitAnswerBlockItemV2 `json:"items,omitempty"`
	Diagram     *emitAnswerDiagramV2    `json:"diagram,omitempty"`
	ClaimUses   []types.RenderedClaimUse `json:"claim_uses,omitempty"`
	FacetIDs    []string                `json:"facet_ids,omitempty"`
	SurfaceRole string                  `json:"surface_role,omitempty"`
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

// peekDocumentModel pre-decodes only the document_model field from
// the raw JSON so we can dispatch V1 vs V2 without forcing the V1
// strict-decode (DisallowUnknownFields) to fail on V2 fields.
//
// Returns (model, true, nil) when document_model is present (even
// when empty); (model, false, nil) when absent; or (-, -, err) on
// JSON decode failure.
func peekDocumentModel(raw json.RawMessage) (string, bool, error) {
	if len(raw) == 0 {
		return "", false, nil
	}
	var probe struct {
		DocumentModel *string `json:"document_model"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	// Allow unknown fields here — we're only peeking.
	if err := dec.Decode(&probe); err != nil {
		return "", false, err
	}
	if probe.DocumentModel == nil {
		return "", false, nil
	}
	return *probe.DocumentModel, true, nil
}

// executeAnswerDocumentV2 handles document_model="v2" emits. It
// validates the V2 schema, rejects mixed V1+V2 fields, and writes
// the typed AnswerDocumentV2 to MutableState via
// SetAnswerDocumentV2. The result Summary string mirrors V1's tone
// so finalize_preview / orchestrator hooks render consistent
// per-call text.
func executeAnswerDocumentV2(toolName string, ctx *types.BusContext, raw json.RawMessage, now time.Time) (types.ToolResult, error) {
	// First pass: detect V1 fields at top level — they must not
	// appear when document_model="v2". This is an explicit rejection
	// rather than relying on DisallowUnknownFields so the error
	// message can NAME the offending field.
	if violation := detectV1FieldsInV2Emit(raw); violation != "" {
		return failEmit(toolName, now,
			"document_model=v2 emit must not include V1 field %q at the top level; "+
				"the V2 carrier expresses the answer through blocks[] only", violation)
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
		return failEmit(toolName, now, "invalid params: %v", err)
	}

	if p.DocumentModel != "v2" {
		return failEmit(toolName, now,
			"document_model must equal \"v2\" on this code path; got %q", p.DocumentModel)
	}
	if len(p.Blocks) == 0 {
		return failEmit(toolName, now,
			"blocks[] is required and must be non-empty for document_model=v2")
	}

	// Build typed AnswerDocumentV2 + run per-block validation.
	doc := &types.AnswerDocumentV2{
		DocumentModel:   "v2",
		Citations:       p.Citations,
		ExactResolution: p.ExactResolution,
		Caveats:         p.Caveats,
		Snippets:        p.Snippets,
	}
	seenIDs := make(map[string]bool, len(p.Blocks))
	for i, raw := range p.Blocks {
		if strings.TrimSpace(raw.ID) == "" {
			return failEmit(toolName, now,
				"blocks[%d]: id is required and must be non-empty", i)
		}
		if seenIDs[raw.ID] {
			return failEmit(toolName, now,
				"blocks[%d]: duplicate id %q (each block must have a unique id)", i, raw.ID)
		}
		seenIDs[raw.ID] = true

		kind := types.AnswerBlockKind(raw.Kind)
		if !types.IsValidAnswerBlockKind(kind) {
			return failEmit(toolName, now,
				"blocks[%d]: kind=%q is not a valid AnswerBlockKind; allowed values: %v",
				i, raw.Kind, types.AllAnswerBlockKinds())
		}

		blk := types.AnswerBlock{
			ID:          raw.ID,
			Kind:        kind,
			Title:       raw.Title,
			Text:        raw.Text,
			ClaimUses:   raw.ClaimUses,
			FacetIDs:    raw.FacetIDs,
			SurfaceRole: types.SurfaceRole(raw.SurfaceRole),
		}
		if blk.SurfaceRole != "" {
			if _, ok := types.NormalizeSurfaceRole(string(blk.SurfaceRole)); !ok {
				return failEmit(toolName, now,
					"blocks[%d]: surface_role=%q is not a valid SurfaceRole",
					i, raw.SurfaceRole)
			}
		}

		if len(raw.Items) > 0 {
			blk.Items = make([]types.AnswerBlockItem, 0, len(raw.Items))
			for j, it := range raw.Items {
				blk.Items = append(blk.Items, types.AnswerBlockItem{
					ID:          it.ID,
					Label:       it.Label,
					Text:        it.Text,
					CitationRef: int(it.CitationRef),
					ClaimUse:    it.ClaimUse,
				})
				_ = j
			}
		}

		if raw.Diagram != nil {
			diag := &types.AnswerDiagramBlock{
				Kind:      types.DiagramKind(raw.Diagram.Kind),
				Language:  raw.Diagram.Language,
				Body:      raw.Diagram.Body,
				ClaimUses: raw.Diagram.ClaimUses,
			}
			if strings.TrimSpace(diag.Body) == "" {
				return failEmit(toolName, now,
					"blocks[%d]: diagram body is required when diagram is present", i)
			}
			blk.Diagram = diag
		} else if blk.Kind == types.BlockDiagram {
			return failEmit(toolName, now,
				"blocks[%d]: kind=diagram requires a non-nil diagram payload", i)
		}

		doc.Blocks = append(doc.Blocks, blk)
	}

	ctx.Mutable.SetAnswerDocumentV2(doc)

	return types.ToolResult{
		ToolName: toolName,
		Success:  true,
		Summary: fmt.Sprintf(
			"emit_answer_document accepted V2 carrier with %d block(s)%s",
			len(doc.Blocks),
			summarizeV2Blocks(doc.Blocks)),
		Timestamp: now,
	}, nil
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
	// Parse the embedded string as a JSON array — bail on any error so
	// the regular decode path produces the real schema rejection.
	var inner []json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &inner); err != nil {
		return nil, false
	}
	// Re-encode just the blocks key, preserving every other field.
	probe["blocks"], _ = json.Marshal(inner)
	patched, err := json.Marshal(probe)
	if err != nil {
		return nil, false
	}
	return patched, true
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
