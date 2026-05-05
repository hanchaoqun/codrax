package tool

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)
var _ = time.Now // ensure time stays imported when failEmit moved


// EmitAnswerDocument is the structured finalizer channel. The
// finalizer LLM makes exactly one emit_answer_document call per
// dispatch, supplying a typed AnswerDocumentV2 (block-only carrier),
// and a deterministic renderer (internal/render/answerdoc.go) turns
// the struct into user-visible prose.
//
// B8-T3+T4 (block_only_carrier.md §5.8, 2026-05-03): V1 carrier
// fully retired. Only document_model="v2" is accepted. B3-F1
// (post_shape_retirement_consolidated_audit.md §8 Batch B3,
// 2026-05-04) tightened the gate further: empty / missing
// document_model is now rejected at the executor with an explicit
// "document_model must equal \"v2\"" error so schema / executor /
// type-level contract all say the same thing. The V2 schema is
// enforced in executeAnswerDocumentV2 (emit_answer_document_v2.go).
//
// Classified ReadOnly because IsWrite() is the filesystem-write
// boundary; mutating BusContext is not a filesystem write.
// Classified NonEvidenceTool: the payload is the final answer slate,
// not a repo fact. Mirrors emit_answer_symbol on both axes.
type EmitAnswerDocument struct {
	ReadOnly
	NonEvidenceTool
}

func (t *EmitAnswerDocument) Name() string { return "emit_answer_document" }

// Description renders the LLM-facing tool description. Block-only
// V2 carrier — the LLM emits an array of blocks tagged by Kind,
// each carrying its own typed payload (Items, Diagram, Text, etc.).
// Per the feedback_no_internal_info_in_llm_prompts red line, this
// description avoids internal Go terminology (AnswerDocumentV2 /
// BlockRequirement etc.) and uses LLM-natural language only.
//
// v3 B4 (2026-05-04): the block-semantic contract body is shared
// with emit_answer_document_patch via
// BuildAnswerDocumentSemanticContractDescription so adding a new
// block field / kind / claim_form / worked example only edits one
// helper.
func (t *EmitAnswerDocument) Description() string {
	return "Emit the FULL final answer document as a structured blocks[] array. " +
		"Use this on first dispatches and whenever the answer needs a complete rewrite. On retry paths where only a few blocks need editing, prefer emit_answer_document_patch which protocol-level preserves typed annotation fields on blocks you do not touch.\n\n" +
		BuildAnswerDocumentSemanticContractDescription()
}

