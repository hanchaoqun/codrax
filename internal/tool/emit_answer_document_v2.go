package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