// Parameters returns the V2 JSON schema. Block kinds + per-block
// fields are LLM-facing; internal naming (BlockRequirement /
// AnswerSemanticView etc.) is intentionally absent.
func (t *EmitAnswerDocument) Parameters() json.RawMessage {
	const schema = `{
  "type": "object",
  "properties": {
    "blocks": {
      "type": "array",
      "description": "Ordered list of answer blocks. REQUIRED; must be non-empty. Each block is a structured payload tagged by kind.",
      "items": {
        "type": "object",
        "properties": {
          "id": {"type": "string", "description": "Stable unique identifier for this block (any non-empty string the LLM picks)."},
          "kind": {
            "type": "string",
            "enum": ["summary", "section", "ordered_list", "bullet_list", "scalar", "decision", "table", "diagram", "caveat"],
            "description": "Block kind. Required."
          },
          "title": {"type": "string", "description": "Optional sub-heading for section / table / diagram / caveat blocks."},
          "text": {"type": "string", "description": "Block body prose. Used by summary / section / scalar / decision / caveat. Markdown-flavoured."},
          "items": {
            "type": "array",
            "description": "Block items for ordered_list / bullet_list / table. For tables each item is one row.",
            "items": {
              "type": "object",
              "properties": {
                "id":           {"type": "string"},
                "label":        {"type": "string", "description": "Primary visible text / row header. For enumeration items, MUST be the verbatim identifier copied from one of the evidence pool's anchor_symbol / subject / object values — fabricated labels are rejected by validateEnumerationItemLabelGrounding."},
                "text":         {"type": "string", "description": "Item body / row content."},
                "citation_ref": {"type": "integer", "description": "Top-level field on the item; zero-based index into citations[], or -1 when no cite backs the item. For scalar / decision blocks (where the literal / verdict sits in block.text), anchor the citation by attaching a one-element items=[{id:\"x\", citation_ref: N}] — there is no top-level value/boolean field on the block. NEVER place citation_ref inside claim_use — it is rejected with 'unknown field \"citation_ref\"'."},
                "claim_use":    {"type": "object", "description": "Optional per-item claim annotation. EXACTLY 4 fields: {claim_form: <enum>, facet_id?: string, evidence_id?: string, surface_role?: <enum>}. Does NOT carry citation_ref (citation_ref is top-level on the item, not inside this object). Does NOT carry from_node / to_node (those live in the block-level edge_anchors[] array — see below — never inside claim_use)."}
              }
            }
          },
          "diagram": {
            "type": "object",
            "description": "Diagram payload for kind=diagram blocks. Body is the raw mermaid (or text) source.",
            "properties": {
              "kind":     {"type": "string", "enum": ["flow", "sequence", "architecture", "call_dag"], "description": "SEMANTIC family of the diagram, NOT a Mermaid keyword. Use the family the contract names: flow (branches/guards), sequence (actor-to-actor over time), architecture (layered components), call_dag (one-to-many dispatch). Mermaid syntax tokens like \"flowchart\" and \"sequenceDiagram\" belong inside diagram.body, NOT here."},
              "language": {"type": "string", "description": "Diagram source language. Defaults to \"mermaid\" — the only currently rendered subset."},
              "body":     {"type": "string", "description": "Raw diagram source (the part inside fenced markers; the renderer adds the fences). For diagram.kind=flow/architecture/call_dag use Mermaid \"flowchart\" syntax (direction LR/TD/RL/BT); for diagram.kind=sequence use Mermaid \"sequenceDiagram\"."}
            }
          },
          "claim_uses":   {"type": "array", "description": "Block-level claim annotations array (the singular form claim_use does NOT exist at block level — only inside items[i].claim_use). REQUIRED on principal blocks (surface_role=principal) when the contract's AcceptableClaimForms list is non-empty. Each entry has EXACTLY 4 fields: {claim_form: <one of definition_fact|call_edge|guard_condition|assignment_fact|return_fact|absence_fact|precedence_role|external_observation|import_edge>, facet_id?: string, evidence_id?: string, surface_role?: <enum>}. Single-form blocks emit a one-element array (claim_uses=[{claim_form=definition_fact}]). Each entry does NOT carry citation_ref — citations live on the enclosing carrier (per-item items[i].citation_ref (scalar / decision blocks anchor the cite via a one-element items=[{citation_ref:N}])). Each entry does NOT carry from_node / to_node — those live in the block-level edge_anchors[] field below."},
          "edge_anchors": {"type": "array", "description": "Optional block-level array of typed edge-anchor entries that bind diagram edges to typed claim shapes. Use this when the block contributes evidence about a directed relation in a diagram (typically when the block is a diagram or when its items describe edge endpoints of a diagram in another block). Each entry shape: {from_node: string, to_node: string, relation_kind?: <one of call|guard|import|precedence|contain|observe>, claim_form?: <one of call_edge|guard_condition|import_edge|precedence_role|external_observation>}. Both from_node and to_node MUST be the verbatim node identifier strings as they appear in the diagram body. PREFERRED: set relation_kind directly — when present it authoritatively names the basic semantic relation, and the rendered Mermaid label is then free prose for readability. When relation_kind is omitted, the validator falls back to recognising the relation from the rendered label vocabulary (see the diagram-edge label vocabulary section in the user message for the recognised keywords). claim_form names the typed claim shape required to support the relation. Empty / absent = no edge anchor on this block (legitimate for non-diagram-edge blocks).", "items": {"type": "object"}},
          "facet_ids":    {"type": "array", "items": {"type": "string"}, "description": "Optional facet ids this block covers — read these from the user section's Required Answer Blocks list."},
          "surface_role": {"type": "string", "enum": ["", "principal", "support", "prose_only", "diagram_only"], "description": "Block's role in the answer surface. principal = carries the answer payload (must usually attach claim_use); support = corroborates principal (e.g. anchor skeleton); prose_only = lead-in / framing; diagram_only = purely visual. The user-section's block list flags which role each Required Block expects."}
        },
        "required": ["id", "kind"]
      }
    },
    "citations": {
      "type": "array",
      "description": "Shared pool of file-line citations. Per-block / per-item citation_ref values are zero-based indexes into this array.",
      "items": {"type": "object"}
    },
    "exact_resolution": {"type": "object", "description": "Optional exact-resolution contract result (status / anchor / context_mode)."},
    "caveats":  {"type": "array", "items": {"type": "string"}, "description": "Optional document-level caveat strings (cross-block scope notes)."},
    "snippets": {"type": "array", "items": {"type": "object"}, "description": "Optional code snippets shown alongside the answer."}
  },
  "required": ["blocks"]
}`
	return json.RawMessage(schema)
}

// Execute routes the emit to the V2 validator + writer. V1 carrier
// is retired (B8-T3); B3-F1 (2026-05-04) tightened the contract so
// empty / missing document_model is now an explicit fail-fast
// rejection here at the dispatch boundary, not silently routed to
// V2 to be rejected later by the executor — keeps schema /
// dispatch / executor / type-level all saying the same thing.
func (t *EmitAnswerDocument) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	now := time.Now()
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_answer_document requires BusContext.Mutable; the caller did not provide one (sub-agents are not supported)",
			Timestamp: now,
		}, nil
	}
	// v3.1 (2026-05-05): the dispatcher no longer inspects
	// document_model at all — there is only one executor path, and
	// the LLM-facing schema no longer mentions the field. The
	// executor itself silently tolerates whatever value (or absence)
	// the LLM supplies; nothing here is worth a round-trip rejection.
	return executeAnswerDocumentV2(t.Name(), ctx, params, now)
}

// requestedAnswerDocumentLanguage returns the requested answer
// language for downstream tool helpers (log_source_drift_surface,
// emit_evidence). Reads AnswerContract.Language first, then
// BusContext.Language fallback. Lowercased + trimmed.
func requestedAnswerDocumentLanguage(ctx *types.BusContext) string {
	if ctx == nil {
		return ""
	}
	if ctx.AnalysisIR != nil {
		if lang := strings.ToLower(strings.TrimSpace(ctx.AnalysisIR.AnswerContract.Language)); lang != "" {
			return lang
		}
	}
	return strings.ToLower(strings.TrimSpace(ctx.Language))
}

// answerDocumentRequiresChinese reports whether the requested
// language matches the Chinese family.
func answerDocumentRequiresChinese(lang string) bool {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "zh", "zh-cn", "cn", "chinese":
		return true
	}
	return false
}

// answerSurfacePlan compiles an AnswerSurfacePlan from the
// BusContext + applies extractor's answer-symbol step backbone.
func answerSurfacePlan(ctx *types.BusContext) *types.AnswerSurfacePlan {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	plan := types.BuildAnswerSurfacePlanForBusContext(ctx)
	if plan != nil && len(ctx.AnswerSymbols) > 0 {
		types.ApplyAnswerSymbolStepBackbone(plan, ctx.AnalysisIR, ctx.AnswerSymbols, ctx.AnswerSymbolCompleteness)
	}
	return plan
}

// answerExactResolutionContract pulls the ExactResolution contract
// from the surface plan.
func answerExactResolutionContract(ctx *types.BusContext) *types.ExactResolutionContract {
	if plan := answerSurfacePlan(ctx); plan != nil {
		return plan.ExactResolution
	}
	return nil
}

